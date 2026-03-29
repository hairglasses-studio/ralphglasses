package views

import (
	tea "github.com/charmbracelet/bubbletea"
)

// ViewHandler defines the interface all views must implement for the view registry.
// Views that implement this interface can be dispatched via the registry instead of
// the monolithic switch statement in app.go View() and app_update.go handleKey().
type ViewHandler interface {
	// Render returns the view's content string for the given dimensions.
	Render(width, height int) string
	// HandleKey processes view-specific key events. Returns true if handled.
	HandleKey(msg tea.KeyMsg) (handled bool, cmd tea.Cmd)
	// SetDimensions updates the view's available space.
	SetDimensions(width, height int)
}

// Registry maps ViewMode int values to ViewHandler implementations.
type Registry struct {
	handlers map[int]ViewHandler
}

// NewRegistry creates an empty view registry.
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[int]ViewHandler),
	}
}

// Register associates a ViewMode with a ViewHandler.
func (r *Registry) Register(mode int, handler ViewHandler) {
	r.handlers[mode] = handler
}

// Get returns the ViewHandler for the given mode, or false if not registered.
func (r *Registry) Get(mode int) (ViewHandler, bool) {
	h, ok := r.handlers[mode]
	return h, ok
}

// Render delegates to the registered handler's Render method.
// Returns empty string if no handler is registered for the mode.
func (r *Registry) Render(mode, width, height int) string {
	if h, ok := r.handlers[mode]; ok {
		return h.Render(width, height)
	}
	return ""
}

// HandleKey delegates to the registered handler's HandleKey method.
// Returns (false, nil) if no handler is registered for the mode.
func (r *Registry) HandleKey(mode int, msg tea.KeyMsg) (bool, tea.Cmd) {
	if h, ok := r.handlers[mode]; ok {
		return h.HandleKey(msg)
	}
	return false, nil
}
