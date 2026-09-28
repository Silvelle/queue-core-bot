package handler

import (
	"context"
	"sync"
	"testing"
	"time"
)

const testDelay = 20 * time.Millisecond

// countingDraw records every draw per queue and signals each one.
type countingDraw struct {
	mu     sync.Mutex
	counts map[int64]int
	calls  chan int64
}

func newCountingDraw() *countingDraw {
	return &countingDraw{counts: make(map[int64]int), calls: make(chan int64, 100)}
}

func (d *countingDraw) draw(_ context.Context, queueID int64) error {
	d.mu.Lock()
	d.counts[queueID]++
	d.mu.Unlock()
	d.calls <- queueID
	return nil
}

func (d *countingDraw) count(queueID int64) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.counts[queueID]
}

// waitDraw waits for the next draw, failing the test if none comes.
func (d *countingDraw) waitDraw(t *testing.T) int64 {
	t.Helper()
	select {
	case id := <-d.calls:
		return id
	case <-time.After(time.Second):
		t.Fatal("no redraw happened")
		return 0
	}
}

// settle waits long enough that any redraw still scheduled would have run.
func settle() { time.Sleep(5 * testDelay) }

// 15 people press Join at once: the board is edited once, not 15 times.
func TestRedrawMergesBurst(t *testing.T) {
	d := newCountingDraw()
	r := newRedrawer(context.Background(), testDelay, d.draw)

	for range 15 {
		r.Schedule(42)
	}
	d.waitDraw(t)
	settle()

	if got := d.count(42); got != 1 {
		t.Errorf("%d redraws, want 1", got)
	}
}

// A change after a redraw has run gets a redraw of its own.
func TestRedrawAgainAfterDraw(t *testing.T) {
	d := newCountingDraw()
	r := newRedrawer(context.Background(), testDelay, d.draw)

	r.Schedule(42)
	d.waitDraw(t)
	r.Schedule(42)
	d.waitDraw(t)
	settle()

	if got := d.count(42); got != 2 {
		t.Errorf("%d redraws, want 2", got)
	}
}

// Different boards don't wait for each other.
func TestRedrawQueuesAreIndependent(t *testing.T) {
	d := newCountingDraw()
	r := newRedrawer(context.Background(), testDelay, d.draw)

	r.Schedule(1)
	r.Schedule(2)
	r.Schedule(1)
	d.waitDraw(t)
	d.waitDraw(t)
	settle()

	if d.count(1) != 1 || d.count(2) != 1 {
		t.Errorf("redraws = %d for queue 1 and %d for queue 2, want 1 each", d.count(1), d.count(2))
	}
}

// Nothing is drawn before the delay has passed.
func TestRedrawWaitsForDelay(t *testing.T) {
	d := newCountingDraw()
	r := newRedrawer(context.Background(), 200*time.Millisecond, d.draw)

	r.Schedule(42)
	time.Sleep(50 * time.Millisecond)
	if got := d.count(42); got != 0 {
		t.Errorf("redrew after 50ms, want to wait 200ms")
	}
}

// After the bot stops, pending redraws are dropped instead of calling
// Telegram with a cancelled context.
func TestRedrawSkippedAfterShutdown(t *testing.T) {
	d := newCountingDraw()
	ctx, cancel := context.WithCancel(context.Background())
	r := newRedrawer(ctx, testDelay, d.draw)

	r.Schedule(42)
	cancel()
	settle()

	if got := d.count(42); got != 0 {
		t.Errorf("%d redraws after shutdown, want 0", got)
	}
}
