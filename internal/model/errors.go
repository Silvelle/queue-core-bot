package model

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyJoined = errors.New("user is already in the queue")
	ErrNotInQueue    = errors.New("user is not in the queue")
	ErrQueueClosed   = errors.New("queue is closed")

	ErrQueueExists      = errors.New("an open queue with this name already exists")
	ErrInvalidName      = errors.New("queue name must be 1 to 64 characters")
	ErrInvalidPosition  = errors.New("no one is at this position")
	ErrSelfSwap         = errors.New("cannot swap with yourself")
	ErrTargetNotInQueue = errors.New("the other user is not in the queue")
	ErrAlreadyThere     = errors.New("user is already at this position")
	ErrTargetInQueue    = errors.New("the other user is already in the queue")
	ErrUnknownUser      = errors.New("no known user with this ID")
)
