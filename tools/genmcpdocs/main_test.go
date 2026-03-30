package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testBuilderSource is a minimal Go source file that mirrors the real
// tools_builders pattern used in internal/mcpserver/.
const testBuilderSource = `package mcpserver

import "github.com/mark3labs/mcp-go/mcp"

func (s *Server) buildCoreGroup() ToolGroup {
	return ToolGroup{
		Name:        "core",
		Description: "Essential fleet management",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_scan",
				mcp.WithDescription("Scan for repos"),
			), s.handleScan},
			{mcp.NewTool("ralphglasses_status",
				mcp.WithDescription("Get status for a repo"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithBoolean("include_config", mcp.Description("Include config")),
				mcp.WithNumber("limit", mcp.Description("Max results")),
			), s.handleStatus},
		},
	}
}

func (s *Server) buildSessionGroup() ToolGroup {
	return ToolGroup{
		Name:        "session",
		Description: "LLM session lifecycle",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_session_launch",
				mcp.WithDescription("Launch a session"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Task prompt")),
				mcp.WithString("provider", mcp.Description("LLM provider")),
			), s.handleSessionLaunch},
		},
	}
}
`

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, "tools_builders.go", testBuilderSource)

	groups, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// Groups come out in AST order (core then session).
	core := groups[0]
	if core.Name != "core" {
		t.Errorf("expected group name 'core', got %q", core.Name)
	}
	if core.Description != "Essential fleet management" {
		t.Errorf("unexpected core description: %q", core.Description)
	}
	if len(core.Tools) != 2 {
		t.Fatalf("expected 2 core tools, got %d", len(core.Tools))
	}

	// First tool: no params.
	scan := core.Tools[0]
	if scan.Name != "ralphglasses_scan" {
		t.Errorf("expected scan tool, got %q", scan.Name)
	}
	if scan.Description != "Scan for repos" {
		t.Errorf("unexpected scan description: %q", scan.Description)
	}
	if len(scan.Params) != 0 {
		t.Errorf("expected 0 params for scan, got %d", len(scan.Params))
	}

	// Second tool: 3 params with different types and required flags.
	status := core.Tools[1]
	if status.Name != "ralphglasses_status" {
		t.Errorf("expected status tool, got %q", status.Name)
	}
	if len(status.Params) != 3 {
		t.Fatalf("expected 3 params for status, got %d", len(status.Params))
	}

	// Param: repo (string, required).
	p0 := status.Params[0]
	if p0.Name != "repo" || p0.Type != "string" || !p0.Required {
		t.Errorf("param 0: got %+v", p0)
	}
	// Param: include_config (boolean, optional).
	p1 := status.Params[1]
	if p1.Name != "include_config" || p1.Type != "boolean" || p1.Required {
		t.Errorf("param 1: got %+v", p1)
	}
	// Param: limit (number, optional).
	p2 := status.Params[2]
	if p2.Name != "limit" || p2.Type != "number" || p2.Required {
		t.Errorf("param 2: got %+v", p2)
	}
}

func TestParseDir(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "tools_builders.go", testBuilderSource)

	groups, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// ParseDir sorts by name: core, session.
	if groups[0].Name != "core" {
		t.Errorf("expected first group 'core', got %q", groups[0].Name)
	}
	if groups[1].Name != "session" {
		t.Errorf("expected second group 'session', got %q", groups[1].Name)
	}
}

func TestParseDirNoFiles(t *testing.T) {
	dir := t.TempDir()
	_, err := ParseDir(dir)
	if err == nil {
		t.Fatal("expected error for empty directory")
	}
	if !strings.Contains(err.Error(), "no tools_builders") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseFileIgnoresNonBuilders(t *testing.T) {
	src := `package mcpserver

func (s *Server) handleScan() {}
func helper() {}
func (s *Server) buildCoreGroup() ToolGroup {
	return ToolGroup{
		Name: "core",
		Description: "Core tools",
		Tools: []ToolEntry{},
	}
}
`
	dir := t.TempDir()
	path := writeTempFile(t, dir, "tools_builders.go", src)

	groups, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].Name != "core" {
		t.Errorf("got %q", groups[0].Name)
	}
}

func TestRender(t *testing.T) {
	groups := []Group{
		{
			Name:        "core",
			Description: "Essential tools",
			Tools: []Tool{
				{
					Name:        "ralphglasses_scan",
					Description: "Scan repos",
				},
				{
					Name:        "ralphglasses_status",
					Description: "Get status",
					Params: []Param{
						{Name: "repo", Type: "string", Required: true, Description: "Repo name"},
						{Name: "lines", Type: "number", Required: false, Description: "Line count"},
					},
				},
			},
		},
		{
			Name:        "session",
			Description: "Session lifecycle",
			Tools: []Tool{
				{
					Name:        "ralphglasses_session_launch",
					Description: "Launch session",
					Params: []Param{
						{Name: "repo", Type: "string", Required: true, Description: "Repo name"},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := Render(&buf, groups); err != nil {
		t.Fatalf("Render: %v", err)
	}

	md := buf.String()

	// Check header.
	if !strings.Contains(md, "# MCP Tool API Reference") {
		t.Error("missing title")
	}
	if !strings.Contains(md, "**3 tools**") {
		t.Errorf("missing tool count, got:\n%s", md)
	}
	if !strings.Contains(md, "**2 namespaces**") {
		t.Errorf("missing namespace count, got:\n%s", md)
	}

	// Check TOC.
	if !strings.Contains(md, "- [core]") {
		t.Error("missing core in TOC")
	}
	if !strings.Contains(md, "- [session]") {
		t.Error("missing session in TOC")
	}

	// Check tool heading.
	if !strings.Contains(md, "### `ralphglasses_scan`") {
		t.Error("missing scan tool heading")
	}
	if !strings.Contains(md, "*No parameters.*") {
		t.Error("missing no-params marker for scan")
	}

	// Check parameter table.
	if !strings.Contains(md, "| `repo` | string | **required** | Repo name |") {
		t.Error("missing required param row")
	}
	if !strings.Contains(md, "| `lines` | number | optional | Line count |") {
		t.Error("missing optional param row")
	}

	// Check namespace heading.
	if !strings.Contains(md, "## core") {
		t.Error("missing core namespace heading")
	}
	if !strings.Contains(md, "## session") {
		t.Error("missing session namespace heading")
	}
}

func TestRenderEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, nil); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(buf.String(), "**0 tools**") {
		t.Error("expected 0 tools in output")
	}
}

func TestParseRealBuilders(t *testing.T) {
	// Integration test: parse the actual mcpserver builder files.
	dir := filepath.Join(".", "..", "..", "internal", "mcpserver")
	if _, err := os.Stat(filepath.Join(dir, "tools_builders.go")); os.IsNotExist(err) {
		t.Skip("real builder files not found; skipping integration test")
	}

	groups, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}

	if len(groups) < 5 {
		t.Errorf("expected at least 5 groups from real files, got %d", len(groups))
	}

	// Count total tools.
	total := 0
	for _, g := range groups {
		total += len(g.Tools)
		if g.Name == "" {
			t.Error("found group with empty name")
		}
		if g.Description == "" {
			t.Errorf("group %q has empty description", g.Name)
		}
		for _, tool := range g.Tools {
			if tool.Name == "" {
				t.Errorf("group %q has tool with empty name", g.Name)
			}
			if tool.Description == "" {
				t.Errorf("tool %q has empty description", tool.Name)
			}
			if !strings.HasPrefix(tool.Name, "ralphglasses_") {
				t.Errorf("tool %q missing ralphglasses_ prefix", tool.Name)
			}
		}
	}

	if total < 50 {
		t.Errorf("expected at least 50 tools from real files, got %d", total)
	}

	// Verify render does not panic on real data.
	var buf bytes.Buffer
	if err := Render(&buf, groups); err != nil {
		t.Fatalf("Render on real data: %v", err)
	}
	if buf.Len() < 1000 {
		t.Errorf("rendered output suspiciously short: %d bytes", buf.Len())
	}
}

func TestUnquote(t *testing.T) {
	tests := []struct {
		name string
		fn   func() string
	}{
		{"empty string returns empty", func() string {
			// Test with nil-like input by calling unquote on non-string node
			return unquote(nil)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn()
			if got != "" {
				t.Errorf("expected empty, got %q", got)
			}
		})
	}
}

func TestNamespaceGrouping(t *testing.T) {
	// Test that multiple groups from different files merge and sort correctly.
	dir := t.TempDir()

	src1 := `package mcpserver
import "github.com/mark3labs/mcp-go/mcp"
func (s *Server) buildZetaGroup() ToolGroup {
	return ToolGroup{
		Name:        "zeta",
		Description: "Last namespace",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_z1",
				mcp.WithDescription("Z1 tool"),
			), s.handleZ1},
		},
	}
}
`
	src2 := `package mcpserver
import "github.com/mark3labs/mcp-go/mcp"
func (s *Server) buildAlphaGroup() ToolGroup {
	return ToolGroup{
		Name:        "alpha",
		Description: "First namespace",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_a1",
				mcp.WithDescription("A1 tool"),
				mcp.WithString("name", mcp.Required(), mcp.Description("The name")),
			), s.handleA1},
		},
	}
}
`

	writeTempFile(t, dir, "tools_builders_z.go", src1)
	writeTempFile(t, dir, "tools_builders_a.go", src2)

	groups, err := ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir: %v", err)
	}

	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}

	// Alphabetical: alpha before zeta.
	if groups[0].Name != "alpha" {
		t.Errorf("expected first group 'alpha', got %q", groups[0].Name)
	}
	if groups[1].Name != "zeta" {
		t.Errorf("expected second group 'zeta', got %q", groups[1].Name)
	}
}
