package render

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

var names = map[int64]string{
	1: "Анна Кузнецова",
	2: "Михаил Петров",
	3: "Иван",
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

	want := `<b>Practice 4</b>
В очереди: 3

1. Михаил Петров
2. Иван
3. @dmitry_s

<b>Сдали:</b>
- Анна Кузнецова`

	if got := Board(q, names); got != want {
		t.Errorf("Board() =\n%s\n\nwant\n%s", got, want)
	}
}

// Everyone has defended: the board shows only who did, without the
// "nobody yet" hint, which would be wrong here.
func TestBoardAllDone(t *testing.T) {
	q := model.Queue{Name: "Lab 2", Entries: []model.Entry{
		{UserID: 1, Done: true},
		{UserID: 3, Done: true},
	}}

	want := `<b>Lab 2</b>
В очереди: 0

<b>Сдали:</b>
- Анна Кузнецова
- Иван`

	if got := Board(q, names); got != want {
		t.Errorf("Board() =\n%s\n\nwant\n%s", got, want)
	}
}

// Queue and student names come from users, so HTML in them must show up as
// text instead of breaking the message or formatting it.
func TestBoardEscapesHTML(t *testing.T) {
	q := model.Queue{Name: "Лаб <3> & co", Entries: []model.Entry{
		{UserID: 7},
		{UserID: 8, Done: true},
	}}
	evil := map[int64]string{7: "<b>Саша</b>", 8: "Аня & Ко"}

	want := `<b>Лаб &lt;3&gt; &amp; co</b>
В очереди: 1

1. &lt;b&gt;Саша&lt;/b&gt;

<b>Сдали:</b>
- Аня &amp; Ко`

	if got := Board(q, evil); got != want {
		t.Errorf("Board() =\n%s\n\nwant\n%s", got, want)
	}
}

// The board doesn't name who created the queue.
func TestBoardHidesCreator(t *testing.T) {
	q := model.Queue{Name: "Lab 2", CreatedBy: 1, Entries: []model.Entry{{UserID: 3}}}

	if got := Board(q, names); strings.Contains(got, "Анна") {
		t.Errorf("Board() shows the creator:\n%s", got)
	}
}

func TestBoardEmpty(t *testing.T) {
	q := model.Queue{Name: "Lab 2", CreatedBy: 3}

	want := `<b>Lab 2</b>
В очереди: 0

Пока никого. Нажмите «+ Записаться».`

	if got := Board(q, names); got != want {
		t.Errorf("Board() =\n%s\n\nwant\n%s", got, want)
	}
}

func TestBoardClosed(t *testing.T) {
	q := model.Queue{Name: "Practice 3", CreatedBy: 3, Closed: true, Entries: []model.Entry{{UserID: 3}}}

	got := Board(q, names)
	if !strings.HasPrefix(got, "<b>Practice 3</b> (закрыта)\n") {
		t.Errorf("Board() first line should mark the queue closed, got:\n%s", got)
	}
}

func TestBoardUnknownName(t *testing.T) {
	q := model.Queue{Name: "Lab 2", CreatedBy: 3, Entries: []model.Entry{{UserID: 99}}}

	if got := Board(q, names); !strings.Contains(got, "1. пользователь 99") {
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
	if !strings.Contains(got, "и ещё") {
		t.Error("a cut list should say how many people are hidden")
	}
	if !strings.Contains(got, "\n<b>Сдали:</b>") {
		t.Error("a long board should still show the start of the done list")
	}
}

func TestName(t *testing.T) {
	if got := Name(names, 1); got != "Анна Кузнецова" {
		t.Errorf("Name(known) = %q, want %q", got, "Анна Кузнецова")
	}
	if got := Name(names, 99); got != "пользователь 99" {
		t.Errorf("Name(unknown) = %q, want %q", got, "пользователь 99")
	}
}
