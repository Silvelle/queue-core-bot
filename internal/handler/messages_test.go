package handler

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

// Every error the service can return must have its own message, so no one
// ever sees "Something went wrong" for a normal situation.
func TestEveryModelErrorHasText(t *testing.T) {
	all := []error{
		model.ErrNotFound,
		model.ErrQueueClosed,
		model.ErrAlreadyJoined,
		model.ErrAlreadyDone,
		model.ErrNotInQueue,
		model.ErrQueueExists,
		model.ErrInvalidName,
		model.ErrInvalidPosition,
		model.ErrSelfSwap,
		model.ErrTargetNotInQueue,
	}
	seen := make(map[string]error)
	for _, err := range all {
		text, known := errorText(err)
		if !known || text == genericErrText {
			t.Errorf("%v has no message of its own", err)
		}
		if prev, ok := seen[text]; ok {
			t.Errorf("%v and %v share the message %q", prev, err, text)
		}
		seen[text] = err
	}
}

func TestErrorTextWrapped(t *testing.T) {
	err := fmt.Errorf("join queue 42: %w", model.ErrAlreadyJoined)
	if got, _ := errorText(err); got != errorTexts[model.ErrAlreadyJoined] {
		t.Errorf("errorText(wrapped) = %q, want the ErrAlreadyJoined message", got)
	}
}

func TestErrorTextUnknown(t *testing.T) {
	got, known := errorText(errors.New("disk full"))
	if got != genericErrText || known {
		t.Errorf("errorText(unknown) = %q, %v, want %q, false", got, known, genericErrText)
	}
}

func TestWhereText(t *testing.T) {
	tests := []struct {
		pos, total int
		want       string
	}{
		{0, 5, "You're not in this queue."},
		{1, 5, "You're #1 of 5. You're next!"},
		{3, 7, "You're #3 of 7. 2 ahead of you."},
	}
	for _, tt := range tests {
		if got := whereText(tt.pos, tt.total); got != tt.want {
			t.Errorf("whereText(%d, %d) = %q, want %q", tt.pos, tt.total, got, tt.want)
		}
	}
}

func TestSwappedText(t *testing.T) {
	got := swappedText("Practice 4", "Ivan Tarasov", "Olga Volkova", 3, 5)
	want := "Practice 4: Ivan Tarasov #3 ⇄ Olga Volkova #5"
	if got != want {
		t.Errorf("swappedText() = %q, want %q", got, want)
	}
}

func TestClosedText(t *testing.T) {
	tests := []struct {
		waiting int
		want    string
	}{
		{0, "Practice 4 is closed. Everyone has defended."},
		{3, "Practice 4 is closed. 3 still waiting."},
	}
	for _, tt := range tests {
		if got := closedText("Practice 4", tt.waiting); got != tt.want {
			t.Errorf("closedText(%d) = %q, want %q", tt.waiting, got, tt.want)
		}
	}
}

func TestPickErrorText(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		usage string
		want  string
	}{
		{"no queues", errNoOpenQueues, "", noQueuesText},
		{"unknown name", errNoSuchName, "", noSuchQueueText},
		{"several queues", errWhichQueue, "", whichQueueText},
		{"several queues, usage given", errWhichQueue, closeUsageText, closeUsageText},
		{"unexpected", errors.New("boom"), "", genericErrText},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickErrorText(tt.err, tt.usage); got != tt.want {
				t.Errorf("pickErrorText() = %q, want %q", got, tt.want)
			}
		})
	}
}
