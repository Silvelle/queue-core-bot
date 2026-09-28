package handler

import (
	"errors"
	"testing"

	"github.com/Silvelle/queue-core-bot/internal/model"
)

func TestPickQueue(t *testing.T) {
	practice := model.Queue{ID: 1, Name: "Practice 4", BoardMsgID: 100}
	lab := model.Queue{ID: 2, Name: "Lab 2", BoardMsgID: 200}
	both := []model.Queue{practice, lab}

	tests := []struct {
		name    string
		open    []model.Queue
		replyTo int
		qname   string
		onlyOne bool
		wantID  int64
		wantErr error
	}{
		{"reply to a board", both, 200, "", true, 2, nil},
		{"reply wins over name", both, 200, "Practice 4", false, 2, nil},
		{"reply to another message is ignored", both, 999, "", true, 0, errWhichQueue},
		{"by name", both, 0, "Lab 2", false, 2, nil},
		{"by name, other case", both, 0, "lab 2", false, 2, nil},
		{"unknown name", both, 0, "Lab 9", false, 0, errNoSuchName},
		{"only queue", []model.Queue{practice}, 0, "", true, 1, nil},
		{"only queue not allowed", []model.Queue{practice}, 0, "", false, 0, errWhichQueue},
		{"several queues", both, 0, "", true, 0, errWhichQueue},
		{"no queues", nil, 0, "", true, 0, errNoOpenQueues},
		{"no queues, with name", nil, 0, "Lab 2", false, 0, errNoSuchName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := pickQueue(tt.open, tt.replyTo, tt.qname, tt.onlyOne)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if q.ID != tt.wantID {
				t.Errorf("picked queue %d, want %d", q.ID, tt.wantID)
			}
		})
	}
}

func TestBoardMarkup(t *testing.T) {
	open := boardMarkup(model.Queue{ID: 1})
	if len(open.InlineKeyboard) == 0 {
		t.Error("an open queue has no buttons")
	}

	closed := boardMarkup(model.Queue{ID: 1, Closed: true})
	if closed == nil || closed.InlineKeyboard == nil || len(closed.InlineKeyboard) != 0 {
		t.Errorf("a closed queue should get an empty, non-nil keyboard, got %+v", closed)
	}
}
