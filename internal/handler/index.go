package handler

import (
	"context"
	"log"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/Silvelle/queue-core-bot/internal/keyboard"
	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/render"
)

// refreshIndex brings the chat's list of queues up to date. It edits the
// list in place if there is one. With repost, or if there is no list yet or
// it was deleted, it posts a new list at the bottom of the chat.
func (h *Handler) refreshIndex(ctx context.Context, chatID int64, repost bool) error {
	open, err := h.svc.OpenQueues(ctx, chatID)
	if err != nil {
		return err
	}
	text, markup := render.Index(open), keyboard.Index(open)

	oldID, err := h.svc.IndexMessage(ctx, chatID)
	if err != nil {
		return err
	}
	if oldID != 0 && !repost {
		_, err := h.b.EditMessageText(ctx, &bot.EditMessageTextParams{
			ChatID:      chatID,
			MessageID:   oldID,
			Text:        text,
			ParseMode:   models.ParseModeHTML,
			ReplyMarkup: markup,
		})
		if err == nil || isNotModified(err) {
			return nil
		}
		log.Printf("edit list of queues in chat %d, posting a new one: %v", chatID, err)
	}

	sent, err := h.send(ctx, chatID, text, markup)
	if err != nil {
		return err
	}
	if err := h.svc.SetIndexMessage(ctx, chatID, sent.ID); err != nil {
		return err
	}
	if oldID != 0 {
		h.retire(ctx, chatID, oldID, movedListText)
	}

	// Pinning only works when the bot is an admin. Without that right the
	// list is still there, so the failure isn't worth bothering anyone.
	_, _ = h.b.PinChatMessage(ctx, &bot.PinChatMessageParams{
		ChatID:              chatID,
		MessageID:           sent.ID,
		DisableNotification: true,
	})
	return nil
}

// refreshIndexQuietly updates the list after a queue is opened or closed.
// Failures are only logged: the queue itself already worked.
func (h *Handler) refreshIndexQuietly(ctx context.Context, chatID int64) {
	if err := h.refreshIndex(ctx, chatID, false); err != nil {
		log.Printf("update list of queues in chat %d: %v", chatID, err)
	}
}

// queues handles /queues: it posts the list of queues again at the bottom
// of the chat.
func (h *Handler) queues(ctx context.Context, msg *models.Message, _ string) {
	if err := h.refreshIndex(ctx, msg.Chat.ID, true); err != nil {
		h.replyError(ctx, msg, "post list of queues", err)
	}
}

// show handles /show <name>, or /show sent as a reply to a board: it posts
// the queue's board again at the bottom of the chat.
func (h *Handler) show(ctx context.Context, msg *models.Message, name string) {
	q, ok := h.commandQueue(ctx, msg, name, true, showUsageText)
	if !ok {
		return
	}
	if err := h.repostBoard(ctx, q.ID); err != nil {
		h.replyError(ctx, msg, "repost board", err)
	}
}

// repostBoard posts the queue's board again at the bottom of the chat and
// retires the old one, so there is always exactly one board with working
// buttons.
func (h *Handler) repostBoard(ctx context.Context, queueID int64) error {
	// Two people asking for the same board at once must not end up with
	// two new boards, so reposts run one at a time. They're rare.
	h.repostMu.Lock()
	defer h.repostMu.Unlock()

	q, err := h.svc.Queue(ctx, queueID)
	if err != nil {
		return err
	}
	if q.Closed {
		return model.ErrQueueClosed
	}
	text, err := h.boardText(ctx, q)
	if err != nil {
		return err
	}

	sent, err := h.send(ctx, q.ChatID, text, boardMarkup(q))
	if err != nil {
		return err
	}
	if err := h.svc.SetBoardMessage(ctx, q.ID, sent.ID); err != nil {
		return err
	}
	if q.BoardMsgID != 0 {
		h.retire(ctx, q.ChatID, q.BoardMsgID, movedText)
	}
	return nil
}

// retire marks an old copy of a message the bot has replaced: its text
// becomes movedText and its buttons go, so nobody presses stale buttons.
// The old copy is kept, not deleted, so nothing disappears from the chat.
func (h *Handler) retire(ctx context.Context, chatID int64, msgID int, movedText string) {
	_, _ = h.b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      chatID,
		MessageID:   msgID,
		Text:        movedText,
		ReplyMarkup: noButtons(),
	})
}

// send posts a board or a list of queues: HTML text with buttons, see
// render. An empty keyboard is left out: Telegram accepts it on an edit,
// where it removes the buttons, but a new message simply has none.
func (h *Handler) send(ctx context.Context, chatID int64, text string, markup *models.InlineKeyboardMarkup) (*models.Message, error) {
	params := &bot.SendMessageParams{ChatID: chatID, Text: text, ParseMode: models.ParseModeHTML}
	if markup != nil && len(markup.InlineKeyboard) > 0 {
		params.ReplyMarkup = markup
	}
	return h.b.SendMessage(ctx, params)
}

// isNotModified reports Telegram refusing an edit that changes nothing.
func isNotModified(err error) bool {
	return err != nil && strings.Contains(err.Error(), "message is not modified")
}
