package views

import (
	"fmt"
	"os/exec"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

var (
	diffAddStyle    = lipgloss.NewStyle().Foreground(styles.ColorGreen)
	diffRemStyle    = lipgloss.NewStyle().Foreground(styles.ColorRed)
	diffHunkStyle   = lipgloss.NewStyle().Foreground(styles.ColorPrimary)
	diffHeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(styles.ColorBrightWhite)
)

// RenderDiffView renders a colorized git diff for a repo.
func RenderDiffView(repoPath string, fromRef string, width, height int) string {
	if fromRef == "" {
		fromRef = "HEAD~1"
	}

	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("Git Diff: %s..HEAD", fromRef)))
	b.WriteString("\n\n")

	// Stat summary
	statCmd := exec.Command("git", "diff", fromRef+"..HEAD", "--stat")
	statCmd.Dir = repoPath
	statOut, err := statCmd.Output()
	if err != nil {
		b.WriteString(styles.StatusFailed.Render(fmt.Sprintf("  git diff failed: %v", err)))
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("  Esc: back"))
		return b.String()
	}

	if len(statOut) > 0 {
		b.WriteString(styles.HeaderStyle.Render("Summary"))
		b.WriteString("\n")
		for line := range strings.SplitSeq(strings.TrimSpace(string(statOut)), "\n") {
			b.WriteString("  " + line + "\n")
		}
		b.WriteString("\n")
	}

	// Full diff
	diffCmd := exec.Command("git", "diff", fromRef+"..HEAD")
	diffCmd.Dir = repoPath
	diffOut, _ := diffCmd.Output()

	if len(diffOut) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No changes"))
		b.WriteString("\n")
	} else {
		lines := strings.Split(string(diffOut), "\n")
		maxLines := max(height-15, 20)
		if len(lines) > maxLines {
			lines = lines[:maxLines]
		}
		for _, line := range lines {
			if width > 0 && len([]rune(line)) > width-2 {
				line = string([]rune(line)[:width-2])
			}
			b.WriteString("  " + colorizeDiffLine(line) + "\n")
		}
		if len(strings.Split(string(diffOut), "\n")) > maxLines {
			b.WriteString(styles.InfoStyle.Render(fmt.Sprintf("  ... truncated (%d more lines)", len(strings.Split(string(diffOut), "\n"))-maxLines)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("  Esc: back"))

	return b.String()
}

// DiffViewport wraps RenderDiffView in a scrollable viewport.
type DiffViewport struct {
	Viewport *ViewportView
	repoPath string
	fromRef  string
	width    int
	height   int
}

// NewDiffViewport creates a new DiffViewport.
func NewDiffViewport() *DiffViewport {
	return &DiffViewport{
		Viewport: NewViewportView(),
	}
}

// SetData updates the repo path and ref, then regenerates content.
func (v *DiffViewport) SetData(repoPath, fromRef string) {
	v.repoPath = repoPath
	v.fromRef = fromRef
	v.regenerate()
}

// SetDimensions updates the available width and height.
func (v *DiffViewport) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	v.Viewport.SetDimensions(width, height)
	v.regenerate()
}

// Render returns the scrollable viewport content.
func (v *DiffViewport) Render() string {
	return v.Viewport.Render()
}

func (v *DiffViewport) regenerate() {
	if v.repoPath == "" {
		return
	}
	content := RenderDiffView(v.repoPath, v.fromRef, v.width, v.height)
	v.Viewport.SetContent(content)
}

func colorizeDiffLine(line string) string {
	switch {
	case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
		return diffHeaderStyle.Render(line)
	case strings.HasPrefix(line, "@@"):
		return diffHunkStyle.Render(line)
	case strings.HasPrefix(line, "+"):
		return diffAddStyle.Render(line)
	case strings.HasPrefix(line, "-"):
		return diffRemStyle.Render(line)
	case strings.HasPrefix(line, "diff "):
		return diffHeaderStyle.Render(line)
	default:
		return line
	}
}
