package keyboard

import (
	"testing"

	"github.com/Silvelle/queue-core-bot/internal/callbacks"
	"github.com/Silvelle/queue-core-bot/internal/model"
)

var wantText = map[callbacks.Action]string{
	callbacks.Join:  "+ Записаться",
	callbacks.Leave: "Выйти",
	callbacks.ToEnd: "В конец",
	callbacks.Where: "Где я?",
	callbacks.All:   "📋 Все очереди",
}

func TestBoardLayout(t *testing.T) {
	want := [][]string{
		{"+ Записаться", "Выйти"},
		{"В конец", "Где я?"},
		{"📋 Все очереди"},
	}

	rows := Board(42).InlineKeyboard
	if len(rows) != len(want) {
		t.Fatalf("%d rows, want %d", len(rows), len(want))
	}
	for i, row := range rows {
		if len(row) != len(want[i]) {
			t.Fatalf("row %d has %d buttons, want %d", i, len(row), len(want[i]))
		}
		for j, btn := range row {
			if btn.Text != want[i][j] {
				t.Errorf("row %d button %d = %q, want %q", i, j, btn.Text, want[i][j])
			}
		}
	}
}

// Every button must decode back to this queue, and each action must appear
// exactly once: a typo like two Join buttons would otherwise go unnoticed.
func TestBoardButtonsDecode(t *testing.T) {
	const queueID = 42

	seen := make(map[callbacks.Action]bool)
	for _, row := range Board(queueID).InlineKeyboard {
		for _, btn := range row {
			id, a, err := callbacks.Decode(btn.CallbackData)
			if err != nil {
				t.Fatalf("button %q: Decode(%q): %v", btn.Text, btn.CallbackData, err)
			}
			if id != queueID {
				t.Errorf("button %q points to queue %d, want %d", btn.Text, id, queueID)
			}
			if btn.Text != wantText[a] {
				t.Errorf("action %q has label %q, want %q", a, btn.Text, wantText[a])
			}
			if seen[a] {
				t.Errorf("action %q appears twice", a)
			}
			seen[a] = true
			if len(btn.CallbackData) > callbacks.MaxLen {
				t.Errorf("button %q data is %d bytes, limit is %d", btn.Text, len(btn.CallbackData), callbacks.MaxLen)
			}
		}
	}
	if len(seen) != len(wantText) {
		t.Errorf("%d actions on the board, want %d", len(seen), len(wantText))
	}
}

func TestIndex(t *testing.T) {
	kb := Index([]model.Queue{{ID: 1, Name: "Практика 4"}, {ID: 2, Name: "Лаб 2"}})
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("%d rows, want one per queue", len(kb.InlineKeyboard))
	}
	for i, wantID := range []int64{1, 2} {
		btn := kb.InlineKeyboard[i][0]
		id, a, err := callbacks.Decode(btn.CallbackData)
		if err != nil || id != wantID || a != callbacks.Show {
			t.Errorf("button %q = %q, want Show for queue %d", btn.Text, btn.CallbackData, wantID)
		}
	}
	if kb.InlineKeyboard[1][0].Text != "Лаб 2" {
		t.Errorf("second button = %q, want the queue's name", kb.InlineKeyboard[1][0].Text)
	}
}

func TestIndexEmpty(t *testing.T) {
	if kb := Index(nil); kb == nil || len(kb.InlineKeyboard) != 0 {
		t.Errorf("Index(nil) = %+v, want an empty keyboard", kb)
	}
}
