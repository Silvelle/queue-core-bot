package handler

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

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
		model.ErrNotDone,
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
		{0, 5, "Вас нет в этой очереди."},
		{1, 5, "Вы №1 из 5. Сейчас ваша очередь!"},
		{3, 7, "Вы №3 из 7. Перед вами: 2."},
	}
	for _, tt := range tests {
		if got := whereText(tt.pos, tt.total); got != tt.want {
			t.Errorf("whereText(%d, %d) = %q, want %q", tt.pos, tt.total, got, tt.want)
		}
	}
}

func TestSwappedText(t *testing.T) {
	got := swappedText("Практика 4", "Иван Тарасов", "Ольга Волкова", 3, 5)
	want := "Практика 4: Иван Тарасов №3 ⇄ Ольга Волкова №5"
	if got != want {
		t.Errorf("swappedText() = %q, want %q", got, want)
	}
}

func TestClosedText(t *testing.T) {
	tests := []struct {
		waiting int
		want    string
	}{
		{0, "Очередь «Практика 4» закрыта. Все сдали."},
		{3, "Очередь «Практика 4» закрыта. Не успели: 3."},
	}
	for _, tt := range tests {
		if got := closedText("Практика 4", tt.waiting); got != tt.want {
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

func TestFitAlert(t *testing.T) {
	short := "Практика 4: вы №3 из 7"
	if got := fitAlert(short); got != short {
		t.Errorf("fitAlert(short) = %q, want it unchanged", got)
	}

	long := strings.Repeat("я", 300)
	got := fitAlert(long)
	if n := utf8.RuneCountInString(got); n != maxAlert {
		t.Errorf("fitAlert(long) is %d characters, want %d", n, maxAlert)
	}
	if !strings.HasSuffix(got, "…") {
		t.Error("a cut alert should end with …")
	}
}
