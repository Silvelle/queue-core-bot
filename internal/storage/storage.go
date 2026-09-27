// Package storage defines how queues and users are persisted.
//
// Storage is intentionally dumb: it saves and loads whole records.
// All queue rules live in the service package, which also serializes
// changes to a single queue.
package storage

import (
	"context"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

type Storage interface {
	// SaveUser inserts the user or replaces the stored one.
	SaveUser(ctx context.Context, u model.User) error
	// User returns model.ErrNotFound if the user is unknown.
	User(ctx context.Context, id int64) (model.User, error)

	// CreateQueue assigns a new ID and returns the stored queue.
	CreateQueue(ctx context.Context, q model.Queue) (model.Queue, error)
	// Queue returns model.ErrNotFound if there is no such queue.
	Queue(ctx context.Context, id int64) (model.Queue, error)
	// UpdateQueue replaces the stored queue, entries included.
	// It returns model.ErrNotFound if the queue does not exist.
	UpdateQueue(ctx context.Context, q model.Queue) error
	// OpenQueues returns the chat's queues that are not closed, oldest first.
	OpenQueues(ctx context.Context, chatID int64) ([]model.Queue, error)
}
