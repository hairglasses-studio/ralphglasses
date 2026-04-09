package views

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"github.com/charmbracelet/x/ansi"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// HelpGroup is a named group of key bindings for the help overlay.
type HelpGroup struct {
	Name     string
	Bindings []key.Binding
}

// RenderHelp renders the help overlay from key binding groups.
func RenderHelp(groups []HelpGroup, width, height int) string {
	if width <= 0 {
		width = 80
	}

	var b strings.Builder

	b.WriteString(fitHelpLine(styles.TitleStyle.Render("  " + styles.IconGlasses + " Ralphglasses Help"), width))
	b.WriteString("\n\n")

	for _, g := range groups {
		b.WriteString(fitHelpLine(styles.HeaderStyle.Render("  " + g.Name), width))
		b.WriteString("\n")
		for _, bind := range g.Bindings {
			h := bind.Help()
			if h.Key == "" && h.Desc == "" {
				continue
			}
			for _, line := range renderHelpEntry(h.Key, h.Desc, width) {
				b.WriteString(line)
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	// Commands section (not key bindings — always rendered)
	b.WriteString(fitHelpLine(styles.HeaderStyle.Render("  " + styles.IconConfig + " Commands"), width))
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
		for _, line := range renderHelpEntry(cmd[0], cmd[1], width) {
			b.WriteString(line)
			b.WriteString("\n")
		}
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

func fitHelpLine(s string, width int) string {
	if width <= 0 {
		return s
	}
	return components.VisualTruncate(s, width)
}

func renderHelpEntry(keyText, desc string, width int) []string {
	const indent = "    "

	if width <= 0 {
		width = 80
	}

	available := width - ansi.StringWidth(indent)
	if available < 16 {
		available = 16
	}

	keyWidth := min(20, max(8, available/3))
	descWidth := max(available-keyWidth, 8)

	keyCell := padRight(components.VisualTruncate(keyText, keyWidth), keyWidth)
	descLines := wrapHelpText(desc, descWidth)
	lines := make([]string, 0, len(descLines))

	for i, line := range descLines {
		if i == 0 {
			lines = append(lines, fitHelpLine(indent+styles.CommandStyle.Render(keyCell)+line, width))
			continue
		}
		lines = append(lines, fitHelpLine(indent+strings.Repeat(" ", keyWidth)+line, width))
	}

	return lines
}

func wrapHelpText(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	lines := []string{words[0]}
	for _, word := range words[1:] {
		current := lines[len(lines)-1]
		candidate := current + " " + word
		if ansi.StringWidth(candidate) <= width {
			lines[len(lines)-1] = candidate
			continue
		}
		if ansi.StringWidth(word) > width {
			lines = append(lines, components.VisualTruncate(word, width))
			continue
		}
		lines = append(lines, word)
	}

	for i, line := range lines {
		lines[i] = components.VisualTruncate(line, width)
	}
	return lines
}
