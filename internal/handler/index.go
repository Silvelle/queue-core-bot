package handler

import (
	"context"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"

	"github.com/Silvelle/queue-core-bot/internal/keyboard"
	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/render"
)

// postIndex posts the chat's list of queues. It's only posted when someone
// asks for it, with /queues or "Все очереди", and it's never pinned. Earlier
// lists stay as they are, buttons included.
func (h *Handler) postIndex(ctx context.Context, chatID int64) error {
	open, err := h.svc.OpenQueues(ctx, chatID)
	if err != nil {
		return err
	}
	_, err = h.send(ctx, chatID, render.Index(open), keyboard.Index(open))
	return err
}

// queues handles /queues: it posts the list of queues.
func (h *Handler) queues(ctx context.Context, msg *models.Message, _ string) {
	if err := h.postIndex(ctx, msg.Chat.ID); err != nil {
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

// repostBoard posts the queue's board again at the bottom of the chat. The
// earlier boards stay as they are, buttons included; the newest one is the
// one the bot keeps up to date.
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

	sent, err := h.send(ctx, q.ChatID, text, keyboard.Board(q.ID))
	if err != nil {
		return err
	}
	return h.svc.SetBoardMessage(ctx, q.ID, sent.ID)
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
