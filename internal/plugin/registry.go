package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Registry holds registered plugins and manages their lifecycle.
// It tracks both regular plugins and gRPC tool-call plugins.
type Registry struct {
	mu      sync.RWMutex
	plugins []pluginEntry

	grpcMu      sync.RWMutex
	grpcPlugins []GRPCPlugin
	// toolIndex maps tool name -> GRPCPlugin for fast dispatch.
	toolIndex map[string]GRPCPlugin
}

// pluginEntry pairs a plugin with its lifecycle status.
type pluginEntry struct {
	plugin Plugin
	status PluginStatus
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		toolIndex: make(map[string]GRPCPlugin),
	}
}

// Register adds a plugin to the registry. Returns an error if a plugin
// with the same name is already registered.
func (r *Registry) Register(p Plugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := p.Name()
	for _, e := range r.plugins {
		if e.plugin.Name() == name {
			return fmt.Errorf("plugin %q already registered", name)
		}
	}
	r.plugins = append(r.plugins, pluginEntry{plugin: p, status: StatusLoaded})
	return nil
}

// InitAll calls Init on every registered plugin in registration order.
// A single plugin failure is logged and recorded but does not prevent
// other plugins from initializing.
func (r *Registry) InitAll(ctx context.Context, host PluginHost) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var firstErr error
	for i := range r.plugins {
		e := &r.plugins[i]
		if err := e.plugin.Init(ctx, host); err != nil {
			e.status = StatusFailed
			slog.Warn("plugin init failed",
				"plugin", e.plugin.Name(),
				"version", e.plugin.Version(),
				"err", err,
			)
			if firstErr == nil {
				firstErr = fmt.Errorf("plugin %q init: %w", e.plugin.Name(), err)
			}
			continue
		}
		e.status = StatusActive
	}
	return firstErr
}

// ShutdownAll calls Shutdown on every active plugin in reverse registration
// order. Errors are logged but do not prevent other plugins from shutting down.
func (r *Registry) ShutdownAll() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	var firstErr error
	for i := len(r.plugins) - 1; i >= 0; i-- {
		e := &r.plugins[i]
		if e.status != StatusActive {
			continue
		}
		if err := e.plugin.Shutdown(); err != nil {
			slog.Warn("plugin shutdown failed",
				"plugin", e.plugin.Name(),
				"version", e.plugin.Version(),
				"err", err,
			)
			if firstErr == nil {
				firstErr = fmt.Errorf("plugin %q shutdown: %w", e.plugin.Name(), err)
			}
		}
	}
	return firstErr
}

// Get returns a plugin by name.
func (r *Registry) Get(name string) (Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.plugins {
		if e.plugin.Name() == name {
			return e.plugin, true
		}
	}
	return nil, false
}

// List returns info about all registered plugins.
func (r *Registry) List() []PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]PluginInfo, len(r.plugins))
	for i, e := range r.plugins {
		out[i] = PluginInfo{
			Name:    e.plugin.Name(),
			Version: e.plugin.Version(),
			Status:  e.status,
		}
	}
	return out
}

// Plugins returns a snapshot of the raw Plugin values in registration order.
func (r *Registry) Plugins() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Plugin, len(r.plugins))
	for i, e := range r.plugins {
		out[i] = e.plugin
	}
	return out
}

// Dispatch calls OnEvent-style dispatch on all registered plugins that
// support the legacy event callback. This bridges the old OnEvent API;
// plugins that only implement Init/Shutdown are silently skipped.
func (r *Registry) Dispatch(ctx context.Context, event Event) {
	r.mu.RLock()
	entries := make([]pluginEntry, len(r.plugins))
	copy(entries, r.plugins)
	r.mu.RUnlock()

	for _, e := range entries {
		type eventHandler interface {
			OnEvent(ctx context.Context, event Event) error
		}
		if eh, ok := e.plugin.(eventHandler); ok {
			if err := eh.OnEvent(ctx, event); err != nil {
				slog.Warn("plugin OnEvent failed", "plugin", e.plugin.Name(), "version", e.plugin.Version(), "err", err)
			}
		}
	}
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
// It also registers the plugin as a regular Plugin for lifecycle management.
func (r *Registry) RegisterGRPC(p GRPCPlugin) error {
	if err := r.Register(p); err != nil {
		return err
	}

	r.grpcMu.Lock()
	defer r.grpcMu.Unlock()
	r.grpcPlugins = append(r.grpcPlugins, p)
	for _, cap := range p.Capabilities() {
		r.toolIndex[cap] = p
	}
	return nil
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
