package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// testHarness simplifies writing tests for individual MCP handler functions.
// It provides a pre-configured Server with temp directories, a mock session
// manager, and helper methods for building requests and asserting results.
type testHarness struct {
	t       *testing.T
	Server  *Server
	RootDir string
	// RepoDir is the path to the test repo created during setup.
	RepoDir  string
	RalphDir string
}

// newTestHarness creates a testHarness backed by a test-scoped temp directory.
// It initializes a Server with a scanned test repo, session manager, and
// git-initialized directory so handlers can operate against realistic state.
func newTestHarness(t *testing.T) *testHarness {
	t.Helper()
	srv, root := setupTestServer(t)

	// Trigger a scan so repos are populated for handler tests.
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoDir := filepath.Join(root, "test-repo")
	ralphDir := filepath.Join(repoDir, ".ralph")

	return &testHarness{
		t:        t,
		Server:   srv,
		RootDir:  root,
		RepoDir:  repoDir,
		RalphDir: ralphDir,
	}
}

// makeHandlerRequest builds a properly formatted MCP CallToolRequest with the
// given tool name and arguments.
func (h *testHarness) makeHandlerRequest(tool string, args map[string]any) mcp.CallToolRequest {
	req := mcp.CallToolRequest{}
	req.Params.Name = tool
	req.Params.Arguments = args
	return req
}

// callHandler executes a handler function and returns the text result.
// If the handler returns a Go-level error (not a tool-level error), the test
// fails immediately. Tool-level errors (IsError=true) are returned as the
// string content so callers can assert on them.
func (h *testHarness) callHandler(handler server.ToolHandlerFunc, req mcp.CallToolRequest) (string, error) {
	h.t.Helper()
	result, err := handler(context.Background(), req)
	if err != nil {
		h.t.Fatalf("handler returned Go error: %v", err)
	}
	text := getResultText(result)
	if result.IsError {
		return text, &toolError{text: text}
	}
	return text, nil
}

// toolError represents a tool-level error (IsError=true in CallToolResult).
type toolError struct {
	text string
}

func (e *toolError) Error() string { return e.text }

// assertJSON checks that the given string is valid JSON and contains all of
// the specified top-level keys.
func assertJSON(t *testing.T, got string, wantKeys ...string) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("assertJSON: invalid JSON: %v\ngot: %s", err, got)
	}
	for _, key := range wantKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("assertJSON: missing key %q in JSON: %s", key, got)
		}
	}
}

// assertError checks that the given string contains the expected substring.
// This is intended for use with tool-level error text returned from callHandler.
func assertError(t *testing.T, got string, wantSubstring string) {
	t.Helper()
	if !strings.Contains(got, wantSubstring) {
		t.Errorf("assertError: expected %q to contain %q", got, wantSubstring)
	}
}

// cleanup removes temp resources. With setupTestServer using t.TempDir(),
// cleanup is automatic via testing.T, but this method is provided for any
// additional teardown a test might register.
func (h *testHarness) cleanup() {
	// t.TempDir() handles removal. This is a hook for additional teardown.
}

// writeFile writes content to a file relative to the ralph directory.
// Parent directories are created as needed.
func (h *testHarness) writeFile(relPath, content string) {
	h.t.Helper()
	abs := filepath.Join(h.RalphDir, relPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		h.t.Fatalf("writeFile: mkdir: %v", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		h.t.Fatalf("writeFile: %v", err)
	}
}

// --- Tests that verify the harness itself works ---

func TestHandlerHarness_Creation(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	if h.Server == nil {
		t.Fatal("Server should not be nil")
	}
	if h.RootDir == "" {
		t.Fatal("RootDir should not be empty")
	}
	if h.RepoDir == "" {
		t.Fatal("RepoDir should not be empty")
	}
	if h.RalphDir == "" {
		t.Fatal("RalphDir should not be empty")
	}
	if len(h.Server.Repos) == 0 {
		t.Fatal("expected repos to be populated after scan")
	}
}

func TestHandlerHarness_MakeHandlerRequest(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	req := h.makeHandlerRequest("ralphglasses_status", map[string]any{
		"repo": "test-repo",
	})

	if req.Params.Name != "ralphglasses_status" {
		t.Errorf("Name = %q, want %q", req.Params.Name, "ralphglasses_status")
	}
	args := req.GetArguments()
	if args["repo"] != "test-repo" {
		t.Errorf("Arguments[repo] = %v, want %q", args["repo"], "test-repo")
	}
}

func TestHandlerHarness_CallHandler_Success(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	req := h.makeHandlerRequest("ralphglasses_status", map[string]any{
		"repo": "test-repo",
	})

	text, err := h.callHandler(h.Server.handleStatus, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "test-repo") {
		t.Errorf("expected result to contain test-repo, got: %s", text)
	}
}

func TestHandlerHarness_CallHandler_ToolError(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	req := h.makeHandlerRequest("ralphglasses_status", map[string]any{
		"repo": "nonexistent",
	})

	text, err := h.callHandler(h.Server.handleStatus, req)
	if err == nil {
		t.Fatal("expected tool error for nonexistent repo")
	}
	assertError(t, text, "not found")
}

func TestHandlerHarness_CallHandler_Config(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	req := h.makeHandlerRequest("ralphglasses_config", map[string]any{
		"repo": "test-repo",
	})

	text, err := h.callHandler(h.Server.handleConfig, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(text, "MODEL") {
		t.Errorf("expected config output to contain MODEL, got: %s", text)
	}
}

func TestHandlerHarness_AssertJSON(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	req := h.makeHandlerRequest("ralphglasses_scan", nil)

	text, err := h.callHandler(h.Server.handleScan, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSON(t, text, "repos_found", "repos")
}

func TestHandlerHarness_AssertError(t *testing.T) {
	t.Parallel()

	// Test with a known error substring.
	assertError(t, `{"error_code":"REPO_NOT_FOUND","message":"repo not found: missing"}`, "REPO_NOT_FOUND")
	assertError(t, `{"error_code":"REPO_NOT_FOUND","message":"repo not found: missing"}`, "repo not found")
}

func TestHandlerHarness_WriteFile(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	h.writeFile("test_scratchpad.md", "# Test\n\n1. Item one\n")

	data, err := os.ReadFile(filepath.Join(h.RalphDir, "test_scratchpad.md"))
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if !strings.Contains(string(data), "Item one") {
		t.Errorf("expected file content, got: %s", data)
	}
}

func TestHandlerHarness_ScratchpadRoundTrip(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	// Use the harness to test scratchpad append + read as a handler-level
	// integration test.
	appendReq := h.makeHandlerRequest("ralphglasses_scratchpad_append", map[string]any{
		"name":    "harness_notes",
		"content": "Testing the harness.",
	})
	text, err := h.callHandler(h.Server.handleScratchpadAppend, appendReq)
	if err != nil {
		t.Fatalf("append error: %v", err)
	}
	if !strings.Contains(text, "Appended") {
		t.Errorf("expected append confirmation, got: %s", text)
	}

	readReq := h.makeHandlerRequest("ralphglasses_scratchpad_read", map[string]any{
		"name": "harness_notes",
	})
	text, err = h.callHandler(h.Server.handleScratchpadRead, readReq)
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if !strings.Contains(text, "Testing the harness.") {
		t.Errorf("expected scratchpad content, got: %s", text)
	}
}

func TestHandlerHarness_StopAllJSON(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	req := h.makeHandlerRequest("ralphglasses_stop_all", nil)
	text, err := h.callHandler(h.Server.handleStopAll, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSON(t, text, "stopped_count", "stopped")
}
