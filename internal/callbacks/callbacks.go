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

const (
	Join  Action = "j"
	Leave Action = "l"
	ToEnd Action = "e"
	Done  Action = "x"
	Where Action = "w"
	// Undo takes back a Done.
	Undo Action = "u"
	// All lists the user's place in every open queue of the chat.
	All Action = "a"
)

func (a Action) valid() bool {
	switch a {
	case Join, Leave, ToEnd, Done, Where, Undo, All:
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
