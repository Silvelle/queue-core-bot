package handler

import "testing"

func TestParseCommand(t *testing.T) {
	const bot = "queue_bot"

	tests := []struct {
		name     string
		text     string
		wantName string
		wantArgs string
		wantOK   bool
	}{
		{"plain", "/new Practice 4", "new", "Practice 4", true},
		{"addressed to this bot", "/new@queue_bot Practice 4", "new", "Practice 4", true},
		{"username in other case", "/new@Queue_Bot Practice 4", "new", "Practice 4", true},
		{"addressed to another bot", "/new@other_bot Practice 4", "", "", false},
		{"no arguments", "/new", "new", "", true},
		{"extra spaces", "/new    Practice 4   ", "new", "Practice 4", true},
		{"addressed, no arguments", "/queues@queue_bot", "queues", "", true},
		{"not a command", "hello", "", "", false},
		{"slash in the middle", "hi /new", "", "", false},
		{"empty", "", "", "", false},
		{"slash only", "/", "", "", false},
		{"username only", "/@queue_bot", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, args, ok := parseCommand(tt.text, bot)
			if name != tt.wantName || args != tt.wantArgs || ok != tt.wantOK {
				t.Errorf("parseCommand(%q) = %q, %q, %v, want %q, %q, %v",
					tt.text, name, args, ok, tt.wantName, tt.wantArgs, tt.wantOK)
			}
		})
	}
}
