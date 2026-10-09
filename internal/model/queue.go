package model

import "time"

// Queue is a defense queue that belongs to one group chat, and to one topic
// of it if the group has topics.
//
// Entries are the people waiting, in order: their index plus one is their
// position.
type Queue struct {
	ID     int64
	ChatID int64
	// ThreadID is the forum topic the queue lives in, or 0 for a group
	// without topics. Each topic has its own queues.
	ThreadID   int
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
}

// Position returns the 1-based position of a user, or 0 if the user is not
// in this queue.
func (q *Queue) Position(userID int64) int {
	for i, e := range q.Entries {
		if e.UserID == userID {
			return i + 1
		}
	}
	return 0
}
