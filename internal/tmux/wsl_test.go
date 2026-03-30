package tmux

import (
	"testing"
)

func TestIsWSLPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/mnt/c/Users/foo", true},
		{"/mnt/d/projects", true},
		{"/home/user/code", false},
		{"/tmp/test", false},
	}
	for _, tt := range tests {
		if got := IsWSLPath(tt.path); got != tt.want {
			t.Errorf("IsWSLPath(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestToWindowsPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/mnt/c/Users/foo", "C:\\Users\\foo"},
		{"/mnt/d/projects/bar", "D:\\projects\\bar"},
		{"/home/user/code", "/home/user/code"}, // not a WSL path
	}
	for _, tt := range tests {
		if got := ToWindowsPath(tt.input); got != tt.want {
			t.Errorf("ToWindowsPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestToWSLPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"C:\\Users\\foo", "/mnt/c/Users/foo"},
		{"D:\\projects\\bar", "/mnt/d/projects/bar"},
		{"/home/user/code", "/home/user/code"}, // not a Windows path
	}
	for _, tt := range tests {
		if got := ToWSLPath(tt.input); got != tt.want {
			t.Errorf("ToWSLPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestWSLTmuxSocketPath(t *testing.T) {
	// Just verify it doesn't panic
	_ = WSLTmuxSocketPath()
}
