package handler

import (
	"context"
	"log"
	"sync"
	"time"
)

// redrawDelay is how long the bot waits after a change before editing the
// board. Telegram allows about 20 edits a minute in a group, so presses that
// arrive close together are drawn with one edit.
const redrawDelay = 1500 * time.Millisecond

// redrawer edits each board at most once per delay. Schedule can be called
// on every change; the draw that eventually runs loads the latest state,
// so it includes every change made while waiting.
type redrawer struct {
	ctx   context.Context
	delay time.Duration
	draw  func(ctx context.Context, queueID int64) error

	mu      sync.Mutex
	pending map[int64]bool
}

func newRedrawer(ctx context.Context, delay time.Duration, draw func(context.Context, int64) error) *redrawer {
	return &redrawer{
		ctx:     ctx,
		delay:   delay,
		draw:    draw,
		pending: make(map[int64]bool),
	}
}

// Schedule asks for the queue's board to be redrawn soon. Calls for a queue
// that is already waiting to be redrawn are merged into that one redraw.
func (r *redrawer) Schedule(queueID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.pending[queueID] {
		return
	}
	r.pending[queueID] = true

	time.AfterFunc(r.delay, func() {
		// Clear the flag before drawing: a change that lands while the
		// board is being drawn schedules one more redraw instead of
		// being lost.
		r.mu.Lock()
		delete(r.pending, queueID)
		r.mu.Unlock()

		if r.ctx.Err() != nil {
			return
		}
		if err := r.draw(r.ctx, queueID); err != nil {
			log.Printf("redraw board of queue %d: %v", queueID, err)
		}
	})
}
