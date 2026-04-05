package memory

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// GatewayToolDefinition returns the memory gateway tool definition.
func GatewayToolDefinition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Tool: mcp.NewTool("webb_memory",
			mcp.WithDescription("Manage session memory: save/search team knowledge, generate insights, load context."),
			mcp.WithString("action",
				mcp.Required(),
				mcp.Enum("remember", "forget", "search", "list", "insight_generate", "insight_list", "context_load"),
				mcp.Description("Action: remember, forget, search, list, insight_generate, insight_list, context_load"),
			),
			mcp.WithString("content",
				mcp.Description("Knowledge to remember (required for remember action)"),
			),
			mcp.WithString("id_or_description",
				mcp.Description("Memory ID or description to match (required for forget action)"),
			),
			mcp.WithString("query",
				mcp.Description("Search query (required for search action)"),
			),
			mcp.WithString("category",
				mcp.Description("Memory category: team, service, workflow, standard, general"),
			),
			mcp.WithString("added_by",
				mcp.Description("Who added this memory (default: current user)"),
			),
			mcp.WithString("tags",
				mcp.Description("Comma-separated tags for remember action"),
			),
			mcp.WithString("session_id",
				mcp.Description("Session ID for insight_generate action"),
			),
			mcp.WithBoolean("save",
				mcp.Description("Save insight to memory for insight_generate (default: true)"),
			),
			mcp.WithString("service",
				mcp.Description("Filter by service for insight_list"),
			),
			mcp.WithString("customer",
				mcp.Description("Filter by customer for insight_list/context_load"),
			),
			mcp.WithString("cluster",
				mcp.Description("Filter by cluster for insight_list/context_load"),
			),
			mcp.WithString("symptoms",
				mcp.Description("Comma-separated symptoms for context_load"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum results (default varies by action)"),
			),
		),
		Handler:     handleMemoryGateway,
		Category:    "memory",
		Subcategory: "gateway",
		Tags:        []string{"memory", "learning", "insights", "context", "gateway"},
		UseCases:    []string{"Save team knowledge", "Search memories", "Generate insights", "Load investigation context"},
		Complexity:  tools.ComplexitySimple,
	}
}

func handleMemoryGateway(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action, err := req.RequireString("action")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("action parameter is required")), nil
	}

	switch action {
	case "remember":
		return handleRemember(ctx, req)
	case "forget":
		return handleForget(ctx, req)
	case "search":
		return handleSearch(ctx, req)
	case "list":
		return handleListMemories(ctx, req)
	case "insight_generate":
		return handleGenerateInsight(ctx, req)
	case "insight_list":
		return handleListInsights(ctx, req)
	case "context_load":
		return handleContextLoad(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("unknown action: %s (valid: remember, forget, search, list, insight_generate, insight_list, context_load)", action)), nil
	}
}
