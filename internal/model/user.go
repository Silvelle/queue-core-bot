package model

import (
	"strconv"
	"unicode/utf8"
)

// User is a Telegram user the bot has seen. It is refreshed on every
// interaction, because people change their names
type User struct {
	ID        int64
	FirstName string
	LastName  string
	Username  string
}

// FullName returns the name for the queue list, like "Anna Kuznetsova".
// It falls back to the username, then to the user ID.
func (u User) FullName() string {
	switch {
	case u.FirstName != "" && u.LastName != "":
		return u.FirstName + " " + u.LastName
	case u.FirstName != "":
		return u.FirstName
	case u.Username != "":
		return "@" + u.Username
	default:
		return "user " + strconv.FormatInt(u.ID, 10)
	}
}

// DisplayName returns a short name for places with little room, such as
// buttons, like "Anna K.". It falls back the same way as FullName.
func (u User) DisplayName() string {
	switch {
	case u.FirstName != "" && u.LastName != "":
		r, _ := utf8.DecodeRuneInString(u.LastName)
		return u.FirstName + " " + string(r) + "."
	case u.FirstName != "":
		return u.FirstName
	case u.Username != "":
		return "@" + u.Username
	default:
		return "user " + strconv.FormatInt(u.ID, 10)
	}
}
