package plugin

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
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

	// pluginDirs holds directories to scan during Reload.
	pluginDirs []string
	// modTimes tracks file modification times for change detection.
	modTimes map[string]time.Time
	// reloadCallbacks holds functions to invoke after a successful Reload.
	reloadCallbacks []func(added, removed []string)
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
		modTimes:  make(map[string]time.Time),
	}
}

// AddPluginDir registers a directory to scan during Reload. It also records
// the current modification times of any plugin.json files found within.
func (r *Registry) AddPluginDir(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pluginDirs = append(r.pluginDirs, dir)
}

// OnReload registers a callback that is invoked after each successful Reload
// with the names of added and removed plugins.
func (r *Registry) OnReload(fn func(added, removed []string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reloadCallbacks = append(r.reloadCallbacks, fn)
}

// HandleSIGHUP starts a goroutine that listens for SIGHUP signals and triggers
// Reload. The goroutine exits when ctx is cancelled.
func (r *Registry) HandleSIGHUP(ctx context.Context) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGHUP)

	go func() {
		defer signal.Stop(ch)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ch:
				if err := r.Reload(); err != nil {
					slog.Warn("plugin reload on SIGHUP failed", "err", err)
				}
			}
		}
	}()
}

// Reload re-scans all registered plugin directories, detects new, changed,
// and removed plugins based on file modification times. New manifests are
// recorded; removed manifests are cleaned up. Callbacks registered via
// OnReload are invoked with the names of added and removed plugins.
func (r *Registry) Reload() error {
	r.mu.Lock()
	dirs := make([]string, len(r.pluginDirs))
	copy(dirs, r.pluginDirs)
	r.mu.Unlock()

	// Collect current manifests from all plugin dirs.
	current := make(map[string]time.Time)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("reload scan %q: %w", dir, err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			manifestPath := filepath.Join(dir, e.Name(), "plugin.json")
			info, err := os.Stat(manifestPath)
			if err != nil {
				continue // no manifest in this subdir
			}
			current[manifestPath] = info.ModTime()
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	var added, removed []string

	// Detect new and changed plugins.
	for path, modTime := range current {
		prevTime, exists := r.modTimes[path]
		if !exists {
			// New plugin discovered.
			m, err := LoadManifest(path)
			if err != nil {
				continue
			}
			if err := ValidateManifest(m); err != nil {
				continue
			}
			added = append(added, m.Name)
			r.modTimes[path] = modTime
		} else if modTime.After(prevTime) {
			// Plugin changed — update mod time. The manifest name is needed
			// for callbacks so we load it.
			m, err := LoadManifest(path)
			if err != nil {
				continue
			}
			// Treat as removed + added for callback purposes.
			added = append(added, m.Name)
			removed = append(removed, m.Name)
			r.modTimes[path] = modTime
		}
		// Unchanged: skip.
	}

	// Detect removed plugins.
	for path := range r.modTimes {
		if _, exists := current[path]; !exists {
			// Plugin was removed from disk.
			m, err := LoadManifest(path)
			if err != nil {
				// Manifest file is gone, extract name from what we can.
				// Just remove the tracking entry.
				removed = append(removed, path)
				delete(r.modTimes, path)
				continue
			}
			removed = append(removed, m.Name)
			delete(r.modTimes, path)
		}
	}

	// Fire callbacks.
	if len(added) > 0 || len(removed) > 0 {
		callbacks := make([]func(added, removed []string), len(r.reloadCallbacks))
		copy(callbacks, r.reloadCallbacks)

		for _, fn := range callbacks {
			fn(added, removed)
		}
	}

	return nil
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
			Type:    r.pluginType(e.plugin),
		}
	}
	return out
}

// pluginType determines the PluginType for a given plugin.
// Must be called with r.mu held (at least RLock).
func (r *Registry) pluginType(p Plugin) PluginType {
	// Check if the plugin implements the GRPCPlugin interface.
	if _, ok := p.(GRPCPlugin); ok {
		return TypeGRPC
	}
	return TypeBuiltin
}

// Enable re-enables a disabled plugin, restoring it to active status.
// Returns an error if the plugin is not found or is not currently disabled.
func (r *Registry) Enable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.plugins {
		if r.plugins[i].plugin.Name() == name {
			if r.plugins[i].status != StatusDisabled {
				return fmt.Errorf("plugin %q is not disabled (status: %s)", name, r.plugins[i].status)
			}
			r.plugins[i].status = StatusActive
			return nil
		}
	}
	return fmt.Errorf("plugin %q not found", name)
}

// Disable disables an active plugin, preventing it from receiving events.
// Returns an error if the plugin is not found or is not currently active.
func (r *Registry) Disable(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.plugins {
		if r.plugins[i].plugin.Name() == name {
			if r.plugins[i].status != StatusActive {
				return fmt.Errorf("plugin %q is not active (status: %s)", name, r.plugins[i].status)
			}
			r.plugins[i].status = StatusDisabled
			return nil
		}
	}
	return fmt.Errorf("plugin %q not found", name)
}

// GetStatus returns the current status of a plugin by name.
func (r *Registry) GetStatus(name string) (PluginStatus, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.plugins {
		if e.plugin.Name() == name {
			return e.status, true
		}
	}
	return "", false
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
