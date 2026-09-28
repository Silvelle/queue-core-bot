package callbacks

import (
	"errors"
	"math"
	"testing"
)

var allActions = []Action{Join, Leave, ToEnd, Done, Where}

// Whatever Encode writes, Decode must read back exactly.
func TestRoundTrip(t *testing.T) {
	for _, id := range []int64{1, 42, math.MaxInt64} {
		for _, a := range allActions {
			data := Encode(id, a)
			gotID, gotA, err := Decode(data)
			if err != nil {
				t.Fatalf("Decode(%q): %v", data, err)
			}
			if gotID != id || gotA != a {
				t.Errorf("Decode(%q) = %d, %q, want %d, %q", data, gotID, gotA, id, a)
			}
		}
	}
}

func TestEncodeFormat(t *testing.T) {
	tests := []struct {
		id   int64
		a    Action
		want string
	}{
		{42, Join, "b:42:j"},
		{42, Leave, "b:42:l"},
		{42, ToEnd, "b:42:e"},
		{42, Done, "b:42:x"},
		{42, Where, "b:42:w"},
		{7, Join, "b:7:j"},
	}
	for _, tt := range tests {
		if got := Encode(tt.id, tt.a); got != tt.want {
			t.Errorf("Encode(%d, %q) = %q, want %q", tt.id, tt.a, got, tt.want)
		}
	}
}

func TestActionsAreDistinct(t *testing.T) {
	seen := make(map[string]Action)
	for _, a := range allActions {
		data := Encode(1, a)
		if prev, ok := seen[data]; ok {
			t.Errorf("actions %q and %q both encode to %q", prev, a, data)
		}
		seen[data] = a
	}
}

func TestFitsTelegramLimit(t *testing.T) {
	for _, a := range allActions {
		if data := Encode(math.MaxInt64, a); len(data) > MaxLen {
			t.Errorf("Encode(MaxInt64, %q) is %d bytes, limit is %d", a, len(data), MaxLen)
		}
	}
}

// Anyone can send any callback data, so everything Encode would never
// produce must be rejected.
func TestDecodeInvalid(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"empty", ""},
		{"prefix only", "b"},
		{"prefix and colon", "b:"},
		{"no action", "b:42"},
		{"empty action", "b:42:"},
		{"empty id", "b::j"},
		{"id not a number", "b:abc:j"},
		{"zero id", "b:0:j"},
		{"negative id", "b:-1:j"},
		{"plus sign", "b:+42:j"},
		{"leading zero", "b:042:j"},
		{"space in id", "b: 42:j"},
		{"id too big for int64", "b:99999999999999999999:j"},
		{"unknown action", "b:42:z"},
		{"uppercase action", "b:42:J"},
		{"action too long", "b:42:jj"},
		{"extra part", "b:42:j:extra"},
		{"wrong prefix", "x:42:j"},
		{"uppercase prefix", "B:42:j"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, a, err := Decode(tt.data)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Decode(%q) error = %v, want ErrInvalid", tt.data, err)
			}
			// A rejected input must not leak a half-parsed result.
			if id != 0 || a != "" {
				t.Errorf("Decode(%q) = %d, %q with an error, want zero values", tt.data, id, a)
			}
		})
	}
}
