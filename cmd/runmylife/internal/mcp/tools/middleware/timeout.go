package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
)

// TimeoutMiddleware enforces a per-call timeout.
func TimeoutMiddleware(defaultTimeout time.Duration) Middleware {
	return func(name string, td tools.ToolDefinition, next tools.ToolHandlerFunc) tools.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			timeout := defaultTimeout
			if td.Timeout > 0 {
				timeout = td.Timeout
			}
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			type result struct {
				res *mcp.CallToolResult
				err error
			}
			ch := make(chan result, 1)
			go func() {
				r, e := next(ctx, req)
				ch <- result{r, e}
			}()

			select {
			case <-ctx.Done():
				return mcp.NewToolResultError(fmt.Sprintf("[TIMEOUT] %s exceeded %s", name, timeout)), nil
			case r := <-ch:
				return r.res, r.err
			}
		}
	}
}
