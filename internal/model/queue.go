package model

import "time"

// Queue is a defense queue that belongs to one group chat.
//
// Entries are kept in order: the waiting entries, in the order they appear,
// define positions 1..n. Done entries keep their place in the slice but are
// skipped when counting positions.
type Queue struct {
	ID         int64
	ChatID     int64
	Name       string
	BoardMsgID int
	CreatedBy  int64
	CreatedAt  time.Time
	Closed     bool
	Entries    []Entry
}

// Entry is one user's place in a queue.
type Entry struct {
	UserID   int64
	JoinedAt time.Time
	Done     bool
	DoneAt   time.Time
}

// Waiting returns the entries that have not defended yet, in queue order.
func (q *Queue) Waiting() []Entry {
	var out []Entry
	for _, e := range q.Entries {
		if !e.Done {
			out = append(out, e)
		}
	}
	return out
}

// Position returns the 1-based position of a waiting user,
// or 0 if the user is not waiting in this queue.
func (q *Queue) Position(userID int64) int {
	pos := 0
	for _, e := range q.Entries {
		if e.Done {
			continue
		}
		pos++
		if e.UserID == userID {
			return pos
		}
	}
	return 0
}

// Has reports whether the user is in the queue, waiting or done.
func (q *Queue) Has(userID int64) bool {
	for _, e := range q.Entries {
		if e.UserID == userID {
			return true
		}
	}
	return false
}
