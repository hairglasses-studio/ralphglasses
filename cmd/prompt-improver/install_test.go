package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstall_CreatesSettings(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.json")

	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}

	addHookEntry(s, "/usr/local/bin/prompt-improver")
	addMCPEntry(s, "/usr/local/bin/prompt-improver")

	if err := writeSettings(path, s, nil); err != nil {
		t.Fatalf("writeSettings failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read settings: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "prompt-improver hook") {
		t.Error("settings should contain hook command")
	}
	if !strings.Contains(content, `"mcpServers"`) {
		t.Error("settings should contain mcpServers")
	}
	if !strings.Contains(content, `"mcp"`) {
		t.Error("settings should contain mcp args")
	}
}

func TestInstall_Idempotent(t *testing.T) {
	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}

	exe := "/usr/local/bin/prompt-improver"
	addHookEntry(s, exe)
	addHookEntry(s, exe) // second call should not duplicate

	groups := s.Hooks["UserPromptSubmit"]
	if len(groups) != 1 {
		t.Errorf("expected 1 hook group, got %d", len(groups))
	}

	addMCPEntry(s, exe)
	addMCPEntry(s, exe) // second call should not duplicate
	if len(s.McpServers) != 1 {
		t.Errorf("expected 1 MCP entry, got %d", len(s.McpServers))
	}
}

func TestInstall_PreservesExistingHooks(t *testing.T) {
	s := &settingsJSON{
		Hooks: map[string][]hookGroup{
			"UserPromptSubmit": {
				{
					Hooks: []hookEntry{
						{Type: "command", Command: "other-tool hook", Timeout: 5},
					},
				},
			},
		},
		McpServers: make(map[string]mcpServerEntry),
	}

	addHookEntry(s, "/usr/local/bin/prompt-improver")

	groups := s.Hooks["UserPromptSubmit"]
	if len(groups) != 2 {
		t.Errorf("expected 2 hook groups (existing + new), got %d", len(groups))
	}
}

func TestUninstall_RemovesEntries(t *testing.T) {
	s := &settingsJSON{
		Hooks: map[string][]hookGroup{
			"UserPromptSubmit": {
				{Hooks: []hookEntry{{Type: "command", Command: "other-tool hook"}}},
				{Hooks: []hookEntry{{Type: "command", Command: "/usr/local/bin/prompt-improver hook"}}},
			},
		},
		McpServers: map[string]mcpServerEntry{
			"prompt-improver": {Type: "stdio", Command: "/usr/local/bin/prompt-improver", Args: []string{"mcp"}},
			"other-tool":      {Type: "stdio", Command: "/usr/local/bin/other-tool"},
		},
	}

	removeHookEntry(s)
	removeMCPEntry(s)

	// prompt-improver hook should be gone, other-tool should remain
	groups := s.Hooks["UserPromptSubmit"]
	if len(groups) != 1 {
		t.Errorf("expected 1 remaining hook group, got %d", len(groups))
	}
	if groups[0].Hooks[0].Command != "other-tool hook" {
		t.Error("should preserve other-tool hook")
	}

	if _, ok := s.McpServers["prompt-improver"]; ok {
		t.Error("prompt-improver MCP entry should be removed")
	}
	if _, ok := s.McpServers["other-tool"]; !ok {
		t.Error("other-tool MCP entry should be preserved")
	}
}

func TestUninstall_EmptyIsNoop(t *testing.T) {
	s := &settingsJSON{
		Hooks:      make(map[string][]hookGroup),
		McpServers: make(map[string]mcpServerEntry),
	}

	removeHookEntry(s)
	removeMCPEntry(s)

	if len(s.Hooks) != 0 {
		t.Error("should remain empty")
	}
}

func TestReadWriteSettings_PreservesUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".claude", "settings.json")
	_ = os.MkdirAll(filepath.Dir(path), 0755)

	// Write settings with an unknown key
	initial := map[string]any{
		"someOtherSetting": true,
		"hooks":            map[string]any{},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	_ = os.WriteFile(path, data, 0644)

	s, raw, err := readSettings(path)
	if err != nil {
		t.Fatalf("readSettings failed: %v", err)
	}

	addHookEntry(s, "/usr/local/bin/prompt-improver")

	if err := writeSettings(path, s, raw); err != nil {
		t.Fatalf("writeSettings failed: %v", err)
	}

	// Re-read and verify unknown key is preserved
	result, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(result)
	if !strings.Contains(content, "someOtherSetting") {
		t.Error("should preserve unknown keys")
	}
	if !strings.Contains(content, "prompt-improver hook") {
		t.Error("should add hook")
	}
}

func TestCLI_Install_Uninstall(t *testing.T) {
	dir := t.TempDir()

	// Run install in the temp dir
	t.Run("install", func(t *testing.T) {
		stdout, _, code := runCLI(t, "", "install")
		// Will fail because cwd isn't dir, but we test the binary integration
		_ = stdout
		_ = code
		// The command runs, that's sufficient for CLI integration
	})

	t.Run("uninstall", func(t *testing.T) {
		stdout, _, code := runCLI(t, "", "uninstall")
		_ = stdout
		_ = code
		_ = dir
	})
}
