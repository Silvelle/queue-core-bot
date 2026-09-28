package handler

import (
	"errors"
	"strings"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

var (
	errNoOpenQueues = errors.New("no open queues in this chat")
	errWhichQueue   = errors.New("several queues are open")
	errNoSuchName   = errors.New("no open queue with this name")
)

// pickQueue finds the queue a command is about, among the chat's open
// queues:
//
//  1. the queue whose board the command replies to;
//  2. otherwise the queue called name, if a name was given;
//  3. otherwise the only open queue, if onlyOne is allowed and there is one.
//
// A reply to some other message is ignored, as if there were no reply.
func pickQueue(open []model.Queue, replyToMsgID int, name string, onlyOne bool) (model.Queue, error) {
	if replyToMsgID != 0 {
		for _, q := range open {
			if q.BoardMsgID == replyToMsgID {
				return q, nil
			}
		}
	}

	if name != "" {
		for _, q := range open {
			if strings.EqualFold(q.Name, name) {
				return q, nil
			}
		}
		return model.Queue{}, errNoSuchName
	}

	switch {
	case len(open) == 0:
		return model.Queue{}, errNoOpenQueues
	case onlyOne && len(open) == 1:
		return open[0], nil
	default:
		return model.Queue{}, errWhichQueue
	}
}
