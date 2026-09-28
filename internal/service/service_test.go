package service

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Silvelle/queue-core-bot/internal/storage/memory"
)

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
