package handler

import (
	"context"
	"errors"
	"log"
	"strings"

	"github.com/Silvelle/queue-core-bot/internal/callbacks"
	"github.com/Silvelle/queue-core-bot/internal/keyboard"
	"github.com/Silvelle/queue-core-bot/internal/model"
	"github.com/Silvelle/queue-core-bot/internal/render"
	"github.com/Silvelle/queue-core-bot/internal/service"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type Handler struct {
	b           *bot.Bot
	svc         *service.Service
	botUsername string
	redraw      *redrawer
}

// New creates the handler. ctx is the bot's lifetime: pending board redraws
// stop when it is cancelled.
func New(ctx context.Context, b *bot.Bot, svc *service.Service, botUsername string) *Handler {
	h := &Handler{b: b, svc: svc, botUsername: botUsername}
	h.redraw = newRedrawer(ctx, redrawDelay, h.drawBoard)
	return h
}

// Register adds the bot's handlers and its command menu.
func (h *Handler) Register(ctx context.Context) {
	h.command("start", h.help)
	h.command("help", h.help)
	h.command("new", h.newQueue)
	h.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "b:", bot.MatchTypePrefix, h.boardPress)

	_, err := h.b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: []models.BotCommand{
			{Command: "new", Description: "Start a queue: /new Practice 4"},
			{Command: "help", Description: "How to use the bot"},
		},
	})
	if err != nil {
		log.Printf("set bot commands: %v", err)
	}
}

// command registers f for /name, with or without @botUsername.
func (h *Handler) command(name string, f func(ctx context.Context, msg *models.Message, args string)) {
	match := func(update *models.Update) bool {
		if update.Message == nil {
			return false
		}
		got, _, ok := parseCommand(update.Message.Text, h.botUsername)
		return ok && got == name
	}
	h.b.RegisterHandlerMatchFunc(match, func(ctx context.Context, _ *bot.Bot, update *models.Update) {
		msg := update.Message
		if msg.From != nil {
			h.saveUser(ctx, *msg.From)
		}
		_, args, _ := parseCommand(msg.Text, h.botUsername)
		f(ctx, msg, args)
	})
}

func (h *Handler) help(ctx context.Context, msg *models.Message, _ string) {
	h.reply(ctx, msg, helpText)
}

func (h *Handler) newQueue(ctx context.Context, msg *models.Message, name string) {
	if msg.Chat.Type == models.ChatTypePrivate {
		h.reply(ctx, msg, privateChatText)
		return
	}
	if msg.From == nil {
		return
	}
	if name == "" {
		h.reply(ctx, msg, newUsageText)
		return
	}

	q, err := h.svc.Create(ctx, msg.Chat.ID, name, msg.From.ID)
	if err != nil {
		h.replyError(ctx, msg, "create queue", err)
		return
	}

	text, err := h.boardText(ctx, q)
	if err != nil {
		h.replyError(ctx, msg, "draw new board", err)
		return
	}
	board, err := h.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:      q.ChatID,
		Text:        text,
		ReplyMarkup: keyboard.Board(q.ID),
	})
	if err != nil {
		log.Printf("post board of queue %d: %v", q.ID, err)
		return
	}
	if err := h.svc.SetBoardMessage(ctx, q.ID, board.ID); err != nil {
		log.Printf("remember board of queue %d: %v", q.ID, err)
	}
}

func (h *Handler) boardPress(ctx context.Context, _ *bot.Bot, update *models.Update) {
	cq := update.CallbackQuery
	h.saveUser(ctx, cq.From)

	queueID, action, err := callbacks.Decode(cq.Data)
	if err != nil {
		h.toast(ctx, cq, badButtonText)
		return
	}
	// The data can be forged, so check the button really sits under this
	// queue's board in this chat.
	q, err := h.svc.Queue(ctx, queueID)
	if err != nil || !pressedOnBoard(cq, q) {
		h.toast(ctx, cq, badButtonText)
		return
	}

	text, changed := h.act(ctx, q, cq.From.ID, action)
	h.toast(ctx, cq, text)
	if changed {
		h.redraw.Schedule(queueID)
	}
}

// act runs one board action and returns the toast text, and whether the
// queue changed and its board needs redrawing.
func (h *Handler) act(ctx context.Context, q model.Queue, userID int64, action callbacks.Action) (string, bool) {
	var (
		text string
		err  error
	)
	switch action {
	case callbacks.Join:
		var pos int
		pos, err = h.svc.Join(ctx, q.ID, userID)
		text = joinedText(pos)
	case callbacks.Leave:
		err = h.svc.Leave(ctx, q.ID, userID)
		text = leftText
	case callbacks.ToEnd:
		var pos int
		pos, err = h.svc.ToEnd(ctx, q.ID, userID)
		text = toEndText(pos)
	case callbacks.Done:
		err = h.svc.Done(ctx, q.ID, userID)
		text = doneText
	case callbacks.Where:
		return whereText(q.Position(userID), len(q.Waiting())), false
	}

	if err != nil {
		if errorText(err) == genericErrText {
			log.Printf("queue %d, action %q, user %d: %v", q.ID, action, userID, err)
		}
		return errorText(err), false
	}
	return text, true
}

// pressedOnBoard reports whether the pressed button is under q's board.
func pressedOnBoard(cq *models.CallbackQuery, q model.Queue) bool {
	msg := cq.Message.Message
	return msg != nil && msg.Chat.ID == q.ChatID && msg.ID == q.BoardMsgID
}

// drawBoard edits the board message to show the queue's current state.
func (h *Handler) drawBoard(ctx context.Context, queueID int64) error {
	q, err := h.svc.Queue(ctx, queueID)
	if err != nil {
		return err
	}
	text, err := h.boardText(ctx, q)
	if err != nil {
		return err
	}

	_, err = h.b.EditMessageText(ctx, &bot.EditMessageTextParams{
		ChatID:      q.ChatID,
		MessageID:   q.BoardMsgID,
		Text:        text,
		ReplyMarkup: keyboard.Board(q.ID),
	})
	// Telegram refuses an edit that changes nothing, for example when
	// someone joined and left before the redraw. That's not a problem.
	if err != nil && strings.Contains(err.Error(), "message is not modified") {
		return nil
	}
	return err
}

func (h *Handler) boardText(ctx context.Context, q model.Queue) (string, error) {
	names, err := h.svc.Names(ctx, q)
	if err != nil {
		return "", err
	}
	return render.Board(q, names), nil
}

func (h *Handler) saveUser(ctx context.Context, u models.User) {
	err := h.svc.SaveUser(ctx, model.User{
		ID:        u.ID,
		FirstName: u.FirstName,
		LastName:  u.LastName,
		Username:  u.Username,
	})
	if err != nil {
		log.Printf("save user %d: %v", u.ID, err)
	}
}

func (h *Handler) toast(ctx context.Context, cq *models.CallbackQuery, text string) {
	_, err := h.b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: cq.ID,
		Text:            text,
	})
	if err != nil {
		log.Printf("answer button press: %v", err)
	}
}

func (h *Handler) reply(ctx context.Context, msg *models.Message, text string) {
	_, err := h.b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID:          msg.Chat.ID,
		Text:            text,
		ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
	})
	if err != nil {
		log.Printf("reply in chat %d: %v", msg.Chat.ID, err)
	}
}

// replyError answers with the error's message. Unexpected errors are also
// logged, since the person only sees a generic message.
func (h *Handler) replyError(ctx context.Context, msg *models.Message, what string, err error) {
	text := errorText(err)
	if text == genericErrText && !errors.Is(err, context.Canceled) {
		log.Printf("%s in chat %d: %v", what, msg.Chat.ID, err)
	}
	h.reply(ctx, msg, text)
}
