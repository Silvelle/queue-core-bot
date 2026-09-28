package service

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/storage"
	"github.com/Silvelle/queue-core-bot/internal/storage/memory"
)

type updateErrorStorage struct {
	storage.Storage
	err error
}

func (s updateErrorStorage) UpdateQueue(context.Context, model.Queue) error {
	return s.err
}

func TestLock_SameIDIsExclusive(t *testing.T) {
	s := New(memory.New())

	counter := 0
	const n = 100

	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			unlock := s.lock(42)
			defer unlock()

			v := counter
			runtime.Gosched()
			counter = v + 1
		})
	}
	wg.Wait()

	if counter != n {
		t.Fatalf("got %d locks, want %d", counter, n)
	}
}

func TestLock_DifferentIDsDoNotBlock(t *testing.T) {
	s := New(memory.New())
	unlock1 := s.lock(1)
	defer unlock1()

	done := make(chan struct{})
	go func() {
		unlock2 := s.lock(2)
		unlock2()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("lock(2) blocked while lock(1) was held")
	}
}

func TestLock_ConcurrentMapAccess(t *testing.T) {
	s := New(memory.New())

	var wg sync.WaitGroup
	for i := range 10 {
		wg.Go(func() {
			unlock := s.lock(int64(i % 10))
			unlock()
		})
	}
	wg.Wait()
	if got := len(s.locks); got != 10 {
		t.Fatalf("got %d locks, want 10", got)
	}
}

func TestUpdate_AppliesAndSaves(t *testing.T) {
	store := memory.New()
	s := New(store)
	ctx := context.Background()

	queue, err := store.CreateQueue(ctx, model.Queue{Name: "original"})
	if err != nil {
		t.Fatalf("create queue: %v", err)
	}

	got, err := s.update(ctx, queue.ID, func(q *model.Queue) error {
		q.Name = "renamed"
		return nil
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.Name != "renamed" {
		t.Errorf("got %q, want renamed %q", got.Name, "renamed")
	}

	fromStore, err := store.Queue(ctx, queue.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if fromStore.Name != "renamed" {
		t.Errorf("stored name = %q, want %q", fromStore.Name, "renamed")
	}
}

func TestUpdate_CallbackErrorDiscardsChanges(t *testing.T) {
	store := memory.New()
	s := New(store)
	ctx := context.Background()

	queue, err := store.CreateQueue(ctx, model.Queue{Name: "original"})
	if err != nil {
		t.Fatalf("create queue: %v", err)
	}

	sentinel := errors.New("boom")

	_, err = s.update(ctx, queue.ID, func(q *model.Queue) error {
		q.Name = "mutated"
		return sentinel
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}

	fromStore, err := store.Queue(ctx, queue.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}

	if fromStore.Name != "original" {
		t.Errorf(
			"stored Name = %q, want %q",
			fromStore.Name,
			"original",
		)
	}
}

func TestUpdate_MissingQueueReturnsError(t *testing.T) {
	s := New(memory.New())
	ctx := context.Background()
	callbackCalled := false

	_, err := s.update(ctx, 999, func(q *model.Queue) error {
		callbackCalled = true
		return nil
	})

	if !errors.Is(err, model.ErrNotFound) {
		t.Fatalf("err = %v, want %v", err, model.ErrNotFound)
	}
	if callbackCalled {
		t.Error("callback was called for a missing queue")
	}
}

func TestUpdate_ClosedQueueRejectsChange(t *testing.T) {
	store := memory.New()
	s := New(store)
	ctx := context.Background()

	queue, err := store.CreateQueue(ctx, model.Queue{
		Name:   "closed queue",
		Closed: true,
	})
	if err != nil {
		t.Fatalf("create queue: %v", err)
	}

	callbackCalled := false
	_, err = s.update(ctx, queue.ID, func(q *model.Queue) error {
		callbackCalled = true
		q.Name = "mutated"
		return nil
	})

	if !errors.Is(err, model.ErrQueueClosed) {
		t.Fatalf("err = %v, want %v", err, model.ErrQueueClosed)
	}
	if callbackCalled {
		t.Error("callback was called for a closed queue")
	}

	fromStore, err := store.Queue(ctx, queue.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if fromStore.Name != "closed queue" {
		t.Errorf("stored Name = %q, want %q", fromStore.Name, "closed queue")
	}
}

func TestUpdate_StorageErrorIsReturned(t *testing.T) {
	store := memory.New()
	ctx := context.Background()

	queue, err := store.CreateQueue(ctx, model.Queue{Name: "original"})
	if err != nil {
		t.Fatalf("create queue: %v", err)
	}

	sentinel := errors.New("update failed")
	s := New(updateErrorStorage{Storage: store, err: sentinel})

	_, err = s.update(ctx, queue.ID, func(q *model.Queue) error {
		q.Name = "mutated"
		return nil
	})

	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}

	fromStore, err := store.Queue(ctx, queue.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if fromStore.Name != "original" {
		t.Errorf("stored Name = %q, want %q", fromStore.Name, "original")
	}
}

func TestUpdate_ConcurrentChangesAreNotLost(t *testing.T) {
	store := memory.New()
	s := New(store)
	ctx := context.Background()

	queue, err := store.CreateQueue(ctx, model.Queue{Name: "concurrent"})
	if err != nil {
		t.Fatalf("create queue: %v", err)
	}

	const updates = 100
	errorsCh := make(chan error, updates)

	var wg sync.WaitGroup
	for i := range updates {
		wg.Go(func() {
			_, err := s.update(ctx, queue.ID, func(q *model.Queue) error {
				q.Entries = append(q.Entries, model.Entry{UserID: int64(i + 1)})
				runtime.Gosched()
				return nil
			})
			if err != nil {
				errorsCh <- err
			}
		})
	}
	wg.Wait()
	close(errorsCh)

	for err := range errorsCh {
		t.Errorf("update: %v", err)
	}

	fromStore, err := store.Queue(ctx, queue.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(fromStore.Entries) != updates {
		t.Errorf("stored %d entries, want %d", len(fromStore.Entries), updates)
	}
}
