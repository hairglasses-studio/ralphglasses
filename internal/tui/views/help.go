package views

import (
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// RenderHelp renders the help overlay.
func RenderHelp(width, height int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("  Ralphglasses Help"))
	b.WriteString("\n\n")

	sections := []struct {
		title string
		binds [][2]string
	}{
		{
			"Global",
			[][2]string{
				{"q / Ctrl+C", "Quit"},
				{":", "Command mode"},
				{"/", "Filter mode"},
				{"?", "Toggle help"},
				{"Esc", "Back / cancel"},
				{"r", "Refresh"},
			},
		},
		{
			"Overview Table",
			[][2]string{
				{"j / k", "Navigate down / up"},
				{"Enter", "Drill into repo"},
				{"s", "Cycle sort column"},
				{"S", "Start loop"},
				{"X", "Stop loop"},
				{"P", "Pause / resume loop"},
			},
		},
		{
			"Repo Detail",
			[][2]string{
				{"Enter", "View logs"},
				{"e", "Edit config"},
				{"S", "Start loop"},
				{"X", "Stop loop"},
				{"P", "Pause / resume"},
			},
		},
		{
			"Log Viewer",
			[][2]string{
				{"j / k", "Scroll down / up"},
				{"G", "Jump to end"},
				{"g", "Jump to start"},
				{"f", "Toggle follow mode"},
				{"Ctrl+U / Ctrl+D", "Page up / down"},
			},
		},
		{
			"Commands",
			[][2]string{
				{":start <repo>", "Start loop for repo"},
				{":stop <repo>", "Stop loop for repo"},
				{":stopall", "Stop all loops"},
				{":scan", "Re-scan for repos"},
				{":quit", "Quit"},
			},
		},
	}

	for _, sec := range sections {
		b.WriteString(styles.HeaderStyle.Render("  " + sec.title))
		b.WriteString("\n")
		for _, bind := range sec.binds {
			b.WriteString("    ")
			b.WriteString(styles.CommandStyle.Render(padRight(bind[0], 20)))
			b.WriteString(bind[1])
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
