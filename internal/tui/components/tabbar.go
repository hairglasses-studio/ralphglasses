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
	return strings.Join(tabs, " ")
}
