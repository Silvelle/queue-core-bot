// Package keyboard builds the inline keyboards the bot attaches to messages.
package keyboard

import (
	"strconv"
	"strings"

	"github.com/go-telegram/bot/models"

	"github.com/Silvelle/queue-core-bot/internal/callbacks"
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

// Link is a button that opens a message.
type Link struct {
	Text string
	URL  string
}

// Links returns one button per row, for the chat's list of queues.
func Links(links []Link) *models.InlineKeyboardMarkup {
	rows := make([][]models.InlineKeyboardButton, 0, len(links))
	for _, l := range links {
		rows = append(rows, []models.InlineKeyboardButton{{Text: l.Text, URL: l.URL}})
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// MessageURL returns a link to a message in a group, or false if the chat
// can't have one. Only supergroups have message links: their IDs look like
// -1001234567890, and the link uses the part after -100. Telegram turns
// most groups into supergroups, but a small new group can still be a basic
// one.
func MessageURL(chatID int64, msgID int) (string, bool) {
	id := strconv.FormatInt(chatID, 10)
	short, ok := strings.CutPrefix(id, "-100")
	if !ok || short == "" || msgID <= 0 {
		return "", false
	}
	return "https://t.me/c/" + short + "/" + strconv.Itoa(msgID), true
}
