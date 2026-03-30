package headless

import (
	"testing"
)

func TestIsHeadless_NoPanic(t *testing.T) {
	// In a test runner, stdout may or may not be a terminal.
	// We just verify it doesn't panic and returns a boolean.
	_ = IsHeadless()
}

func TestIsTmuxSession_WithEnv(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")
	if !IsTmuxSession() {
		t.Error("IsTmuxSession should return true when TMUX is set")
	}
}

func TestIsTmuxSession_WithoutEnv(t *testing.T) {
	t.Setenv("TMUX", "")
	if IsTmuxSession() {
		t.Error("IsTmuxSession should return false when TMUX is empty")
	}
}

func TestIsSSH_WithSSHClient(t *testing.T) {
	t.Setenv("SSH_CLIENT", "192.168.1.1 54321 22")
	t.Setenv("SSH_TTY", "")
	if !IsSSH() {
		t.Error("IsSSH should return true when SSH_CLIENT is set")
	}
}

func TestIsSSH_WithSSHTTY(t *testing.T) {
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("SSH_TTY", "/dev/pts/0")
	if !IsSSH() {
		t.Error("IsSSH should return true when SSH_TTY is set")
	}
}

func TestIsSSH_NeitherSet(t *testing.T) {
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("SSH_TTY", "")
	if IsSSH() {
		t.Error("IsSSH should return false when neither SSH_CLIENT nor SSH_TTY is set")
	}
}

func TestContainsBytes_EdgeCases(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		haystack string
		needle   string
		want     bool
	}{
		{"both empty", "", "", true},
		{"needle longer than haystack", "ab", "abcdef", false},
		{"exact match", "abc", "abc", true},
		{"at start", "abcdef", "abc", true},
		{"at end", "abcdef", "def", true},
		{"single char found", "abc", "b", true},
		{"single char not found", "abc", "z", false},
		{"repeated pattern", "aabaa", "aa", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := containsBytes([]byte(tt.haystack), []byte(tt.needle))
			if got != tt.want {
				t.Errorf("containsBytes(%q, %q) = %v, want %v", tt.haystack, tt.needle, got, tt.want)
			}
		})
	}
}
