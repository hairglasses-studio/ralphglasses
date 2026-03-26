package components

import "testing"

func TestVisualTruncate(t *testing.T) {
	tests := []struct {
		name string
		s    string
		maxW int
		want string
	}{
		{"zero width", "hello", 0, ""},
		{"negative width", "hello", -1, ""},
		{"fits", "hi", 10, "hi"},
		{"exact fit", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello"},
		{"empty string", "", 5, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VisualTruncate(tt.s, tt.maxW)
			if got != tt.want {
				t.Errorf("VisualTruncate(%q, %d) = %q, want %q", tt.s, tt.maxW, got, tt.want)
			}
		})
	}
}
