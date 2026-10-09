package handler

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/go-telegram/bot/models"

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
		model.ErrNotInQueue,
		model.ErrQueueExists,
		model.ErrInvalidName,
		model.ErrInvalidPosition,
		model.ErrSelfSwap,
		model.ErrTargetNotInQueue,
		model.ErrAlreadyThere,
		model.ErrUnknownUser,
		model.ErrTargetInQueue,
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
	if want := "Иван Тарасов добавлен в очередь под номером 3"; inserted != want {
		t.Errorf("inserted: %q, want %q", inserted, want)
	}
}

func TestParseNumbers(t *testing.T) {
	tests := []struct {
		args   string
		want   []int64
		wantOK bool
	}{
		{"7 3", []int64{7, 3}, true},
		{"  7   3 ", []int64{7, 3}, true},
		{"5", []int64{5}, true},
		{"123456789 2", []int64{123456789, 2}, true},
		{"", []int64{}, true},
		{"a 3", nil, false},
		{"7 b", nil, false},
	}
	for _, tt := range tests {
		got, ok := parseNumbers(tt.args)
		if ok != tt.wantOK || !slices.Equal(got, tt.want) {
			t.Errorf("parseNumbers(%q) = %v, %v, want %v, %v", tt.args, got, ok, tt.want, tt.wantOK)
		}
	}
}

// Only real topic messages count as a topic: in a group without topics,
// replies also carry a MessageThreadID.
func TestTopicOf(t *testing.T) {
	if got := topicOf(&models.Message{IsTopicMessage: true, MessageThreadID: 42}); got != 42 {
		t.Errorf("topic message: %d, want 42", got)
	}
	if got := topicOf(&models.Message{MessageThreadID: 42}); got != 0 {
		t.Errorf("reply in a group without topics: %d, want 0", got)
	}
}

func TestMemberUser(t *testing.T) {
	anna := &models.User{ID: 1, FirstName: "Анна"}
	tests := []struct {
		name   string
		m      models.ChatMember
		wantOK bool
	}{
		{"owner", models.ChatMember{Type: models.ChatMemberTypeOwner, Owner: &models.ChatMemberOwner{User: anna}}, true},
		{"admin", models.ChatMember{Type: models.ChatMemberTypeAdministrator, Administrator: &models.ChatMemberAdministrator{User: *anna}}, true},
		{"member", models.ChatMember{Type: models.ChatMemberTypeMember, Member: &models.ChatMemberMember{User: anna}}, true},
		{"restricted, in the chat", models.ChatMember{Type: models.ChatMemberTypeRestricted,
			Restricted: &models.ChatMemberRestricted{User: anna, IsMember: true}}, true},
		{"restricted, left", models.ChatMember{Type: models.ChatMemberTypeRestricted,
			Restricted: &models.ChatMemberRestricted{User: anna, IsMember: false}}, false},
		{"left", models.ChatMember{Type: models.ChatMemberTypeLeft}, false},
		{"banned", models.ChatMember{Type: models.ChatMemberTypeBanned}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, ok := memberUser(&tt.m)
			if ok != tt.wantOK {
				t.Fatalf("memberUser() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && u.ID != 1 {
				t.Errorf("user = %+v, want Анна", u)
			}
		})
	}
}

func TestRemovedText(t *testing.T) {
	got := removedText("Практика 4", "Иван Тарасов", 4)
	if want := "Практика 4: Иван Тарасов убран(а) из очереди, место №4 освободилось"; got != want {
		t.Errorf("removedText() = %q, want %q", got, want)
	}
}

func TestWhoAndPlace(t *testing.T) {
	tests := []struct {
		nums   []int64
		who    int64
		to     int
		wantOK bool
	}{
		{[]int64{7}, 7, service.AtEnd, true},
		{[]int64{7, 3}, 7, 3, true},
		{[]int64{7, 0}, 7, 0, false},
	}
	for _, tt := range tests {
		who, to, ok := whoAndPlace(tt.nums)
		if who != tt.who || to != tt.to || ok != tt.wantOK {
			t.Errorf("whoAndPlace(%v) = %d, %d, %v, want %d, %d, %v", tt.nums, who, to, ok, tt.who, tt.to, tt.wantOK)
		}
	}
}

// Telegram refuses the whole command menu if one entry breaks its rules:
// names of 1 to 32 lowercase letters, digits and underscores, descriptions
// of 1 to 256 characters.
func TestMenuCommandsFitTelegram(t *testing.T) {
	seen := make(map[string]bool)
	for _, c := range menuCommands {
		if n := len(c.Command); n < 1 || n > 32 {
			t.Errorf("/%s: name is %d characters", c.Command, n)
		}
		for _, r := range c.Command {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
				t.Errorf("/%s: name has %q", c.Command, r)
			}
		}
		if n := utf8.RuneCountInString(c.Description); n < 1 || n > 256 {
			t.Errorf("/%s: description is %d characters", c.Command, n)
		}
		if seen[c.Command] {
			t.Errorf("/%s is listed twice", c.Command)
		}
		seen[c.Command] = true
	}
}
