package styles

// Nerd Font icon constants grouped by purpose.
// Requires a Nerd Font patched terminal font for correct rendering.
const (
	// Providers
	IconClaude = "\uf544" // nf-md-robot
	IconGemini = "\uf219" // nf-fa-diamond
	IconCodex  = "\ue795" // nf-dev-terminal

	// Status
	IconRunning    = "\uf144" // nf-fa-play_circle
	IconCompleted  = "\uf058" // nf-fa-check_circle
	IconErrored    = "\uf071" // nf-fa-exclamation_triangle
	IconStopped    = "\uf28d" // nf-fa-stop_circle
	IconPaused     = "\uf28b" // nf-fa-pause_circle
	IconLaunching  = "\uf135" // nf-fa-rocket
	IconIdle       = "\uf192" // nf-fa-dot_circle_o

	// Circuit breaker
	IconCBClosed   = "\uf132" // nf-fa-shield
	IconCBHalfOpen = "\uf3ed" // nf-md-shield_half_full
	IconCBOpen     = "\uf0e7" // nf-fa-bolt

	// Navigation / labels
	IconRepo       = "\ue725" // nf-dev-git_branch
	IconSession    = "\uf489" // nf-oct-terminal
	IconTeam       = "\uf0c0" // nf-fa-users
	IconFleet      = "\uf00a" // nf-fa-th
	IconBudget     = "\uf155" // nf-fa-dollar
	IconCost       = "\uf201" // nf-fa-line_chart
	IconTurns      = "\uf021" // nf-fa-refresh
	IconClock      = "\uf017" // nf-fa-clock_o
	IconAlert      = "\uf0f3" // nf-fa-bell
	IconAgent      = "\uf544" // nf-md-robot (same as claude)
	IconConfig     = "\uf013" // nf-fa-cog
	IconLog        = "\uf15c" // nf-fa-file_text_o

	// Severity
	IconCritical = "\uf7ba" // nf-md-radioactive
	IconWarning  = "\uf071" // nf-fa-exclamation_triangle
	IconInfo     = "\uf05a" // nf-fa-info_circle

	// Title
	IconGlasses = "\uf530" // nf-md-glasses
)

// StatusIcon returns a colored icon for a status string.
func StatusIcon(status string) string {
	switch status {
	case "running":
		return StatusRunning.Render(IconRunning)
	case "completed":
		return StatusCompleted.Render(IconCompleted)
	case "failed", "errored":
		return StatusFailed.Render(IconErrored)
	case "stopped":
		return StatusIdle.Render(IconStopped)
	case "paused":
		return WarningStyle.Render(IconPaused)
	case "launching":
		return WarningStyle.Render(IconLaunching)
	case "idle":
		return StatusIdle.Render(IconIdle)
	default:
		return InfoStyle.Render(IconIdle)
	}
}

// ProviderIcon returns a colored icon for a provider string.
func ProviderIcon(provider string) string {
	switch provider {
	case "claude":
		return ProviderClaudeStyle.Render(IconClaude)
	case "gemini":
		return ProviderGeminiStyle.Render(IconGemini)
	case "codex":
		return ProviderCodexStyle.Render(IconCodex)
	default:
		return InfoStyle.Render(IconSession)
	}
}

// CBIcon returns a colored icon for a circuit breaker state.
func CBIcon(state string) string {
	switch state {
	case "CLOSED":
		return CircuitClosed.Render(IconCBClosed)
	case "HALF_OPEN":
		return CircuitHalfOpen.Render(IconCBHalfOpen)
	case "OPEN":
		return CircuitOpen.Render(IconCBOpen)
	default:
		return InfoStyle.Render("-")
	}
}

// AlertIcon returns a colored icon for an alert severity.
func AlertIcon(severity string) string {
	switch severity {
	case "critical":
		return AlertCritical.Render(IconCritical)
	case "warning":
		return AlertWarning.Render(IconWarning)
	default:
		return AlertInfo.Render(IconInfo)
	}
}
