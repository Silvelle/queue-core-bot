package render

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

const maxText = 4000

func Board(q model.Queue, names map[int64]string) string {
	name := func(id int64) string {
		if n, ok := names[id]; ok {
			return n
		}
		return fmt.Sprintf("user %d", id)
	}

	var b strings.Builder
	b.WriteString(q.Name)
	if q.Closed {
		b.WriteString(" (closed)")
	}

	waiting := q.Waiting()
	fmt.Fprintf(&b, "\n%d waiting · started by %s\n\n", len(waiting), name(q.CreatedBy))

	if len(waiting) == 0 {
		b.WriteString("No one yet. Press + Join.")
	}
	for i, e := range waiting {
		line := fmt.Sprintf("%d. %s\n", i+1, name(e.UserID))
		if utf8.RuneCountInString(b.String())+utf8.RuneCountInString(line) > maxText {
			fmt.Fprintf(&b, "… and %d more\n", len(waiting)-i)
			break
		}
		b.WriteString(line)
	}

	var done []string
	for _, e := range q.Entries {
		if e.Done {
			done = append(done, name(e.UserID))
		}
	}
	if len(done) > 0 {
		doneLine := "\nDone: " + strings.Join(done, ", ")
		if utf8.RuneCountInString(b.String())+utf8.RuneCountInString(doneLine) > maxText {
			doneLine = fmt.Sprintf("\nDone: %d people", len(done))
		}
		b.WriteString(doneLine)
	}

	return strings.TrimRight(b.String(), "\n")
}
