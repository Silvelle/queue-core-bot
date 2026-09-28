package keyboard

import (
	"github.com/Silvelle/queue-core-bot/internal/callbacks"
	"github.com/go-telegram/bot/models"
)

func Board(queueID int64) *models.InlineKeyboardMarkup {
	b := func(text string, a callbacks.Action) models.InlineKeyboardButton {
		return models.InlineKeyboardButton{
			Text:         text,
			CallbackData: callbacks.Encode(queueID, a),
		}
	}

	return &models.InlineKeyboardMarkup{
		InlineKeyboard: [][]models.InlineKeyboardButton{
			{b("+ Join", callbacks.Join), b("Leave", callbacks.Leave)},
			{b("To end", callbacks.ToEnd), b("✓ Done", callbacks.Done)},
			{b("Where am I?", callbacks.Where)},
		},
	}
}
