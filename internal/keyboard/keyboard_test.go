package keyboard

import (
	"testing"

	"github.com/Silvelle/queue-core-bot/internal/callbacks"
)

var wantText = map[callbacks.Action]string{
	callbacks.Join:  "+ Записаться",
	callbacks.Leave: "Выйти",
	callbacks.ToEnd: "В конец",
	callbacks.Done:  "✓ Сдано",
	callbacks.Undo:  "↩ Сброс",
	callbacks.Where: "Где я?",
	callbacks.All:   "📋 Все очереди",
}

func TestBoardLayout(t *testing.T) {
	want := [][]string{
		{"+ Записаться", "Выйти"},
		{"В конец", "Где я?"},
		{"✓ Сдано", "↩ Сброс"},
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

func TestLinks(t *testing.T) {
	kb := Links([]Link{
		{Text: "Practice 4", URL: "https://t.me/c/123/10"},
		{Text: "Lab 2", URL: "https://t.me/c/123/20"},
	})
	if len(kb.InlineKeyboard) != 2 {
		t.Fatalf("%d rows, want one per link", len(kb.InlineKeyboard))
	}
	btn := kb.InlineKeyboard[1][0]
	if btn.Text != "Lab 2" || btn.URL != "https://t.me/c/123/20" || btn.CallbackData != "" {
		t.Errorf("second button = %+v, want a plain link to Lab 2", btn)
	}
}

func TestMessageURL(t *testing.T) {
	tests := []struct {
		name   string
		chatID int64
		msgID  int
		want   string
		wantOK bool
	}{
		{"supergroup", -1001234567890, 42, "https://t.me/c/1234567890/42", true},
		{"basic group", -123456, 42, "", false},
		{"private chat", 123456, 42, "", false},
		{"no message yet", -1001234567890, 0, "", false},
		{"only the prefix", -100, 42, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MessageURL(tt.chatID, tt.msgID)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("MessageURL(%d, %d) = %q, %v, want %q, %v", tt.chatID, tt.msgID, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}
