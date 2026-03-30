package styles

import "charm.land/lipgloss/v2"

// k9s-inspired raw color strings (ANSI color codes as strings).
// Use these when a plain string color is required (e.g., bubbles/progress).
const (
	ColorRedStr         = "196"
	ColorGreenStr       = "42"
	ColorYellowStr      = "214"
	ColorPrimaryStr     = "39"
	ColorSecondaryStr   = "62"
	ColorAccentStr      = "205"
	ColorGrayStr        = "244"
	ColorDarkGrayStr    = "241"
	ColorBrightWhiteStr = "255"
	ColorDarkBgStr      = "236"
)

// k9s-inspired color palette.
var (
	ColorPrimary     = lipgloss.Color(ColorPrimaryStr)     // Cyan
	ColorSecondary   = lipgloss.Color(ColorSecondaryStr)   // Blue
	ColorAccent      = lipgloss.Color(ColorAccentStr)      // Magenta
	ColorGreen       = lipgloss.Color(ColorGreenStr)
	ColorYellow      = lipgloss.Color(ColorYellowStr)
	ColorRed         = lipgloss.Color(ColorRedStr)
	ColorGray        = lipgloss.Color(ColorGrayStr)
	ColorDarkGray    = lipgloss.Color(ColorDarkGrayStr)
	ColorBrightWhite = lipgloss.Color(ColorBrightWhiteStr)
	ColorDarkBg      = lipgloss.Color(ColorDarkBgStr)

	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorAccent)

	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary)

	SelectedStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBrightWhite).
			Background(ColorDarkBg)

	StatusRunning   = lipgloss.NewStyle().Foreground(ColorGreen)
	StatusCompleted = lipgloss.NewStyle().Foreground(ColorPrimary)
	StatusFailed    = lipgloss.NewStyle().Foreground(ColorRed)
	StatusIdle      = lipgloss.NewStyle().Foreground(ColorGray)

	CircuitClosed   = lipgloss.NewStyle().Foreground(ColorGreen)
	CircuitHalfOpen = lipgloss.NewStyle().Foreground(ColorYellow)
	CircuitOpen     = lipgloss.NewStyle().Foreground(ColorRed)

	HelpStyle = lipgloss.NewStyle().Foreground(ColorDarkGray)
	InfoStyle = lipgloss.NewStyle().Foreground(ColorGray)

	BreadcrumbStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			Bold(true)

	BreadcrumbSep = lipgloss.NewStyle().
			Foreground(ColorDarkGray)

	StatusBarStyle = lipgloss.NewStyle().
			Foreground(ColorBrightWhite).
			Background(ColorDarkBg).
			Padding(0, 1)

	CommandStyle = lipgloss.NewStyle().
			Foreground(ColorAccent).
			Bold(true)

	NotificationStyle = lipgloss.NewStyle().
				Foreground(ColorBrightWhite).
				Background(lipgloss.Color("55")).
				Padding(0, 2).
				Bold(true)

	WarningStyle = lipgloss.NewStyle().
			Foreground(ColorYellow)

	// Provider styles
	ProviderClaudeStyle = lipgloss.NewStyle().Foreground(ColorPrimary) // cyan
	ProviderGeminiStyle = lipgloss.NewStyle().Foreground(ColorAccent)  // magenta
	ProviderCodexStyle  = lipgloss.NewStyle().Foreground(ColorYellow)  // yellow

	// Tab bar
	TabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorBrightWhite).
			Background(ColorDarkBg).
			Padding(0, 1)
	TabInactive = lipgloss.NewStyle().
			Foreground(ColorGray).
			Padding(0, 1)

	// Alert severity
	AlertCritical = lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
	AlertWarning  = lipgloss.NewStyle().Foreground(ColorYellow)
	AlertInfo     = lipgloss.NewStyle().Foreground(ColorGray)

	// Fleet dashboard stat boxes
	StatBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorDarkGray).
		Padding(0, 1)

	// Modal/Menu styles
	ModalBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorAccent).
			Padding(1, 2)

	ModalButtonStyle = lipgloss.NewStyle().
				Foreground(ColorGray).
				Padding(0, 2)

	ModalButtonActiveStyle = lipgloss.NewStyle().
				Foreground(ColorBrightWhite).
				Background(ColorDarkBg).
				Bold(true).
				Padding(0, 2)

	MenuStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(0, 1)

	MenuItemStyle = lipgloss.NewStyle().
			Foreground(ColorGray)

	MenuItemActiveStyle = lipgloss.NewStyle().
				Foreground(ColorBrightWhite).
				Background(ColorDarkBg).
				Bold(true)

	MenuItemDestructiveStyle = lipgloss.NewStyle().
					Foreground(ColorRed)
)

// ProviderStyle returns the appropriate style for a provider string.
func ProviderStyle(provider string) lipgloss.Style {
	switch provider {
	case "claude":
		return ProviderClaudeStyle
	case "gemini":
		return ProviderGeminiStyle
	case "codex":
		return ProviderCodexStyle
	default:
		return InfoStyle
	}
}

// AlertStyle returns the appropriate style for an alert severity.
func AlertStyle(severity string) lipgloss.Style {
	switch severity {
	case "critical":
		return AlertCritical
	case "warning":
		return AlertWarning
	default:
		return AlertInfo
	}
}

// StatusStyle returns the appropriate style for a status string.
func StatusStyle(status string) lipgloss.Style {
	switch status {
	case "running":
		return StatusRunning
	case "completed":
		return StatusCompleted
	case "failed":
		return StatusFailed
	case "idle", "stopped":
		return StatusIdle
	default:
		return InfoStyle
	}
}

// CBStyle returns the appropriate style for a circuit breaker state.
func CBStyle(state string) lipgloss.Style {
	switch state {
	case "CLOSED":
		return CircuitClosed
	case "HALF_OPEN":
		return CircuitHalfOpen
	case "OPEN":
		return CircuitOpen
	default:
		return InfoStyle
	}
}
