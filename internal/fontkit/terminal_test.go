package fontkit

import "testing"

func TestTerminalSupportsConfig(t *testing.T) {
	tests := []struct {
		term    Terminal
		support bool
	}{
		{TerminalITerm2, true},
		{TerminalGhostty, true},
		{TerminalWezTerm, true},
		{TerminalApple, false},
		{TerminalUnknown, false},
	}
	for _, tt := range tests {
		if got := tt.term.SupportsConfig(); got != tt.support {
			t.Errorf("%s.SupportsConfig() = %v, want %v", tt.term, got, tt.support)
		}
	}
}

func TestDetectTerminal(t *testing.T) {
	// Just verify it doesn't panic and returns a valid terminal
	info := DetectTerminal()
	if info.Name == "" {
		t.Error("DetectTerminal().Name should not be empty")
	}
}
