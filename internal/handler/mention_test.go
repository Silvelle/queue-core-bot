package handler

import (
	"errors"
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestReplaceMentions(t *testing.T) {
	ids := map[string]string{"@anna_k": "111", "Иван": "222"}
	value := func(_ models.MessageEntity, mentioned string) (string, error) {
		if id, ok := ids[mentioned]; ok {
			return id, nil
		}
		return "", errors.New("unknown " + mentioned)
	}

	tests := []struct {
		name     string
		text     string
		entities []models.MessageEntity
		want     string
	}{
		{"username", "/add @anna_k 3",
			[]models.MessageEntity{{Type: models.MessageEntityTypeMention, Offset: 5, Length: 7}},
			"/add 111 3"},
		// "Иван" is a text mention, picked from the "@" list: Cyrillic
		// letters are one UTF-16 unit each, like here.
		{"text mention", "/add Иван",
			[]models.MessageEntity{{Type: models.MessageEntityTypeTextMention, Offset: 5, Length: 4}},
			"/add 222"},
		{"two mentions", "/swap @anna_k Иван",
			[]models.MessageEntity{
				{Type: models.MessageEntityTypeMention, Offset: 6, Length: 7},
				{Type: models.MessageEntityTypeTextMention, Offset: 14, Length: 4},
			},
			"/swap 111 222"},
		// An emoji is two UTF-16 units, so the mention after it starts at
		// offset 8, not 7.
		{"after an emoji", "/add 🐱 @anna_k",
			[]models.MessageEntity{{Type: models.MessageEntityTypeMention, Offset: 8, Length: 7}},
			"/add 🐱 111"},
		{"other entities are kept", "/add 5",
			[]models.MessageEntity{{Type: models.MessageEntityTypeBotCommand, Offset: 0, Length: 4}},
			"/add 5"},
		{"offset out of range is ignored", "/add 5",
			[]models.MessageEntity{{Type: models.MessageEntityTypeMention, Offset: 50, Length: 3}},
			"/add 5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := replaceMentions(tt.text, tt.entities, value)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("replaceMentions() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReplaceMentionsUnknown(t *testing.T) {
	_, err := replaceMentions("/add @nobody", []models.MessageEntity{{Type: models.MessageEntityTypeMention, Offset: 5, Length: 7}},
		func(_ models.MessageEntity, mentioned string) (string, error) { return "", unknownMention{mentioned} })
	var u unknownMention
	if !errors.As(err, &u) || u.name != "@nobody" {
		t.Errorf("error = %v, want unknownMention for @nobody", err)
	}
}

func TestRepliedPerson(t *testing.T) {
	anna := &models.User{ID: 1, FirstName: "Анна"}
	botUser := &models.User{ID: 2, IsBot: true}

	tests := []struct {
		name   string
		msg    models.Message
		wantOK bool
	}{
		{"reply to a person", models.Message{ReplyToMessage: &models.Message{ID: 10, From: anna}}, true},
		{"no reply", models.Message{}, false},
		{"reply to the bot, like a board", models.Message{ReplyToMessage: &models.Message{ID: 10, From: botUser}}, false},
		{"topic's first message", models.Message{ReplyToMessage: &models.Message{ID: 10, From: anna, ForumTopicCreated: &models.ForumTopicCreated{}}}, false},
		{"plain message in a topic", models.Message{IsTopicMessage: true, MessageThreadID: 10,
			ReplyToMessage: &models.Message{ID: 10, From: anna}}, false},
		{"reply to a person in a topic", models.Message{IsTopicMessage: true, MessageThreadID: 10,
			ReplyToMessage: &models.Message{ID: 15, From: anna}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, ok := repliedPerson(&tt.msg)
			if ok != tt.wantOK || (ok && u.ID != 1) {
				t.Errorf("repliedPerson() = %+v, %v, want ok=%v", u, ok, tt.wantOK)
			}
		})
	}
}
