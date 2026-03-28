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

func TestDetectTerminal_ITerm2(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "iTerm.app")
	t.Setenv("TERM_PROGRAM_VERSION", "3.5.0")

	info := DetectTerminal()
	if info.Terminal != TerminalITerm2 {
		t.Errorf("Terminal = %q, want %q", info.Terminal, TerminalITerm2)
	}
	if info.Name != "iTerm2" {
		t.Errorf("Name = %q, want %q", info.Name, "iTerm2")
	}
	if info.Version != "3.5.0" {
		t.Errorf("Version = %q, want %q", info.Version, "3.5.0")
	}
}

func TestDetectTerminal_Ghostty(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "ghostty")
	t.Setenv("TERM_PROGRAM_VERSION", "1.0.0")

	info := DetectTerminal()
	if info.Terminal != TerminalGhostty {
		t.Errorf("Terminal = %q, want %q", info.Terminal, TerminalGhostty)
	}
	if info.Name != "Ghostty" {
		t.Errorf("Name = %q, want %q", info.Name, "Ghostty")
	}
}

func TestDetectTerminal_WezTerm(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "WezTerm")
	t.Setenv("TERM_PROGRAM_VERSION", "20240101")

	info := DetectTerminal()
	if info.Terminal != TerminalWezTerm {
		t.Errorf("Terminal = %q, want %q", info.Terminal, TerminalWezTerm)
	}
	if info.Name != "WezTerm" {
		t.Errorf("Name = %q, want %q", info.Name, "WezTerm")
	}
}

func TestDetectTerminal_AppleTerminal(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "Apple_Terminal")
	t.Setenv("TERM_PROGRAM_VERSION", "450")

	info := DetectTerminal()
	if info.Terminal != TerminalApple {
		t.Errorf("Terminal = %q, want %q", info.Terminal, TerminalApple)
	}
	if info.Name != "Apple Terminal" {
		t.Errorf("Name = %q, want %q", info.Name, "Apple Terminal")
	}
}

func TestDetectTerminal_GhosttyViaBundleID(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("__CFBundleIdentifier", "com.mitchellh.ghostty")

	info := DetectTerminal()
	if info.Terminal != TerminalGhostty {
		t.Errorf("Terminal = %q, want %q", info.Terminal, TerminalGhostty)
	}
	if info.Name != "Ghostty" {
		t.Errorf("Name = %q, want %q", info.Name, "Ghostty")
	}
}

func TestDetectTerminal_Unknown(t *testing.T) {
	t.Setenv("TERM_PROGRAM", "")
	t.Setenv("__CFBundleIdentifier", "")

	info := DetectTerminal()
	if info.Terminal != TerminalUnknown {
		t.Errorf("Terminal = %q, want %q", info.Terminal, TerminalUnknown)
	}
	if info.Name != "Unknown" {
		t.Errorf("Name = %q, want %q", info.Name, "Unknown")
	}
}
