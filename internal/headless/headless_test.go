package headless

import (
	"testing"
)

func TestContainsBytes(t *testing.T) {
	tests := []struct {
		haystack string
		needle   string
		want     bool
	}{
		{"hello world", "world", true},
		{"hello world", "mars", false},
		{"", "a", false},
		{"a", "", true},
		{"Microsoft WSL", "WSL", true},
		{"Linux version 5.15 microsoft-standard", "microsoft", true},
	}
	for _, tt := range tests {
		got := containsBytes([]byte(tt.haystack), []byte(tt.needle))
		if got != tt.want {
			t.Errorf("containsBytes(%q, %q) = %v, want %v", tt.haystack, tt.needle, got, tt.want)
		}
	}
}

func TestIsWSL(t *testing.T) {
	// Just ensure it doesn't panic; actual result depends on platform
	_ = IsWSL()
}

func TestIsSSH(t *testing.T) {
	_ = IsSSH()
}

func TestIsTmuxSession(t *testing.T) {
	_ = IsTmuxSession()
}
