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
// Per-tool overrides: a duration > 0 sets a custom timeout; 0 exempts the tool entirely.
func TimeoutMiddleware(defaultTimeout time.Duration, overrides map[string]time.Duration) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			timeout := defaultTimeout
			if d, ok := overrides[req.Params.Name]; ok {
				if d == 0 {
					return next(ctx, req) // exempt — no timeout
				}
				timeout = d
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
			case r := <-ch:
				return r.res, r.err
			case <-ctx.Done():
				return codedError(ErrInternal, fmt.Sprintf("handler timed out after %s for tool %s", timeout, req.Params.Name)), nil
			}
		}
	}
}
