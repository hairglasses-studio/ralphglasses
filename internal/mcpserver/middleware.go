package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/tracing"
)

// InstrumentationMiddleware records timing, success, and size metrics for every
// tool call via a ToolCallRecorder. It also pushes counters to Prometheus when
// a PrometheusRecorder is configured.
func InstrumentationMiddleware(rec *ToolCallRecorder) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if rec == nil {
				return next(ctx, req)
			}

			start := time.Now()

			// Measure input size.
			var inputSize int
			if raw, err := json.Marshal(req.Params.Arguments); err == nil {
				inputSize = len(raw)
			}

			result, err := next(ctx, req)

			latency := time.Since(start)
			entry := ToolCallEntry{
				ToolName:  req.Params.Name,
				Timestamp: start,
				LatencyMs: latency.Milliseconds(),
				Success:   err == nil && result != nil && !result.IsError,
				InputSize: inputSize,
			}
			if err != nil {
				entry.ErrorMsg = err.Error()
			} else if result != nil && result.IsError {
				// Extract error text from result content.
				for _, c := range result.Content {
					if tc, ok := c.(mcp.TextContent); ok {
						entry.ErrorMsg = tc.Text
						break
					}
				}
			}
			if result != nil {
				if raw, merr := json.Marshal(result.Content); merr == nil {
					entry.OutputSize = len(raw)
				}
			}

			rec.Record(entry)

			// Structured log for every tool invocation.
			logAttrs := []any{
				"tool", entry.ToolName,
				"duration_ms", entry.LatencyMs,
				"success", entry.Success,
			}
			// Extract repo from request args when available for log correlation.
			if args := req.GetArguments(); args != nil {
				if repo, ok := args["repo"]; ok {
					if rs, ok := repo.(string); ok && rs != "" {
						logAttrs = append(logAttrs, "repo", rs)
					}
				}
			}
			if entry.ErrorMsg != "" {
				logAttrs = append(logAttrs, "error", entry.ErrorMsg)
			}
			if entry.Success {
				slog.InfoContext(ctx, "mcp.tool.call", logAttrs...)
			} else {
				slog.WarnContext(ctx, "mcp.tool.call", logAttrs...)
			}

			return result, err
		}
	}
}

// EventBusMiddleware publishes a "tool.called" event for every handler
// invocation, so the event bus captures all tool activity without modifying
// individual handlers.
func EventBusMiddleware(bus *events.Bus) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			start := time.Now()
			result, err := next(ctx, req)
			latency := time.Since(start)

			if bus != nil {
				success := err == nil && result != nil && !result.IsError
				bus.PublishCtx(ctx, events.Event{
					Type: events.ToolCalled,
					Data: map[string]any{
						"tool":       req.Params.Name,
						"success":    success,
						"latency_ms": latency.Milliseconds(),
					},
				})
			}

			return result, err
		}
	}
}

// ValidationMiddleware validates common parameters (repo, path) before the
// handler runs, returning invalidParams errors early.
func ValidationMiddleware(scanRoot string) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()

			if repo, ok := args["repo"]; ok {
				if s, ok := repo.(string); ok && s != "" {
					if filepath.IsAbs(s) {
						if err := ValidatePath(s, scanRoot); err != nil {
							return codedError(ErrInvalidParams, fmt.Sprintf("repo: %v", err)), nil
						}
					} else {
						if err := ValidateRepoName(s); err != nil {
							return codedError(ErrInvalidParams, fmt.Sprintf("repo: %v", err)), nil
						}
					}
				}
			}

			if p, ok := args["path"]; ok {
				if s, ok := p.(string); ok && s != "" {
					if err := ValidatePath(s, scanRoot); err != nil {
						return codedError(ErrInvalidParams, fmt.Sprintf("path: %v", err)), nil
					}
				}
			}

			return next(ctx, req)
		}
	}
}

// RecordToolCallPrometheus pushes a tool call metric to Prometheus.
func RecordToolCallPrometheus(prom *tracing.PrometheusRecorder, toolName string, latencyMs int64, success bool) {
	if prom == nil {
		return
	}
	status := "ok"
	if !success {
		status = "error"
	}
	prom.RecordToolCall(toolName, latencyMs, status)
}
