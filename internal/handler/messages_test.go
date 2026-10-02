package handler

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/service"
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
		model.ErrAlreadyThere,
		model.ErrTargetDone,
		model.ErrUnknownUser,
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
	want := "Очередь «Практика 4» закрыта."
	if got := closedText("Практика 4"); got != want {
		t.Errorf("closedText() = %q, want %q", got, want)
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

func TestPlacedText(t *testing.T) {
	moved := placedText("Практика 4", "Иван Тарасов", service.Placed{UserID: 1, From: 7, To: 3})
	if want := "Практика 4: Иван Тарасов №7 → №3"; moved != want {
		t.Errorf("moved: %q, want %q", moved, want)
	}
	inserted := placedText("Практика 4", "Иван Тарасов", service.Placed{UserID: 1, To: 3})
	if want := "Практика 4: Иван Тарасов встаёт на №3"; inserted != want {
		t.Errorf("inserted: %q, want %q", inserted, want)
	}
}

func TestParsePlaceArgs(t *testing.T) {
	tests := []struct {
		args   string
		who    int64
		to     int
		wantOK bool
	}{
		{"7 3", 7, 3, true},
		{"  7   3 ", 7, 3, true},
		{"123456789 2", 123456789, 2, true},
		{"7", 0, 0, false},
		{"", 0, 0, false},
		{"7 3 1", 0, 0, false},
		{"a 3", 0, 0, false},
		{"7 b", 0, 0, false},
	}
	for _, tt := range tests {
		who, to, ok := parsePlaceArgs(tt.args)
		if who != tt.who || to != tt.to || ok != tt.wantOK {
			t.Errorf("parsePlaceArgs(%q) = %d, %d, %v, want %d, %d, %v", tt.args, who, to, ok, tt.who, tt.to, tt.wantOK)
		}
	}
}
