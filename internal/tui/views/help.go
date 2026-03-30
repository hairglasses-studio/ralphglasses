package views

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/x/ansi"
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

	b.WriteString(styles.TitleStyle.Render("  " + styles.IconGlasses + " Ralphglasses Help"))
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
	b.WriteString(styles.HeaderStyle.Render("  " + styles.IconConfig + " Commands"))
	b.WriteString("\n")
	commands := [][2]string{
		{":repos", "Switch to repos tab"},
		{":sessions", "Switch to sessions tab"},
		{":teams", "Switch to teams tab"},
		{":fleet", "Switch to fleet dashboard"},
		{":start <repo>", "Start loop for repo"},
		{":launch <repo>", "Launch session for repo"},
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

// HelpView wraps RenderHelp in a scrollable viewport.
type HelpView struct {
	Viewport *ViewportView
	groups   []HelpGroup
	width    int
	height   int
}

// NewHelpView creates a new HelpView.
func NewHelpView() *HelpView {
	return &HelpView{
		Viewport: NewViewportView(),
	}
}

// SetData updates the help groups and regenerates content.
func (v *HelpView) SetData(groups []HelpGroup) {
	v.groups = groups
	v.regenerate()
}

// SetDimensions updates the available width and height.
func (v *HelpView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	v.Viewport.SetDimensions(width, height)
	v.regenerate()
}

// Render returns the scrollable viewport content.
func (v *HelpView) Render() string {
	return v.Viewport.Render()
}

func (v *HelpView) regenerate() {
	if v.groups == nil {
		return
	}
	content := RenderHelp(v.groups, v.width, v.height)
	v.Viewport.SetContent(content)
}

func padRight(s string, n int) string {
	w := ansi.StringWidth(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}
