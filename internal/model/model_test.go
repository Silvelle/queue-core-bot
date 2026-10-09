package model

import "testing"

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name string
		user User
		want string
	}{
		{"first and last", User{FirstName: "Anna", LastName: "Kuznetsova"}, "Anna K."},
		{"cyrillic last name", User{FirstName: "Анна", LastName: "Кузнецова"}, "Анна К."},
		{"first only", User{FirstName: "Anna"}, "Anna"},
		{"username only", User{Username: "anna_k"}, "@anna_k"},
		{"nothing", User{ID: 42}, "user 42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.DisplayName(); got != tt.want {
				t.Errorf("DisplayName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFullName(t *testing.T) {
	tests := []struct {
		name string
		user User
		want string
	}{
		{"first and last", User{FirstName: "Anna", LastName: "Kuznetsova"}, "Anna Kuznetsova"},
		{"cyrillic", User{FirstName: "Анна", LastName: "Кузнецова"}, "Анна Кузнецова"},
		{"first only", User{FirstName: "Anna"}, "Anna"},
		{"username only", User{Username: "anna_k"}, "@anna_k"},
		{"nothing", User{ID: 42}, "user 42"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.FullName(); got != tt.want {
				t.Errorf("FullName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestQueuePositions(t *testing.T) {
	q := Queue{Entries: []Entry{{UserID: 2}, {UserID: 4}, {UserID: 7}}}

	tests := []struct {
		user    int64
		wantPos int
	}{
		{user: 2, wantPos: 1},
		{user: 4, wantPos: 2},
		{user: 7, wantPos: 3},
		{user: 5, wantPos: 0},
	}
	for _, tt := range tests {
		if got := q.Position(tt.user); got != tt.wantPos {
			t.Errorf("Position(%d) = %d, want %d", tt.user, got, tt.wantPos)
		}
	}
}
