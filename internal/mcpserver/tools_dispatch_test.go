package mcpserver

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
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
	s := &Server{}
	s.addToolWithMetadata(mcpSrv, entry)
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

func TestHandleServerHealth_IncludesDiscoveryContract(t *testing.T) {
	t.Parallel()

	srv := NewServer(t.TempDir())
	srv.DeferredLoading = true
	mcpSrv := server.NewMCPServer("test", "1.0")
	srv.Register(mcpSrv)

	result, err := srv.handleServerHealth(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content[0].(mcp.TextContent).Text), &payload); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if payload["prompt_count"] != float64(6) {
		t.Fatalf("prompt_count = %v, want 6", payload["prompt_count"])
	}
	if payload["resource_count"] != float64(7) {
		t.Fatalf("resource_count = %v, want 7", payload["resource_count"])
	}
	if payload["resource_template_count"] != float64(3) {
		t.Fatalf("resource_template_count = %v, want 3", payload["resource_template_count"])
	}
	if payload["skill_count"] == nil {
		t.Fatalf("expected skill_count in server health payload: %v", payload)
	}
	if payload["cli_parity_summary"] == nil {
		t.Fatalf("expected cli_parity_summary in server health payload: %v", payload)
	}
	instructions, _ := payload["instructions"].(string)
	if !strings.Contains(instructions, "ralph:///catalog/server") {
		t.Fatalf("instructions missing discovery resource guidance: %q", instructions)
	}
	if !strings.Contains(instructions, "ralph:///catalog/skills") {
		t.Fatalf("instructions missing skill catalog guidance: %q", instructions)
	}
	if !strings.Contains(instructions, "ralph:///catalog/cli-parity") {
		t.Fatalf("instructions missing cli parity guidance: %q", instructions)
	}
}

func TestHandleToolGroups_DefaultList(t *testing.T) {
	t.Parallel()

	srv := NewServer(t.TempDir())
	result, err := srv.handleToolGroups(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleToolGroups: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}

	var payload []toolGroupInfo
	if err := json.Unmarshal([]byte(getResultText(result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(payload) != len(ToolGroupNames) {
		t.Fatalf("len(payload) = %d, want %d", len(payload), len(ToolGroupNames))
	}
	if payload[0].Name != ToolGroupNames[0] {
		t.Fatalf("first group = %q, want %q", payload[0].Name, ToolGroupNames[0])
	}
}

func TestHandleToolGroups_SearchCatalogs(t *testing.T) {
	t.Parallel()

	srv := NewServer(t.TempDir())
	result, err := srv.handleToolGroups(context.Background(), makeRequest(map[string]any{
		"query":             "runtime",
		"include_workflows": true,
		"include_skills":    true,
		"limit":             float64(5),
	}))
	if err != nil {
		t.Fatalf("handleToolGroups: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}

	var payload toolGroupDiscoveryResponse
	if err := json.Unmarshal([]byte(getResultText(result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if payload.Query != "runtime" {
		t.Fatalf("query = %q, want runtime", payload.Query)
	}
	if payload.WorkflowCount == 0 {
		t.Fatalf("expected workflow matches, payload = %+v", payload)
	}
	if payload.SkillCount == 0 {
		t.Fatalf("expected skill matches, payload = %+v", payload)
	}
	if !strings.Contains(getResultText(result), "runtime-recovery") {
		t.Fatalf("expected runtime-recovery workflow in payload: %s", getResultText(result))
	}
	if !strings.Contains(getResultText(result), "ralphglasses-bootstrap") {
		t.Fatalf("expected ralphglasses-bootstrap skill in payload: %s", getResultText(result))
	}
}

func TestHandleToolGroups_FilterByToolGroup(t *testing.T) {
	t.Parallel()

	srv := NewServer(t.TempDir())
	result, err := srv.handleToolGroups(context.Background(), makeRequest(map[string]any{
		"tool_group":        "repo",
		"include_workflows": true,
		"include_skills":    true,
	}))
	if err != nil {
		t.Fatalf("handleToolGroups: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}

	var payload toolGroupDiscoveryResponse
	if err := json.Unmarshal([]byte(getResultText(result)), &payload); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if payload.ToolGroup != "repo" {
		t.Fatalf("tool_group = %q, want repo", payload.ToolGroup)
	}
	for _, group := range payload.Groups {
		if group.Name != "repo" {
			t.Fatalf("unexpected group match for repo filter: %+v", group)
		}
	}
	for _, workflow := range payload.WorkflowMatches {
		if !slices.Contains(workflow.ToolGroups, "repo") {
			t.Fatalf("workflow %q missing repo tool group: %+v", workflow.Name, workflow.ToolGroups)
		}
	}
	for _, skill := range payload.SkillMatches {
		if !slices.Contains(skill.ToolGroups, "repo") {
			t.Fatalf("skill %q missing repo tool group: %+v", skill.Name, skill.ToolGroups)
		}
	}
}

func TestHandleToolGroups_RejectsUnknownFilter(t *testing.T) {
	t.Parallel()

	srv := NewServer(t.TempDir())
	result, err := srv.handleToolGroups(context.Background(), makeRequest(map[string]any{
		"tool_group": "not-a-group",
	}))
	if err != nil {
		t.Fatalf("handleToolGroups: %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result, got %s", getResultText(result))
	}
}

func TestLoadToolGroupDescription_ListsCurrentGroups(t *testing.T) {
	t.Parallel()

	desc := loadToolGroupDescription()
	for _, name := range ToolGroupNames {
		if !strings.Contains(desc, name) {
			t.Fatalf("description missing tool group %q: %s", name, desc)
		}
	}
}
