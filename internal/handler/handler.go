package handler

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"

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
	repostMu    sync.Mutex
}

// New creates the handler. ctx is the bot's lifetime: pending board redraws
// stop when it is cancelled.
func New(ctx context.Context, b *bot.Bot, svc *service.Service, botUsername string) *Handler {
	h := &Handler{b: b, svc: svc, botUsername: botUsername}
	h.redraw = newRedrawer(ctx, h.drawBoard)
	return h
}

// Register adds the bot's handlers and its command menu.
func (h *Handler) Register(ctx context.Context) {
	h.command("start", h.help)
	h.command("help", h.help)
	h.groupCommand("new", h.newQueue)
	h.groupCommand("swap", h.swap)
	h.groupCommand("add", h.add)
	h.groupCommand("place", h.place)
	h.groupCommand("delete", h.remove)
	h.groupCommand("close", h.closeQueue)
	h.groupCommand("queues", h.queues)
	h.groupCommand("show", h.show)
	h.b.RegisterHandler(bot.HandlerTypeCallbackQueryData, "b:", bot.MatchTypePrefix, h.boardPress)

	_, err := h.b.SetMyCommands(ctx, &bot.SetMyCommandsParams{
		Commands: menuCommands,
	})
	if err != nil {
		log.Printf("set bot commands: %v", err)
	}
}

// menuCommands is the list Telegram shows when someone types "/". It has
// one line of description per command and nothing else, so each line says
// in plain words what to type after the command, most important part
// first: phones cut long lines off.
var menuCommands = []models.BotCommand{
	{Command: "new", Description: "Создать очередь. Напишите название: /new Практика 4"},
	{Command: "add", Description: "Добавить человека: @имя или ID, затем место (без места — в конец). /add @username 3"},
	{Command: "place", Description: "Переставить человека: номер, @имя или ID, затем место (без места — в конец). /place 7 3"},
	{Command: "swap", Description: "Поменяться местами: /swap 5 — вы и №5, /swap 1 3 — №1 и №3"},
	{Command: "delete", Description: "Убрать человека из очереди: номер, @имя или ID. /delete 4"},
	{Command: "close", Description: "Закрыть очередь. Напишите название: /close Практика 4"},
	{Command: "queues", Description: "Список открытых очередей"},
	{Command: "show", Description: "Показать доску очереди внизу чата: /show Практика 4"},
	{Command: "help", Description: "Как пользоваться ботом"},
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

// groupCommand registers f like command, for commands that only make sense
// in a group: queues belong to a group chat, and every queue action needs
// to know who sent it. f can rely on msg.From being set.
func (h *Handler) groupCommand(name string, f func(ctx context.Context, msg *models.Message, args string)) {
	h.command(name, func(ctx context.Context, msg *models.Message, args string) {
		if msg.Chat.Type == models.ChatTypePrivate {
			h.reply(ctx, msg, privateChatText)
			return
		}
		if msg.From == nil {
			return
		}
		f(ctx, msg, args)
	})
}

func (h *Handler) help(ctx context.Context, msg *models.Message, _ string) {
	h.reply(ctx, msg, helpText)
}

func (h *Handler) newQueue(ctx context.Context, msg *models.Message, name string) {
	if name == "" {
		h.reply(ctx, msg, newUsageText)
		return
	}

	q, err := h.svc.Create(ctx, msg.Chat.ID, topicOf(msg), name, msg.From.ID)
	if err != nil {
		h.replyError(ctx, msg, "create queue", err)
		return
	}

	text, err := h.boardText(ctx, q)
	if err != nil {
		h.replyError(ctx, msg, "draw new board", err)
		return
	}
	board, err := h.send(ctx, q.ChatID, q.ThreadID, text, keyboard.Board(q.ID))
	if err != nil {
		log.Printf("post board of queue %d: %v", q.ID, err)
		return
	}
	if err := h.svc.SetBoardMessage(ctx, q.ID, board.ID); err != nil {
		log.Printf("remember board of queue %d: %v", q.ID, err)
	}
}

// swap handles /swap <pos>, which swaps the sender with whoever is at that
// position, and /swap <who> <who>, which swaps two other people, each given
// by position or user ID. Swapping others is allowed, so every swap is
// announced in the chat.
func (h *Handler) swap(ctx context.Context, msg *models.Message, _ string) {
	args, ok := h.mentionArgs(ctx, msg)
	if !ok {
		return
	}
	nums, ok := parseNumbers(args)
	if !ok || len(nums) < 1 || len(nums) > 2 {
		h.reply(ctx, msg, swapUsageText)
		return
	}

	q, ok := h.commandQueue(ctx, msg, "", true, "")
	if !ok {
		return
	}

	var (
		res service.Swapped
		err error
	)
	if len(nums) == 1 {
		res, err = h.svc.SwapWithPosition(ctx, q.ID, msg.From.ID, int(nums[0]))
	} else {
		res, err = h.svc.Swap(ctx, q.ID, nums[0], nums[1])
	}
	if err != nil {
		h.replyError(ctx, msg, "swap", err)
		return
	}
	h.redraw.Schedule(q.ID)

	names := h.namesFor(ctx, q.ID)
	h.reply(ctx, msg, swappedText(q.Name, render.Name(names, res.UserID), render.Name(names, res.TargetID), res.From, res.To))
}

// remove handles /delete <who>: it takes a person, given by position or
// user ID, out of the queue, for example after their defense. Anyone can
// do it, so it's announced in the chat.
func (h *Handler) remove(ctx context.Context, msg *models.Message, _ string) {
	args, ok := h.mentionArgs(ctx, msg)
	if !ok {
		return
	}
	nums, ok := parseNumbers(args)
	if !ok || len(nums) != 1 {
		h.reply(ctx, msg, deleteUsageText)
		return
	}

	q, ok := h.commandQueue(ctx, msg, "", true, "")
	if !ok {
		return
	}

	// Look the name up first: once removed, the person isn't in the queue
	// any more, and the name lookup only covers people in it.
	names := h.namesFor(ctx, q.ID)
	res, err := h.svc.Remove(ctx, q.ID, nums[0])
	if err != nil {
		h.replyError(ctx, msg, "delete", err)
		return
	}
	h.redraw.Schedule(q.ID)
	h.reply(ctx, msg, removedText(q.Name, render.Name(names, res.UserID), res.From))
}

// place handles /place <who> [position]: it moves someone already in the
// queue, given by position, user ID or mention, for fixing a wrong press.
// Without a position they go to the end. Placing someone else is allowed,
// like /swap, so it's always announced in the chat. Adding someone new is
// /add.
func (h *Handler) place(ctx context.Context, msg *models.Message, _ string) {
	args, ok := h.mentionArgs(ctx, msg)
	if !ok {
		return
	}
	nums, ok := parseNumbers(args)
	if !ok || len(nums) < 1 || len(nums) > 2 {
		h.reply(ctx, msg, placeUsageText)
		return
	}
	who, to, ok := whoAndPlace(nums)
	if !ok {
		h.reply(ctx, msg, errorTexts[model.ErrInvalidPosition])
		return
	}

	q, ok := h.commandQueue(ctx, msg, "", true, "")
	if !ok {
		return
	}

	res, err := h.svc.Place(ctx, q.ID, who, to)
	if err != nil {
		h.replyError(ctx, msg, "place", err)
		return
	}
	h.redraw.Schedule(q.ID)
	h.reply(ctx, msg, placedText(q.Name, render.Name(h.namesFor(ctx, q.ID), res.UserID), res))
}

// add handles /add <who> [position]: it puts someone who isn't in the queue
// at that position, or at the end by default. who is a user ID or a
// mention; or the command replies to the person's message, and then only
// the position is given, if any. Anyone can add anyone in the chat, so it's
// announced.
func (h *Handler) add(ctx context.Context, msg *models.Message, _ string) {
	args, ok := h.mentionArgs(ctx, msg)
	if !ok {
		return
	}
	nums, ok := parseNumbers(args)
	if !ok {
		h.reply(ctx, msg, addUsageText)
		return
	}

	// A reply to someone's message names them, so only the position is
	// left in the command.
	if person, replied := repliedPerson(msg); replied {
		h.saveUser(ctx, *person)
		nums = append([]int64{person.ID}, nums...)
	}
	if len(nums) < 1 || len(nums) > 2 {
		h.reply(ctx, msg, addUsageText)
		return
	}
	who, to, ok := whoAndPlace(nums)
	if !ok {
		h.reply(ctx, msg, errorTexts[model.ErrInvalidPosition])
		return
	}

	q, ok := h.commandQueue(ctx, msg, "", true, "")
	if !ok {
		return
	}

	res, err := h.svc.Add(ctx, q.ID, who, to)
	// Someone the bot has never seen can still be added if Telegram says
	// they're in this chat; that also gives the bot their name.
	if errors.Is(err, model.ErrUnknownUser) && h.learnMember(ctx, q.ChatID, who) {
		res, err = h.svc.Add(ctx, q.ID, who, to)
	}
	if err != nil {
		h.replyError(ctx, msg, "add", err)
		return
	}
	h.redraw.Schedule(q.ID)

	// The names are looked up after adding, so the new person has a name.
	h.reply(ctx, msg, placedText(q.Name, render.Name(h.namesFor(ctx, q.ID), res.UserID), res))
}

// whoAndPlace reads "<who> [position]". Without a position it's AtEnd; an
// explicit 0 is a typo, not "the end", so it's refused.
func whoAndPlace(nums []int64) (who int64, to int, ok bool) {
	if len(nums) == 1 {
		return nums[0], service.AtEnd, true
	}
	to = int(nums[1])
	return nums[0], to, to != service.AtEnd
}

// parseNumbers reads the whole numbers of a command, like "7 3".
func parseNumbers(args string) ([]int64, bool) {
	fields := strings.Fields(args)
	nums := make([]int64, len(fields))
	for i, f := range fields {
		n, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return nil, false
		}
		nums[i] = n
	}
	return nums, true
}

// learnMember asks Telegram whether userID is a member of the chat and, if
// so, saves their name, so they can be added to a queue.
func (h *Handler) learnMember(ctx context.Context, chatID, userID int64) bool {
	m, err := h.b.GetChatMember(ctx, &bot.GetChatMemberParams{ChatID: chatID, UserID: userID})
	if err != nil {
		return false
	}
	u, ok := memberUser(m)
	if !ok {
		return false
	}
	h.saveUser(ctx, u)
	return true
}

// memberUser returns the user behind a chat member, if they're still in the
// chat.
func memberUser(m *models.ChatMember) (models.User, bool) {
	switch m.Type {
	case models.ChatMemberTypeOwner:
		if m.Owner != nil && m.Owner.User != nil {
			return *m.Owner.User, true
		}
	case models.ChatMemberTypeAdministrator:
		if m.Administrator != nil {
			return m.Administrator.User, true
		}
	case models.ChatMemberTypeMember:
		if m.Member != nil && m.Member.User != nil {
			return *m.Member.User, true
		}
	case models.ChatMemberTypeRestricted:
		if m.Restricted != nil && m.Restricted.IsMember && m.Restricted.User != nil {
			return *m.Restricted.User, true
		}
	}
	return models.User{}, false
}

// namesFor loads a queue's current names for a public line. A failure only
// costs the names, which fall back to user IDs.
func (h *Handler) namesFor(ctx context.Context, queueID int64) map[int64]string {
	q, err := h.svc.Queue(ctx, queueID)
	if err != nil {
		log.Printf("reload queue %d for names: %v", queueID, err)
		return nil
	}
	names, err := h.svc.Names(ctx, q)
	if err != nil {
		log.Printf("names in queue %d: %v", queueID, err)
	}
	return names
}

// topicOf returns the forum topic a message was sent in, or 0. Only topic
// messages count: in a group without topics, MessageThreadID can also be
// set for replies, and that isn't a topic.
func topicOf(msg *models.Message) int {
	if msg.IsTopicMessage {
		return msg.MessageThreadID
	}
	return 0
}

// closeQueue handles /close <name>, or /close sent as a reply to a board.
// The board stays in the chat, marked closed; its buttons stay too and
// answer that the queue is closed.
func (h *Handler) closeQueue(ctx context.Context, msg *models.Message, name string) {
	q, ok := h.commandQueue(ctx, msg, name, false, closeUsageText)
	if !ok {
		return
	}

	if err := h.svc.Close(ctx, q.ID); err != nil {
		h.replyError(ctx, msg, "close queue", err)
		return
	}
	if err := h.drawBoard(ctx, q.ID); err != nil {
		log.Printf("redraw closed board of queue %d: %v", q.ID, err)
	}
	h.reply(ctx, msg, closedText(q.Name))
}

// commandQueue finds the queue a command is about, see pickQueue. If there
// is none, it explains why in the chat and returns false.
func (h *Handler) commandQueue(ctx context.Context, msg *models.Message, name string, onlyOne bool, usage string) (model.Queue, bool) {
	open, err := h.svc.OpenQueues(ctx, msg.Chat.ID, topicOf(msg))
	if err != nil {
		h.replyError(ctx, msg, "list open queues", err)
		return model.Queue{}, false
	}

	replyTo := 0
	if msg.ReplyToMessage != nil {
		replyTo = msg.ReplyToMessage.ID
	}
	q, err := pickQueue(open, replyTo, name, onlyOne)
	if err != nil {
		h.reply(ctx, msg, pickErrorText(err, usage))
		return model.Queue{}, false
	}
	return q, true
}

func (h *Handler) boardPress(ctx context.Context, _ *bot.Bot, update *models.Update) {
	cq := update.CallbackQuery
	h.saveUser(ctx, cq.From)

	queueID, action, err := callbacks.Decode(cq.Data)
	if err != nil {
		h.toast(ctx, cq, badButtonText)
		return
	}
	q, err := h.svc.Queue(ctx, queueID)
	if err != nil {
		h.toast(ctx, cq, badButtonText)
		return
	}

	if action == callbacks.Show {
		h.showPress(ctx, cq, q)
		return
	}

	if !pressedOn(cq, q.ChatID) {
		h.toast(ctx, cq, badButtonText)
		return
	}

	ans := h.act(ctx, q, cq.From.ID, action)
	h.toast(ctx, cq, ans.text)
	if ans.redraw {
		h.redraw.Schedule(queueID)
	}
}

// showPress handles a queue's button in the chat's list of queues: it posts
// that queue's board again at the bottom of the chat.
func (h *Handler) showPress(ctx context.Context, cq *models.CallbackQuery, q model.Queue) {
	// As with board buttons, only a button that really sits under this
	// chat's list counts.
	if !pressedOn(cq, q.ChatID) {
		h.toast(ctx, cq, badButtonText)
		return
	}

	if err := h.repostBoard(ctx, q.ID); err != nil {
		h.toast(ctx, cq, h.userError(err, "repost board of queue %d", q.ID))
		return
	}
	// An empty answer just stops the button's loading spinner.
	h.toast(ctx, cq, "")
}

// answer is how the bot responds to a board button.
type answer struct {
	text string
	// redraw is set when the queue changed and its board needs redrawing.
	redraw bool
}

// act runs one board action and returns the answer to show.
func (h *Handler) act(ctx context.Context, q model.Queue, userID int64, action callbacks.Action) answer {
	var (
		text string
		pos  int
		err  error
	)
	switch action {
	case callbacks.Join:
		pos, err = h.svc.Join(ctx, q.ID, userID)
		text = joinedText(pos)
	case callbacks.Leave:
		err = h.svc.Leave(ctx, q.ID, userID)
		text = leftText
	case callbacks.ToEnd:
		pos, err = h.svc.ToEnd(ctx, q.ID, userID)
		text = toEndText(pos)
	case callbacks.Where:
		return answer{text: whereText(q.Position(userID), len(q.Entries))}
	case callbacks.All:
		// Same as /queues: the list of the topic's queues.
		if err := h.postIndex(ctx, q.ChatID, q.ThreadID); err != nil {
			return answer{text: h.userError(err, "post list of queues in chat %d", q.ChatID)}
		}
		return answer{}
	}

	if err != nil {
		return answer{text: h.userError(err, "queue %d, action %q, user %d", q.ID, action, userID)}
	}
	return answer{text: text, redraw: true}
}

// pressedOn reports whether the pressed button sits in the queue's own
// chat. Button data can be forged, so a button for this queue pressed in
// any other chat is refused. Any copy of a board or list in its own chat
// works, old ones included.
func pressedOn(cq *models.CallbackQuery, chatID int64) bool {
	msg := cq.Message.Message
	return msg != nil && msg.Chat.ID == chatID
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
		ParseMode:   models.ParseModeHTML,
		ReplyMarkup: keyboard.Board(q.ID),
	})
	// Telegram refuses an edit that changes nothing, for example when
	// someone joined and left before the redraw. That's not a problem.
	if isNotModified(err) {
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

// toast answers a button press with a short text only the presser sees. An
// empty text just stops the button's loading spinner.
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
		MessageThreadID: topicOf(msg),
		Text:            text,
		ReplyParameters: &models.ReplyParameters{MessageID: msg.ID},
	})
	if err != nil {
		log.Printf("reply in chat %d: %v", msg.Chat.ID, err)
	}
}

// replyError answers with the error's message, see userError.
func (h *Handler) replyError(ctx context.Context, msg *models.Message, what string, err error) {
	h.reply(ctx, msg, h.userError(err, "%s in chat %d", what, msg.Chat.ID))
}

// userError returns the message a person sees for err. Unexpected errors
// are also logged with the given context, since the person only sees a
// generic message. A cancelled context means the bot is shutting down,
// which isn't worth logging.
func (h *Handler) userError(err error, format string, args ...any) string {
	text, known := errorText(err)
	if !known && !errors.Is(err, context.Canceled) {
		log.Printf(format+": %v", append(args, err)...)
	}
	return text
}
