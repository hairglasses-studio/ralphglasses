package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// handlePrefetchStatus returns the list of registered prefetch hooks.
// This is a diagnostic tool — no parameters required.
func (s *Server) handlePrefetchStatus(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	runner := s.prefetchRunner()

	type hookInfo struct {
		Name string `json:"name"`
	}

	names := runner.Hooks()
	hooks := make([]hookInfo, len(names))
	for i, n := range names {
		hooks[i] = hookInfo{Name: n}
	}

	return jsonResult(map[string]any{
		"hook_count": len(hooks),
		"hooks":      hooks,
	}), nil
}

// buildPrefetchGroup returns the tool group for prefetch diagnostics.
func (s *Server) buildPrefetchGroup() ToolGroup {
	return ToolGroup{
		Name:        "prefetch",
		Description: "Deterministic context pre-fetching: inspect registered hooks",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_prefetch_status",
				mcp.WithDescription("List registered prefetch hooks that run before session launch to pre-gather context"),
			), s.handlePrefetchStatus},
		},
	}
}

// prefetchRunner returns the shared PrefetchRunner, lazily initialised
// with the default built-in hooks.
func (s *Server) prefetchRunner() *session.PrefetchRunner {
	s.mu.RLock()
	r := s.PrefetchRunnerInstance
	s.mu.RUnlock()
	if r != nil {
		return r
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.PrefetchRunnerInstance == nil {
		s.PrefetchRunnerInstance = session.DefaultPrefetchRunner()
	}
	return s.PrefetchRunnerInstance
}
