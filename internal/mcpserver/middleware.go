package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/sync/semaphore"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/tracing"
)

// DefaultMaxConcurrent is the default number of concurrent MCP tool handlers.
const DefaultMaxConcurrent = 32

// ConcurrencyMiddleware limits the number of concurrent MCP tool handler
// executions using a weighted semaphore. When all slots are occupied, incoming
// requests block until the context is cancelled (e.g. timeout), at which point
// an ErrRateLimited error is returned. The limit is configurable via the
// RG_MCP_MAX_CONCURRENT environment variable; 0 or negative disables the limit.
func ConcurrencyMiddleware(limit int64) server.ToolHandlerMiddleware {
	if e := os.Getenv("RG_MCP_MAX_CONCURRENT"); e != "" {
		if n, err := strconv.ParseInt(e, 10, 64); err == nil {
			limit = n
		}
	}
	if limit <= 0 {
		return func(next server.ToolHandlerFunc) server.ToolHandlerFunc { return next }
	}
	sem := semaphore.NewWeighted(limit)
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if err := sem.Acquire(ctx, 1); err != nil {
				slog.WarnContext(ctx, "mcp.concurrency.rejected",
					"tool", req.Params.Name,
					"limit", limit,
				)
				return codedError(ErrRateLimited, fmt.Sprintf(
					"too many concurrent requests (limit %d); try again shortly", limit,
				)), nil
			}
			defer sem.Release(1)
			return next(ctx, req)
		}
	}
}

// TraceMiddleware generates a trace ID for each tool call and propagates it
// via context. If the context already carries a trace ID (e.g. from an
// upstream caller), it is preserved. The trace ID is included in structured
// log output for end-to-end observability.
func TraceMiddleware() server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			traceID := tracing.TraceIDFromContext(ctx)
			if traceID == "" {
				traceID = tracing.NewTraceID()
				ctx = tracing.WithTraceID(ctx, traceID)
			}

			slog.InfoContext(ctx, "mcp.tool.trace",
				"trace_id", traceID,
				"tool", req.Params.Name,
			)

			result, err := next(ctx, req)
			if result != nil {
				injectTraceID(result, traceID)
			}
			return result, err
		}
	}
}

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

			// Inject request-scoped context fields for handler logging.
			ctx = tracing.WithToolName(ctx, req.Params.Name)
			ctx = tracing.WithRequestStart(ctx, start)
			if args := req.GetArguments(); args != nil {
				if repo, ok := args["repo"]; ok {
					if rs, ok := repo.(string); ok && rs != "" {
						ctx = tracing.WithRepo(ctx, rs)
					}
				}
			}

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
				// Estimate output size from text content lengths to avoid
				// marshalling the full response just for metrics.
				for _, c := range result.Content {
					if tc, ok := c.(mcp.TextContent); ok {
						entry.OutputSize += len(tc.Text)
					}
				}
			}

			rec.Record(entry)

			// Structured log for every tool invocation.
			logAttrs := []any{
				"tool", entry.ToolName,
				"duration_ms", entry.LatencyMs,
				"success", entry.Success,
			}
			if tid := tracing.TraceIDFromContext(ctx); tid != "" {
				logAttrs = append(logAttrs, "trace_id", tid)
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

// injectTraceID appends a _trace_id metadata field to the tool result.
// If the first content item is a TextContent containing JSON, the trace ID is
// merged into the JSON object. Otherwise a separate text content block is appended.
func injectTraceID(result *mcp.CallToolResult, traceID string) {
	if traceID == "" || len(result.Content) == 0 {
		return
	}

	// Try to merge into the first JSON text content.
	for i, c := range result.Content {
		tc, ok := c.(mcp.TextContent)
		if !ok {
			continue
		}
		text := tc.Text
		if len(text) < 2 || text[0] != '{' {
			continue
		}
		// Parse existing JSON, add _trace_id, re-marshal.
		var m map[string]any
		if err := json.Unmarshal([]byte(text), &m); err != nil {
			continue
		}
		m["_trace_id"] = traceID
		data, err := json.Marshal(m)
		if err != nil {
			continue
		}
		result.Content[i] = mcp.TextContent{
			Type: "text",
			Text: string(data),
		}
		return
	}

	// Fallback: append a metadata text block.
	result.Content = append(result.Content, mcp.TextContent{
		Type: "text",
		Text: fmt.Sprintf(`{"_trace_id": %q}`, traceID),
	})
}

// DefaultMaxResponseSize is the default maximum response size in bytes (4KB).
// Responses exceeding this are truncated to reduce JSON parse failures in
// downstream LLM consumers (root cause #1 of the 25.7% retry rate).
const DefaultMaxResponseSize = 4096

// ResponseSizeLimitMiddleware truncates tool responses that exceed maxBytes.
// When a response is truncated, the original content is replaced with a
// truncated version plus a metadata note indicating the original size.
// This addresses the #1 root cause (35%) of JSON retry failures: responses
// over 4KB causing parse failures in LLM consumers.
func ResponseSizeLimitMiddleware(maxBytes int) server.ToolHandlerMiddleware {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxResponseSize
	}
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			result, err := next(ctx, req)
			if err != nil || result == nil {
				return result, err
			}

			// Measure total text content size.
			totalSize := 0
			for _, c := range result.Content {
				if tc, ok := c.(mcp.TextContent); ok {
					totalSize += len(tc.Text)
				}
			}

			if totalSize <= maxBytes {
				return result, nil
			}

			slog.WarnContext(ctx, "mcp.response.truncated",
				"tool", req.Params.Name,
				"original_bytes", totalSize,
				"max_bytes", maxBytes,
			)

			// Truncate text content to fit within the budget.
			truncated := truncateResponseContent(result.Content, maxBytes, totalSize)
			result.Content = truncated
			return result, nil
		}
	}
}

// truncateResponseContent reduces text content to fit within maxBytes,
// preserving as much of the first content block as possible and appending
// a truncation notice with the original size.
func truncateResponseContent(content []mcp.Content, maxBytes, originalSize int) []mcp.Content {
	notice := fmt.Sprintf("\n[TRUNCATED: response was %d bytes, showing first %d]", originalSize, maxBytes)
	budget := maxBytes - len(notice)
	if budget < 0 {
		budget = 0
	}

	var result []mcp.Content
	remaining := budget

	for _, c := range content {
		tc, ok := c.(mcp.TextContent)
		if !ok {
			// Preserve non-text content (images, etc.) as-is.
			result = append(result, c)
			continue
		}

		if remaining <= 0 {
			// Budget exhausted; skip remaining text blocks.
			continue
		}

		text := tc.Text
		if len(text) > remaining {
			text = text[:remaining]
		}
		remaining -= len(text)
		result = append(result, mcp.TextContent{
			Type: "text",
			Text: text,
		})
	}

	// Append truncation notice as a separate content block.
	result = append(result, mcp.TextContent{
		Type: "text",
		Text: notice,
	})

	return result
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
