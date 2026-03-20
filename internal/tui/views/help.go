package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// HelpGroup is a named group of key bindings for the help overlay.
type HelpGroup struct {
	Name     string
	Bindings []key.Binding
}

// RenderHelp renders the help overlay from key binding groups.
func RenderHelp(groups []HelpGroup, width, height int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("  Ralphglasses Help"))
	b.WriteString("\n\n")

	for _, g := range groups {
		b.WriteString(styles.HeaderStyle.Render("  " + g.Name))
		b.WriteString("\n")
		for _, bind := range g.Bindings {
			h := bind.Help()
			if h.Key == "" && h.Desc == "" {
				continue
			}
			b.WriteString("    ")
			b.WriteString(styles.CommandStyle.Render(padRight(h.Key, 20)))
			b.WriteString(h.Desc)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Commands section (not key bindings — always rendered)
	b.WriteString(styles.HeaderStyle.Render("  Commands"))
	b.WriteString("\n")
	commands := [][2]string{
		{":repos", "Switch to repos tab"},
		{":sessions", "Switch to sessions tab"},
		{":teams", "Switch to teams tab"},
		{":fleet", "Switch to fleet dashboard"},
		{":start <repo>", "Start loop for repo"},
		{":stop <repo>", "Stop loop for repo"},
		{":stopall", "Stop all loops"},
		{":scan", "Re-scan for repos"},
		{":quit", "Quit"},
	}
	for _, cmd := range commands {
		b.WriteString("    ")
		b.WriteString(styles.CommandStyle.Render(padRight(cmd[0], 20)))
		b.WriteString(cmd[1])
		b.WriteString("\n")
	}
	b.WriteString("\n")

	return b.String()
}

func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}
