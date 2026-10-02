package service

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/storage/memory"
)

const chatID = 10

// setup creates a service with one queue: done users first, then waiting
// users in order. The queue is written straight to storage, so each test
// only depends on the function it checks.
func setup(t *testing.T, waiting []int64, done []int64) (*Service, int64) {
	t.Helper()
	store := memory.New()

	var entries []model.Entry
	for _, id := range done {
		entries = append(entries, model.Entry{UserID: id, Done: true})
	}
	for _, id := range waiting {
		entries = append(entries, model.Entry{UserID: id})
	}

	q, err := store.CreateQueue(context.Background(), model.Queue{
		ChatID:  chatID,
		Name:    "Practice 4",
		Entries: entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	return New(store), q.ID
}

// waitingIDs returns the waiting users in order and checks the invariants
// that must hold after every operation: nobody appears twice, and positions
// run 1..n.
func waitingIDs(t *testing.T, s *Service, queueID int64) []int64 {
	t.Helper()
	q, err := s.Queue(context.Background(), queueID)
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[int64]bool)
	for _, e := range q.Entries {
		if seen[e.UserID] {
			t.Fatalf("user %d is in the queue twice", e.UserID)
		}
		seen[e.UserID] = true
	}

	var ids []int64
	for i, e := range q.Waiting() {
		if got := q.Position(e.UserID); got != i+1 {
			t.Fatalf("user %d: Position() = %d, want %d", e.UserID, got, i+1)
		}
		ids = append(ids, e.UserID)
	}
	return ids
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	s := New(memory.New())

	first, err := s.Create(ctx, chatID, "  Practice 4  ", 7)
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "Practice 4" {
		t.Errorf("Name = %q, want the trimmed %q", first.Name, "Practice 4")
	}
	if first.ChatID != chatID || first.CreatedBy != 7 {
		t.Errorf("ChatID, CreatedBy = %d, %d, want %d, 7", first.ChatID, first.CreatedBy, chatID)
	}
	if first.CreatedAt.IsZero() {
		t.Error("CreatedAt is not set")
	}

	tests := []struct {
		name    string
		chatID  int64
		qname   string
		wantErr error
	}{
		{"empty", chatID, "", model.ErrInvalidName},
		{"only spaces", chatID, "   ", model.ErrInvalidName},
		{"65 letters", chatID, strings.Repeat("a", 65), model.ErrInvalidName},
		{"64 letters", chatID, strings.Repeat("a", 64), nil},
		// Cyrillic letters are 2 bytes each: 64 of them must still fit.
		{"64 cyrillic letters", chatID, strings.Repeat("я", 64), nil},
		{"same name", chatID, "Practice 4", model.ErrQueueExists},
		{"same name, other case", chatID, "PRACTICE 4", model.ErrQueueExists},
		{"same name with spaces", chatID, " Practice 4 ", model.ErrQueueExists},
		{"same name, other chat", 20, "Practice 4", nil},
		{"new name", chatID, "Lab 2", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Create(ctx, tt.chatID, tt.qname, 1)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Create(%q) error = %v, want %v", tt.qname, err, tt.wantErr)
			}
		})
	}
}

func TestCreateReusesNameOfClosedQueue(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	if _, err := store.CreateQueue(ctx, model.Queue{ChatID: chatID, Name: "Practice 4", Closed: true}); err != nil {
		t.Fatal(err)
	}

	if _, err := New(store).Create(ctx, chatID, "Practice 4", 1); err != nil {
		t.Errorf("reusing the name of a closed queue: %v", err)
	}
}

// Two /new commands with the same name at the same moment: exactly one wins.
func TestConcurrentCreateSameName(t *testing.T) {
	ctx := context.Background()
	s := New(memory.New())

	const n = 50
	var wg sync.WaitGroup
	var mu sync.Mutex
	created := 0
	for range n {
		wg.Go(func() {
			_, err := s.Create(ctx, chatID, "Practice 4", 1)
			switch {
			case err == nil:
				mu.Lock()
				created++
				mu.Unlock()
			case !errors.Is(err, model.ErrQueueExists):
				t.Error(err)
			}
		})
	}
	wg.Wait()

	if created != 1 {
		t.Errorf("%d queues created, want 1", created)
	}
}

func TestQueueAndOpenQueues(t *testing.T) {
	ctx := context.Background()
	s := New(memory.New())

	a, err := s.Create(ctx, chatID, "A", 1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(ctx, chatID, "B", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, 20, "other chat", 1); err != nil {
		t.Fatal(err)
	}

	got, err := s.Queue(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "A" {
		t.Errorf("Queue(%d).Name = %q, want %q", a.ID, got.Name, "A")
	}
	if _, err := s.Queue(ctx, 999); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("Queue(999) error = %v, want ErrNotFound", err)
	}

	open, err := s.OpenQueues(ctx, chatID)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 || open[0].ID != a.ID || open[1].ID != b.ID {
		t.Errorf("OpenQueues(%d) = %+v, want A then B", chatID, open)
	}
}

func TestSetBoardMessage(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{1, 2}, nil)

	if err := s.SetBoardMessage(ctx, qid, 555); err != nil {
		t.Fatal(err)
	}

	q, err := s.Queue(ctx, qid)
	if err != nil {
		t.Fatal(err)
	}
	if q.BoardMsgID != 555 {
		t.Errorf("BoardMsgID = %d, want 555", q.BoardMsgID)
	}
	if got := waitingIDs(t, s, qid); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("waiting = %v, want the queue unchanged", got)
	}

	if err := s.SetBoardMessage(ctx, 999, 1); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("unknown queue: error = %v, want ErrNotFound", err)
	}
}

func TestJoin(t *testing.T) {
	tests := []struct {
		name    string
		user    int64
		want    []int64
		wantPos int
		wantErr error
	}{
		{"new user goes last", 9, []int64{1, 2, 3, 9}, 4, nil},
		{"already waiting", 2, []int64{1, 2, 3}, 0, model.ErrAlreadyJoined},
		{"already done", 5, []int64{1, 2, 3}, 0, model.ErrAlreadyDone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, qid := setup(t, []int64{1, 2, 3}, []int64{5})
			pos, err := s.Join(context.Background(), qid, tt.user)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if pos != tt.wantPos {
				t.Errorf("position = %d, want %d", pos, tt.wantPos)
			}
			if got := waitingIDs(t, s, qid); !slices.Equal(got, tt.want) {
				t.Errorf("waiting = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJoinEmptyQueue(t *testing.T) {
	s, qid := setup(t, nil, nil)
	pos, err := s.Join(context.Background(), qid, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pos != 1 {
		t.Errorf("position = %d, want 1", pos)
	}
}

func TestLeave(t *testing.T) {
	tests := []struct {
		name    string
		user    int64
		want    []int64
		wantErr error
	}{
		{"first", 1, []int64{2, 3}, nil},
		{"middle", 2, []int64{1, 3}, nil},
		{"last", 3, []int64{1, 2}, nil},
		{"not in queue", 9, []int64{1, 2, 3}, model.ErrNotInQueue},
		{"already done", 5, []int64{1, 2, 3}, model.ErrNotInQueue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, qid := setup(t, []int64{1, 2, 3}, []int64{5})
			err := s.Leave(context.Background(), qid, tt.user)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got := waitingIDs(t, s, qid); !slices.Equal(got, tt.want) {
				t.Errorf("waiting = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLeaveThenJoinAgain(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{1, 2, 3}, nil)

	if err := s.Leave(ctx, qid, 1); err != nil {
		t.Fatal(err)
	}
	pos, err := s.Join(ctx, qid, 1)
	if err != nil {
		t.Fatal(err)
	}
	if pos != 3 {
		t.Errorf("position after rejoining = %d, want 3", pos)
	}
}

func TestToEnd(t *testing.T) {
	tests := []struct {
		name    string
		user    int64
		want    []int64
		wantPos int
		wantErr error
	}{
		{"first goes last", 1, []int64{2, 3, 1}, 3, nil},
		{"middle goes last", 2, []int64{1, 3, 2}, 3, nil},
		{"already last", 3, []int64{1, 2, 3}, 3, nil},
		{"not in queue", 9, []int64{1, 2, 3}, 0, model.ErrNotInQueue},
		{"already done", 5, []int64{1, 2, 3}, 0, model.ErrNotInQueue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, qid := setup(t, []int64{1, 2, 3}, []int64{5})
			pos, err := s.ToEnd(context.Background(), qid, tt.user)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if pos != tt.wantPos {
				t.Errorf("position = %d, want %d", pos, tt.wantPos)
			}
			if got := waitingIDs(t, s, qid); !slices.Equal(got, tt.want) {
				t.Errorf("waiting = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDone(t *testing.T) {
	tests := []struct {
		name    string
		user    int64
		want    []int64
		wantErr error
	}{
		{"first", 1, []int64{2, 3}, nil},
		{"not first", 2, []int64{1, 3}, nil},
		{"not in queue", 9, []int64{1, 2, 3}, model.ErrNotInQueue},
		{"twice", 5, []int64{1, 2, 3}, model.ErrNotInQueue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, qid := setup(t, []int64{1, 2, 3}, []int64{5})
			err := s.Done(context.Background(), qid, tt.user)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got := waitingIDs(t, s, qid); !slices.Equal(got, tt.want) {
				t.Errorf("waiting = %v, want %v", got, tt.want)
			}
		})
	}
}

// A done user stays on the sheet with a time, so the board can list them,
// and they can't join again.
func TestDoneKeepsEntry(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{1, 2}, nil)

	if err := s.Done(ctx, qid, 1); err != nil {
		t.Fatal(err)
	}

	q, err := s.Queue(ctx, qid)
	if err != nil {
		t.Fatal(err)
	}
	if !q.Has(1) {
		t.Fatal("done user was removed from the queue")
	}
	if q.Entries[0].DoneAt.IsZero() {
		t.Error("DoneAt is not set")
	}
	if _, err := s.Join(ctx, qid, 1); !errors.Is(err, model.ErrAlreadyDone) {
		t.Errorf("Join after Done: error = %v, want ErrAlreadyDone", err)
	}
}

func TestClose(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{1, 2}, nil)

	if err := s.Close(ctx, qid); err != nil {
		t.Fatal(err)
	}

	q, err := s.Queue(ctx, qid)
	if err != nil {
		t.Fatal(err)
	}
	if !q.Closed {
		t.Error("queue is not closed")
	}
	if got := waitingIDs(t, s, qid); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("waiting = %v, want the people kept after closing", got)
	}

	open, err := s.OpenQueues(ctx, chatID)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Errorf("OpenQueues still lists %d queues after closing", len(open))
	}
}

func TestSwapWith(t *testing.T) {
	tests := []struct {
		name    string
		user    int64
		target  int64
		want    []int64
		wantRes Swapped
		wantErr error
	}{
		{"forward", 3, 1, []int64{3, 2, 1, 4}, Swapped{TargetID: 1, From: 3, To: 1}, nil},
		{"backward", 1, 4, []int64{4, 2, 3, 1}, Swapped{TargetID: 4, From: 1, To: 4}, nil},
		{"neighbours", 2, 3, []int64{1, 3, 2, 4}, Swapped{TargetID: 3, From: 2, To: 3}, nil},
		{"self", 2, 2, []int64{1, 2, 3, 4}, Swapped{}, model.ErrSelfSwap},
		{"user not in queue", 9, 1, []int64{1, 2, 3, 4}, Swapped{}, model.ErrNotInQueue},
		{"user done", 5, 1, []int64{1, 2, 3, 4}, Swapped{}, model.ErrNotInQueue},
		{"target not in queue", 1, 9, []int64{1, 2, 3, 4}, Swapped{}, model.ErrTargetNotInQueue},
		{"target done", 1, 5, []int64{1, 2, 3, 4}, Swapped{}, model.ErrTargetNotInQueue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, qid := setup(t, []int64{1, 2, 3, 4}, []int64{5})
			res, err := s.SwapWith(context.Background(), qid, tt.user, tt.target)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if res != tt.wantRes {
				t.Errorf("result = %+v, want %+v", res, tt.wantRes)
			}
			if got := waitingIDs(t, s, qid); !slices.Equal(got, tt.want) {
				t.Errorf("waiting = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSwapWithPosition(t *testing.T) {
	tests := []struct {
		name    string
		user    int64
		pos     int
		want    []int64
		wantRes Swapped
		wantErr error
	}{
		{"with last", 1, 3, []int64{3, 2, 1}, Swapped{TargetID: 3, From: 1, To: 3}, nil},
		{"with first", 3, 1, []int64{3, 2, 1}, Swapped{TargetID: 1, From: 3, To: 1}, nil},
		{"own position", 1, 1, []int64{1, 2, 3}, Swapped{}, model.ErrSelfSwap},
		{"zero", 1, 0, []int64{1, 2, 3}, Swapped{}, model.ErrInvalidPosition},
		{"negative", 1, -1, []int64{1, 2, 3}, Swapped{}, model.ErrInvalidPosition},
		{"past the end", 1, 4, []int64{1, 2, 3}, Swapped{}, model.ErrInvalidPosition},
		{"user not in queue", 9, 2, []int64{1, 2, 3}, Swapped{}, model.ErrNotInQueue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, qid := setup(t, []int64{1, 2, 3}, []int64{5})
			res, err := s.SwapWithPosition(context.Background(), qid, tt.user, tt.pos)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if res != tt.wantRes {
				t.Errorf("result = %+v, want %+v", res, tt.wantRes)
			}
			if got := waitingIDs(t, s, qid); !slices.Equal(got, tt.want) {
				t.Errorf("waiting = %v, want %v", got, tt.want)
			}
		})
	}
}

// Positions count only waiting people: with a done user in front,
// "#1" on the board is the first person still waiting.
func TestSwapWithPositionSkipsDone(t *testing.T) {
	s, qid := setup(t, []int64{1, 2, 3}, []int64{5})

	res, err := s.SwapWithPosition(context.Background(), qid, 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.TargetID != 1 {
		t.Errorf("swapped with user %d, want user 1 (the first waiting)", res.TargetID)
	}
}

// Swapped reports queue positions, not lines in Entries. With done users
// in front the two differ, so this catches mixing them up.
func TestSwappedReportsPositions(t *testing.T) {
	for _, done := range [][]int64{nil, {5}, {5, 6}} {
		s, qid := setup(t, []int64{1, 2, 3}, done)

		res, err := s.SwapWith(context.Background(), qid, 2, 3)
		if err != nil {
			t.Fatal(err)
		}
		want := Swapped{TargetID: 3, From: 2, To: 3}
		if res != want {
			t.Errorf("with %d done in front: result = %+v, want %+v", len(done), res, want)
		}
	}
}

func TestClosedQueueRejectsChanges(t *testing.T) {
	ctx := context.Background()
	store := memory.New()
	q, err := store.CreateQueue(ctx, model.Queue{
		ChatID:  chatID,
		Name:    "Practice 4",
		Closed:  true,
		Entries: []model.Entry{{UserID: 1}, {UserID: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := New(store)

	ops := map[string]func() error{
		"join":             func() error { _, err := s.Join(ctx, q.ID, 9); return err },
		"leave":            func() error { return s.Leave(ctx, q.ID, 1) },
		"toEnd":            func() error { _, err := s.ToEnd(ctx, q.ID, 1); return err },
		"swapWith":         func() error { _, err := s.SwapWith(ctx, q.ID, 1, 2); return err },
		"swapWithPosition": func() error { _, err := s.SwapWithPosition(ctx, q.ID, 1, 2); return err },
		"setBoardMessage":  func() error { return s.SetBoardMessage(ctx, q.ID, 1) },
		"done":             func() error { return s.Done(ctx, q.ID, 1) },
		"close":            func() error { return s.Close(ctx, q.ID) },
		"undo":             func() error { _, err := s.Undo(ctx, q.ID, 1); return err },
		"move":             func() error { _, err := s.Move(ctx, q.ID, 1, 2); return err },
	}
	for name, op := range ops {
		if err := op(); !errors.Is(err, model.ErrQueueClosed) {
			t.Errorf("%s on a closed queue: error = %v, want ErrQueueClosed", name, err)
		}
	}
}

func TestUnknownQueue(t *testing.T) {
	ctx := context.Background()
	s := New(memory.New())

	ops := map[string]func() error{
		"join":             func() error { _, err := s.Join(ctx, 999, 1); return err },
		"leave":            func() error { return s.Leave(ctx, 999, 1) },
		"toEnd":            func() error { _, err := s.ToEnd(ctx, 999, 1); return err },
		"swapWith":         func() error { _, err := s.SwapWith(ctx, 999, 1, 2); return err },
		"swapWithPosition": func() error { _, err := s.SwapWithPosition(ctx, 999, 1, 2); return err },
		"done":             func() error { return s.Done(ctx, 999, 1) },
		"close":            func() error { return s.Close(ctx, 999) },
	}
	for name, op := range ops {
		if err := op(); !errors.Is(err, model.ErrNotFound) {
			t.Errorf("%s on an unknown queue: error = %v, want ErrNotFound", name, err)
		}
	}
}

// 100 people press Join at the same moment. Without the queue lock some joins
// would be lost, because each one loads, changes and saves the whole queue.
func TestConcurrentJoin(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, nil, nil)

	const n = 100
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			if _, err := s.Join(ctx, qid, int64(i+1)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	if got := waitingIDs(t, s, qid); len(got) != n {
		t.Fatalf("%d users waiting, want %d", len(got), n)
	}
}

// Everyone presses random buttons at once. Whatever order they run in,
// the queue must stay valid: no duplicates, positions 1..n.
// Errors are ignored on purpose: random presses often fail, and that's fine.
func TestConcurrentMixedOperations(t *testing.T) {
	ctx := context.Background()
	const n = 30
	users := make([]int64, n)
	for i := range users {
		users[i] = int64(i + 1)
	}
	s, qid := setup(t, users, nil)

	var wg sync.WaitGroup
	for _, u := range users {
		wg.Go(func() {
			for range 20 {
				switch rand.IntN(6) {
				case 0:
					_, _ = s.Join(ctx, qid, u)
				case 1:
					_ = s.Leave(ctx, qid, u)
				case 2:
					_, _ = s.ToEnd(ctx, qid, u)
				case 3:
					_, _ = s.SwapWith(ctx, qid, u, users[rand.IntN(n)])
				case 4:
					_, _ = s.SwapWithPosition(ctx, qid, u, rand.IntN(n)+1)
				case 5:
					_ = s.Done(ctx, qid, u)
				}
			}
		})
	}
	wg.Wait()

	waitingIDs(t, s, qid)
}

func TestNames(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{2, 3}, []int64{5})

	for _, u := range []model.User{
		{ID: 1, FirstName: "Anna", LastName: "Kuznetsova"},
		{ID: 2, FirstName: "Ivan"},
		{ID: 5, Username: "olga_v"},
	} {
		if err := s.SaveUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}

	q, err := s.Queue(ctx, qid)
	if err != nil {
		t.Fatal(err)
	}
	q.CreatedBy = 1

	got, err := s.Names(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	// User 1 created the queue but isn't in it, and user 3 was never saved.
	want := map[int64]string{2: "Ivan", 5: "@olga_v"}
	if len(got) != len(want) {
		t.Fatalf("Names() = %v, want %v", got, want)
	}
	for id, name := range want {
		if got[id] != name {
			t.Errorf("Names()[%d] = %q, want %q", id, got[id], name)
		}
	}
}

func TestSaveUserUpdatesName(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{2}, nil)

	if err := s.SaveUser(ctx, model.User{ID: 2, FirstName: "Ivan"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveUser(ctx, model.User{ID: 2, FirstName: "Ivan", LastName: "Tarasov"}); err != nil {
		t.Fatal(err)
	}

	q, err := s.Queue(ctx, qid)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Names(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if got[2] != "Ivan Tarasov" {
		t.Errorf("name after rename = %q, want %q", got[2], "Ivan Tarasov")
	}
}

func TestUndo(t *testing.T) {
	tests := []struct {
		name    string
		user    int64
		want    []int64
		wantPos int
		wantErr error
	}{
		// setup puts done users first, so 5 comes back as #1.
		{"done user returns", 5, []int64{5, 1, 2, 3}, 1, nil},
		{"still waiting", 2, []int64{1, 2, 3}, 0, model.ErrNotDone},
		{"not in queue", 9, []int64{1, 2, 3}, 0, model.ErrNotInQueue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, qid := setup(t, []int64{1, 2, 3}, []int64{5})
			pos, err := s.Undo(context.Background(), qid, tt.user)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if pos != tt.wantPos {
				t.Errorf("position = %d, want %d", pos, tt.wantPos)
			}
			if got := waitingIDs(t, s, qid); !slices.Equal(got, tt.want) {
				t.Errorf("waiting = %v, want %v", got, tt.want)
			}
		})
	}
}

// Done then Undo puts the user back exactly where they were, even after
// others joined behind them in the meantime.
func TestDoneThenUndoRestoresPlace(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{1, 2, 3}, nil)

	if err := s.Done(ctx, qid, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Join(ctx, qid, 4); err != nil {
		t.Fatal(err)
	}
	pos, err := s.Undo(ctx, qid, 2)
	if err != nil {
		t.Fatal(err)
	}
	if pos != 2 {
		t.Errorf("position after undo = %d, want 2", pos)
	}
	if got := waitingIDs(t, s, qid); !slices.Equal(got, []int64{1, 2, 3, 4}) {
		t.Errorf("waiting = %v, want [1 2 3 4]", got)
	}

	q, err := s.Queue(ctx, qid)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range q.Entries {
		if e.UserID == 2 && !e.DoneAt.IsZero() {
			t.Error("DoneAt should be cleared by Undo")
		}
	}
}

func TestMove(t *testing.T) {
	tests := []struct {
		name    string
		who     int64
		to      int
		want    []int64
		wantRes Moved
		wantErr error
	}{
		// Waiting: 1 2 3 4 5 (and 9 done, first in the slice).
		{"last back to third, by position", 5, 3, []int64{1, 2, 5, 3, 4}, Moved{UserID: 5, From: 5, To: 3}, nil},
		{"to the front", 4, 1, []int64{4, 1, 2, 3, 5}, Moved{UserID: 4, From: 4, To: 1}, nil},
		{"forward to the end", 1, 5, []int64{2, 3, 4, 5, 1}, Moved{UserID: 1, From: 1, To: 5}, nil},
		{"one step back", 2, 3, []int64{1, 3, 2, 4, 5}, Moved{UserID: 2, From: 2, To: 3}, nil},
		{"by user ID", 1005, 2, []int64{1, 1005, 2, 3, 4}, Moved{UserID: 1005, From: 5, To: 2}, nil},
		{"already there", 3, 3, nil, Moved{}, model.ErrAlreadyThere},
		{"target position 0", 2, 0, nil, Moved{}, model.ErrInvalidPosition},
		{"target past the end", 2, 6, nil, Moved{}, model.ErrInvalidPosition},
		{"unknown user ID", 777, 2, nil, Moved{}, model.ErrTargetNotInQueue},
		{"done user's ID", 9, 2, nil, Moved{}, model.ErrTargetNotInQueue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			waiting := []int64{1, 2, 3, 4, 5}
			if tt.name == "by user ID" {
				waiting = []int64{1, 2, 3, 4, 1005}
			}
			s, qid := setup(t, waiting, []int64{9})

			res, err := s.Move(context.Background(), qid, tt.who, tt.to)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if res != tt.wantRes {
				t.Errorf("result = %+v, want %+v", res, tt.wantRes)
			}
			want := tt.want
			if tt.wantErr != nil {
				want = waiting // a refused move changes nothing
			}
			if got := waitingIDs(t, s, qid); !slices.Equal(got, want) {
				t.Errorf("waiting = %v, want %v", got, want)
			}
		})
	}
}

// Done entries in the middle of the slice stay where they are, and
// positions still count only people waiting.
func TestMoveKeepsDoneInPlace(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{1, 2, 3}, nil)
	if err := s.Done(ctx, qid, 2); err != nil {
		t.Fatal(err)
	}

	// Waiting is now 1 3; move 3 to the front.
	if _, err := s.Move(ctx, qid, 2, 1); err != nil {
		t.Fatal(err)
	}
	if got := waitingIDs(t, s, qid); !slices.Equal(got, []int64{3, 1}) {
		t.Errorf("waiting = %v, want [3 1]", got)
	}
	q, err := s.Queue(ctx, qid)
	if err != nil {
		t.Fatal(err)
	}
	if !q.Has(2) || q.Position(2) != 0 {
		t.Error("the done user should still be in the queue, not waiting")
	}
}
