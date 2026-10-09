package callbacks

import (
	"errors"
	"strconv"
	"strings"
)

const MaxLen = 64

const boardPrefix = "b"

var ErrInvalid = errors.New("invalid callback")

type Action string

// The letters "x" and "u" belonged to the removed "Сдано" and "Сброс"
// buttons. They stay unused, so those buttons on old boards decode as
// invalid instead of doing something else.
const (
	Join  Action = "j"
	Leave Action = "l"
	ToEnd Action = "e"
	Where Action = "w"
	// All posts the chat's list of queues, like /queues.
	All Action = "a"
	// Show posts the queue's board again at the bottom of the chat. It's
	// pressed from the list of queues, not from the board itself.
	Show Action = "s"
)

func (a Action) valid() bool {
	switch a {
	case Join, Leave, ToEnd, Where, All, Show:
		return true
	}
	return false
}

func Encode(queueID int64, a Action) string {
	return boardPrefix + ":" + strconv.FormatInt(queueID, 10) + ":" + string(a)
}

func Decode(data string) (queueID int64, a Action, err error) {
	parts := strings.Split(data, ":")
	if len(parts) != 3 || parts[0] != boardPrefix {
		return 0, "", ErrInvalid
	}

	queueID, err = strconv.ParseInt(parts[1], 10, 64)
	if err != nil || queueID <= 0 || strconv.FormatInt(queueID, 10) != parts[1] {
		return 0, "", ErrInvalid
	}

	a = Action(parts[2])
	if !a.valid() {
		return 0, "", ErrInvalid
	}
	return queueID, a, nil
}
