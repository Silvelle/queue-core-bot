// Package memory is an in-memory storage.Storage, used in tests and
// until the SQLite implementation lands. Data is lost on restart.
package memory

import (
	"cmp"
	"context"
	"slices"
	"sync"

	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/storage"
)

var _ storage.Storage = (*Storage)(nil)

type Storage struct {
	mu     sync.RWMutex
	users  map[int64]model.User
	queues map[int64]model.Queue
	nextID int64
}

func New() *Storage {
	return &Storage{
		users:  make(map[int64]model.User),
		queues: make(map[int64]model.Queue),
	}
}

func (s *Storage) SaveUser(_ context.Context, u model.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.users[u.ID] = u
	return nil
}

func (s *Storage) User(_ context.Context, id int64) (model.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	u, ok := s.users[id]
	if !ok {
		return model.User{}, model.ErrNotFound
	}
	return u, nil
}

func (s *Storage) CreateQueue(_ context.Context, q model.Queue) (model.Queue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	q.ID = s.nextID
	s.queues[q.ID] = clone(q)
	return clone(q), nil
}

func (s *Storage) Queue(_ context.Context, id int64) (model.Queue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	q, ok := s.queues[id]
	if !ok {
		return model.Queue{}, model.ErrNotFound
	}
	return clone(q), nil
}

func (s *Storage) UpdateQueue(_ context.Context, q model.Queue) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.queues[q.ID]; !ok {
		return model.ErrNotFound
	}
	s.queues[q.ID] = clone(q)
	return nil
}

func (s *Storage) OpenQueues(_ context.Context, chatID int64) ([]model.Queue, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []model.Queue
	for _, q := range s.queues {
		if q.ChatID == chatID && !q.Closed {
			out = append(out, clone(q))
		}
	}
	slices.SortFunc(out, func(a, b model.Queue) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

// clone copies the entries slice so callers can't mutate stored data
// through a shared backing array.
func clone(q model.Queue) model.Queue {
	q.Entries = slices.Clone(q.Entries)
	return q
}
