package middleware

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
)

// TruncationMiddleware caps response content at maxBytes.
func TruncationMiddleware(maxBytes int) Middleware {
	return func(name string, td tools.ToolDefinition, next tools.ToolHandlerFunc) tools.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			result, err := next(ctx, req)
			if err != nil || result == nil || maxBytes <= 0 {
				return result, err
			}

			totalBytes := 0
			for i, c := range result.Content {
				tc, ok := c.(mcp.TextContent)
				if !ok {
					continue
				}
				totalBytes += len(tc.Text)
				if totalBytes > maxBytes {
					overage := totalBytes - maxBytes
					cutAt := len(tc.Text) - overage
					if cutAt < 0 {
						cutAt = 0
					}
					tc.Text = tc.Text[:cutAt] + "\n\n[TRUNCATED — response exceeded limit]"
					result.Content[i] = tc
					result.Content = result.Content[:i+1]
					break
				}
			}
			return result, err
		}
	}
}
