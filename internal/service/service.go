package service

import (
	"context"
	"sync"
	"time"

	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/storage"
)

type Service struct {
	store storage.Storage
	now   func() time.Time
	// one person per sheet can edit the table
	createMu sync.Mutex
	locksMu  sync.Mutex

	locks map[int64]*sync.Mutex
}

func New(store storage.Storage) *Service {
	return &Service{
		store: store,
		now:   time.Now,
		locks: make(map[int64]*sync.Mutex),
	}
}

// lock one queue, different queues don't block each other
func (s *Service) lock(queueID int64) (unlock func()) {
	s.locksMu.Lock()
	l, ok := s.locks[queueID]
	if !ok {
		l = &sync.Mutex{}
		s.locks[queueID] = l
	}
	s.locksMu.Unlock()
	l.Lock()
	return l.Unlock
}

// update loads a queue, lets fn change it and save the result.
// if function occurs error: storage hands out copies.
func (s *Service) update(
	ctx context.Context,
	queueID int64,
	fn func(q *model.Queue) error) (model.Queue, error) {
	unlock := s.lock(queueID)
	defer unlock()

	q, err := s.store.Queue(ctx, queueID)
	if err != nil {
		return model.Queue{}, err
	}
	if q.Closed {
		return model.Queue{}, model.ErrQueueClosed
	}
	if err := fn(&q); err != nil {
		return model.Queue{}, err
	}
	if err := s.store.UpdateQueue(ctx, q); err != nil {
		return model.Queue{}, err
	}
	return q, nil
}

// waitingIndex returns the index in q.Entries of the user's waiting entry,
// or -1 if the user is not waiting.
func waitingIndex(q *model.Queue, userID int64) int {
	for i, e := range q.Entries {
		if e.UserID == userID && !e.Done {
			return i
		}
	}
	return -1
}

func (s *Service) Queue(ctx context.Context, queueID int64) (model.Queue, error) {
	return s.store.Queue(ctx, queueID)
}

func (s *Service) OpenQueues(ctx context.Context, chatID int64) ([]model.Queue, error) {
	return s.store.OpenQueues(ctx, chatID)
}
