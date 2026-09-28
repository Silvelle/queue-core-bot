package handler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-telegram/bot"
)

// fakeDraw stands in for editing the board. Each draw waits for a value on
// release when blocking is on, so a test can hold a draw "in progress".
type fakeDraw struct {
	mu       sync.Mutex
	counts   map[int64]int
	calls    chan int64
	release  chan struct{}
	blocking bool

	running, maxRunning atomic.Int32
	failFirst           error
}

func newFakeDraw() *fakeDraw {
	return &fakeDraw{
		counts:  make(map[int64]int),
		calls:   make(chan int64, 100),
		release: make(chan struct{}),
	}
}

func (d *fakeDraw) draw(_ context.Context, queueID int64) error {
	n := d.running.Add(1)
	defer d.running.Add(-1)
	for {
		old := d.maxRunning.Load()
		if n <= old || d.maxRunning.CompareAndSwap(old, n) {
			break
		}
	}

	d.mu.Lock()
	d.counts[queueID]++
	first := d.counts[queueID] == 1
	blocking := d.blocking
	d.mu.Unlock()

	d.calls <- queueID
	if blocking {
		<-d.release
	}
	if first && d.failFirst != nil {
		return d.failFirst
	}
	return nil
}

func (d *fakeDraw) count(queueID int64) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.counts[queueID]
}

func (d *fakeDraw) waitDraw(t *testing.T) int64 {
	t.Helper()
	select {
	case id := <-d.calls:
		return id
	case <-time.After(time.Second):
		t.Fatal("no redraw happened")
		return 0
	}
}

// settle gives any stray redraw time to show up.
func settle() { time.Sleep(50 * time.Millisecond) }

// A single press is drawn at once, with no waiting.
func TestRedrawIsImmediate(t *testing.T) {
	d := newFakeDraw()
	r := newRedrawer(context.Background(), d.draw)

	start := time.Now()
	r.Schedule(42)
	d.waitDraw(t)
	if since := time.Since(start); since > 50*time.Millisecond {
		t.Errorf("redraw took %v, want no delay", since)
	}
}

// Presses one after another, like Сдано then Сброс, each get their own
// redraw right away.
func TestRedrawEveryPress(t *testing.T) {
	d := newFakeDraw()
	r := newRedrawer(context.Background(), d.draw)

	for range 3 {
		r.Schedule(42)
		d.waitDraw(t)
		settle()
	}
	if got := d.count(42); got != 3 {
		t.Errorf("%d redraws for 3 separate presses, want 3", got)
	}
}

// 15 presses while a draw is in progress become one more draw, not 15.
func TestRedrawMergesBurst(t *testing.T) {
	d := newFakeDraw()
	d.blocking = true
	r := newRedrawer(context.Background(), d.draw)

	r.Schedule(42)
	d.waitDraw(t) // first draw is now in progress
	for range 15 {
		r.Schedule(42)
	}
	d.release <- struct{}{}
	d.waitDraw(t) // the one merged draw
	d.release <- struct{}{}
	settle()

	if got := d.count(42); got != 2 {
		t.Errorf("%d redraws, want 2", got)
	}
}

// Edits of one board never overlap, so an old state can't land last.
func TestRedrawNeverOverlaps(t *testing.T) {
	d := newFakeDraw()
	r := newRedrawer(context.Background(), d.draw)

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { r.Schedule(42) })
	}
	wg.Wait()
	settle()

	if got := d.maxRunning.Load(); got != 1 {
		t.Errorf("%d draws of the same board ran at once, want 1", got)
	}
}

// Different boards are drawn independently.
func TestRedrawQueuesAreIndependent(t *testing.T) {
	d := newFakeDraw()
	d.blocking = true
	r := newRedrawer(context.Background(), d.draw)

	r.Schedule(1)
	d.waitDraw(t) // board 1 is held in progress
	r.Schedule(2)
	d.waitDraw(t) // board 2 isn't blocked by board 1
	d.release <- struct{}{}
	d.release <- struct{}{}
}

// After Telegram's "too many requests", the board is drawn again, so the
// change isn't lost.
func TestRedrawRetriesWhenRateLimited(t *testing.T) {
	d := newFakeDraw()
	d.failFirst = &bot.TooManyRequestsError{Message: "too many requests", RetryAfter: 0}
	r := newRedrawer(context.Background(), d.draw)

	r.Schedule(42)
	d.waitDraw(t)
	d.waitDraw(t)
	settle()

	if got := d.count(42); got != 2 {
		t.Errorf("%d redraws, want the failed one plus a retry", got)
	}
}

// After the bot stops, nothing is drawn with a cancelled context.
func TestRedrawSkippedAfterShutdown(t *testing.T) {
	d := newFakeDraw()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := newRedrawer(ctx, d.draw)

	r.Schedule(42)
	settle()
	if got := d.count(42); got != 0 {
		t.Errorf("%d redraws after shutdown, want 0", got)
	}
}
