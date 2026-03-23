package plugin

import "context"

// Plugin is the interface that all ralphglasses plugins must implement.
type Plugin interface {
	Name() string
	Version() string
	// OnEvent is called for each fleet event. Return an error to log a warning.
	OnEvent(ctx context.Context, event Event) error
}

// Event mirrors the fleet event types from internal/events.
// Using a separate type keeps the plugin API stable and decoupled from internals.
type Event struct {
	Type    string
	Repo    string
	Payload map[string]any
}
