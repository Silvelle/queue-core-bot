package service

import (
	"context"
	"errors"
	"slices"
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

	// createMu makes "check the name is free, then create" one step, so
	// two /new commands with the same name can't both succeed.
	createMu sync.Mutex

	// locks holds one lock per queue, see lock.
	locksMu sync.Mutex
	locks   map[int64]*sync.Mutex
}

func New(store storage.Storage) *Service {
	return &Service{
		store: store,
		now:   time.Now,
		locks: make(map[int64]*sync.Mutex),
	}
}

// lock serializes all changes to one queue. Different queues don't block
// each other.
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

// update loads a queue, lets fn change it and saves the result. If fn
// returns an error, nothing is saved: storage hands out copies, so a
// half-done change never leaks.
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

// entryIndex returns the index in q.Entries of the user's entry, waiting or
// done, or -1 if the user isn't in the queue.
func entryIndex(q *model.Queue, userID int64) int {
	return slices.IndexFunc(q.Entries, func(e model.Entry) bool { return e.UserID == userID })
}

// waitingIndex is like entryIndex, but only for a user who is still waiting.
func waitingIndex(q *model.Queue, userID int64) int {
	if i := entryIndex(q, userID); i >= 0 && !q.Entries[i].Done {
		return i
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

// Join adds the user to the end of the queue and returns their position.
func (s *Service) Join(ctx context.Context, queueID, userID int64) (int, error) {
	q, err := s.update(ctx, queueID, func(q *model.Queue) error {
		if i := entryIndex(q, userID); i >= 0 {
			if q.Entries[i].Done {
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

// Leave removes a waiting user from the queue. They can join again later.
func (s *Service) Leave(ctx context.Context, queueID, userID int64) error {
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		i := waitingIndex(q, userID)
		if i < 0 {
			return model.ErrNotInQueue
		}
		q.Entries = slices.Delete(q.Entries, i, i+1)
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
		q.Entries = append(slices.Delete(q.Entries, i, i+1), e)
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

// Undo takes back the user's Done. They return to the place they had,
// because a done entry never leaves its spot in the queue.
func (s *Service) Undo(ctx context.Context, queueID, userID int64) (int, error) {
	q, err := s.update(ctx, queueID, func(q *model.Queue) error {
		i := entryIndex(q, userID)
		if i < 0 {
			return model.ErrNotInQueue
		}
		if !q.Entries[i].Done {
			return model.ErrNotDone
		}
		q.Entries[i].Done = false
		q.Entries[i].DoneAt = time.Time{}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return q.Position(userID), nil
}

// Swapped describes a finished swap, for the public "who swapped with whom"
// line. Positions are 1-based and taken before the swap.
type Swapped struct {
	TargetID int64
	From     int
	To       int
}

// SwapWith swaps the user with another waiting user. It's meant for a swap
// picker, which knows who was picked rather than their position.
func (s *Service) SwapWith(ctx context.Context, queueID, userID, targetID int64) (Swapped, error) {
	var res Swapped
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		var err error
		res, err = swap(q, userID, targetID)
		return err
	})
	return res, err
}

// SwapWithPosition swaps the user with whoever is at pos. The /swap command
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

// SaveUser stores the user's current names. Handlers call it on every
// update, so renamed users show up with their new name.
func (s *Service) SaveUser(ctx context.Context, u model.User) error {
	return s.store.SaveUser(ctx, u)
}

// Names returns the full name of everyone in the queue, for drawing the
// board. Users the bot never saw are left out.
func (s *Service) Names(ctx context.Context, q model.Queue) (map[int64]string, error) {
	names := make(map[int64]string, len(q.Entries))
	for _, e := range q.Entries {
		u, err := s.store.User(ctx, e.UserID)
		if errors.Is(err, model.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		names[e.UserID] = u.FullName()
	}
	return names, nil
}
