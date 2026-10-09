package handler

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/go-telegram/bot/models"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

// unknownMention is a @username the bot can't match to anyone it knows.
type unknownMention struct{ name string }

func (u unknownMention) Error() string { return "unknown mention " + u.name }

// mentionArgs returns a command's arguments with every mentioned person
// replaced by their user ID, so "/add @anna_k 3" reads as "/add 123456 3".
//
// Picking someone without a username from the "@" list gives Telegram's
// text mention, which carries the person, so it works for anyone. A plain
// @username carries only the name, and Telegram gives bots no way to look
// it up, so it works for people the bot has already seen. If a mention
// can't be resolved, mentionArgs explains that in the chat and returns
// false.
func (h *Handler) mentionArgs(ctx context.Context, msg *models.Message) (string, bool) {
	text, err := replaceMentions(msg.Text, msg.Entities, func(e models.MessageEntity, mentioned string) (string, error) {
		if e.Type == models.MessageEntityTypeTextMention && e.User != nil {
			h.saveUser(ctx, *e.User)
			return strconv.FormatInt(e.User.ID, 10), nil
		}
		u, err := h.svc.UserByUsername(ctx, strings.TrimPrefix(mentioned, "@"))
		if errors.Is(err, model.ErrNotFound) {
			return "", unknownMention{mentioned}
		}
		if err != nil {
			return "", err
		}
		return strconv.FormatInt(u.ID, 10), nil
	})

	var unknown unknownMention
	switch {
	case errors.As(err, &unknown):
		h.reply(ctx, msg, unknownMentionText(unknown.name))
		return "", false
	case err != nil:
		h.replyError(ctx, msg, "resolve mentions", err)
		return "", false
	}
	_, args, _ := parseCommand(text, h.botUsername)
	return args, true
}

// replaceMentions returns text with each mention replaced by what value
// returns for it. Telegram gives entity offsets in UTF-16 code units, so
// the text is edited in those; Cyrillic before a mention would otherwise
// shift every offset.
func replaceMentions(text string, entities []models.MessageEntity, value func(e models.MessageEntity, mentioned string) (string, error)) (string, error) {
	units := utf16.Encode([]rune(text))

	// Later mentions first, so the offsets of earlier ones stay valid.
	sorted := slices.Clone(entities)
	slices.SortFunc(sorted, func(a, b models.MessageEntity) int { return cmp.Compare(b.Offset, a.Offset) })

	for _, e := range sorted {
		if e.Type != models.MessageEntityTypeMention && e.Type != models.MessageEntityTypeTextMention {
			continue
		}
		if e.Offset < 0 || e.Length <= 0 || e.Offset+e.Length > len(units) {
			continue
		}
		mentioned := string(utf16.Decode(units[e.Offset : e.Offset+e.Length]))
		v, err := value(e, mentioned)
		if err != nil {
			return "", err
		}
		units = slices.Concat(units[:e.Offset], utf16.Encode([]rune(v)), units[e.Offset+e.Length:])
	}
	return string(utf16.Decode(units)), nil
}

// repliedPerson returns the person a command replies to, if it replies to a
// person's message rather than to a board or other bot message. In a forum
// topic every message counts as a reply to the topic's first message; that
// isn't a reply to its author, so it's ignored.
func repliedPerson(msg *models.Message) (*models.User, bool) {
	r := msg.ReplyToMessage
	if r == nil || r.From == nil || r.From.IsBot || r.ForumTopicCreated != nil {
		return nil, false
	}
	if msg.IsTopicMessage && r.ID == msg.MessageThreadID {
		return nil, false
	}
	return r.From, true
}
