package keyboard

import (
	"testing"

	"github.com/Silvelle/queue-core-bot/internal/callbacks"
)

func TestBoardLayout(t *testing.T) {
	want := [][]string{
		{"+ Join", "Leave"},
		{"To end", "✓ Done"},
		{"Where am I?"},
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

func TestBoardButtonsDecode(t *testing.T) {
	const queueID = 42
	wantText := map[callbacks.Action]string{
		callbacks.Join:  "+ Join",
		callbacks.Leave: "Leave",
		callbacks.ToEnd: "To end",
		callbacks.Done:  "✓ Done",
		callbacks.Where: "Where am I?",
	}

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
