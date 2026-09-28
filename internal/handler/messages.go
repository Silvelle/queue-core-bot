package handler

import (
	"errors"
	"fmt"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

const (
	helpText = `I keep queues for defenses in this chat.

/new <name> — start a queue, for example /new Practice 4
/swap <position> — swap places with whoever is at that position
/close <name> — close a queue when the defense is over

Then use the buttons under the queue:
+ Join, Leave, To end, ✓ Done, Where am I?

With several queues open, send /swap or /close as a reply to the queue's board.`

	privateChatText = "Queues live in group chats. Add me to your group and send /new <name> there."
	newUsageText    = "Give the queue a name, for example /new Practice 4"
	badButtonText   = "This button no longer works."
	genericErrText  = "Something went wrong. Try again."

	swapUsageText   = "Say who to swap with by position, for example /swap 5"
	closeUsageText  = "Name the queue, for example /close Practice 4, or send /close as a reply to its board."
	noQueuesText    = "There are no open queues here. Start one with /new <name>"
	whichQueueText  = "Several queues are open. Send the command as a reply to the board of the one you mean."
	noSuchQueueText = "There's no open queue with that name here."

	leftText = "You left the queue."
	doneText = "Marked as done. Good luck!"
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

// errorText turns a service error into a short message. known is false for
// errors that aren't the person's fault, like a broken database: they get a
// generic message, and their details belong in the log, not the chat.
func errorText(err error) (text string, known bool) {
	for target, text := range errorTexts {
		if errors.Is(err, target) {
			return text, true
		}
	}
	return genericErrText, false
}

func joinedText(pos int) string { return fmt.Sprintf("Joined. You're #%d.", pos) }
func toEndText(pos int) string  { return fmt.Sprintf("Moved to the end. You're #%d.", pos) }

func whereText(pos, total int) string {
	if pos == 0 {
		return "You're not in this queue."
	}
	if pos == 1 {
		return fmt.Sprintf("You're #1 of %d. You're next!", total)
	}
	return fmt.Sprintf("You're #%d of %d. %d ahead of you.", pos, total, pos-1)
}

// swappedText is the public line posted after a swap, so everyone can see
// who moved whom. Positions are the ones before the swap.
func swappedText(queue, user, target string, from, to int) string {
	return fmt.Sprintf("%s: %s #%d ⇄ %s #%d", queue, user, from, target, to)
}

func closedText(queue string, waiting int) string {
	if waiting == 0 {
		return fmt.Sprintf("%s is closed. Everyone has defended.", queue)
	}
	return fmt.Sprintf("%s is closed. %d still waiting.", queue, waiting)
}

// pickErrorText explains why pickQueue couldn't find the queue a command
// is about. usage is shown when the command needs more information.
func pickErrorText(err error, usage string) string {
	switch {
	case errors.Is(err, errNoOpenQueues):
		return noQueuesText
	case errors.Is(err, errNoSuchName):
		return noSuchQueueText
	case errors.Is(err, errWhichQueue):
		if usage != "" {
			return usage
		}
		return whichQueueText
	}
	return genericErrText
}
