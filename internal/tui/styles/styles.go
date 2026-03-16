package styles

import "github.com/charmbracelet/lipgloss"

// k9s-inspired color palette.
var (
	ColorPrimary    = lipgloss.Color("39")  // Cyan
	ColorSecondary  = lipgloss.Color("62")  // Blue
	ColorAccent     = lipgloss.Color("205") // Magenta
	ColorGreen      = lipgloss.Color("42")
	ColorYellow     = lipgloss.Color("214")
	ColorRed        = lipgloss.Color("196")
	ColorGray       = lipgloss.Color("244")
	ColorDarkGray   = lipgloss.Color("241")
	ColorBrightWhite = lipgloss.Color("255")
	ColorDarkBg     = lipgloss.Color("236")

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
)

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
