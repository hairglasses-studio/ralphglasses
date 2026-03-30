package mcpserver

import (
	"context"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TestHarness wraps a Server and its registered tool handlers for table-driven
// testing without real MCP transport. It pre-indexes all tool handlers from
// every tool group for direct dispatch by name.
type TestHarness struct {
	Server   *Server
	RootDir  string
	handlers map[string]server.ToolHandlerFunc
}

// NewTestHarness creates a TestHarness backed by a test-scoped temp directory.
// It calls setupTestServer internally so the server has a valid repo, session
// manager, and git-initialized test repo. All tool handlers are indexed for
// fast lookup.
func NewTestHarness(t *testing.T) *TestHarness {
	t.Helper()
	srv, root := setupTestServer(t)

	h := &TestHarness{
		Server:   srv,
		RootDir:  root,
		handlers: make(map[string]server.ToolHandlerFunc),
	}

	// Index handlers from all tool groups (core + deferred).
	for _, g := range srv.ToolGroups() {
		for _, entry := range g.Tools {
			h.handlers[entry.Tool.Name] = entry.Handler
		}
	}
	// Also index core tools.
	core := srv.buildCoreGroup()
	for _, entry := range core.Tools {
		h.handlers[entry.Tool.Name] = entry.Handler
	}

	return h
}

// CallTool invokes a registered tool handler by name with the given arguments.
// It returns the CallToolResult without going through MCP transport.
func (h *TestHarness) CallTool(name string, args map[string]any) (*mcp.CallToolResult, error) {
	handler, ok := h.handlers[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not registered", name)
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args

	ctx := context.Background()
	return handler(ctx, req)
}

// ToolNames returns all registered tool names in the harness.
func (h *TestHarness) ToolNames() []string {
	names := make([]string, 0, len(h.handlers))
	for name := range h.handlers {
		names = append(names, name)
	}
	return names
}

// --- TestHarness tests ---

func TestTestHarness_Creation(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)

	if h.Server == nil {
		t.Fatal("Server should not be nil")
	}
	if h.RootDir == "" {
		t.Fatal("RootDir should not be empty")
	}
	if len(h.handlers) == 0 {
		t.Fatal("expected at least one tool handler to be registered")
	}
}

func TestTestHarness_CallTool_Scan(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)

	result, err := h.CallTool("ralphglasses_scan", nil)
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatalf("scan returned error: %s", getResultText(result))
	}
	text := getResultText(result)
	if text == "" {
		t.Fatal("expected non-empty result from scan")
	}
}

func TestTestHarness_CallTool_NotFound(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)

	_, err := h.CallTool("nonexistent_tool", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}

func TestTestHarness_CallTool_SessionList(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)

	// Scan first so repos are loaded.
	_, _ = h.CallTool("ralphglasses_scan", nil)

	result, err := h.CallTool("ralphglasses_session_list", nil)
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	// With no sessions, should return empty (not error).
	if result.IsError {
		t.Fatalf("session_list returned error: %s", getResultText(result))
	}
}

func TestTestHarness_ToolNames(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)

	names := h.ToolNames()
	if len(names) < 10 {
		t.Errorf("expected at least 10 tools, got %d", len(names))
	}
}
