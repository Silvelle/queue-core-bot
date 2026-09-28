package render

import (
	"fmt"
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
	return fmt.Sprintf("user %d", id)
}

// Board returns the text of a queue board. names maps user IDs to the names
// to show, see Name.
//
//	Practice 4
//	3 waiting · started by Anna Kuznetsova
//
//	1. Михаил Петров
//	2. Ivan
//	3. @dmitry_s
//
//	Done: Anna Kuznetsova
func Board(q model.Queue, names map[int64]string) string {
	var t text
	t.write(q.Name)
	if q.Closed {
		t.write(" (closed)")
	}

	waiting := q.Waiting()
	t.write(fmt.Sprintf("\n%d waiting · started by %s\n\n", len(waiting), Name(names, q.CreatedBy)))

	if len(waiting) == 0 {
		t.write("No one yet. Press + Join.")
	}
	for i, e := range waiting {
		line := fmt.Sprintf("%d. %s\n", i+1, Name(names, e.UserID))
		if !t.fits(line) {
			t.write(fmt.Sprintf("… and %d more\n", len(waiting)-i))
			break
		}
		t.write(line)
	}

	var done []string
	for _, e := range q.Entries {
		if e.Done {
			done = append(done, Name(names, e.UserID))
		}
	}
	if len(done) > 0 {
		doneLine := "\nDone: " + strings.Join(done, ", ")
		if !t.fits(doneLine) {
			doneLine = fmt.Sprintf("\nDone: %d people", len(done))
		}
		t.write(doneLine)
	}

	return strings.TrimRight(t.b.String(), "\n")
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

// fits reports whether s can be added without going over maxText.
func (t *text) fits(s string) bool {
	return t.runes+utf8.RuneCountInString(s) <= maxText
}
