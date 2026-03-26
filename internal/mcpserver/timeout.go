package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// TimeoutMiddleware wraps each handler with a context deadline.
// If the handler exceeds the timeout, a codedError is returned.
func TimeoutMiddleware(defaultTimeout time.Duration) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
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
			case r := <-ch:
				return r.res, r.err
			case <-ctx.Done():
				return codedError(ErrInternal, fmt.Sprintf("handler timed out after %s for tool %s", defaultTimeout, req.Params.Name)), nil
			}
		}
	}
}
