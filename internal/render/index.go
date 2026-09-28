package render

import (
	"html"
	"strings"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

// Index returns the text of the chat's list of open queues, in Telegram's
// HTML format. The list doesn't show how many people are in each queue, so
// it only changes when a queue is opened or closed, not on every press.
//
//	<b>Доступные очереди:</b>
//	- Практика 4
//	- Лаб 2
func Index(open []model.Queue) string {
	if len(open) == 0 {
		return "Открытых очередей нет.\nСоздайте новую: /new " + html.EscapeString("<название>")
	}

	var b strings.Builder
	b.WriteString("<b>Доступные очереди:</b>")
	for _, q := range open {
		b.WriteString("\n- " + html.EscapeString(q.Name))
	}
	return b.String()
}
