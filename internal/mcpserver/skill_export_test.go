package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestExportSkillMarkdown_Empty(t *testing.T) {
	t.Parallel()
	md := ExportSkillMarkdown(nil)
	if !strings.Contains(md, "No tool groups defined") {
		t.Errorf("expected empty message, got: %s", md)
	}
}

func TestExportSkillMarkdown_Structure(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())
	groups := srv.buildToolGroups()

	md := ExportSkillMarkdown(groups)

	// Should have the header.
	if !strings.HasPrefix(md, "# Ralphglasses Skills") {
		t.Error("missing header")
	}

	// Should have table of contents.
	if !strings.Contains(md, "## Table of Contents") {
		t.Error("missing table of contents")
	}

	// Should mention tool count.
	if !strings.Contains(md, "tools across") {
		t.Error("missing tool count summary")
	}

	// Every group name should appear as a section.
	for _, name := range ToolGroupNames {
		if !strings.Contains(md, "## "+name) {
			t.Errorf("missing section for group %q", name)
		}
	}

	// Should contain parameter tables.
	if !strings.Contains(md, "| Parameter |") {
		t.Error("missing parameter table header")
	}

	// Should contain example blocks.
	if !strings.Contains(md, "**Example:**") {
		t.Error("missing example block")
	}
}

func TestExportSkillMarkdown_SingleGroup(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())
	coreGroup := srv.buildCoreGroup()

	md := ExportSkillMarkdown([]ToolGroup{coreGroup})

	if !strings.Contains(md, "## core") {
		t.Error("missing core section")
	}
	// Should NOT contain other groups.
	if strings.Contains(md, "## session") {
		t.Error("should not contain session group")
	}
	// Should contain core tools.
	if !strings.Contains(md, "ralphglasses_scan") {
		t.Error("missing ralphglasses_scan tool")
	}
}

func TestExportSkillsFromGroups(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())
	groups := srv.buildToolGroups()

	skills := ExportSkillsFromGroups(groups)
	if len(skills) == 0 {
		t.Fatal("expected skills, got none")
	}

	// Each skill should have a name and category.
	for _, s := range skills {
		if s.Name == "" {
			t.Error("skill with empty name")
		}
		if s.Category == "" {
			t.Errorf("skill %q has empty category", s.Name)
		}
	}
}

func TestHandleSkillExport_DefaultMarkdown(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleSkillExport(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatal("unexpected error result")
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.HasPrefix(text, "# Ralphglasses Skills") {
		t.Error("expected markdown output")
	}
}

func TestHandleSkillExport_JSON(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"format": "json"}

	result, err := srv.handleSkillExport(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !json.Valid([]byte(text)) {
		t.Fatalf("invalid JSON: %s", text)
	}

	var skills []SkillDef
	if err := json.Unmarshal([]byte(text), &skills); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(skills) == 0 {
		t.Error("expected skills in JSON output")
	}
}

func TestHandleSkillExport_GroupFilter(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"group": "core"}

	result, err := srv.handleSkillExport(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "## core") {
		t.Error("expected core section in output")
	}
	if strings.Contains(text, "## session") {
		t.Error("should not contain session when filtering to core")
	}
}

func TestHandleSkillExport_InvalidGroup(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"group": "nonexistent"}

	result, err := srv.handleSkillExport(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for invalid group")
	}
}

func TestHandleSkillExport_InvalidFormat(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"format": "xml"}

	result, err := srv.handleSkillExport(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Error("expected error for invalid format")
	}
}

func TestExampleValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ  string
		want any
	}{
		{"string", "..."},
		{"number", 1},
		{"integer", 1},
		{"boolean", true},
		{"array", []any{}},
		{"object", map[string]any{}},
		{"unknown", "..."},
	}
	for _, tc := range cases {
		got := exampleValue(tc.typ)
		// Compare via JSON for slice/map types.
		gotJSON, _ := json.Marshal(got)
		wantJSON, _ := json.Marshal(tc.want)
		if string(gotJSON) != string(wantJSON) {
			t.Errorf("exampleValue(%q) = %v, want %v", tc.typ, got, tc.want)
		}
	}
}
