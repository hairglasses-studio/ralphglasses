// Package tui provides a BubbleTea interactive terminal dashboard.
package tui

import "github.com/charmbracelet/lipgloss"

// Color palette
var (
	colorPrimary   = lipgloss.Color("#7C3AED") // purple
	colorSecondary = lipgloss.Color("#3B82F6") // blue
	colorSuccess   = lipgloss.Color("#10B981") // green
	colorWarning   = lipgloss.Color("#F59E0B") // amber
	colorDanger    = lipgloss.Color("#EF4444") // red
	colorMuted     = lipgloss.Color("#6B7280") // gray
	colorBg        = lipgloss.Color("#1F2937") // dark bg

	// Chart / visualization colors
	colorSeries1 = lipgloss.Color("#8B5CF6") // violet
	colorSeries2 = lipgloss.Color("#06B6D4") // cyan
	colorAccent  = lipgloss.Color("#EC4899") // pink
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSecondary)

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMuted).
			Padding(0, 1).
			MarginBottom(1)

	alertStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorDanger)

	successStyle = lipgloss.NewStyle().
			Foreground(colorSuccess)

	warningStyle = lipgloss.NewStyle().
			Foreground(colorWarning)

	mutedStyle = lipgloss.NewStyle().
			Foreground(colorMuted)

	tabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(colorPrimary).
			Padding(0, 2)

	tabInactiveStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Padding(0, 2)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)
)

// progressBar renders a simple text-based progress bar.
func progressBar(current, total int, width int) string {
	if total == 0 {
		return mutedStyle.Render("—")
	}
	pct := float64(current) / float64(total)
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(width))
	empty := width - filled

	bar := ""
	for i := 0; i < filled; i++ {
		bar += "█"
	}
	for i := 0; i < empty; i++ {
		bar += "░"
	}

	style := successStyle
	if pct > 0.9 {
		style = alertStyle
	} else if pct > 0.7 {
		style = warningStyle
	}
	return style.Render(bar)
}
