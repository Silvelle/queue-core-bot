package handler

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/go-telegram/bot"
)

// redrawer keeps boards up to date without delay. A change draws its board
// at once. Changes that arrive while that board is being drawn are
// collected and drawn together right after, which gives two guarantees:
//
//   - edits of one board never overlap, so an edit showing an older state
//     can't reach Telegram after a newer one;
//   - a burst of presses becomes two or three edits, not one per press.
type redrawer struct {
	ctx  context.Context
	draw func(ctx context.Context, queueID int64) error

	mu sync.Mutex
	// again holds the boards being drawn right now. The value is true if
	// the board changed during the draw and must be drawn once more.
	again map[int64]bool
}

func newRedrawer(ctx context.Context, draw func(context.Context, int64) error) *redrawer {
	return &redrawer{ctx: ctx, draw: draw, again: make(map[int64]bool)}
}

// Schedule redraws the queue's board now, or right after the draw already
// in progress.
func (r *redrawer) Schedule(queueID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, drawing := r.again[queueID]; drawing {
		r.again[queueID] = true
		return
	}
	r.again[queueID] = false
	go r.run(queueID)
}

// run draws the board until no change is left to show.
func (r *redrawer) run(queueID int64) {
	for {
		if r.ctx.Err() != nil {
			r.finish(queueID)
			return
		}

		err := r.draw(r.ctx, queueID)
		if wait, limited := retryAfter(err); limited {
			// Telegram says how long to wait. The board is drawn again
			// afterwards, so the change isn't lost.
			log.Printf("redraw board of queue %d: rate limited, waiting %v", queueID, wait)
			select {
			case <-time.After(wait):
			case <-r.ctx.Done():
			}
			r.mu.Lock()
			r.again[queueID] = true
			r.mu.Unlock()
		} else if err != nil {
			log.Printf("redraw board of queue %d: %v", queueID, err)
		}

		r.mu.Lock()
		if !r.again[queueID] {
			delete(r.again, queueID)
			r.mu.Unlock()
			return
		}
		r.again[queueID] = false
		r.mu.Unlock()
	}
}

func (r *redrawer) finish(queueID int64) {
	r.mu.Lock()
	delete(r.again, queueID)
	r.mu.Unlock()
}

// retryAfter reports Telegram's "too many requests" answer and how long it
// asks to wait.
func retryAfter(err error) (time.Duration, bool) {
	var tooMany *bot.TooManyRequestsError
	if !errors.As(err, &tooMany) {
		return 0, false
	}
	return time.Duration(tooMany.RetryAfter) * time.Second, true
}
