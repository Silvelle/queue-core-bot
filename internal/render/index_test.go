package render

import (
	"testing"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

var (
	practice = model.Queue{Name: "Practice 4", Entries: []model.Entry{
		{UserID: 1, Done: true}, {UserID: 2}, {UserID: 3}, {UserID: 4},
	}}
	lab = model.Queue{Name: "Lab 2", Entries: []model.Entry{{UserID: 3}}}
)

func TestIndex(t *testing.T) {
	tests := []struct {
		name string
		open []model.Queue
		want string
	}{
		{"two queues", []model.Queue{practice, lab},
			"<b>Доступные очереди:</b>\n- Practice 4\n- Lab 2"},
		{"name with HTML", []model.Queue{{Name: "Лаб <3>"}},
			"<b>Доступные очереди:</b>\n- Лаб &lt;3&gt;"},
		// "<название>" would be read as an HTML tag and break the message.
		{"empty", nil,
			"Открытых очередей нет.\nСоздайте новую: /new &lt;название&gt;"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Index(tt.open); got != tt.want {
				t.Errorf("Index() =\n%s\n\nwant\n%s", got, tt.want)
			}
		})
	}
}
