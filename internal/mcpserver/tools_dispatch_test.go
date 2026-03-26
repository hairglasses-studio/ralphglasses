package mcpserver

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestApplyToolMetadata_Annotations(t *testing.T) {
	t.Parallel()

	// Pick a tool name that has annotations defined.
	toolName := "ralphglasses_scan"
	tool := mcp.NewTool(toolName, mcp.WithDescription("test"))

	applyToolMetadata(&tool)

	ann, ok := ToolAnnotations[toolName]
	if !ok {
		t.Skipf("no annotation for %q — skip", toolName)
	}
	if tool.Annotations.Title != ann.Title {
		t.Errorf("Title = %q, want %q", tool.Annotations.Title, ann.Title)
	}
}

func TestApplyToolMetadata_OutputSchema(t *testing.T) {
	t.Parallel()

	// Find a tool that has an output schema.
	var toolName string
	for name := range OutputSchemas {
		toolName = name
		break
	}
	if toolName == "" {
		t.Skip("no output schemas defined — skip")
	}

	tool := mcp.NewTool(toolName, mcp.WithDescription("test"))
	applyToolMetadata(&tool)

	if len(tool.RawOutputSchema) == 0 {
		t.Errorf("expected RawOutputSchema to be set for %q", toolName)
	}
}

func TestApplyToolMetadata_UnknownTool(t *testing.T) {
	t.Parallel()
	tool := mcp.NewTool("unknown_tool_xyz", mcp.WithDescription("nope"))
	// Should not panic or mutate annotations for unknown tool.
	applyToolMetadata(&tool)
	if tool.Annotations.Title != "" {
		t.Errorf("expected empty Title for unknown tool, got %q", tool.Annotations.Title)
	}
}

func TestAddToolWithMetadata(t *testing.T) {
	t.Parallel()
	mcpSrv := server.NewMCPServer("test", "1.0")

	entry := ToolEntry{
		Tool: mcp.NewTool("ralphglasses_scan", mcp.WithDescription("Scan")),
		Handler: func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return textResult("ok"), nil
		},
	}

	// Should not panic.
	addToolWithMetadata(mcpSrv, entry)
}

func TestRegister_AllTools_DeferredFalse(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	s.DeferredLoading = false
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.Register(mcpSrv)

	// Every group should be loaded.
	for _, name := range ToolGroupNames {
		if !s.loadedGroups[name] {
			t.Errorf("group %q should be loaded when DeferredLoading=false", name)
		}
	}
}

func TestRegister_CoreOnly_DeferredTrue(t *testing.T) {
	t.Parallel()
	s := NewServer("/tmp/test")
	s.DeferredLoading = true
	mcpSrv := server.NewMCPServer("test", "1.0")
	s.Register(mcpSrv)

	if !s.loadedGroups["core"] {
		t.Error("core should be loaded in deferred mode")
	}
	for _, name := range ToolGroupNames {
		if name == "core" {
			continue
		}
		if s.loadedGroups[name] {
			t.Errorf("group %q should NOT be loaded in deferred mode", name)
		}
	}
}

func TestToolDescriptions_NonEmpty(t *testing.T) {
	t.Parallel()
	srv := NewServer("/tmp/test")
	groups := srv.ToolGroups()

	for _, g := range groups {
		for _, te := range g.Tools {
			if te.Tool.Description == "" {
				t.Errorf("tool %q in group %q has empty description", te.Tool.Name, g.Name)
			}
		}
	}
}
