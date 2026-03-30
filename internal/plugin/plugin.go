package plugin

import (
	"context"
	"log/slog"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// Plugin is the interface that all ralphglasses plugins must implement.
type Plugin interface {
	Name() string
	Version() string
	// Init is called once when the plugin is activated. The host provides
	// access to shared infrastructure (event bus, session listing, tool
	// registration, logging).
	Init(ctx context.Context, host PluginHost) error
	// Shutdown is called when the plugin system is tearing down.
	Shutdown() error
}

// PluginHost is the surface area the host application exposes to plugins.
// It is deliberately narrow to keep the plugin API stable.
type PluginHost interface {
	// EventBus returns the shared event bus for subscribing/publishing.
	EventBus() *events.Bus
	// SessionManager returns a read-only view of active sessions.
	SessionManager() SessionLister
	// RegisterTool registers an MCP tool that the plugin provides.
	RegisterTool(name, description string, handler ToolHandler)
	// Logger returns a structured logger scoped to the plugin.
	Logger() *slog.Logger
}

// SessionLister is a read-only interface for querying sessions.
// It is a subset of the session.Manager surface so plugins cannot
// launch or mutate sessions.
type SessionLister interface {
	// ListSessions returns all sessions, optionally filtered by repo path.
	// Pass "" to list all sessions.
	ListSessions(repoPath string) []SessionInfo
}

// SessionInfo is a read-only snapshot of session state exposed to plugins.
type SessionInfo struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	RepoPath string `json:"repo_path"`
	Status   string `json:"status"`
	SpentUSD float64 `json:"spent_usd"`
}

// ToolHandler is the callback signature for plugin-registered MCP tools.
type ToolHandler func(ctx context.Context, args map[string]any) (any, error)

// PluginStatus tracks the lifecycle state of a registered plugin.
type PluginStatus string

const (
	StatusLoaded PluginStatus = "loaded" // registered but not yet initialized
	StatusActive PluginStatus = "active" // Init succeeded
	StatusFailed PluginStatus = "failed" // Init returned an error
)

// PluginInfo is returned by Registry.List() to describe registered plugins.
type PluginInfo struct {
	Name    string       `json:"name"`
	Version string       `json:"version"`
	Status  PluginStatus `json:"status"`
}

// Event mirrors the fleet event types from internal/events.
// Using a separate type keeps the plugin API stable and decoupled from internals.
type Event struct {
	Type    string
	Repo    string
	Payload map[string]any
}
