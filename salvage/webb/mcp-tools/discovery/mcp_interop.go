package discovery

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/clients"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// MCPInteropTools returns MCP interoperability and standards tools (v13.5)
func MCPInteropTools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		// MCP Spec Compliance Check
		{
			Tool: mcp.NewTool("webb_mcp_spec_compliance",
				mcp.WithDescription("Check compliance with MCP specification. Validates required and optional features against the target spec version."),
				mcp.WithString("spec_version",
					mcp.Description("Target MCP spec version: '2025-06-18' (default) or '2025-11-25'"),
				),
			),
			Handler:     handleMCPSpecCompliance,
			Category:    "discovery",
			Subcategory: "mcp_standards",
			Tags:        []string{"mcp", "compliance", "spec", "standards", "interoperability"},
			UseCases:    []string{"check MCP compliance", "validate spec adherence", "audit interoperability"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// MCP Registry Status
		{
			Tool: mcp.NewTool("webb_mcp_registry_status",
				mcp.WithDescription("Check webb's listing status in the MCP Registry at registry.modelcontextprotocol.io. Shows listing status and recommendations."),
			),
			Handler:     handleMCPRegistryStatus,
			Category:    "discovery",
			Subcategory: "mcp_standards",
			Tags:        []string{"mcp", "registry", "status", "listing"},
			UseCases:    []string{"check registry listing", "verify discoverability", "get submission guidance"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// MCP Server Capabilities
		{
			Tool: mcp.NewTool("webb_mcp_capabilities",
				mcp.WithDescription("List server capabilities for MCP discovery. Shows tools, resources, prompts, experimental features, and extensions."),
			),
			Handler:     handleMCPCapabilities,
			Category:    "discovery",
			Subcategory: "mcp_standards",
			Tags:        []string{"mcp", "capabilities", "discovery", "features"},
			UseCases:    []string{"view server capabilities", "check feature support", "capability discovery"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
	}
}

// handleMCPSpecCompliance checks compliance with MCP specification
func handleMCPSpecCompliance(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	specVersion := req.GetString("spec_version", "2025-06-18")

	client := clients.NewMCPComplianceClient()
	report := client.CheckCompliance(ctx, specVersion)

	output := clients.FormatComplianceReport(report)
	return tools.TextResult(output), nil
}

// handleMCPRegistryStatus checks webb's listing status in MCP Registry
func handleMCPRegistryStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client := clients.NewMCPComplianceClient()
	status := client.CheckRegistryStatus(ctx)

	output := clients.FormatRegistryStatus(status)
	return tools.TextResult(output), nil
}

// handleMCPCapabilities lists server capabilities
func handleMCPCapabilities(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client := clients.NewMCPComplianceClient()
	caps := client.GetServerCapabilities()

	output := clients.FormatServerCapabilities(caps)
	return tools.TextResult(output), nil
}
