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

	want := `Practice 4
В очереди: 3

1. Михаил Петров
2. Иван
3. @dmitry_s

Сдали: Анна Кузнецова`

	if got := Board(q, names); got != want {
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

	want := `Lab 2
В очереди: 0

Пока никого. Нажмите «+ Записаться».`

	if got := Board(q, names); got != want {
		t.Errorf("Board() =\n%s\n\nwant\n%s", got, want)
	}
}

func TestBoardClosed(t *testing.T) {
	q := model.Queue{Name: "Practice 3", CreatedBy: 3, Closed: true, Entries: []model.Entry{{UserID: 3}}}

	got := Board(q, names)
	if !strings.HasPrefix(got, "Practice 3 (закрыта)\n") {
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
	if !strings.Contains(got, "Сдали: 100 чел.") {
		t.Error("a long done list should be shortened to a count")
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
