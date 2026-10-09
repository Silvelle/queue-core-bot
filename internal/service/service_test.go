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

// setup creates a service with one queue holding users in order. The queue
// is written straight to storage, so each test only depends on the function
// it checks.
func setup(t *testing.T, users []int64) (*Service, int64) {
	t.Helper()
	store := memory.New()

	var entries []model.Entry
	for _, id := range users {
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

// ids returns the users in the queue in order and checks that nobody
// appears twice.
func ids(t *testing.T, s *Service, queueID int64) []int64 {
	t.Helper()
	q, err := s.Queue(context.Background(), queueID)
	if err != nil {
		t.Fatal(err)
	}

	seen := make(map[int64]bool)
	var out []int64
	for _, e := range q.Entries {
		if seen[e.UserID] {
			t.Fatalf("user %d is in the queue twice", e.UserID)
		}
		seen[e.UserID] = true
		out = append(out, e.UserID)
	}
	return out
}

// known saves users, so Place can insert them.
func known(t *testing.T, s *Service, users ...int64) {
	t.Helper()
	for _, id := range users {
		if err := s.SaveUser(context.Background(), model.User{ID: id, FirstName: "Тест"}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCreate(t *testing.T) {
	ctx := context.Background()
	s := New(memory.New())

	first, err := s.Create(ctx, chatID, 0, "  Practice 4  ", 7)
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
		name     string
		chatID   int64
		threadID int
		qname    string
		wantErr  error
	}{
		{"empty", chatID, 0, "", model.ErrInvalidName},
		{"only spaces", chatID, 0, "   ", model.ErrInvalidName},
		{"65 letters", chatID, 0, strings.Repeat("a", 65), model.ErrInvalidName},
		{"64 letters", chatID, 0, strings.Repeat("a", 64), nil},
		// Cyrillic letters are 2 bytes each: 64 of them must still fit.
		{"64 cyrillic letters", chatID, 0, strings.Repeat("я", 64), nil},
		{"same name", chatID, 0, "Practice 4", model.ErrQueueExists},
		{"same name, other case", chatID, 0, "PRACTICE 4", model.ErrQueueExists},
		{"same name with spaces", chatID, 0, " Practice 4 ", model.ErrQueueExists},
		{"same name, other chat", 20, 0, "Practice 4", nil},
		{"same name, other topic", chatID, 5, "Practice 4", nil},
		{"new name", chatID, 0, "Lab 2", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := s.Create(ctx, tt.chatID, tt.threadID, tt.qname, 1)
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

	if _, err := New(store).Create(ctx, chatID, 0, "Practice 4", 1); err != nil {
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
			_, err := s.Create(ctx, chatID, 0, "Practice 4", 1)
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

// Each topic has its own queues.
func TestQueueAndOpenQueues(t *testing.T) {
	ctx := context.Background()
	s := New(memory.New())

	a, err := s.Create(ctx, chatID, 0, "A", 1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(ctx, chatID, 0, "B", 1)
	if err != nil {
		t.Fatal(err)
	}
	topic, err := s.Create(ctx, chatID, 7, "In topic", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(ctx, 20, 0, "other chat", 1); err != nil {
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

	open, err := s.OpenQueues(ctx, chatID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 2 || open[0].ID != a.ID || open[1].ID != b.ID {
		t.Errorf("OpenQueues(%d, 0) = %+v, want A then B", chatID, open)
	}

	inTopic, err := s.OpenQueues(ctx, chatID, 7)
	if err != nil {
		t.Fatal(err)
	}
	if len(inTopic) != 1 || inTopic[0].ID != topic.ID || inTopic[0].ThreadID != 7 {
		t.Errorf("OpenQueues(%d, 7) = %+v, want only the topic's queue", chatID, inTopic)
	}
}

func TestSetBoardMessage(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{1, 2})

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
	if got := ids(t, s, qid); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("queue = %v, want it unchanged", got)
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
		{"already in the queue", 2, []int64{1, 2, 3}, 0, model.ErrAlreadyJoined},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, qid := setup(t, []int64{1, 2, 3})
			pos, err := s.Join(context.Background(), qid, tt.user)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if pos != tt.wantPos {
				t.Errorf("position = %d, want %d", pos, tt.wantPos)
			}
			if got := ids(t, s, qid); !slices.Equal(got, tt.want) {
				t.Errorf("queue = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJoinEmptyQueue(t *testing.T) {
	s, qid := setup(t, nil)
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, qid := setup(t, []int64{1, 2, 3})
			err := s.Leave(context.Background(), qid, tt.user)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got := ids(t, s, qid); !slices.Equal(got, tt.want) {
				t.Errorf("queue = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLeaveThenJoinAgain(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{1, 2, 3})

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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, qid := setup(t, []int64{1, 2, 3})
			pos, err := s.ToEnd(context.Background(), qid, tt.user)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if pos != tt.wantPos {
				t.Errorf("position = %d, want %d", pos, tt.wantPos)
			}
			if got := ids(t, s, qid); !slices.Equal(got, tt.want) {
				t.Errorf("queue = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClose(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{1, 2})

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
	if got := ids(t, s, qid); !slices.Equal(got, []int64{1, 2}) {
		t.Errorf("queue = %v, want the people kept after closing", got)
	}

	open, err := s.OpenQueues(ctx, chatID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 0 {
		t.Errorf("OpenQueues still lists %d queues after closing", len(open))
	}
}

// /swap <pos>: the sender swaps with whoever is at pos.
func TestSwapWithPosition(t *testing.T) {
	tests := []struct {
		name    string
		user    int64
		pos     int
		want    []int64
		wantRes Swapped
		wantErr error
	}{
		{"with last", 1, 3, []int64{3, 2, 1}, Swapped{UserID: 1, TargetID: 3, From: 1, To: 3}, nil},
		{"with first", 3, 1, []int64{3, 2, 1}, Swapped{UserID: 3, TargetID: 1, From: 3, To: 1}, nil},
		{"neighbours", 2, 3, []int64{1, 3, 2}, Swapped{UserID: 2, TargetID: 3, From: 2, To: 3}, nil},
		{"own position", 1, 1, []int64{1, 2, 3}, Swapped{}, model.ErrSelfSwap},
		{"zero", 1, 0, []int64{1, 2, 3}, Swapped{}, model.ErrInvalidPosition},
		{"past the end", 1, 4, []int64{1, 2, 3}, Swapped{}, model.ErrInvalidPosition},
		{"sender not in queue", 9, 2, []int64{1, 2, 3}, Swapped{}, model.ErrNotInQueue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, qid := setup(t, []int64{1, 2, 3})
			res, err := s.SwapWithPosition(context.Background(), qid, tt.user, tt.pos)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if res != tt.wantRes {
				t.Errorf("result = %+v, want %+v", res, tt.wantRes)
			}
			if got := ids(t, s, qid); !slices.Equal(got, tt.want) {
				t.Errorf("queue = %v, want %v", got, tt.want)
			}
		})
	}
}

// /swap <who> <who>: two other people swap; each is a position or an ID.
func TestSwap(t *testing.T) {
	tests := []struct {
		name    string
		a, b    int64
		want    []int64
		wantRes Swapped
		wantErr error
	}{
		{"by positions", 1, 3, []int64{1003, 1002, 1001, 1004}, Swapped{UserID: 1001, TargetID: 1003, From: 1, To: 3}, nil},
		{"by IDs", 1002, 1004, []int64{1001, 1004, 1003, 1002}, Swapped{UserID: 1002, TargetID: 1004, From: 2, To: 4}, nil},
		{"position and ID", 4, 1001, []int64{1004, 1002, 1003, 1001}, Swapped{UserID: 1004, TargetID: 1001, From: 4, To: 1}, nil},
		{"same person", 2, 1002, nil, Swapped{}, model.ErrSelfSwap},
		{"first not in queue", 7777, 1, nil, Swapped{}, model.ErrTargetNotInQueue},
		{"second not in queue", 1, 7777, nil, Swapped{}, model.ErrTargetNotInQueue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := []int64{1001, 1002, 1003, 1004}
			s, qid := setup(t, before)
			res, err := s.Swap(context.Background(), qid, tt.a, tt.b)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if res != tt.wantRes {
				t.Errorf("result = %+v, want %+v", res, tt.wantRes)
			}
			want := tt.want
			if tt.wantErr != nil {
				want = before // a refused swap changes nothing
			}
			if got := ids(t, s, qid); !slices.Equal(got, want) {
				t.Errorf("queue = %v, want %v", got, want)
			}
		})
	}
}

// /delete <who>: a person leaves, everyone after them moves up.
func TestRemove(t *testing.T) {
	tests := []struct {
		name    string
		who     int64
		want    []int64
		wantRes Removed
		wantErr error
	}{
		{"by position", 2, []int64{1001, 1003, 1004}, Removed{UserID: 1002, From: 2}, nil},
		{"first", 1, []int64{1002, 1003, 1004}, Removed{UserID: 1001, From: 1}, nil},
		{"by ID", 1004, []int64{1001, 1002, 1003}, Removed{UserID: 1004, From: 4}, nil},
		{"position past the end", 5, nil, Removed{}, model.ErrTargetNotInQueue},
		{"unknown ID", 7777, nil, Removed{}, model.ErrTargetNotInQueue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := []int64{1001, 1002, 1003, 1004}
			s, qid := setup(t, before)
			res, err := s.Remove(context.Background(), qid, tt.who)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if res != tt.wantRes {
				t.Errorf("result = %+v, want %+v", res, tt.wantRes)
			}
			want := tt.want
			if tt.wantErr != nil {
				want = before
			}
			if got := ids(t, s, qid); !slices.Equal(got, want) {
				t.Errorf("queue = %v, want %v", got, want)
			}
		})
	}
}

// /place moves someone already in the queue.
func TestPlace(t *testing.T) {
	tests := []struct {
		name    string
		who     int64
		to      int
		want    []int64
		wantRes Placed
		wantErr error
	}{
		// Queue: 1 2 3 4 1005.
		{"last back to third, by position", 5, 3, []int64{1, 2, 1005, 3, 4}, Placed{UserID: 1005, From: 5, To: 3}, nil},
		{"to the front", 4, 1, []int64{4, 1, 2, 3, 1005}, Placed{UserID: 4, From: 4, To: 1}, nil},
		{"forward to the end", 1, 5, []int64{2, 3, 4, 1005, 1}, Placed{UserID: 1, From: 1, To: 5}, nil},
		{"one step back", 2, 3, []int64{1, 3, 2, 4, 1005}, Placed{UserID: 2, From: 2, To: 3}, nil},
		{"by ID", 1005, 2, []int64{1, 1005, 2, 3, 4}, Placed{UserID: 1005, From: 5, To: 2}, nil},
		{"to the end by default", 2, AtEnd, []int64{1, 3, 4, 1005, 2}, Placed{UserID: 2, From: 2, To: 5}, nil},
		{"last to the end by default", 5, AtEnd, nil, Placed{}, model.ErrAlreadyThere},
		{"already there", 3, 3, nil, Placed{}, model.ErrAlreadyThere},
		{"negative place", 2, -1, nil, Placed{}, model.ErrInvalidPosition},
		{"past the end", 2, 6, nil, Placed{}, model.ErrInvalidPosition},
		{"not in the queue", 2000, 2, nil, Placed{}, model.ErrTargetNotInQueue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := []int64{1, 2, 3, 4, 1005}
			s, qid := setup(t, before)

			res, err := s.Place(context.Background(), qid, tt.who, tt.to)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if res != tt.wantRes {
				t.Errorf("result = %+v, want %+v", res, tt.wantRes)
			}
			want := tt.want
			if tt.wantErr != nil {
				want = before // a refused place changes nothing
			}
			if got := ids(t, s, qid); !slices.Equal(got, want) {
				t.Errorf("queue = %v, want %v", got, want)
			}
		})
	}
}

// /add puts someone new into the queue, at the end by default.
func TestAdd(t *testing.T) {
	tests := []struct {
		name    string
		user    int64
		to      int
		want    []int64
		wantRes Placed
		wantErr error
	}{
		// Queue: 1001 1002 1003; 2000 is known to the bot, 7777 isn't.
		{"at the end by default", 2000, AtEnd, []int64{1001, 1002, 1003, 2000}, Placed{UserID: 2000, To: 4}, nil},
		{"at a place", 2000, 2, []int64{1001, 2000, 1002, 1003}, Placed{UserID: 2000, To: 2}, nil},
		{"at the front", 2000, 1, []int64{2000, 1001, 1002, 1003}, Placed{UserID: 2000, To: 1}, nil},
		{"right after the last", 2000, 4, []int64{1001, 1002, 1003, 2000}, Placed{UserID: 2000, To: 4}, nil},
		{"past the end", 2000, 5, nil, Placed{}, model.ErrInvalidPosition},
		{"negative place", 2000, -1, nil, Placed{}, model.ErrInvalidPosition},
		{"already in the queue", 1002, AtEnd, nil, Placed{}, model.ErrTargetInQueue},
		{"unknown user", 7777, AtEnd, nil, Placed{}, model.ErrUnknownUser},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := []int64{1001, 1002, 1003}
			s, qid := setup(t, before)
			known(t, s, 1002, 2000)

			res, err := s.Add(context.Background(), qid, tt.user, tt.to)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if res != tt.wantRes {
				t.Errorf("result = %+v, want %+v", res, tt.wantRes)
			}
			want := tt.want
			if tt.wantErr != nil {
				want = before
			}
			if got := ids(t, s, qid); !slices.Equal(got, want) {
				t.Errorf("queue = %v, want %v", got, want)
			}
		})
	}
}

// Someone who pressed "Выйти" by mistake comes back to their old place.
func TestAddAfterLeaveByMistake(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{1001, 1002, 1003, 1004})
	known(t, s, 1002)
	if err := s.Leave(ctx, qid, 1002); err != nil {
		t.Fatal(err)
	}

	res, err := s.Add(ctx, qid, 1002, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Added() || res.To != 2 {
		t.Errorf("result = %+v, want an add at №2", res)
	}
	if got := ids(t, s, qid); !slices.Equal(got, []int64{1001, 1002, 1003, 1004}) {
		t.Errorf("queue = %v, want everyone back in their old order", got)
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
	known(t, s, 3)

	ops := map[string]func() error{
		"join":             func() error { _, err := s.Join(ctx, q.ID, 9); return err },
		"leave":            func() error { return s.Leave(ctx, q.ID, 1) },
		"toEnd":            func() error { _, err := s.ToEnd(ctx, q.ID, 1); return err },
		"swapWithPosition": func() error { _, err := s.SwapWithPosition(ctx, q.ID, 1, 2); return err },
		"swap":             func() error { _, err := s.Swap(ctx, q.ID, 1, 2); return err },
		"remove":           func() error { _, err := s.Remove(ctx, q.ID, 1); return err },
		"place":            func() error { _, err := s.Place(ctx, q.ID, 1, 2); return err },
		"add":              func() error { _, err := s.Add(ctx, q.ID, 3, AtEnd); return err },
		"setBoardMessage":  func() error { return s.SetBoardMessage(ctx, q.ID, 1) },
		"close":            func() error { return s.Close(ctx, q.ID) },
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
		"join":   func() error { _, err := s.Join(ctx, 999, 1); return err },
		"leave":  func() error { return s.Leave(ctx, 999, 1) },
		"toEnd":  func() error { _, err := s.ToEnd(ctx, 999, 1); return err },
		"swap":   func() error { _, err := s.Swap(ctx, 999, 1, 2); return err },
		"remove": func() error { _, err := s.Remove(ctx, 999, 1); return err },
		"place":  func() error { _, err := s.Place(ctx, 999, 1, 1); return err },
		"add":    func() error { _, err := s.Add(ctx, 999, 1, AtEnd); return err },
		"close":  func() error { return s.Close(ctx, 999) },
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
	s, qid := setup(t, nil)

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

	if got := ids(t, s, qid); len(got) != n {
		t.Fatalf("%d users in the queue, want %d", len(got), n)
	}
}

// Everyone does random things at once. Whatever order they run in, nobody
// may end up in the queue twice.
func TestConcurrentMixedOperations(t *testing.T) {
	ctx := context.Background()
	const n = 30
	users := make([]int64, n)
	for i := range users {
		users[i] = int64(1000 + i)
	}
	s, qid := setup(t, users)
	known(t, s, users...)

	var wg sync.WaitGroup
	for _, u := range users {
		wg.Go(func() {
			for range 20 {
				switch rand.IntN(8) {
				case 0:
					_, _ = s.Join(ctx, qid, u)
				case 1:
					_ = s.Leave(ctx, qid, u)
				case 2:
					_, _ = s.ToEnd(ctx, qid, u)
				case 3:
					_, _ = s.SwapWithPosition(ctx, qid, u, rand.IntN(n)+1)
				case 4:
					_, _ = s.Swap(ctx, qid, int64(rand.IntN(n)+1), u)
				case 5:
					_, _ = s.Remove(ctx, qid, int64(rand.IntN(n)+1))
				case 6:
					_, _ = s.Place(ctx, qid, u, rand.IntN(n)+1)
				case 7:
					_, _ = s.Add(ctx, qid, u, rand.IntN(n)+1)
				}
			}
		})
	}
	wg.Wait()

	ids(t, s, qid)
}

func TestNames(t *testing.T) {
	ctx := context.Background()
	s, qid := setup(t, []int64{2, 3, 5})

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
	got, err := s.Names(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	// User 1 is known but not in the queue, and user 3 was never saved.
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
	s, qid := setup(t, []int64{2})

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
