package render

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

var names = map[int64]string{
	1: "Anna Kuznetsova",
	2: "Михаил Петров",
	3: "Ivan",
	4: "@dmitry_s",
}

func TestBoard(t *testing.T) {
	q := model.Queue{
		Name:      "Practice 4",
		CreatedBy: 1,
		Entries: []model.Entry{
			{UserID: 1, Done: true},
			{UserID: 2},
			{UserID: 3},
			{UserID: 4},
		},
	}

	want := `Practice 4
3 waiting · started by Anna Kuznetsova

1. Михаил Петров
2. Ivan
3. @dmitry_s

Done: Anna Kuznetsova`

	if got := Board(q, names); got != want {
		t.Errorf("Board() =\n%s\n\nwant\n%s", got, want)
	}
}

func TestBoardEmpty(t *testing.T) {
	q := model.Queue{Name: "Lab 2", CreatedBy: 3}

	want := `Lab 2
0 waiting · started by Ivan

No one yet. Press + Join.`

	if got := Board(q, names); got != want {
		t.Errorf("Board() =\n%s\n\nwant\n%s", got, want)
	}
}

func TestBoardClosed(t *testing.T) {
	q := model.Queue{Name: "Practice 3", CreatedBy: 3, Closed: true, Entries: []model.Entry{{UserID: 3}}}

	got := Board(q, names)
	if !strings.HasPrefix(got, "Practice 3 (closed)\n") {
		t.Errorf("Board() first line should mark the queue closed, got:\n%s", got)
	}
}

func TestBoardUnknownName(t *testing.T) {
	q := model.Queue{Name: "Lab 2", CreatedBy: 3, Entries: []model.Entry{{UserID: 99}}}

	if got := Board(q, names); !strings.Contains(got, "1. user 99") {
		t.Errorf("Board() should fall back to the user ID, got:\n%s", got)
	}
}

// A very long queue must still fit into one Telegram message.
func TestBoardFitsTelegramLimit(t *testing.T) {
	long := make(map[int64]string)
	q := model.Queue{Name: "Huge", CreatedBy: 1}
	for i := int64(1); i <= 300; i++ {
		long[i] = strings.Repeat("я", 60)
		q.Entries = append(q.Entries, model.Entry{UserID: i, Done: i <= 100})
	}

	got := Board(q, long)
	if n := utf8.RuneCountInString(got); n > 4096 {
		t.Fatalf("board is %d characters, Telegram allows 4096", n)
	}
	if !strings.Contains(got, "more") {
		t.Error("a cut list should say how many people are hidden")
	}
	if !strings.Contains(got, "Done: 100 people") {
		t.Error("a long done list should be shortened to a count")
	}
}

func TestName(t *testing.T) {
	if got := Name(names, 1); got != "Anna Kuznetsova" {
		t.Errorf("Name(known) = %q, want %q", got, "Anna Kuznetsova")
	}
	if got := Name(names, 99); got != "user 99" {
		t.Errorf("Name(unknown) = %q, want %q", got, "user 99")
	}
}
