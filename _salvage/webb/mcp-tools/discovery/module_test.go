package discovery

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
)

func TestModuleRegistration(t *testing.T) {
	m := &Module{}

	if m.Name() != "discovery" {
		t.Errorf("expected name 'discovery', got %s", m.Name())
	}

	toolDefs := m.Tools()
	// 4 base tools + 9 mcp_registry tools + 10 MCP interop tools + 4 scoring tools + 7 new tools = 34
	if len(toolDefs) != 34 {
		t.Errorf("expected 34 tools, got %d", len(toolDefs))
	}

	// Verify core discovery tools
	coreTools := map[string]bool{
		"webb_tool_discover": false,
		"webb_tool_schema":   false,
		"webb_tool_stats":    false,
		"webb_tool_help":     false,
	}

	// Verify MCP registry tools
	registryTools := map[string]bool{
		"webb_mcp_registry_list":     false,
		"webb_mcp_registry_add":      false,
		"webb_mcp_registry_remove":   false,
		"webb_mcp_registry_search":   false,
		"webb_mcp_registry_sync":     false,
		"webb_mcp_registry_export":   false,
		"webb_mcp_registry_update":   false,
		"webb_mcp_server_versions":   false,
		"webb_mcp_registry_scaffold": false,
	}

	for _, td := range toolDefs {
		if _, ok := coreTools[td.Tool.Name]; ok {
			coreTools[td.Tool.Name] = true
		}
		if _, ok := registryTools[td.Tool.Name]; ok {
			registryTools[td.Tool.Name] = true
		}
	}

	for name, found := range coreTools {
		if !found {
			t.Errorf("core tool %s not found", name)
		}
	}

	for name, found := range registryTools {
		if !found {
			t.Errorf("registry tool %s not found", name)
		}
	}
}

func TestToolDiscover(t *testing.T) {
	// Ensure registry has tools
	registry := tools.GetRegistry()
	if registry.GetToolStats().TotalTools == 0 {
		t.Skip("registry has no tools, skipping")
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"detail_level": "names",
		"limit":        float64(10),
	}

	result, err := handleToolDiscover(context.Background(), req)
	if err != nil {
		t.Fatalf("handleToolDiscover failed: %v", err)
	}

	if result == nil {
		t.Fatal("result is nil")
	}

	// Check result has content
	content := result.Content[0].(mcp.TextContent)
	if !strings.Contains(content.Text, "Tool Discovery") {
		t.Error("expected 'Tool Discovery' header in result")
	}
}

func TestToolStats(t *testing.T) {
	req := mcp.CallToolRequest{}

	result, err := handleToolStats(context.Background(), req)
	if err != nil {
		t.Fatalf("handleToolStats failed: %v", err)
	}

	content := result.Content[0].(mcp.TextContent)
	if !strings.Contains(content.Text, "Tool Registry Statistics") {
		t.Error("expected 'Tool Registry Statistics' header")
	}
	if !strings.Contains(content.Text, "Token Savings") {
		t.Error("expected 'Token Savings' section")
	}
}

func TestToolSchema(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"tool_names": "webb_tool_discover,nonexistent_tool",
	}

	result, err := handleToolSchema(context.Background(), req)
	if err != nil {
		t.Fatalf("handleToolSchema failed: %v", err)
	}

	content := result.Content[0].(mcp.TextContent)
	if !strings.Contains(content.Text, "Tool Schemas") {
		t.Error("expected 'Tool Schemas' header")
	}
	if !strings.Contains(content.Text, "Not found") {
		t.Error("expected 'Not found' for nonexistent tool")
	}
}
