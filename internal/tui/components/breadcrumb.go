package components

import (
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// Breadcrumb renders a navigation path like "Overview > repo-name > Logs".
type Breadcrumb struct {
	Parts []string
}

// View renders the breadcrumb.
func (b *Breadcrumb) View() string {
	if len(b.Parts) == 0 {
		return ""
	}
	var rendered []string
	for _, p := range b.Parts {
		rendered = append(rendered, styles.BreadcrumbStyle.Render(p))
	}
	return strings.Join(rendered, styles.BreadcrumbSep.Render(" › "))
}

// Push appends a navigation level.
func (b *Breadcrumb) Push(name string) {
	b.Parts = append(b.Parts, name)
}

// Pop removes the last navigation level.
func (b *Breadcrumb) Pop() {
	if len(b.Parts) > 0 {
		b.Parts = b.Parts[:len(b.Parts)-1]
	}
}

// Reset sets the breadcrumb to just the root.
func (b *Breadcrumb) Reset() {
	if len(b.Parts) > 1 {
		b.Parts = b.Parts[:1]
	}
}
