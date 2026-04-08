package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestRegisterAllTools_ToolCount(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.RegisterAllTools(mcpSrv)

	// Count expected tools from all groups.
	expectedTotal := 0
	for _, g := range s.ToolGroups() {
		expectedTotal += len(g.Tools)
	}
	// Plus 4 always-available management tools.
	expectedTotal += len(s.ManagementTools())
	if expectedTotal != TotalToolCount() {
		t.Errorf("total tools = %d, want %d", TotalToolCount(), expectedTotal)
	}

	// All groups should be marked loaded.
	loadedCount := 0
	for _, name := range ToolGroupNames {
		if s.loadedGroups[name] {
			loadedCount++
		}
	}
	if loadedCount != len(ToolGroupNames) {
		t.Errorf("loaded %d groups, want %d", loadedCount, len(ToolGroupNames))
	}
}

func TestRegisterCoreTools_OnlyCore(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.RegisterCoreTools(mcpSrv)

	if !s.loadedGroups["core"] {
		t.Error("core group should be loaded")
	}

	nonCoreLoaded := 0
	for _, name := range ToolGroupNames {
		if name == "core" {
			continue
		}
		if s.loadedGroups[name] {
			nonCoreLoaded++
		}
	}
	if nonCoreLoaded != 0 {
		t.Errorf("expected 0 non-core groups loaded, got %d", nonCoreLoaded)
	}
}

func TestHandleToolGroups_AllGroupNames(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.RegisterCoreTools(mcpSrv)

	result, err := s.handleToolGroups(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("handleToolGroups returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	var groups []struct {
		Name      string `json:"name"`
		ToolCount int    `json:"tool_count"`
		Loaded    bool   `json:"loaded"`
	}
	if err := json.Unmarshal([]byte(text), &groups); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(groups) != len(ToolGroupNames) {
		t.Errorf("expected %d groups, got %d", len(ToolGroupNames), len(groups))
	}

	nameSet := make(map[string]bool)
	for _, g := range groups {
		nameSet[g.Name] = true
		if g.ToolCount == 0 {
			t.Errorf("group %q reports 0 tools", g.Name)
		}
	}
	for _, name := range ToolGroupNames {
		if !nameSet[name] {
			t.Errorf("group %q missing from handleToolGroups output", name)
		}
	}
}

func TestHandleToolGroups_LoadedFlag(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.RegisterCoreTools(mcpSrv)

	result, err := s.handleToolGroups(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}

	text := getResultText(result)
	var groups []struct {
		Name   string `json:"name"`
		Loaded bool   `json:"loaded"`
	}
	if err := json.Unmarshal([]byte(text), &groups); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}

	for _, g := range groups {
		if g.Name == "core" && !g.Loaded {
			t.Error("core group should be reported as loaded")
		}
		if g.Name == "session" && g.Loaded {
			t.Error("session group should NOT be loaded yet")
		}
	}
}

func TestHandleLoadToolGroup_ValidGroup(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.RegisterCoreTools(mcpSrv)

	result, err := s.handleLoadToolGroup(context.Background(), makeRequest(map[string]any{
		"group": "session",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("handleLoadToolGroup returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}

	if resp["status"] != "loaded" {
		t.Errorf("status = %q, want %q", resp["status"], "loaded")
	}
	if !s.loadedGroups["session"] {
		t.Error("session group should be marked as loaded")
	}
}

func TestHandleLoadToolGroup_InvalidGroup(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.RegisterCoreTools(mcpSrv)

	result, err := s.handleLoadToolGroup(context.Background(), makeRequest(map[string]any{
		"group": "nonexistent_group",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid group name")
	}
	text := getResultText(result)
	if !contains(text, "INVALID_PARAMS") {
		t.Errorf("expected INVALID_PARAMS in error, got: %s", text)
	}
}

func TestHandleLoadToolGroup_MissingParam(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.RegisterCoreTools(mcpSrv)

	result, err := s.handleLoadToolGroup(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing group param")
	}
}

func TestHandleLoadToolGroup_AlreadyLoaded(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.RegisterCoreTools(mcpSrv)

	// Load session group.
	_, _ = s.handleLoadToolGroup(context.Background(), makeRequest(map[string]any{
		"group": "session",
	}))

	// Load again - should return already_loaded.
	result, err := s.handleLoadToolGroup(context.Background(), makeRequest(map[string]any{
		"group": "session",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("second load should not be an error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !contains(text, "already_loaded") {
		t.Errorf("expected 'already_loaded', got: %s", text)
	}
}
