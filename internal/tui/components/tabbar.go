package components

import (
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// TabBar is a horizontal tab bar component.
type TabBar struct {
	Tabs   []string
	Active int
	Width  int
}

// View renders the tab bar.
func (t *TabBar) View() string {
	var tabs []string
	for i, name := range t.Tabs {
		if i == t.Active {
			tabs = append(tabs, styles.TabActive.Render(name))
		} else {
			tabs = append(tabs, styles.TabInactive.Render(name))
		}
	}
	line := strings.Join(tabs, " ")
	if t.Width > 0 {
		return VisualTruncate(line, t.Width)
	}
	return line
}

// HandleMouse processes a mouse event for the tab bar.
// On left-click, it determines which tab was clicked based on X position.
// Returns the tab index and true if a tab was clicked, or (-1, false) otherwise.
// Both TabActive and TabInactive styles use Padding(0, 1), adding 1 char on each side.
func (t *TabBar) HandleMouse(x, y int, button, action int) (int, bool) {
	// Only handle left-click press events.
	if button != 1 || action != 0 {
		return -1, false
	}

	// Only respond on the tab bar row (y == 0 relative to the tab bar).
	if y != 0 {
		return -1, false
	}

	if len(t.Tabs) == 0 {
		return -1, false
	}

	// Calculate tab positions. Each tab is rendered with Padding(0,1) = +2 chars,
	// and tabs are joined with " " (1 space separator).
	pos := 0
	for i, name := range t.Tabs {
		tabWidth := len(name) + 2 // padding: 1 left + 1 right
		if x >= pos && x < pos+tabWidth {
			return i, true
		}
		pos += tabWidth + 1 // +1 for the space separator
	}

	return -1, false
}
