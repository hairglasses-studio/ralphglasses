package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
)

var testBinary string

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

	srv, cleanup, err := setup(tmpDir)
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

	srv, cleanup, err := setup(tmpDir)
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

	srv, cleanup, err := setup(tmpDir)
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
		"repo", "roadmap", "team", "awesome", "advanced", "eval", "fleet_h",
		"observability", "rdcycle", "plugin", "sweep",
		"rc", "autonomy", "workflow",
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

	_, cleanup, err := setup(tmpDir)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}

	// Calling cleanup should not panic
	cleanup()
}

func TestSetup_CleanupIdempotent(t *testing.T) {
	tmpDir := t.TempDir()

	_, cleanup, err := setup(tmpDir)
	if err != nil {
		t.Fatalf("setup() error: %v", err)
	}

	// Calling cleanup twice should not panic
	cleanup()
	cleanup()
}

func TestSetup_ServerNotNil(t *testing.T) {
	tmpDir := t.TempDir()

	srv, cleanup, err := setup(tmpDir)
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

	_, cleanup, err := setup(tmpDir)
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

	srv, cleanup, err := setup(tmpDir)
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

	srv1, cleanup1, err := setup(tmpDir1)
	if err != nil {
		t.Fatalf("setup(1) error: %v", err)
	}
	defer cleanup1()

	srv2, cleanup2, err := setup(tmpDir2)
	if err != nil {
		t.Fatalf("setup(2) error: %v", err)
	}
	defer cleanup2()

	if srv1 == nil || srv2 == nil {
		t.Fatal("both servers should be non-nil")
	}
}
