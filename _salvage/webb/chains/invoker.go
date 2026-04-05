package chains

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	mcptools "github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// MCPToolInvoker implements ToolInvoker using the MCP tool registry
type MCPToolInvoker struct {
	registry *mcptools.ToolRegistry
}

// NewMCPToolInvoker creates a new MCP tool invoker
func NewMCPToolInvoker(registry *mcptools.ToolRegistry) *MCPToolInvoker {
	return &MCPToolInvoker{registry: registry}
}

// InvokeTool executes an MCP tool and returns its result
func (m *MCPToolInvoker) InvokeTool(ctx context.Context, toolName string, params map[string]interface{}) (map[string]interface{}, error) {
	// Find the tool in the registry
	toolDef, ok := m.registry.GetTool(toolName)
	if !ok {
		return nil, fmt.Errorf("tool %q not found", toolName)
	}

	// Build the request
	request := mcp.CallToolRequest{}
	request.Params.Name = toolName
	request.Params.Arguments = params

	// Execute the handler
	result, err := toolDef.Handler(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("tool %q execution failed: %w", toolName, err)
	}

	// Parse the result
	return m.parseResult(result)
}

func (m *MCPToolInvoker) parseResult(result *mcp.CallToolResult) (map[string]interface{}, error) {
	output := make(map[string]interface{})

	if result == nil {
		return output, nil
	}

	// Check for error
	if result.IsError {
		var errMsg string
		for _, content := range result.Content {
			if textContent, ok := content.(mcp.TextContent); ok {
				errMsg = textContent.Text
				break
			}
		}
		return nil, fmt.Errorf("tool returned error: %s", errMsg)
	}

	// Extract text content
	for i, content := range result.Content {
		switch c := content.(type) {
		case mcp.TextContent:
			if i == 0 {
				output["text"] = c.Text
				// Try to parse as JSON
				var jsonData map[string]interface{}
				if err := json.Unmarshal([]byte(c.Text), &jsonData); err == nil {
					for k, v := range jsonData {
						output[k] = v
					}
				}
			} else {
				output[fmt.Sprintf("text_%d", i)] = c.Text
			}
		case mcp.ImageContent:
			output[fmt.Sprintf("image_%d", i)] = c.Data
		case mcp.EmbeddedResource:
			output[fmt.Sprintf("resource_%d", i)] = c.Resource
		}
	}

	output["success"] = true
	return output, nil
}

// ToolExists checks if a tool is available
func (m *MCPToolInvoker) ToolExists(toolName string) bool {
	_, ok := m.registry.GetTool(toolName)
	return ok
}

// ListAvailableTools returns all available tool names
func (m *MCPToolInvoker) ListAvailableTools() []string {
	return m.registry.ListTools()
}
