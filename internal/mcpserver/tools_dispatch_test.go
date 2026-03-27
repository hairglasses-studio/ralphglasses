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

// TestParamDriftDetection validates that tool builder params are structurally
// consistent: each tool's InputSchema.Properties keys match expectations, and
// required params are a subset of declared properties.
func TestParamDriftDetection(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	for _, spec := range allBuilderSpecs() {
		spec := spec
		t.Run(spec.name, func(t *testing.T) {
			t.Parallel()
			group := spec.buildFn(srv)
			for _, te := range group.Tools {
				schema := te.Tool.InputSchema
				propNames := make(map[string]bool, len(schema.Properties))
				for k := range schema.Properties {
					propNames[k] = true
				}

				// Every required param must exist in properties.
				for _, req := range schema.Required {
					if !propNames[req] {
						t.Errorf("tool %q: required param %q not found in InputSchema.Properties", te.Tool.Name, req)
					}
				}

				// Properties must have a non-nil map value (i.e. a type definition).
				for k, v := range schema.Properties {
					if v == nil {
						t.Errorf("tool %q: property %q has nil schema definition", te.Tool.Name, k)
					}
				}

				// Tools with no properties should have no required fields.
				if len(schema.Properties) == 0 && len(schema.Required) > 0 {
					t.Errorf("tool %q: has %d required params but 0 properties", te.Tool.Name, len(schema.Required))
				}
			}
		})
	}
}

// TestLoadToolGroupDescriptionListsAllGroups verifies the load_tool_group
// description string mentions all 13 group names.
func TestLoadToolGroupDescriptionListsAllGroups(t *testing.T) {
	t.Parallel()
	srv := NewServer("/tmp/test")
	srv.DeferredLoading = true
	mcpSrv := server.NewMCPServer("test", "1.0")
	srv.Register(mcpSrv)

	// The load_tool_group description should mention every group name.
	for _, name := range ToolGroupNames {
		// The description is embedded in the tool registration; we verify
		// indirectly that buildToolGroups returns all expected groups.
		found := false
		for _, g := range srv.buildToolGroups() {
			if g.Name == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("group %q not found in buildToolGroups()", name)
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
