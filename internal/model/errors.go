package model

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyJoined = errors.New("user is already in the queue")
	ErrNotInQueue    = errors.New("user is not in the queue")
	ErrQueueClosed   = errors.New("queue is closed")
)
