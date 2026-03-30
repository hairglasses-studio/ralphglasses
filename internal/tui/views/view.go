package views

import (
	"charm.land/bubbles/v2/viewport"
)

// View is the interface that all TUI views implement.
// It enables incremental migration — views can be converted one at a time.
type View interface {
	// Render returns the view's content as a string.
	Render() string
	// SetDimensions updates the available width and height.
	SetDimensions(width, height int)
}

// ViewportView wraps content in a scrollable viewport.
// Use it to add scrolling to views that may exceed terminal height.
type ViewportView struct {
	vp      viewport.Model
	content string
	width   int
	height  int
}

// NewViewportView creates a new ViewportView with disabled built-in key bindings.
func NewViewportView() *ViewportView {
	vp := viewport.New()
	// Disable built-in key bindings — we handle keys ourselves.
	vp.KeyMap = viewport.KeyMap{}
	return &ViewportView{vp: vp}
}

// SetContent updates the viewport's content string.
func (v *ViewportView) SetContent(s string) {
	v.content = s
	v.vp.SetContent(s)
}

// SetDimensions updates the available width and height for the viewport.
func (v *ViewportView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	if height < 1 {
		height = 1
	}
	v.vp.SetWidth(width)
	v.vp.SetHeight(height)
}

// Render returns the viewport's rendered content.
func (v *ViewportView) Render() string {
	return v.vp.View()
}

// ScrollUp moves the viewport up one line.
func (v *ViewportView) ScrollUp() {
	v.vp.ScrollUp(1)
}

// ScrollDown moves the viewport down one line.
func (v *ViewportView) ScrollDown() {
	v.vp.ScrollDown(1)
}

// PageUp scrolls up by half a page.
func (v *ViewportView) PageUp() {
	v.vp.HalfPageUp()
}

// PageDown scrolls down by half a page.
func (v *ViewportView) PageDown() {
	v.vp.HalfPageDown()
}

// GotoTop scrolls to the top of the content.
func (v *ViewportView) GotoTop() {
	v.vp.GotoTop()
}

// GotoBottom scrolls to the bottom of the content.
func (v *ViewportView) GotoBottom() {
	v.vp.GotoBottom()
}

// AtBottom returns true if the viewport is scrolled to the bottom.
func (v *ViewportView) AtBottom() bool {
	return v.vp.AtBottom()
}

// ScrollPercent returns the viewport's scroll position as a percentage (0.0–1.0).
func (v *ViewportView) ScrollPercent() float64 {
	return v.vp.ScrollPercent()
}
