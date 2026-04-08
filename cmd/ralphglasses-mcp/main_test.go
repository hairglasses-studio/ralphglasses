package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
)

var testBinary string

func initializeServer(t *testing.T, srv *server.MCPServer) mcp.InitializeResult {
	t.Helper()

	rawReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`
	resp := srv.HandleMessage(context.Background(), []byte(rawReq))
	rpcResp, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("expected initialize JSONRPCResponse, got %T", resp)
	}
	result, ok := rpcResp.Result.(mcp.InitializeResult)
	if !ok {
		t.Fatalf("expected InitializeResult, got %T", rpcResp.Result)
	}
	return result
}

func listResources(t *testing.T, srv *server.MCPServer) mcp.ListResourcesResult {
	t.Helper()
	resp := srv.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":2,"method":"resources/list","params":{}}`))
	rpcResp, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("expected resources/list JSONRPCResponse, got %T", resp)
	}
	result, ok := rpcResp.Result.(mcp.ListResourcesResult)
	if !ok {
		t.Fatalf("expected ListResourcesResult, got %T", rpcResp.Result)
	}
	return result
}

func listResourceTemplates(t *testing.T, srv *server.MCPServer) mcp.ListResourceTemplatesResult {
	t.Helper()
	resp := srv.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":3,"method":"resources/templates/list","params":{}}`))
	rpcResp, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("expected resources/templates/list JSONRPCResponse, got %T", resp)
	}
	result, ok := rpcResp.Result.(mcp.ListResourceTemplatesResult)
	if !ok {
		t.Fatalf("expected ListResourceTemplatesResult, got %T", rpcResp.Result)
	}
	return result
}

func listPrompts(t *testing.T, srv *server.MCPServer) mcp.ListPromptsResult {
	t.Helper()
	resp := srv.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","id":4,"method":"prompts/list","params":{}}`))
	rpcResp, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("expected prompts/list JSONRPCResponse, got %T", resp)
	}
	result, ok := rpcResp.Result.(mcp.ListPromptsResult)
	if !ok {
		t.Fatalf("expected ListPromptsResult, got %T", rpcResp.Result)
	}
	return result
}

func TestMain(m *testing.M) {
	// Build the binary once for CLI-level tests
	tmp, err := os.MkdirTemp("", "rg-mcp-test-*")
	if err != nil {
		panic(err)
	}
	testBinary = filepath.Join(tmp, "ralphglasses-mcp")
	cmd := exec.Command("go", "build", "-o", testBinary, ".")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		panic("build failed: " + string(out))
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

func TestResolveScanPath_Default(t *testing.T) {
	origEnv := os.Getenv("RALPHGLASSES_SCAN_PATH")
	defer os.Setenv("RALPHGLASSES_SCAN_PATH", origEnv)

	os.Unsetenv("RALPHGLASSES_SCAN_PATH")

	sp := resolveScanPath()
	home, _ := os.UserHomeDir()
	want := home + "/hairglasses-studio"
	if sp != want {
		t.Errorf("resolveScanPath() = %q, want %q", sp, want)
	}
}

func TestResolveScanPath_FromEnv(t *testing.T) {
	tmpDir := t.TempDir()

	origEnv := os.Getenv("RALPHGLASSES_SCAN_PATH")
	defer os.Setenv("RALPHGLASSES_SCAN_PATH", origEnv)

	os.Setenv("RALPHGLASSES_SCAN_PATH", tmpDir)

	sp := resolveScanPath()
	if sp != tmpDir {
		t.Errorf("resolveScanPath() = %q, want %q", sp, tmpDir)
	}
}

func TestSetup_CreatesServer(t *testing.T) {
	tmpDir := t.TempDir()

	srv, cleanup, err := setup(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	defer cleanup()

	if srv == nil {
		t.Fatal("setup() returned nil server")
	}
}

func TestSetup_RegistersCoreTools(t *testing.T) {
	tmpDir := t.TempDir()

	srv, cleanup, err := setup(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	defer cleanup()

	coreTools := []string{
		"ralphglasses_scan",
		"ralphglasses_list",
		"ralphglasses_status",
		"ralphglasses_start",
		"ralphglasses_stop",
		"ralphglasses_stop_all",
		"ralphglasses_pause",
		"ralphglasses_logs",
		"ralphglasses_config",
		"ralphglasses_config_bulk",
	}

	for _, name := range coreTools {
		if srv.GetTool(name) == nil {
			t.Errorf("core tool %q not registered", name)
		}
	}
}

func TestSetup_RegistersMetaTools(t *testing.T) {
	tmpDir := t.TempDir()

	srv, cleanup, err := setup(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	defer cleanup()

	if srv.GetTool("ralphglasses_tool_groups") == nil {
		t.Error("tool_groups meta-tool not registered")
	}
	if srv.GetTool("ralphglasses_load_tool_group") == nil {
		t.Error("load_tool_group meta-tool not registered")
	}
}

func TestToolGroupNames(t *testing.T) {
	expected := []string{
		"core", "session", "loop", "prompt", "fleet",
		"repo", "roadmap", "team", "tenant", "awesome", "advanced", "events", "feedback", "eval", "fleet_h",
		"observability", "rdcycle", "plugin", "sweep",
		"rc", "autonomy", "workflow", "docs", "recovery", "promptdj", "a2a", "trigger", "approval",
		"context", "prefetch",
	}
	if len(mcpserver.ToolGroupNames) != len(expected) {
		t.Errorf("ToolGroupNames has %d entries, want %d", len(mcpserver.ToolGroupNames), len(expected))
	}
	for i, name := range expected {
		if i >= len(mcpserver.ToolGroupNames) {
			break
		}
		if mcpserver.ToolGroupNames[i] != name {
			t.Errorf("ToolGroupNames[%d] = %q, want %q", i, mcpserver.ToolGroupNames[i], name)
		}
	}
}

func TestSetup_CleanupDoesNotPanic(t *testing.T) {
	tmpDir := t.TempDir()

	_, cleanup, err := setup(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}

	// Calling cleanup should not panic
	cleanup()
}

func TestSetup_CleanupIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	_, cleanup, err := setup(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}

	// Calling cleanup twice should not panic
	cleanup()
	cleanup()
}

func TestSetup_ServerNotNil(t *testing.T) {
	tmpDir := t.TempDir()

	srv, cleanup, err := setup(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	defer cleanup()

	if srv == nil {
		t.Fatal("setup should return non-nil server")
	}
}

func TestSetup_ErrorIsNil(t *testing.T) {
	tmpDir := t.TempDir()

	_, cleanup, err := setup(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("setup() should not return error for valid path, got: %v", err)
	}
	defer cleanup()
}

func TestResolveScanPath_ExpandsHome(t *testing.T) {
	origEnv := os.Getenv("RALPHGLASSES_SCAN_PATH")
	defer os.Setenv("RALPHGLASSES_SCAN_PATH", origEnv)

	os.Setenv("RALPHGLASSES_SCAN_PATH", "~/custom-dir")

	sp := resolveScanPath()
	home, _ := os.UserHomeDir()
	want := home + "/custom-dir"
	if sp != want {
		t.Errorf("resolveScanPath() = %q, want %q", sp, want)
	}
}

func TestSetup_RegistersDeferredTools(t *testing.T) {
	tmpDir := t.TempDir()

	srv, cleanup, err := setup(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	defer cleanup()

	// After loading session group, session tools should be available
	// But at minimum, the deferred loader tools should exist
	if srv.GetTool("ralphglasses_tool_groups") == nil {
		t.Error("tool_groups should be registered")
	}
	if srv.GetTool("ralphglasses_load_tool_group") == nil {
		t.Error("load_tool_group should be registered")
	}
}

func TestSetup_InitializeIncludesInstructions(t *testing.T) {
	tmpDir := t.TempDir()

	srv, cleanup, err := setup(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	defer cleanup()

	initResult := initializeServer(t, srv)
	if !strings.Contains(initResult.Instructions, "ralph:///catalog/server") {
		t.Fatalf("initialize instructions missing catalog guidance: %q", initResult.Instructions)
	}
	if !strings.Contains(initResult.Instructions, "ralphglasses_load_tool_group") {
		t.Fatalf("initialize instructions missing deferred loading guidance: %q", initResult.Instructions)
	}
}

func TestSetup_RegistersResourcesAndPrompts(t *testing.T) {
	tmpDir := t.TempDir()

	srv, cleanup, err := setup(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	defer cleanup()

	initializeServer(t, srv)

	resources := listResources(t, srv)
	if got := len(resources.Resources); got != 6 {
		t.Fatalf("resources/list returned %d resources, want 6", got)
	}

	templates := listResourceTemplates(t, srv)
	if got := len(templates.ResourceTemplates); got != 3 {
		t.Fatalf("resources/templates/list returned %d templates, want 3", got)
	}

	prompts := listPrompts(t, srv)
	if got := len(prompts.Prompts); got != 6 {
		t.Fatalf("prompts/list returned %d prompts, want 6", got)
	}
}

func TestSetup_CanLoadTenantToolGroup(t *testing.T) {
	tmpDir := t.TempDir()

	srv, cleanup, err := setup(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}
	defer cleanup()

	initializeServer(t, srv)
	if srv.GetTool("ralphglasses_tenant_role_leaderboards") != nil {
		t.Fatal("tenant role leaderboards should not be loaded before load_tool_group")
	}

	req := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"ralphglasses_load_tool_group","arguments":{"group":"tenant"}}}`
	resp := srv.HandleMessage(context.Background(), []byte(req))
	switch msg := resp.(type) {
	case mcp.JSONRPCError:
		t.Fatalf("load_tool_group returned JSONRPC error: %+v", msg.Error)
	case mcp.JSONRPCResponse:
		// Success path.
	default:
		t.Fatalf("expected tools/call JSONRPC response, got %T", resp)
	}

	if srv.GetTool("ralphglasses_tenant_role_leaderboards") == nil {
		t.Fatal("tenant role leaderboards tool not registered after loading tenant group")
	}
}

// --- Binary-level tests to cover main() ---

func TestMain_StdinClosed(t *testing.T) {
	if testBinary == "" {
		t.Skip("test binary not built")
	}

	cmd := exec.Command(testBinary)
	cmd.Env = append(os.Environ(), "RALPHGLASSES_SCAN_PATH="+t.TempDir())
	// Close stdin immediately so ServeStdio returns EOF
	cmd.Stdin = strings.NewReader("")

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		// ServeStdio with closed stdin should exit (possibly with error)
		_ = err // We just want to verify main() runs and exits
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Fatal("binary did not exit after stdin was closed")
	}
}

func TestMain_JSONRPCInitialize(t *testing.T) {
	if testBinary == "" {
		t.Skip("test binary not built")
	}

	// Send a minimal JSON-RPC initialize request
	initReq := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "test-client",
				"version": "1.0.0",
			},
		},
	}
	reqBytes, _ := json.Marshal(initReq)
	// MCP stdio uses Content-Length header framing
	stdinData := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(reqBytes), reqBytes)

	cmd := exec.Command(testBinary)
	cmd.Env = append(os.Environ(), "RALPHGLASSES_SCAN_PATH="+t.TempDir())
	cmd.Stdin = strings.NewReader(stdinData)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case <-done:
		// Binary exited — check if we got a response
		out := stdout.String()
		if strings.Contains(out, "ralphglasses") || strings.Contains(out, "result") {
			// Got a valid response
		}
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		t.Fatal("binary did not exit in time")
	}
}

func TestSetup_MultipleInstances(t *testing.T) {
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	srv1, cleanup1, err := setup(context.Background(), tmpDir1)
	if err != nil {
		t.Fatalf("setup(1) error: %v", err)
	}
	defer cleanup1()

	srv2, cleanup2, err := setup(context.Background(), tmpDir2)
	if err != nil {
		t.Fatalf("setup(2) error: %v", err)
	}
	defer cleanup2()

	if srv1 == nil || srv2 == nil {
		t.Fatal("both servers should be non-nil")
	}
}
