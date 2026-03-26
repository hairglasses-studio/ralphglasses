package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Registry holds registered plugins and dispatches events to them.
type Registry struct {
	mu      sync.RWMutex
	plugins []Plugin
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a plugin to the registry.
func (r *Registry) Register(p Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins = append(r.plugins, p)
}

// Dispatch calls OnEvent on all registered plugins.
// Errors are logged as warnings; dispatch continues on failure.
func (r *Registry) Dispatch(ctx context.Context, event Event) {
	r.mu.RLock()
	plugins := make([]Plugin, len(r.plugins))
	copy(plugins, r.plugins)
	r.mu.RUnlock()

	for _, p := range plugins {
		if err := p.OnEvent(ctx, event); err != nil {
			slog.Warn("plugin OnEvent failed", "plugin", p.Name(), "version", p.Version(), "err", err)
		}
	}
}

// List returns a snapshot of registered plugins.
func (r *Registry) List() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Plugin, len(r.plugins))
	copy(out, r.plugins)
	return out
}

// DispatchFunc returns a function suitable for use as an event handler.
// The returned func translates an events.Event-shaped call into plugin.Event.
func (r *Registry) DispatchFunc() func(ctx context.Context, eventType, repo string, data map[string]any) {
	return func(ctx context.Context, eventType, repo string, data map[string]any) {
		r.Dispatch(ctx, Event{
			Type:    eventType,
			Repo:    repo,
			Payload: data,
		})
	}
}

// String returns a summary for logging.
func (r *Registry) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return fmt.Sprintf("plugin.Registry{count: %d}", len(r.plugins))
}
