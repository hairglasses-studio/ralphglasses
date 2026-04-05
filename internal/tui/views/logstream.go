package views

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// DefaultLogCapacity is the default number of lines a LogView retains.
const DefaultLogCapacity = 10_000

// LogView is a scrollable log viewer with follow mode, backed by bubbles/viewport.
type LogView struct {
	vp     viewport.Model
	ring   *lineRing
	Follow bool
	Search string
	Width  int
	Height int
}

// Lines returns a snapshot of all stored lines, oldest first.
// Callers that previously read lv.Lines directly should use this instead.
func (lv *LogView) Lines() []string {
	return lv.ring.slice()
}

// Len returns the number of stored lines.
func (lv *LogView) Len() int {
	return lv.ring.len()
}

// NewLogView creates a log view with the default line capacity.
func NewLogView() *LogView {
	return NewLogViewWithCapacity(DefaultLogCapacity)
}

// NewLogViewWithCapacity creates a log view with a custom line capacity.
// A capacity of 0 uses DefaultLogCapacity.
func NewLogViewWithCapacity(cap int) *LogView {
	if cap <= 0 {
		cap = DefaultLogCapacity
	}
	vp := viewport.New()
	// Disable built-in key bindings — we handle keys ourselves.
	vp.KeyMap = viewport.KeyMap{}
	return &LogView{
		vp:     vp,
		ring:   newLineRing(cap),
		Follow: true,
	}
}

// SetDimensions updates the viewport dimensions.
func (lv *LogView) SetDimensions(width, height int) {
	lv.Width = width
	lv.Height = height
	vpHeight := max(height-3, 1)
	lv.vp.SetWidth(width)
	lv.vp.SetHeight(vpHeight)
}

// AppendLines adds new log lines and auto-scrolls if following.
func (lv *LogView) AppendLines(lines []string) {
	before := lv.ring.evicted
	lv.ring.pushAll(lines)
	evictedThisBatch := lv.ring.evicted - before
	lv.rebuildContent()
	if !lv.Follow && evictedThisBatch > 0 {
		// Compensate for dropped lines so the user's scroll position stays stable.
		lv.vp.ScrollUp(evictedThisBatch)
	}
}

// SetLines replaces all lines.
func (lv *LogView) SetLines(lines []string) {
	lv.ring.reset()
	lv.ring.pushAll(lines)
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
	lv.vp.ScrollUp(1)
}

// ScrollDown moves down one line.
func (lv *LogView) ScrollDown() {
	lv.vp.ScrollDown(1)
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
	lv.vp.HalfPageUp()
}

// PageDown scrolls down by half a page.
func (lv *LogView) PageDown() {
	lv.vp.HalfPageDown()
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
	all := lv.ring.slice()
	if lv.Search == "" {
		return all
	}
	needle := strings.ToLower(lv.Search)
	var filtered []string
	for _, line := range all {
		if strings.Contains(strings.ToLower(line), needle) {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

// View renders the log view.
func (lv *LogView) View() string {
	// Sync viewport dimensions from Width/Height fields.
	vpHeight := max(lv.Height-3, 1)
	lv.vp.SetWidth(lv.Width)
	lv.vp.SetHeight(vpHeight)

	var b strings.Builder

	// Header
	followIndicator := styles.InfoStyle.Render("follow: off")
	if lv.Follow {
		followIndicator = styles.StatusRunning.Render("follow: on")
	}
	lineCount := lv.ring.len()
	capSuffix := ""
	if lv.ring.isFull() {
		capSuffix = fmt.Sprintf("/%d (capped)", lv.ring.cap)
	}
	header := fmt.Sprintf("  Lines: %d%s  Scroll: %.0f%%  %s",
		lineCount, capSuffix, lv.vp.ScrollPercent()*100, followIndicator)
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
