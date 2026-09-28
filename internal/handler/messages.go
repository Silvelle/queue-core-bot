package handler

import (
	"errors"
	"fmt"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

const (
	helpText = `I keep queues for defenses in this chat.

/new <name> — start a queue, for example /new Practice 4

Then use the buttons under the queue:
+ Join, Leave, To end, ✓ Done, Where am I?`

	privateChatText = "Queues live in group chats. Add me to your group and send /new <name> there."
	newUsageText    = "Give the queue a name, for example /new Practice 4"
	badButtonText   = "This button no longer works."
	genericErrText  = "Something went wrong. Try again."
)

// errorTexts is what a person sees for each rule the service enforces.
var errorTexts = map[error]string{
	model.ErrNotFound:         "This queue no longer exists.",
	model.ErrQueueClosed:      "This queue is closed.",
	model.ErrAlreadyJoined:    "You're already in the queue.",
	model.ErrAlreadyDone:      "You've already defended in this queue.",
	model.ErrNotInQueue:       "You're not in this queue. Press + Join first.",
	model.ErrQueueExists:      "A queue with this name is already open here.",
	model.ErrInvalidName:      "The name must be 1 to 64 characters.",
	model.ErrInvalidPosition:  "No one is at that position.",
	model.ErrSelfSwap:         "You can't swap with yourself.",
	model.ErrTargetNotInQueue: "That person isn't waiting in the queue.",
}

// errorText turns a service error into a short message. Errors that aren't
// the person's fault, like a broken database, get a generic message
func errorText(err error) string {
	for target, text := range errorTexts {
		if errors.Is(err, target) {
			return text
		}
	}
	return genericErrText
}

func joinedText(pos int) string { return fmt.Sprintf("Joined. You're #%d.", pos) }
func toEndText(pos int) string  { return fmt.Sprintf("Moved to the end. You're #%d.", pos) }

const (
	leftText = "You left the queue."
	doneText = "Marked as done. Good luck!"
)

func whereText(pos, total int) string {
	if pos == 0 {
		return "You're not in this queue."
	}
	if pos == 1 {
		return fmt.Sprintf("You're #1 of %d. You're next!", total)
	}
	return fmt.Sprintf("You're #%d of %d. %d ahead of you.", pos, total, pos-1)
}
