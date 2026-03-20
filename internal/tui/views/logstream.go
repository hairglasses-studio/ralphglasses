package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// LogView is a scrollable log viewer with follow mode, backed by bubbles/viewport.
type LogView struct {
	vp     viewport.Model
	Lines  []string
	Follow bool
	Search string
	Width  int
	Height int
}

// NewLogView creates a log view.
func NewLogView() *LogView {
	vp := viewport.New(0, 0)
	// Disable built-in key bindings — we handle keys ourselves.
	vp.KeyMap = viewport.KeyMap{}
	return &LogView{
		vp:     vp,
		Follow: true,
	}
}

// SetDimensions updates the viewport dimensions.
func (lv *LogView) SetDimensions(width, height int) {
	lv.Width = width
	lv.Height = height
	vpHeight := height - 3
	if vpHeight < 1 {
		vpHeight = 1
	}
	lv.vp.Width = width
	lv.vp.Height = vpHeight
}

// AppendLines adds new log lines and auto-scrolls if following.
func (lv *LogView) AppendLines(lines []string) {
	lv.Lines = append(lv.Lines, lines...)
	lv.rebuildContent()
}

// SetLines replaces all lines.
func (lv *LogView) SetLines(lines []string) {
	lv.Lines = lines
	lv.rebuildContent()
}

func (lv *LogView) rebuildContent() {
	lines := lv.filteredLines()
	var b strings.Builder
	for i, line := range lines {
		if lv.Width > 0 && len([]rune(line)) > lv.Width {
			line = string([]rune(line)[:lv.Width])
		}
		b.WriteString(colorizeLine(line))
		if i < len(lines)-1 {
			b.WriteRune('\n')
		}
	}
	lv.vp.SetContent(b.String())
	if lv.Follow {
		lv.vp.GotoBottom()
	}
}

// colorizeLine applies color based on log level keywords.
func colorizeLine(line string) string {
	upper := strings.ToUpper(line)
	switch {
	case strings.Contains(upper, "ERROR") || strings.Contains(upper, "FATAL"):
		return styles.StatusFailed.Render(line)
	case strings.Contains(upper, "WARN"):
		return styles.WarningStyle.Render(line)
	case strings.Contains(upper, "INFO"):
		return styles.StatusCompleted.Render(line)
	case strings.Contains(upper, "DEBUG"):
		return styles.InfoStyle.Render(line)
	default:
		return line
	}
}

// ScrollUp moves up one line.
func (lv *LogView) ScrollUp() {
	lv.Follow = false
	lv.vp.LineUp(1)
}

// ScrollDown moves down one line.
func (lv *LogView) ScrollDown() {
	lv.vp.LineDown(1)
	lv.Follow = lv.vp.AtBottom()
}

// ScrollToEnd jumps to the end.
func (lv *LogView) ScrollToEnd() {
	lv.vp.GotoBottom()
	lv.Follow = true
}

// ScrollToStart jumps to the beginning.
func (lv *LogView) ScrollToStart() {
	lv.vp.GotoTop()
	lv.Follow = false
}

// PageUp scrolls up by half a page.
func (lv *LogView) PageUp() {
	lv.Follow = false
	lv.vp.HalfViewUp()
}

// PageDown scrolls down by half a page.
func (lv *LogView) PageDown() {
	lv.vp.HalfViewDown()
	lv.Follow = lv.vp.AtBottom()
}

// ToggleFollow toggles follow mode.
func (lv *LogView) ToggleFollow() {
	lv.Follow = !lv.Follow
	if lv.Follow {
		lv.vp.GotoBottom()
	}
}

// filteredLines returns lines matching the search filter, or all lines if no search is set.
func (lv *LogView) filteredLines() []string {
	if lv.Search == "" {
		return lv.Lines
	}
	needle := strings.ToLower(lv.Search)
	var filtered []string
	for _, line := range lv.Lines {
		if strings.Contains(strings.ToLower(line), needle) {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

// View renders the log view.
func (lv *LogView) View() string {
	// Sync viewport dimensions from Width/Height fields.
	vpHeight := lv.Height - 3
	if vpHeight < 1 {
		vpHeight = 1
	}
	lv.vp.Width = lv.Width
	lv.vp.Height = vpHeight

	var b strings.Builder

	lines := lv.filteredLines()

	// Header
	followIndicator := styles.InfoStyle.Render("follow: off")
	if lv.Follow {
		followIndicator = styles.StatusRunning.Render("follow: on")
	}
	header := fmt.Sprintf("  Lines: %d  Scroll: %.0f%%  %s",
		len(lines), lv.vp.ScrollPercent()*100, followIndicator)
	if lv.Search != "" {
		header += fmt.Sprintf("  Search: %q", lv.Search)
	}
	b.WriteString(header)
	b.WriteRune('\n')
	b.WriteString(styles.InfoStyle.Render(strings.Repeat("─", lv.Width)))
	b.WriteRune('\n')

	// Viewport content
	b.WriteString(lv.vp.View())

	b.WriteRune('\n')
	b.WriteString(styles.HelpStyle.Render("  j/k: scroll  G/g: end/start  f: follow  ctrl+u/d: page  Esc: back"))

	return b.String()
}
