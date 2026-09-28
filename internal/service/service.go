package service

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/storage"
)

const maxNameLen = 64

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

// Create opens a new queue in the chat. Names are trimmed and must be unique
// among the chat's open queues, ignoring case.
func (s *Service) Create(ctx context.Context, chatID int64, name string, createdBy int64) (model.Queue, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return model.Queue{}, model.ErrInvalidName
	}

	s.createMu.Lock()
	defer s.createMu.Unlock()

	open, err := s.store.OpenQueues(ctx, chatID)
	if err != nil {
		return model.Queue{}, err
	}
	for _, q := range open {
		if strings.EqualFold(name, q.Name) {
			return model.Queue{}, model.ErrQueueExists
		}
	}

	return s.store.CreateQueue(ctx, model.Queue{
		ChatID:    chatID,
		Name:      name,
		CreatedBy: createdBy,
		CreatedAt: s.now(),
	})
}

// SetBoardMessage remembers which message shows the queue, so it can be
// edited later.
func (s *Service) SetBoardMessage(ctx context.Context, queueID int64, msgID int) error {
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		q.BoardMsgID = msgID
		return nil
	})
	return err
}

// Join adds user to the end of the queue and returns their position
func (s *Service) Join(ctx context.Context, queueID, userID int64) (int, error) {
	q, err := s.update(ctx, queueID, func(q *model.Queue) error {
		for _, e := range q.Entries {
			if e.UserID != userID {
				continue
			}
			if e.Done {
				return model.ErrAlreadyDone
			}
			return model.ErrAlreadyJoined
		}
		q.Entries = append(q.Entries, model.Entry{UserID: userID, JoinedAt: s.now()})
		return nil
	})
	if err != nil {
		return 0, err
	}
	return q.Position(userID), nil
}

// Leave removes a waiting user from teh queue. They can join again later
func (s *Service) Leave(ctx context.Context, queueID, userID int64) error {
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		i := waitingIndex(q, userID)
		if i < 0 {
			return model.ErrNotInQueue
		}
		q.Entries = append(q.Entries[:i], q.Entries[i+1:]...)
		return nil
	})
	return err
}

// ToEnd moves a waiting user behind everyone else and returns their new position.
func (s *Service) ToEnd(ctx context.Context, queueID, userID int64) (int, error) {
	q, err := s.update(ctx, queueID, func(q *model.Queue) error {
		i := waitingIndex(q, userID)
		if i < 0 {
			return model.ErrNotInQueue
		}
		e := q.Entries[i]
		q.Entries = append(q.Entries[:i], q.Entries[i+1:]...)
		q.Entries = append(q.Entries, e)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return q.Position(userID), nil
}

// Done marks the user as defended. Only the user can mark themselves.
func (s *Service) Done(ctx context.Context, queueID, userID int64) error {
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		i := waitingIndex(q, userID)
		if i < 0 {
			return model.ErrNotInQueue
		}
		q.Entries[i].Done = true
		q.Entries[i].DoneAt = s.now()
		return nil
	})
	return err
}

type Swapped struct {
	TargetID int64
	From     int
	To       int
}

// Swaps the user with another waiting user. The inline picker uses it
// because it knows who was picked
func (s *Service) SwapWith(ctx context.Context, queueID, userID, targetID int64) (Swapped, error) {
	var res Swapped
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		var err error
		res, err = swap(q, userID, targetID)
		return err
	})
	return res, err
}

// SwapWithPosition swaps the usee with whoever is at pos. The /swap command
// uses it.
func (s *Service) SwapWithPosition(ctx context.Context, queueID, userID int64, pos int) (Swapped, error) {
	var res Swapped
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		waiting := q.Waiting()
		if pos < 1 || pos > len(waiting) {
			return model.ErrInvalidPosition
		}
		var err error
		res, err = swap(q, userID, waiting[pos-1].UserID)
		return err
	})
	return res, err
}

func swap(q *model.Queue, userID int64, targetID int64) (Swapped, error) {
	if userID == targetID {
		return Swapped{}, model.ErrSelfSwap
	}
	i := waitingIndex(q, userID)
	if i < 0 {
		return Swapped{}, model.ErrNotInQueue
	}
	j := waitingIndex(q, targetID)
	if j < 0 {
		return Swapped{}, model.ErrTargetNotInQueue
	}

	res := Swapped{TargetID: targetID, From: q.Position(userID), To: q.Position(targetID)}

	q.Entries[i], q.Entries[j] = q.Entries[j], q.Entries[i]
	return res, nil
}

// Close freezes the queue. Closing twice returns model.ErrQueueClosed.
func (s *Service) Close(ctx context.Context, queueID int64) error {
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		q.Closed = true
		return nil
	})
	return err
}
