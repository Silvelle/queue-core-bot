// Package keyboard builds the inline keyboards the bot attaches to messages.
package keyboard

import (
	"github.com/go-telegram/bot/models"

	"github.com/Silvelle/queue-core-bot/internal/callbacks"
	"github.com/Silvelle/queue-core-bot/internal/model"
)

// Board returns the buttons under a queue board. They're the same for
// everyone: each button acts on whoever presses it.
//
//	[ + Записаться ]  [ Выйти ]
//	[ В конец ]       [ Где я? ]
//	[ ✓ Сдано ]       [ ↩ Сброс ]
//	[     📋 Все очереди      ]
func Board(queueID int64) *models.InlineKeyboardMarkup {
	b := func(text string, a callbacks.Action) models.InlineKeyboardButton {
		return models.InlineKeyboardButton{Text: text, CallbackData: callbacks.Encode(queueID, a)}
	}

	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{b("+ Записаться", callbacks.Join), b("Выйти", callbacks.Leave)},
			{b("В конец", callbacks.ToEnd), b("Где я?", callbacks.Where)},
			{b("✓ Сдано", callbacks.Done), b("↩ Сброс", callbacks.Undo)},
			{b("📋 Все очереди", callbacks.All)},
		},
	}
}

// Index returns the buttons under the chat's list of queues: one per
// queue, which posts that queue's board again at the bottom of the chat.
func Index(open []model.Queue) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(open))
	for _, q := range open {
		rows = append(rows, []models.InlineKeyboardButton{{
			Text:         q.Name,
			CallbackData: callbacks.Encode(q.ID, callbacks.Show),
		}})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}
