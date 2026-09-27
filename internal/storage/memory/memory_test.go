package memory

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

func TestUsers(t *testing.T) {
	ctx := context.Background()
	s := New()

	if _, err := s.User(ctx, 1); !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("User(unknown) error = %v, want ErrNotFound", err)
	}

	if err := s.SaveUser(ctx, model.User{ID: 1, FirstName: "Anna"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveUser(ctx, model.User{ID: 1, FirstName: "Anya"}); err != nil {
		t.Fatal(err)
	}

	u, err := s.User(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if u.FirstName != "Anya" {
		t.Errorf("FirstName = %q, want the updated %q", u.FirstName, "Anya")
	}
}

func TestQueueLifecycle(t *testing.T) {
	ctx := context.Background()
	s := New()

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

	if _, err := s.Queue(ctx, 999); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("Queue(unknown) error = %v, want ErrNotFound", err)
	}
	if err := s.UpdateQueue(ctx, model.Queue{ID: 999}); !errors.Is(err, model.ErrNotFound) {
		t.Errorf("UpdateQueue(unknown) error = %v, want ErrNotFound", err)
	}
}

func TestOpenQueues(t *testing.T) {
	ctx := context.Background()
	s := New()

	mustCreate := func(q model.Queue) model.Queue {
		t.Helper()
		q, err := s.CreateQueue(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		return q
	}
	a := mustCreate(model.Queue{ChatID: 10, Name: "A"})
	mustCreate(model.Queue{ChatID: 20, Name: "other chat"})
	mustCreate(model.Queue{ChatID: 10, Name: "closed", Closed: true})
	b := mustCreate(model.Queue{ChatID: 10, Name: "B"})

	got, err := s.OpenQueues(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != a.ID || got[1].ID != b.ID {
		t.Errorf("OpenQueues(10) = %+v, want A then B", got)
	}
}

// Callers get copies: changing a returned queue must not change what's stored
// until UpdateQueue is called. The service relies on this to validate a change
// before saving it.
func TestQueueIsCopied(t *testing.T) {
	ctx := context.Background()
	s := New()

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

func TestConcurrentCreate(t *testing.T) {
	ctx := context.Background()
	s := New()

	const n = 100
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
