package process

import (
	"testing"
	"time"
)

func TestWatcherCompletionPatterns(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		matches bool
	}{
		{"session complete lowercase", "session complete", true},
		{"session complete mixed case", "Session Complete!", true},
		{"completed keyword", "Task completed successfully", true},
		{"checkmark unicode", "All done \u2713", true},
		{"shell prompt dollar", "$  ", true},
		{"shell prompt chevron", ">  ", true},
		{"exit code numeric", "exit code: 0", true},
		{"exit code non-zero", "Exit Code: 1", true},
		{"exited with", "Process exited with status 0", true},
		{"no match random text", "compiling main.go...", false},
		{"no match partial", "incompletely done", false},
		{"no match exit substring", "next iteration", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := false
			for _, pat := range completionPatterns {
				if pat.MatchString(tt.line) {
					matched = true
					break
				}
			}
			if matched != tt.matches {
				t.Errorf("line %q: matched=%v, want %v", tt.line, matched, tt.matches)
			}
		})
	}
}

func TestWatcherWaitForSessionCompleteNonExistentSession(t *testing.T) {
	// WaitForSessionComplete should return an error quickly for a session
	// that does not exist (tmux capture-pane will fail).
	_, err := WaitForSessionComplete("ralphglasses-test-nonexistent-session-xyz", 2*time.Second)
	if err == nil {
		t.Fatal("expected error for non-existent tmux session, got nil")
	}
}
