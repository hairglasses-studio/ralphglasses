package mcpserver

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestToolGroupNames(t *testing.T) {
	t.Parallel()
	expected := []string{
		"core", "session", "loop", "prompt", "fleet",
		"repo", "roadmap", "team", "awesome", "advanced", "eval", "fleet_h",
		"observability", "rdcycle",
	}
	if len(ToolGroupNames) != len(expected) {
		t.Fatalf("ToolGroupNames len = %d, want %d", len(ToolGroupNames), len(expected))
	}
	for i, name := range expected {
		if ToolGroupNames[i] != name {
			t.Errorf("ToolGroupNames[%d] = %q, want %q", i, ToolGroupNames[i], name)
		}
	}
}

func TestBuildToolGroups_AllGroupsPresent(t *testing.T) {
	t.Parallel()
	srv := NewServer("/tmp/test")
	groups := srv.ToolGroups()

	if len(groups) != len(ToolGroupNames) {
		t.Fatalf("got %d groups, want %d", len(groups), len(ToolGroupNames))
	}

	nameSet := make(map[string]bool)
	for _, g := range groups {
		nameSet[g.Name] = true
		if g.Description == "" {
			t.Errorf("group %q has empty description", g.Name)
		}
		if len(g.Tools) == 0 {
			t.Errorf("group %q has no tools", g.Name)
		}
	}

	for _, name := range ToolGroupNames {
		if !nameSet[name] {
			t.Errorf("missing tool group: %q", name)
		}
	}
}

func TestBuildToolGroups_ExpectedCounts(t *testing.T) {
	t.Parallel()
	srv := NewServer("/tmp/test")
	groups := srv.ToolGroups()

	// Map groups by name for easy lookup.
	byName := make(map[string]*ToolGroup)
	for i := range groups {
		byName[groups[i].Name] = &groups[i]
	}

	expectations := map[string]int{
		"core":     10,
		"session":  13,
		"loop":     10,
		"prompt":   8,
		"fleet":    7,
		"repo":     5,
		"roadmap":  5,
		"team":     6,
		"awesome":  5,
		"advanced": 24,
		"eval":     4,
		"fleet_h":       4,
		"observability": 17,
		"rdcycle":       17,
	}

	totalExpected := 0
	for name, want := range expectations {
		totalExpected += want
		g, ok := byName[name]
		if !ok {
			t.Errorf("missing group %q", name)
			continue
		}
		if len(g.Tools) != want {
			t.Errorf("group %q: got %d tools, want %d", name, len(g.Tools), want)
			// Print tool names for debugging.
			for _, te := range g.Tools {
				t.Logf("  %s", te.Tool.Name)
			}
		}
	}

	// Total across all groups.
	total := 0
	for _, g := range groups {
		total += len(g.Tools)
	}
	if total != totalExpected {
		t.Errorf("total tools across all groups = %d, want %d", total, totalExpected)
	}
}

func TestBuildToolGroups_NoDuplicateToolNames(t *testing.T) {
	t.Parallel()
	srv := NewServer("/tmp/test")
	groups := srv.ToolGroups()

	seen := make(map[string]string) // tool name -> group name
	for _, g := range groups {
		for _, te := range g.Tools {
			if prev, ok := seen[te.Tool.Name]; ok {
				t.Errorf("duplicate tool %q in groups %q and %q", te.Tool.Name, prev, g.Name)
			}
			seen[te.Tool.Name] = g.Name
		}
	}
}

func TestRegisterCoreTools(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	s.DeferredLoading = true
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.RegisterCoreTools(mcpSrv)

	// Core group should be loaded.
	if !s.loadedGroups["core"] {
		t.Error("core group should be marked as loaded")
	}

	// Non-core groups should not be loaded.
	for _, name := range ToolGroupNames {
		if name == "core" {
			continue
		}
		if s.loadedGroups[name] {
			t.Errorf("group %q should not be loaded after RegisterCoreTools", name)
		}
	}
}

func TestRegisterToolGroup(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.loadedGroups = make(map[string]bool)

	// Register a valid group.
	err := s.RegisterToolGroup(mcpSrv, "session")
	if err != nil {
		t.Fatalf("RegisterToolGroup(session): %v", err)
	}
	if !s.loadedGroups["session"] {
		t.Error("session group should be marked as loaded")
	}

	// Register an invalid group.
	err = s.RegisterToolGroup(mcpSrv, "nonexistent")
	if err == nil {
		t.Error("expected error for unknown group")
	}
}

func TestRegisterAllTools(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.RegisterAllTools(mcpSrv)

	// All groups should be loaded.
	for _, name := range ToolGroupNames {
		if !s.loadedGroups[name] {
			t.Errorf("group %q should be loaded after RegisterAllTools", name)
		}
	}
}

func TestRegister_BackwardCompat_AllTools(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	// DeferredLoading defaults to false.
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.Register(mcpSrv)

	for _, name := range ToolGroupNames {
		if !s.loadedGroups[name] {
			t.Errorf("group %q should be loaded in backward-compat mode", name)
		}
	}
}

func TestRegister_DeferredLoading(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	s.DeferredLoading = true
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.Register(mcpSrv)

	if !s.loadedGroups["core"] {
		t.Error("core should be loaded in deferred mode")
	}
	if s.loadedGroups["session"] {
		t.Error("session should NOT be loaded in deferred mode")
	}
}

func TestHandleToolGroups(t *testing.T) {
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
	// Should list all group names.
	for _, name := range ToolGroupNames {
		if !contains(text, name) {
			t.Errorf("output should mention group %q", name)
		}
	}
}

func TestHandleLoadToolGroup(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.RegisterCoreTools(mcpSrv)

	// Load "prompt" group.
	result, err := s.handleLoadToolGroup(context.Background(), makeRequest(map[string]any{
		"group": "prompt",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("handleLoadToolGroup returned error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !contains(text, "loaded") {
		t.Errorf("expected 'loaded' in output, got: %s", text)
	}
	if !s.loadedGroups["prompt"] {
		t.Error("prompt group should be marked as loaded")
	}

	// Loading again should say already_loaded.
	result, err = s.handleLoadToolGroup(context.Background(), makeRequest(map[string]any{
		"group": "prompt",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text = getResultText(result)
	if !contains(text, "already_loaded") {
		t.Errorf("expected 'already_loaded', got: %s", text)
	}

	// Invalid group.
	result, err = s.handleLoadToolGroup(context.Background(), makeRequest(map[string]any{
		"group": "bogus",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for invalid group")
	}

	// Missing group param.
	result, err = s.handleLoadToolGroup(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for missing group param")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
