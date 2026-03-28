package main

import (
	"os"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
)

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
		"observability", "rdcycle",
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
