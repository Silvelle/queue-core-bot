package render

import (
	"fmt"
	"html"
	"strings"
	"unicode/utf8"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

const maxText = 4000

// Name returns the name to show for a user, falling back to the user ID
// when the bot has never seen them.
func Name(names map[int64]string, id int64) string {
	if n, ok := names[id]; ok {
		return n
	}
	return fmt.Sprintf("пользователь %d", id)
}

// Board returns the text of a queue board, in Telegram's HTML format.
// names maps user IDs to the names to show, see Name.
//
//	<b>Практика 4</b>
//	В очереди: 2
//
//	1. Михаил Петров
//	2. Иван
//
//	<b>Сдали:</b>
//	- Анна Кузнецова
func Board(q model.Queue, names map[int64]string) string {
	var t text
	t.write("<b>" + html.EscapeString(q.Name) + "</b>")
	if q.Closed {
		t.write(" (закрыта)")
	}

	waiting := q.Waiting()
	t.write(fmt.Sprintf("\nВ очереди: %d", len(waiting)))

	if len(q.Entries) == 0 {
		t.write("\n\nПока никого. Нажмите «+ Записаться».")
	}

	if len(waiting) > 0 {
		lines := make([]string, len(waiting))
		for i, e := range waiting {
			lines[i] = fmt.Sprintf("%d. %s", i+1, escapedName(names, e.UserID))
		}
		t.write("\n")
		t.list(lines)
	}

	var done []string
	for _, e := range q.Entries {
		if e.Done {
			done = append(done, "- "+escapedName(names, e.UserID))
		}
	}
	if len(done) > 0 {
		t.write("\n\n<b>Сдали:</b>")
		t.list(done)
	}

	return t.b.String()
}

// escapedName is Name made safe for HTML: a student called "<b>Саша</b>"
// must show up as that text, not break the message.
func escapedName(names map[int64]string, id int64) string {
	return html.EscapeString(Name(names, id))
}

// text builds a message and keeps count of its characters, so checking
// the limit doesn't mean counting the whole message again for every line.
type text struct {
	b     strings.Builder
	runes int
}

func (t *text) write(s string) {
	t.b.WriteString(s)
	t.runes += utf8.RuneCountInString(s)
}

// list writes lines, one per row. If they don't all fit, the rest is
// replaced by a line saying how many are hidden.
func (t *text) list(lines []string) {
	for i, line := range lines {
		if !t.fits("\n" + line) {
			t.write(fmt.Sprintf("\n… и ещё %d", len(lines)-i))
			return
		}
		t.write("\n" + line)
	}
}

// fits reports whether s can be added without going over maxText. Tags and
// escapes are counted too, although Telegram doesn't count them, so the
// check errs on the safe side.
func (t *text) fits(s string) bool {
	return t.runes+utf8.RuneCountInString(s) <= maxText
}
