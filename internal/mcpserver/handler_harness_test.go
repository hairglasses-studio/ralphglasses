package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
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

	// handlers maps tool names to their handler functions for CallTool dispatch.
	handlers map[string]server.ToolHandlerFunc

	// eventRecorder captures events published to the bus (when WithEventBus is used).
	eventRecorder *eventRecorder
}

// eventRecorder records events published to an event bus for test assertions.
type eventRecorder struct {
	mu     sync.Mutex
	events []events.Event
	sub    <-chan events.Event
	done   chan struct{}
}

// newEventRecorder subscribes to a bus and records all events in the background.
func newEventRecorder(bus *events.Bus, id string) *eventRecorder {
	r := &eventRecorder{
		done: make(chan struct{}),
		sub:  bus.Subscribe(id),
	}
	go func() {
		for {
			select {
			case ev, ok := <-r.sub:
				if !ok {
					return
				}
				r.mu.Lock()
				r.events = append(r.events, ev)
				r.mu.Unlock()
			case <-r.done:
				return
			}
		}
	}()
	return r
}

// Events returns a snapshot of all recorded events.
func (r *eventRecorder) Events() []events.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]events.Event, len(r.events))
	copy(out, r.events)
	return out
}

// EventsOfType returns recorded events matching the given type.
func (r *eventRecorder) EventsOfType(t events.EventType) []events.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []events.Event
	for _, ev := range r.events {
		if ev.Type == t {
			out = append(out, ev)
		}
	}
	return out
}

// Stop terminates the background recording goroutine.
func (r *eventRecorder) Stop() {
	close(r.done)
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

	h := &testHarness{
		t:        t,
		Server:   srv,
		RootDir:  root,
		RepoDir:  repoDir,
		RalphDir: ralphDir,
		handlers: make(map[string]server.ToolHandlerFunc),
	}
	h.indexHandlers()
	return h
}

// newTestHarnessWithBus creates a testHarness with an event bus wired into
// the Server. The bus enables testing of event-dependent handlers (event_list,
// event_poll) and records published events for assertions.
func newTestHarnessWithBus(t *testing.T) *testHarness {
	t.Helper()

	bus := events.NewBus(1000)
	root := t.TempDir()

	// Create a ralph-enabled repo (mirrors setupTestServer).
	repoPath := filepath.Join(root, "test-repo")
	ralphDir := filepath.Join(repoPath, ".ralph")
	logsDir := filepath.Join(ralphDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".ralphrc"), []byte("MODEL=sonnet\nBUDGET=5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(logsDir, "ralph.log"), []byte("log line 1\nlog line 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	initGitRepo(t, repoPath)

	srv := NewServerWithBus(root, bus)
	srv.SessMgr.SetStateDir(filepath.Join(root, ".session-state"))

	// Trigger a scan so repos are populated.
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	recorder := newEventRecorder(bus, "test-harness")
	t.Cleanup(func() {
		recorder.Stop()
		bus.Unsubscribe("test-harness")
		// Stop loops before temp dir cleanup.
		if srv.SessMgr == nil {
			return
		}
		for _, run := range srv.SessMgr.ListLoops() {
			run.Lock()
			run.RepoPath = ""
			run.Unlock()
			_ = srv.SessMgr.StopLoop(run.ID)
		}
	})

	h := &testHarness{
		t:             t,
		Server:        srv,
		RootDir:       root,
		RepoDir:       repoPath,
		RalphDir:      ralphDir,
		handlers:      make(map[string]server.ToolHandlerFunc),
		eventRecorder: recorder,
	}
	h.indexHandlers()
	return h
}

// newTestHarnessWithFleet creates a testHarness with both an event bus and
// a fleet coordinator wired in. Useful for testing fleet-related handlers.
func newTestHarnessWithFleet(t *testing.T) *testHarness {
	t.Helper()
	h := newTestHarnessWithBus(t)

	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", h.Server.EventBus, h.Server.SessMgr)
	h.Server.FleetCoordinator = coord
	return h
}

// indexHandlers builds the tool-name-to-handler map from all tool groups.
func (h *testHarness) indexHandlers() {
	for _, g := range h.Server.ToolGroups() {
		for _, entry := range g.Tools {
			h.handlers[entry.Tool.Name] = entry.Handler
		}
	}
	core := h.Server.buildCoreGroup()
	for _, entry := range core.Tools {
		h.handlers[entry.Tool.Name] = entry.Handler
	}
}

// CallTool dispatches a tool call by name through the indexed handler map.
// It returns the raw CallToolResult for flexible assertions.
func (h *testHarness) CallTool(name string, args map[string]any) (*mcp.CallToolResult, error) {
	h.t.Helper()
	handler, ok := h.handlers[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not registered in harness", name)
	}
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return handler(context.Background(), req)
}

// CallToolText is a convenience wrapper around CallTool that extracts text.
// Tool-level errors (IsError=true) are returned as toolError.
func (h *testHarness) CallToolText(name string, args map[string]any) (string, error) {
	h.t.Helper()
	result, err := h.CallTool(name, args)
	if err != nil {
		return "", err
	}
	text := getResultText(result)
	if result.IsError {
		return text, &toolError{text: text}
	}
	return text, nil
}

// publishEvent publishes a test event to the server's event bus. Requires
// the harness to have been created with newTestHarnessWithBus.
func (h *testHarness) publishEvent(evType events.EventType, data map[string]any) {
	h.t.Helper()
	if h.Server.EventBus == nil {
		h.t.Fatal("publishEvent requires an event bus; use newTestHarnessWithBus")
	}
	h.Server.EventBus.Publish(events.Event{
		Type:      evType,
		Timestamp: time.Now(),
		Data:      data,
	})
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

// assertContains checks that text contains the expected substring.
func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("assertContains: expected output to contain %q, got: %s", want, got)
	}
}

// assertJSONArray checks that the given string is valid JSON and is an array
// with at least minLen elements.
func assertJSONArray(t *testing.T, got string, minLen int) {
	t.Helper()
	var arr []any
	if err := json.Unmarshal([]byte(got), &arr); err != nil {
		t.Fatalf("assertJSONArray: not a JSON array: %v\ngot: %s", err, got)
	}
	if len(arr) < minLen {
		t.Errorf("assertJSONArray: expected at least %d elements, got %d", minLen, len(arr))
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

// --- CallTool dispatch tests ---

func TestHandlerHarness_CallTool_List(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	text, err := h.CallToolText("ralphglasses_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// handleList returns a JSON array of repo summaries.
	assertJSONArray(t, text, 1)
	assertContains(t, text, "test-repo")
}

func TestHandlerHarness_CallTool_Status(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	text, err := h.CallToolText("ralphglasses_status", map[string]any{
		"repo": "test-repo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, text, "test-repo")
}

func TestHandlerHarness_CallTool_StatusMissingRepo(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	_, err := h.CallToolText("ralphglasses_status", map[string]any{
		"repo": "does-not-exist",
	})
	if err == nil {
		t.Fatal("expected tool error for nonexistent repo")
	}
	assertContains(t, err.Error(), "not found")
}

func TestHandlerHarness_CallTool_SessionList(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	// With no active sessions, session_list should return an empty result (not error).
	result, err := h.CallTool("ralphglasses_session_list", nil)
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatalf("session_list returned tool error: %s", getResultText(result))
	}
}

func TestHandlerHarness_CallTool_Scan(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	text, err := h.CallToolText("ralphglasses_scan", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertJSON(t, text, "repos_found", "repos")
}

func TestHandlerHarness_CallTool_Config(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	text, err := h.CallToolText("ralphglasses_config", map[string]any{
		"repo": "test-repo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, text, "MODEL")
}

func TestHandlerHarness_CallTool_Logs(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	text, err := h.CallToolText("ralphglasses_logs", map[string]any{
		"repo": "test-repo",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, text, "log line")
}

func TestHandlerHarness_CallTool_NotRegistered(t *testing.T) {
	t.Parallel()
	h := newTestHarness(t)
	defer h.cleanup()

	_, err := h.CallTool("totally_fake_tool", nil)
	if err == nil {
		t.Fatal("expected error for unregistered tool")
	}
	assertContains(t, err.Error(), "not registered")
}

// --- Event bus harness tests ---

func TestHandlerHarness_WithBus_EventList(t *testing.T) {
	t.Parallel()
	h := newTestHarnessWithBus(t)

	// Publish some events so event_list has data to return.
	h.publishEvent(events.ScanComplete, map[string]any{"repos_found": 1})
	h.publishEvent(events.ConfigChanged, map[string]any{"key": "MODEL"})

	// Small pause to allow async fan-out.
	time.Sleep(50 * time.Millisecond)

	text, err := h.CallToolText("ralphglasses_event_list", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, text, "events")
}

func TestHandlerHarness_WithBus_EventRecorder(t *testing.T) {
	t.Parallel()
	h := newTestHarnessWithBus(t)

	h.publishEvent(events.SessionStarted, map[string]any{"id": "sess-1"})
	h.publishEvent(events.SessionStarted, map[string]any{"id": "sess-2"})
	h.publishEvent(events.CostUpdate, map[string]any{"usd": 0.5})

	// Allow async delivery.
	time.Sleep(50 * time.Millisecond)

	all := h.eventRecorder.Events()
	if len(all) < 3 {
		t.Errorf("expected at least 3 recorded events, got %d", len(all))
	}

	sessionEvents := h.eventRecorder.EventsOfType(events.SessionStarted)
	if len(sessionEvents) != 2 {
		t.Errorf("expected 2 session.started events, got %d", len(sessionEvents))
	}
}

func TestHandlerHarness_WithBus_EventListEmpty(t *testing.T) {
	t.Parallel()
	h := newTestHarnessWithBus(t)

	// No events published; event_list should succeed with an empty result.
	result, err := h.CallTool("ralphglasses_event_list", nil)
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatalf("event_list returned tool error: %s", getResultText(result))
	}
}

// --- Fleet harness tests ---

func TestHandlerHarness_WithFleet_FleetStatus(t *testing.T) {
	t.Parallel()
	h := newTestHarnessWithFleet(t)

	text, err := h.CallToolText("ralphglasses_fleet_status", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Fleet status returns JSON with fleet metadata.
	assertContains(t, text, "repos")
}

func TestHandlerHarness_WithFleet_FleetWorkers(t *testing.T) {
	t.Parallel()
	h := newTestHarnessWithFleet(t)

	// With no workers registered, fleet_workers should return an empty list.
	result, err := h.CallTool("ralphglasses_fleet_workers", nil)
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatalf("fleet_workers returned tool error: %s", getResultText(result))
	}
}
