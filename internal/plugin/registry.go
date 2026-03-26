package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Registry holds registered plugins and dispatches events to them.
// It tracks both regular event-based plugins and gRPC tool-call plugins.
type Registry struct {
	mu      sync.RWMutex
	plugins []Plugin

	grpcMu      sync.RWMutex
	grpcPlugins []GRPCPlugin
	// toolIndex maps tool name -> GRPCPlugin for fast dispatch.
	toolIndex map[string]GRPCPlugin
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		toolIndex: make(map[string]GRPCPlugin),
	}
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

// RegisterGRPC adds a gRPC-capable plugin and indexes its tool capabilities.
// It also registers the plugin as a regular Plugin for event dispatch.
func (r *Registry) RegisterGRPC(p GRPCPlugin) {
	r.Register(p) // register for event dispatch too

	r.grpcMu.Lock()
	defer r.grpcMu.Unlock()
	r.grpcPlugins = append(r.grpcPlugins, p)
	for _, cap := range p.Capabilities() {
		r.toolIndex[cap] = p
	}
}

// DispatchToolCall routes a tool call to the GRPCPlugin that declared the
// given tool name. Returns an error if no plugin handles the tool.
func (r *Registry) DispatchToolCall(ctx context.Context, name string, args map[string]any) (string, error) {
	r.grpcMu.RLock()
	p, ok := r.toolIndex[name]
	r.grpcMu.RUnlock()

	if !ok {
		return "", fmt.Errorf("no plugin registered for tool %q", name)
	}
	return p.HandleToolCall(ctx, name, args)
}

// ListGRPC returns a snapshot of registered gRPC plugins.
func (r *Registry) ListGRPC() []GRPCPlugin {
	r.grpcMu.RLock()
	defer r.grpcMu.RUnlock()
	out := make([]GRPCPlugin, len(r.grpcPlugins))
	copy(out, r.grpcPlugins)
	return out
}

// ListTools returns all tool names registered by gRPC plugins.
func (r *Registry) ListTools() []string {
	r.grpcMu.RLock()
	defer r.grpcMu.RUnlock()
	tools := make([]string, 0, len(r.toolIndex))
	for name := range r.toolIndex {
		tools = append(tools, name)
	}
	return tools
}

// String returns a summary for logging.
func (r *Registry) String() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	r.grpcMu.RLock()
	defer r.grpcMu.RUnlock()
	return fmt.Sprintf("plugin.Registry{count: %d, grpc: %d, tools: %d}",
		len(r.plugins), len(r.grpcPlugins), len(r.toolIndex))
}
