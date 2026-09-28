package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/Silvelle/queue-core-bot/internal/keyboard"
	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/render"
)

// errPinFailed means the list was posted but couldn't be pinned, usually
// because the bot isn't an admin.
var errPinFailed = errors.New("pin the list of queues")

// indexMessage returns the text and buttons of the chat's list of queues.
// Each queue gets a button that opens its board, where Telegram allows
// message links.
func indexMessage(chatID int64, open []model.Queue) (string, *models.InlineKeyboardMarkup) {
	var links []keyboard.Link
	for _, q := range open {
		if url, ok := keyboard.MessageURL(chatID, q.BoardMsgID); ok {
			links = append(links, keyboard.Link{Text: q.Name, URL: url})
		}
	}
	return render.Index(open, len(links) > 0), keyboard.Links(links)
}

// refreshIndex brings the chat's list of queues up to date. It edits the
// list in place if there is one. With repost, or if there is no list yet or
// it was deleted, it posts a new list at the bottom of the chat and pins it.
func (h *Handler) refreshIndex(ctx context.Context, chatID int64, repost bool) error {
	open, err := h.svc.OpenQueues(ctx, chatID)
	if err != nil {
		return err
	}
	text, markup := indexMessage(chatID, open)

	oldID, err := h.svc.IndexMessage(ctx, chatID)
	if err != nil {
		return err
	}
	if oldID != 0 && !repost {
		_, err := h.b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   oldID,
			Text:        text,
			ReplyMarkup: markup,
		})
		if err == nil || isNotModified(err) {
			return nil
		}
		log.Printf("edit list of queues in chat %d, posting a new one: %v", chatID, err)
	}

	params := &bot.SendMessageParams{ChatID: chatID, Text: text}
	if len(markup.InlineKeyboard) > 0 {
		params.ReplyMarkup = markup
	}
	sent, err := h.b.SendMessage(ctx, params)
	if err != nil {
		return err
	}
	if err := h.svc.SetIndexMessage(ctx, chatID, sent.ID); err != nil {
		return err
	}

	// The old list is replaced; deleting it may fail if it's too old or
	// already gone, which doesn't matter.
	if oldID != 0 {
		_, _ = h.b.DeleteMessage(ctx, &bot.DeleteMessageParams{ChatID: chatID, MessageID: oldID})
	}

	_, err = h.b.PinChatMessage(ctx, &bot.PinChatMessageParams{
		ChatID:              chatID,
		MessageID:           sent.ID,
		DisableNotification: true,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", errPinFailed, err)
	}
	return nil
}

// refreshIndexQuietly updates the list after a queue is opened or closed.
// Failures are only logged: the queue itself already worked.
func (h *Handler) refreshIndexQuietly(ctx context.Context, chatID int64) {
	if err := h.refreshIndex(ctx, chatID, false); err != nil && !errors.Is(err, errPinFailed) {
		log.Printf("update list of queues in chat %d: %v", chatID, err)
	}
}

// queues handles /queues: it posts the list of queues again at the bottom of
// the chat and pins it.
func (h *Handler) queues(ctx context.Context, msg *models.Message, _ string) {
	err := h.refreshIndex(ctx, msg.Chat.ID, true)
	switch {
	case errors.Is(err, errPinFailed):
		h.reply(ctx, msg, pinFailedText)
	case err != nil:
		h.replyError(ctx, msg, "post list of queues", err)
	}
}

// placesText lists the user's place in every open queue of the chat.
func (h *Handler) placesText(ctx context.Context, chatID, userID int64) (string, error) {
	open, err := h.svc.OpenQueues(ctx, chatID)
	if err != nil {
		return "", err
	}
	if len(open) == 0 {
		return noQueuesText, nil
	}
	return placesTop + render.Where(open, userID), nil
}

// isNotModified reports Telegram refusing an edit that changes nothing.
func isNotModified(err error) bool {
	return err != nil && strings.Contains(err.Error(), "message is not modified")
}
