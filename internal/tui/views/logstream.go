package views

import (
	"fmt"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// LogView is a scrollable log viewer with follow mode.
type LogView struct {
	Lines    []string
	Offset   int
	Height   int
	Width    int
	Follow   bool
	Search   string
}

// NewLogView creates a log view.
func NewLogView() *LogView {
	return &LogView{Follow: true}
}

// AppendLines adds new log lines and auto-scrolls if following.
func (lv *LogView) AppendLines(lines []string) {
	lv.Lines = append(lv.Lines, lines...)
	if lv.Follow {
		lv.ScrollToEnd()
	}
}

// SetLines replaces all lines and scrolls to end.
func (lv *LogView) SetLines(lines []string) {
	lv.Lines = lines
	if lv.Follow {
		lv.ScrollToEnd()
	}
}

// ScrollUp moves up one line.
func (lv *LogView) ScrollUp() {
	lv.Follow = false
	if lv.Offset > 0 {
		lv.Offset--
	}
}

// ScrollDown moves down one line.
func (lv *LogView) ScrollDown() {
	if lv.Offset < len(lv.Lines)-lv.visibleLines() {
		lv.Offset++
	}
	if lv.Offset >= len(lv.Lines)-lv.visibleLines() {
		lv.Follow = true
	}
}

// ScrollToEnd jumps to the end.
func (lv *LogView) ScrollToEnd() {
	vis := lv.visibleLines()
	if len(lv.Lines) > vis {
		lv.Offset = len(lv.Lines) - vis
	} else {
		lv.Offset = 0
	}
	lv.Follow = true
}

// ScrollToStart jumps to the beginning.
func (lv *LogView) ScrollToStart() {
	lv.Offset = 0
	lv.Follow = false
}

// PageUp scrolls up by half a page.
func (lv *LogView) PageUp() {
	lv.Follow = false
	lv.Offset -= lv.visibleLines() / 2
	if lv.Offset < 0 {
		lv.Offset = 0
	}
}

// PageDown scrolls down by half a page.
func (lv *LogView) PageDown() {
	lv.Offset += lv.visibleLines() / 2
	max := len(lv.Lines) - lv.visibleLines()
	if max < 0 {
		max = 0
	}
	if lv.Offset >= max {
		lv.Offset = max
		lv.Follow = true
	}
}

func (lv *LogView) visibleLines() int {
	if lv.Height <= 3 {
		return 20
	}
	return lv.Height - 3
}

// ToggleFollow toggles follow mode.
func (lv *LogView) ToggleFollow() {
	lv.Follow = !lv.Follow
	if lv.Follow {
		lv.ScrollToEnd()
	}
}

// View renders the log view.
func (lv *LogView) View() string {
	var b strings.Builder

	// Header
	followIndicator := styles.InfoStyle.Render("follow: off")
	if lv.Follow {
		followIndicator = styles.StatusRunning.Render("follow: on")
	}
	b.WriteString(fmt.Sprintf("  Lines: %d  Offset: %d  %s\n",
		len(lv.Lines), lv.Offset, followIndicator))
	b.WriteString(styles.InfoStyle.Render(strings.Repeat("─", lv.Width)))
	b.WriteRune('\n')

	vis := lv.visibleLines()
	end := lv.Offset + vis
	if end > len(lv.Lines) {
		end = len(lv.Lines)
	}

	for i := lv.Offset; i < end; i++ {
		line := lv.Lines[i]
		if lv.Width > 0 && len([]rune(line)) > lv.Width {
			line = string([]rune(line)[:lv.Width])
		}
		b.WriteString(line)
		b.WriteRune('\n')
	}

	// Pad remaining lines
	rendered := end - lv.Offset
	for i := rendered; i < vis; i++ {
		b.WriteRune('\n')
	}

	b.WriteString(styles.HelpStyle.Render("  j/k: scroll  G/g: end/start  f: follow  Esc: back"))

	return b.String()
}
