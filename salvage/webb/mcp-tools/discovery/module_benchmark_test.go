package discovery

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"

	// Import all modules to trigger registration for benchmarks
	_ "github.com/hairglasses-studio/webb/internal/mcp/tools/aws"
	_ "github.com/hairglasses-studio/webb/internal/mcp/tools/collaboration"
	_ "github.com/hairglasses-studio/webb/internal/mcp/tools/consolidated"
	_ "github.com/hairglasses-studio/webb/internal/mcp/tools/database"
	_ "github.com/hairglasses-studio/webb/internal/mcp/tools/devops"
	_ "github.com/hairglasses-studio/webb/internal/mcp/tools/devtools"
	_ "github.com/hairglasses-studio/webb/internal/mcp/tools/kubernetes"
	_ "github.com/hairglasses-studio/webb/internal/mcp/tools/operations"
	_ "github.com/hairglasses-studio/webb/internal/mcp/tools/presentations"
	_ "github.com/hairglasses-studio/webb/internal/mcp/tools/slack"
	_ "github.com/hairglasses-studio/webb/internal/mcp/tools/tickets"
)

// mockRequest creates a mock MCP request with the given parameters
func mockRequest(params map[string]interface{}) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: params,
		},
	}
}

// BenchmarkHandleToolDiscoverNames measures discovery with names-only detail
func BenchmarkHandleToolDiscoverNames(b *testing.B) {
	ctx := context.Background()
	req := mockRequest(map[string]interface{}{
		"detail_level": "names",
		"limit":        50,
	})

	// Warm up
	handleToolDiscover(ctx, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleToolDiscover(ctx, req)
	}
}

// BenchmarkHandleToolDiscoverDescriptions measures discovery with descriptions
func BenchmarkHandleToolDiscoverDescriptions(b *testing.B) {
	ctx := context.Background()
	req := mockRequest(map[string]interface{}{
		"detail_level": "descriptions",
		"limit":        50,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleToolDiscover(ctx, req)
	}
}

// BenchmarkHandleToolDiscoverFull measures discovery with full schemas
func BenchmarkHandleToolDiscoverFull(b *testing.B) {
	ctx := context.Background()
	req := mockRequest(map[string]interface{}{
		"detail_level": "full",
		"limit":        10, // Limited because full is expensive
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleToolDiscover(ctx, req)
	}
}

// BenchmarkHandleToolDiscoverWithCategory measures discovery with category filter
func BenchmarkHandleToolDiscoverWithCategory(b *testing.B) {
	ctx := context.Background()
	req := mockRequest(map[string]interface{}{
		"detail_level": "descriptions",
		"category":     "kubernetes",
		"limit":        50,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleToolDiscover(ctx, req)
	}
}

// BenchmarkHandleToolDiscoverWithSearch measures discovery with search filter
func BenchmarkHandleToolDiscoverWithSearch(b *testing.B) {
	ctx := context.Background()
	req := mockRequest(map[string]interface{}{
		"detail_level": "descriptions",
		"search":       "health",
		"limit":        50,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleToolDiscover(ctx, req)
	}
}

// BenchmarkHandleToolDiscoverPaginated measures discovery with pagination
func BenchmarkHandleToolDiscoverPaginated(b *testing.B) {
	ctx := context.Background()
	req := mockRequest(map[string]interface{}{
		"detail_level": "names",
		"limit":        50,
		"offset":       100,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleToolDiscover(ctx, req)
	}
}

// BenchmarkHandleToolSchemaSingle measures schema lookup for one tool
func BenchmarkHandleToolSchemaSingle(b *testing.B) {
	ctx := context.Background()
	req := mockRequest(map[string]interface{}{
		"tool_names": "webb_cluster_health_full",
	})

	// Warm up
	handleToolSchema(ctx, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleToolSchema(ctx, req)
	}
}

// BenchmarkHandleToolSchemaMultiple measures schema lookup for multiple tools
func BenchmarkHandleToolSchemaMultiple(b *testing.B) {
	ctx := context.Background()
	req := mockRequest(map[string]interface{}{
		"tool_names": "webb_cluster_health_full,webb_k8s_pods,webb_ticket_summary,webb_slack_search,webb_pylon_list",
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleToolSchema(ctx, req)
	}
}

// BenchmarkHandleToolStats measures stats generation
func BenchmarkHandleToolStats(b *testing.B) {
	ctx := context.Background()
	req := mockRequest(map[string]interface{}{})

	// Warm up
	handleToolStats(ctx, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handleToolStats(ctx, req)
	}
}

// BenchmarkFormatFullSchema measures full schema formatting
func BenchmarkFormatFullSchema(b *testing.B) {
	// Get a tool definition from the registry
	registry := tools.GetRegistry()
	td, ok := registry.GetTool("webb_cluster_health_full")
	if !ok {
		b.Skip("Tool not found")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		formatFullSchema(td, false)
	}
}

// BenchmarkTruncateDescription measures description truncation
func BenchmarkTruncateDescription(b *testing.B) {
	longDesc := "This is a very long description that needs to be truncated for display purposes. It contains multiple sentences and goes on for quite a while to test the truncation logic properly."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		truncateDescription(longDesc, 100)
	}
}
