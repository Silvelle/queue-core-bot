package service

import (
	"sync"
	"time"

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
