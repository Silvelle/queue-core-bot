// Package storagetest checks that a storage.Storage behaves the way the
// service expects. Every implementation runs the same suite, which is what
// makes them interchangeable.
package storagetest

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/storage"
)

// Run runs the whole suite. newStore must return a new, empty storage for
// each call, so tests don't see each other's data.
func Run(t *testing.T, newStore func(t *testing.T) storage.Storage) {
	tests := []struct {
		name string
		fn   func(t *testing.T, s storage.Storage)
	}{
		{"Users", testUsers},
		{"UserNotFound", testUserNotFound},
		{"QueueLifecycle", testQueueLifecycle},
		{"QueueNotFound", testQueueNotFound},
		{"QueueRoundTrip", testQueueRoundTrip},
		{"UpdateReplacesEntries", testUpdateReplacesEntries},
		{"OpenQueues", testOpenQueues},
		{"QueueIsCopied", testQueueIsCopied},
		{"ConcurrentCreate", testConcurrentCreate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn(t, newStore(t))
		})
	}
}

func testUsers(t *testing.T, s storage.Storage) {
	ctx := context.Background()

	if err := s.SaveUser(ctx, model.User{ID: 1, FirstName: "Anna"}); err != nil {
		t.Fatal(err)
	}
	want := model.User{ID: 1, FirstName: "Anya", LastName: "Kuznetsova", Username: "anna_k"}
	if err := s.SaveUser(ctx, want); err != nil {
		t.Fatal(err)
	}

	got, err := s.User(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("User() = %+v, want the updated %+v", got, want)
	}
}

func testUserNotFound(t *testing.T, s storage.Storage) {
	if _, err := s.User(context.Background(), 1); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("User(unknown) error = %v, want ErrNotFound", err)
	}
}

func testQueueLifecycle(t *testing.T, s storage.Storage) {
	ctx := context.Background()

	q, err := s.CreateQueue(ctx, model.Queue{ChatID: 10, Name: "Practice 4"})
	if err != nil {
		t.Fatal(err)
	}
	if q.ID == 0 {
		t.Fatal("CreateQueue did not assign an ID")
	}

	q.Entries = append(q.Entries, model.Entry{UserID: 1}, model.Entry{UserID: 2})
	if err := s.UpdateQueue(ctx, q); err != nil {
		t.Fatal(err)
	}

	got, err := s.Queue(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Practice 4" || len(got.Entries) != 2 {
		t.Errorf("Queue() = %+v, want Practice 4 with 2 entries", got)
	}
}

func testQueueNotFound(t *testing.T, s storage.Storage) {
	ctx := context.Background()

	if _, err := s.Queue(ctx, 999); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("Queue(unknown) error = %v, want ErrNotFound", err)
	}
	if err := s.UpdateQueue(ctx, model.Queue{ID: 999}); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("UpdateQueue(unknown) error = %v, want ErrNotFound", err)
	}
}

// Every field must come back exactly as it was saved, including the order
// of entries, done flags and times.
func testQueueRoundTrip(t *testing.T, s storage.Storage) {
	ctx := context.Background()
	created := time.Date(2026, 9, 28, 10, 0, 0, 123456789, time.UTC)
	joined := created.Add(time.Minute)
	done := created.Add(time.Hour)

	want := model.Queue{
		ChatID:     -100123,
		Name:       "Практика 4",
		BoardMsgID: 555,
		CreatedBy:  7,
		CreatedAt:  created,
		Closed:     true,
		Entries: []model.Entry{
			{UserID: 3, JoinedAt: joined, Done: true, DoneAt: done},
			{UserID: 1, JoinedAt: joined},
			{UserID: 2},
		},
	}
	q, err := s.CreateQueue(ctx, want)
	if err != nil {
		t.Fatal(err)
	}
	want.ID = q.ID

	got, err := s.Queue(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertQueue(t, got, want)
}

// UpdateQueue replaces the entries, so removed entries must disappear.
func testUpdateReplacesEntries(t *testing.T, s storage.Storage) {
	ctx := context.Background()

	q, err := s.CreateQueue(ctx, model.Queue{ChatID: 10, Name: "A", Entries: []model.Entry{
		{UserID: 1}, {UserID: 2}, {UserID: 3},
	}})
	if err != nil {
		t.Fatal(err)
	}

	q.Name = "B"
	q.BoardMsgID = 42
	q.Entries = []model.Entry{{UserID: 3}, {UserID: 1}}
	if err := s.UpdateQueue(ctx, q); err != nil {
		t.Fatal(err)
	}

	got, err := s.Queue(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertQueue(t, got, q)
}

func testOpenQueues(t *testing.T, s storage.Storage) {
	ctx := context.Background()

	mustCreate := func(q model.Queue) model.Queue {
		t.Helper()
		q, err := s.CreateQueue(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		return q
	}
	a := mustCreate(model.Queue{ChatID: 10, Name: "A", Entries: []model.Entry{{UserID: 1}}})
	mustCreate(model.Queue{ChatID: 20, Name: "other chat"})
	mustCreate(model.Queue{ChatID: 10, Name: "closed", Closed: true})
	b := mustCreate(model.Queue{ChatID: 10, Name: "B"})

	got, err := s.OpenQueues(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != a.ID || got[1].ID != b.ID {
		t.Fatalf("OpenQueues(10) = %+v, want A then B", got)
	}
	if len(got[0].Entries) != 1 {
		t.Errorf("OpenQueues should load entries, got %+v", got[0].Entries)
	}

	none, err := s.OpenQueues(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Errorf("OpenQueues(30) = %+v, want none", none)
	}
}

// Callers get copies: changing a returned queue must not change what's stored
// until UpdateQueue is called. The service relies on this to validate a change
// before saving it.
func testQueueIsCopied(t *testing.T, s storage.Storage) {
	ctx := context.Background()

	q, err := s.CreateQueue(ctx, model.Queue{ChatID: 10, Entries: []model.Entry{{UserID: 1}}})
	if err != nil {
		t.Fatal(err)
	}

	q.Entries[0].UserID = 99
	loaded, err := s.Queue(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Entries[0].UserID != 1 {
		t.Fatal("mutating the queue returned by CreateQueue changed stored data")
	}

	loaded.Entries[0].UserID = 77
	again, err := s.Queue(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.Entries[0].UserID != 1 {
		t.Fatal("mutating the queue returned by Queue changed stored data")
	}
}

func testConcurrentCreate(t *testing.T, s storage.Storage) {
	ctx := context.Background()

	const n = 50
	var wg sync.WaitGroup
	ids := make([]int64, n)
	for i := range n {
		wg.Go(func() {
			q, err := s.CreateQueue(ctx, model.Queue{ChatID: 10})
			if err != nil {
				t.Error(err)
				return
			}
			ids[i] = q.ID
		})
	}
	wg.Wait()

	seen := make(map[int64]bool, n)
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate queue ID %d", id)
		}
		seen[id] = true
	}
}

// assertQueue compares queues field by field. Times are compared with Equal,
// since a storage may return them in another time zone.
func assertQueue(t *testing.T, got, want model.Queue) {
	t.Helper()

	if got.ID != want.ID || got.ChatID != want.ChatID || got.Name != want.Name ||
		got.BoardMsgID != want.BoardMsgID || got.CreatedBy != want.CreatedBy ||
		got.Closed != want.Closed || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("queue = %+v, want %+v", got, want)
	}

	if len(got.Entries) != len(want.Entries) {
		t.Fatalf("%d entries, want %d: %+v", len(got.Entries), len(want.Entries), got.Entries)
	}
	for i, g := range got.Entries {
		w := want.Entries[i]
		if g.UserID != w.UserID || g.Done != w.Done ||
			!g.JoinedAt.Equal(w.JoinedAt) || !g.DoneAt.Equal(w.DoneAt) {
			t.Errorf("entry %d = %+v, want %+v", i, g, w)
		}
	}
}
