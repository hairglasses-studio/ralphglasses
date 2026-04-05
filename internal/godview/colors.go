package godview

// ANSI color codes for provider-specific coloring.
// Matches Snazzy palette from dotfiles.
const (
	Reset   = "\033[0m"
	Bold    = "\033[1m"
	Dim     = "\033[2m"
	Italic  = "\033[3m"
	Reverse = "\033[7m"

	// Provider colors (Snazzy palette)
	Claude  = "\033[36m" // Cyan (#57c7ff)
	Gemini  = "\033[32m" // Green (#5af78e)
	Codex   = "\033[33m" // Yellow (#f3f99d)
	Unknown = "\033[37m" // White (#f1f1f0)

	// Status colors
	StatusOK   = "\033[32m" // Green
	StatusWarn  = "\033[33m" // Yellow
	StatusErr   = "\033[31m" // Red (#ff5c57)
	StatusIdle  = "\033[90m" // Dim gray
	StatusDone  = "\033[35m" // Magenta (#ff6ac1)

	// UI elements
	Header  = "\033[1;36m" // Bold cyan
	Border  = "\033[90m"   // Dim gray
	CostHi  = "\033[1;31m" // Bold red (high cost)
	CostMid = "\033[33m"   // Yellow (moderate)
	CostLo  = "\033[32m"   // Green (low)

	// Cursor control
	CursorHome = "\033[H"
	CursorHide = "\033[?25l"
	CursorShow = "\033[?25h"
	ClearLine  = "\033[K"
	ClearDown  = "\033[J"
)

// ProviderColor returns the ANSI color for a provider name.
func ProviderColor(provider string) string {
	switch provider {
	case "claude":
		return Claude
	case "gemini":
		return Gemini
	case "codex":
		return Codex
	default:
		return Unknown
	}
}

// StatusColor returns the ANSI color for a status string.
func StatusColor(status string) string {
	switch status {
	case "running", "ok":
		return StatusOK
	case "warn", "degraded":
		return StatusWarn
	case "error", "failed", "errored":
		return StatusErr
	case "idle", "pending", "unknown":
		return StatusIdle
	case "completed", "done", "converged":
		return StatusDone
	default:
		return Reset
	}
}
