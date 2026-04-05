package middleware

import (
	"context"
	"errors"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/resilience"
)

// CircuitBreakerMiddleware wraps handlers with circuit breaker protection.
func CircuitBreakerMiddleware(reg *resilience.CircuitBreakerRegistry) Middleware {
	return func(name string, td tools.ToolDefinition, next tools.ToolHandlerFunc) tools.ToolHandlerFunc {
		if td.CircuitBreakerGroup == "" {
			return next
		}
		group := td.CircuitBreakerGroup

		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var result *mcp.CallToolResult
			var handlerErr error

			cbErr := reg.Execute(group, func() error {
				r, e := next(ctx, req)
				result = r
				handlerErr = e

				if e != nil {
					return e
				}
				if r != nil && r.IsError {
					return fmt.Errorf("tool returned error result")
				}
				return nil
			})

			if cbErr != nil && errors.Is(cbErr, resilience.ErrCircuitOpen) {
				return mcp.NewToolResultError(
					fmt.Sprintf("[API_ERROR] circuit open: %s — external API is failing, try again later", group),
				), nil
			}

			return result, handlerErr
		}
	}
}
