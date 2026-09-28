package render

import (
	"fmt"
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

// Where returns one line per open queue with the user's place in it, for
// the "Все очереди" button.
//
//	Practice 4: вы №3 из 7
//	Lab 2: сдано
//	Практика 5: вас нет (в очереди 4)
func Where(open []model.Queue, userID int64) string {
	if len(open) == 0 {
		return "В этом чате нет открытых очередей."
	}

	lines := make([]string, 0, len(open))
	for _, q := range open {
		waiting, pos := len(q.Waiting()), q.Position(userID)
		var status string
		switch {
		case pos > 0:
			status = fmt.Sprintf("вы №%d из %d", pos, waiting)
		case q.Has(userID):
			status = "сдано"
		default:
			status = fmt.Sprintf("вас нет (в очереди %d)", waiting)
		}
		lines = append(lines, q.Name+": "+status)
	}
	return strings.Join(lines, "\n")
}
