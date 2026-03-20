package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{"tilde slash", "~/foo", filepath.Join(home, "foo")},
		{"absolute path", "/abs/path", "/abs/path"},
		{"empty", "", ""},
		{"tilde alone", "~", "~"},
		{"tilde user", "~user/foo", "~user/foo"},
		{"relative", "relative/path", "relative/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExpandHome(tt.path)
			if got != tt.want {
				t.Errorf("ExpandHome(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
