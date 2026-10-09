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

// index returns the index in q.Entries of the user's entry, or -1 if the
// user isn't in the queue.
func index(q *model.Queue, userID int64) int {
	return slices.IndexFunc(q.Entries, func(e model.Entry) bool { return e.UserID == userID })
}

// resolve turns the "who" of a command into a user ID. It's a position if
// it's within the queue, and a user ID otherwise: positions are small
// numbers, Telegram user IDs never are. Commands resolve it under the queue
// lock, so it can't go stale between reading the board and acting.
func resolve(q *model.Queue, who int64) int64 {
	if who >= 1 && who <= int64(len(q.Entries)) {
		return q.Entries[who-1].UserID
	}
	return who
}

func (s *Service) Queue(ctx context.Context, queueID int64) (model.Queue, error) {
	return s.store.Queue(ctx, queueID)
}

// OpenQueues returns the open queues of a chat's topic; threadID is 0 for a
// group without topics.
func (s *Service) OpenQueues(ctx context.Context, chatID int64, threadID int) ([]model.Queue, error) {
	return s.store.OpenQueues(ctx, chatID, threadID)
}

// Create opens a new queue in a chat's topic. Names are trimmed and must be
// unique among the topic's open queues, ignoring case.
func (s *Service) Create(ctx context.Context, chatID int64, threadID int, name string, createdBy int64) (model.Queue, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return model.Queue{}, model.ErrInvalidName
	}

	s.createMu.Lock()
	defer s.createMu.Unlock()

	open, err := s.store.OpenQueues(ctx, chatID, threadID)
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
		ThreadID:  threadID,
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
		if index(q, userID) >= 0 {
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

// Leave removes the user from the queue, for example after their defense.
// They can join again later.
func (s *Service) Leave(ctx context.Context, queueID, userID int64) error {
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		i := index(q, userID)
		if i < 0 {
			return model.ErrNotInQueue
		}
		q.Entries = slices.Delete(q.Entries, i, i+1)
		return nil
	})
	return err
}

// ToEnd moves the user behind everyone else and returns their new position.
func (s *Service) ToEnd(ctx context.Context, queueID, userID int64) (int, error) {
	q, err := s.update(ctx, queueID, func(q *model.Queue) error {
		i := index(q, userID)
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

// Swapped describes a finished swap, for the public "who swapped with whom"
// line. Positions are 1-based and taken before the swap: UserID was at
// From, TargetID at To.
type Swapped struct {
	UserID   int64
	TargetID int64
	From     int
	To       int
}

// SwapWithPosition swaps the user with whoever is at pos: /swap <pos>.
func (s *Service) SwapWithPosition(ctx context.Context, queueID, userID int64, pos int) (Swapped, error) {
	var res Swapped
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		if pos < 1 || pos > len(q.Entries) {
			return model.ErrInvalidPosition
		}
		var err error
		res, err = swap(q, userID, q.Entries[pos-1].UserID)
		return err
	})
	return res, err
}

// Swap swaps two other people: /swap <who> <who>, where each who is a
// position or a user ID, see resolve.
func (s *Service) Swap(ctx context.Context, queueID, a, b int64) (Swapped, error) {
	var res Swapped
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		var err error
		res, err = swap(q, resolve(q, a), resolve(q, b))
		// Both are other people here, so "you're not in the queue" would
		// be the wrong message.
		if errors.Is(err, model.ErrNotInQueue) {
			return model.ErrTargetNotInQueue
		}
		return err
	})
	return res, err
}

func swap(q *model.Queue, userID, targetID int64) (Swapped, error) {
	if userID == targetID {
		return Swapped{}, model.ErrSelfSwap
	}
	i := index(q, userID)
	if i < 0 {
		return Swapped{}, model.ErrNotInQueue
	}
	j := index(q, targetID)
	if j < 0 {
		return Swapped{}, model.ErrTargetNotInQueue
	}

	q.Entries[i], q.Entries[j] = q.Entries[j], q.Entries[i]
	return Swapped{UserID: userID, TargetID: targetID, From: i + 1, To: j + 1}, nil
}

// Removed describes a finished /delete, for the public line.
type Removed struct {
	UserID int64
	From   int
}

// Remove takes a person out of the queue: /delete <who>, where who is a
// position or a user ID, see resolve. Everyone after them moves up.
func (s *Service) Remove(ctx context.Context, queueID, who int64) (Removed, error) {
	var res Removed
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		userID := resolve(q, who)
		i := index(q, userID)
		if i < 0 {
			return model.ErrTargetNotInQueue
		}
		q.Entries = slices.Delete(q.Entries, i, i+1)
		res = Removed{UserID: userID, From: i + 1}
		return nil
	})
	return res, err
}

// Placed describes a finished /place or /add, for the public line. To is
// the new position; From is the old one, or 0 if the person was added.
type Placed struct {
	UserID int64
	From   int
	To     int
}

// Added reports whether the person wasn't in the queue before.
func (p Placed) Added() bool { return p.From == 0 }

// AtEnd as the position for Place or Add means "at the end of the queue".
const AtEnd = 0

// Place moves someone in the queue to position to: /place <who> [position],
// where who is a position or a user ID, see resolve. Everyone in between
// shifts by one, so unlike a swap nobody is sent backwards. With to set to
// AtEnd, the person goes last. Adding someone new is Add.
func (s *Service) Place(ctx context.Context, queueID, who int64, to int) (Placed, error) {
	var res Placed
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		userID := resolve(q, who)
		i := index(q, userID)
		if i < 0 {
			return model.ErrTargetNotInQueue
		}
		if to == AtEnd {
			to = len(q.Entries)
		}
		if to < 1 || to > len(q.Entries) {
			return model.ErrInvalidPosition
		}
		if i+1 == to {
			return model.ErrAlreadyThere
		}

		e := q.Entries[i]
		q.Entries = slices.Insert(slices.Delete(q.Entries, i, i+1), to-1, e)
		res = Placed{UserID: userID, From: i + 1, To: to}
		return nil
	})
	return res, err
}

// Add puts someone who isn't in the queue at position to, for example after
// "Выйти" by mistake, or for someone who can't press the button: /add <who>
// [position]. Everyone from that position on moves down by one; with to set
// to AtEnd, the person goes last. They must be someone the bot knows, so a
// mistyped ID can't add a stranger; the handler makes chat members known by
// asking Telegram.
func (s *Service) Add(ctx context.Context, queueID, userID int64, to int) (Placed, error) {
	var res Placed
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		if index(q, userID) >= 0 {
			return model.ErrTargetInQueue
		}
		if to == AtEnd {
			to = len(q.Entries) + 1
		}
		if to < 1 || to > len(q.Entries)+1 {
			return model.ErrInvalidPosition
		}
		if _, err := s.store.User(ctx, userID); errors.Is(err, model.ErrNotFound) {
			return model.ErrUnknownUser
		} else if err != nil {
			return err
		}

		q.Entries = slices.Insert(q.Entries, to-1, model.Entry{UserID: userID, JoinedAt: s.now()})
		res = Placed{UserID: userID, To: to}
		return nil
	})
	return res, err
}

// Close freezes the queue. Closing twice returns model.ErrQueueClosed.
func (s *Service) Close(ctx context.Context, queueID int64) error {
	_, err := s.update(ctx, queueID, func(q *model.Queue) error {
		q.Closed = true
		return nil
	})
	return err
}

// UserByUsername finds a known user by their Telegram username, without
// the "@".
func (s *Service) UserByUsername(ctx context.Context, username string) (model.User, error) {
	return s.store.UserByUsername(ctx, username)
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
