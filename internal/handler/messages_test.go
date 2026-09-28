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
		text := errorText(err)
		if text == genericErrText {
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
	if got := errorText(err); got != errorTexts[model.ErrAlreadyJoined] {
		t.Errorf("errorText(wrapped) = %q, want the ErrAlreadyJoined message", got)
	}
}

func TestErrorTextUnknown(t *testing.T) {
	if got := errorText(errors.New("disk full")); got != genericErrText {
		t.Errorf("errorText(unknown) = %q, want %q", got, genericErrText)
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
