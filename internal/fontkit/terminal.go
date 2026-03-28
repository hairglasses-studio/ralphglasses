package fontkit

import "os"

// Terminal represents a detected terminal emulator.
type Terminal string

const (
	TerminalITerm2  Terminal = "iterm2"
	TerminalGhostty Terminal = "ghostty"
	TerminalWezTerm Terminal = "wezterm"
	TerminalApple   Terminal = "apple_terminal"
	TerminalUnknown Terminal = "unknown"
)

// TerminalInfo describes the detected terminal environment.
type TerminalInfo struct {
	Terminal Terminal
	Name     string // Human-readable name
	Version  string // Version if detectable
}

// DetectTerminal identifies the current terminal emulator.
func DetectTerminal() TerminalInfo {
	// iTerm2 sets TERM_PROGRAM=iTerm.app
	if tp := os.Getenv("TERM_PROGRAM"); tp != "" {
		switch tp {
		case "iTerm.app":
			return TerminalInfo{
				Terminal: TerminalITerm2,
				Name:     "iTerm2",
				Version:  os.Getenv("TERM_PROGRAM_VERSION"),
			}
		case "ghostty":
			return TerminalInfo{
				Terminal: TerminalGhostty,
				Name:     "Ghostty",
				Version:  os.Getenv("TERM_PROGRAM_VERSION"),
			}
		case "WezTerm":
			return TerminalInfo{
				Terminal: TerminalWezTerm,
				Name:     "WezTerm",
				Version:  os.Getenv("TERM_PROGRAM_VERSION"),
			}
		case "Apple_Terminal":
			return TerminalInfo{
				Terminal: TerminalApple,
				Name:     "Apple Terminal",
				Version:  os.Getenv("TERM_PROGRAM_VERSION"),
			}
		}
	}

	// Ghostty also sets __CFBundleIdentifier
	if bid := os.Getenv("__CFBundleIdentifier"); bid == "com.mitchellh.ghostty" {
		return TerminalInfo{
			Terminal: TerminalGhostty,
			Name:     "Ghostty",
		}
	}

	return TerminalInfo{
		Terminal: TerminalUnknown,
		Name:     "Unknown",
	}
}

// SupportsConfig returns whether we can write font configuration for this terminal.
func (t Terminal) SupportsConfig() bool {
	switch t {
	case TerminalITerm2, TerminalGhostty, TerminalWezTerm:
		return true
	default:
		return false
	}
}
