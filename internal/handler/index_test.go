package handler

import (
	"strings"
	"testing"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

func TestIndexMessage(t *testing.T) {
	open := []model.Queue{
		{Name: "Practice 4", BoardMsgID: 10},
		{Name: "Lab 2", BoardMsgID: 0}, // board not posted yet
	}

	t.Run("supergroup gets links", func(t *testing.T) {
		text, kb := indexMessage(-1001234567890, open)
		if len(kb.InlineKeyboard) != 1 {
			t.Fatalf("%d buttons, want 1 (Lab 2 has no board yet)", len(kb.InlineKeyboard))
		}
		btn := kb.InlineKeyboard[0][0]
		if btn.Text != "Practice 4" || btn.URL != "https://t.me/c/1234567890/10" {
			t.Errorf("button = %+v, want a link to Practice 4's board", btn)
		}
		if !strings.Contains(text, "Нажмите на очередь") {
			t.Errorf("text should mention the links:\n%s", text)
		}
	})

	t.Run("basic group gets no links", func(t *testing.T) {
		text, kb := indexMessage(-123456, open)
		if len(kb.InlineKeyboard) != 0 {
			t.Errorf("%d buttons, want none", len(kb.InlineKeyboard))
		}
		if strings.Contains(text, "Нажмите на очередь") {
			t.Errorf("text mentions links that aren't there:\n%s", text)
		}
		if !strings.Contains(text, "Practice 4") || !strings.Contains(text, "Lab 2") {
			t.Errorf("text should still list every queue:\n%s", text)
		}
	})
}
