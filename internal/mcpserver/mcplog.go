package mcpserver

import (
	"context"
	"log/slog"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPLogger wraps the MCP server to provide spec-compliant logging.
// Per MCP spec (2025-11-25), logging uses notifications/message with levels:
// debug, info, notice, warning, error, critical, alert, emergency.
//
// When a client session is available in the context, log messages are sent as
// MCP notifications via SendLogMessageToClient. When no session is available
// (e.g., during tests or background work), messages fall back to slog.
type MCPLogger struct {
	srv  *server.MCPServer
	name string
}

// NewMCPLogger creates a new MCPLogger that sends notifications through srv
// with the given logger name (e.g., "ralphglasses").
func NewMCPLogger(srv *server.MCPServer, name string) *MCPLogger {
	return &MCPLogger{srv: srv, name: name}
}

// Debug logs a message at debug level.
func (l *MCPLogger) Debug(ctx context.Context, msg string, data map[string]any) {
	l.Log(ctx, mcp.LoggingLevelDebug, msg, data)
}

// Info logs a message at info level.
func (l *MCPLogger) Info(ctx context.Context, msg string, data map[string]any) {
	l.Log(ctx, mcp.LoggingLevelInfo, msg, data)
}

// Warn logs a message at warning level.
func (l *MCPLogger) Warn(ctx context.Context, msg string, data map[string]any) {
	l.Log(ctx, mcp.LoggingLevelWarning, msg, data)
}

// Error logs a message at error level.
func (l *MCPLogger) Error(ctx context.Context, msg string, data map[string]any) {
	l.Log(ctx, mcp.LoggingLevelError, msg, data)
}

// Log sends a structured log message at the given level. It constructs an MCP
// notifications/message notification per spec and attempts to deliver it to
// the connected client session. If delivery fails (no session, not initialized,
// channel blocked), it falls back to slog.
func (l *MCPLogger) Log(ctx context.Context, level mcp.LoggingLevel, msg string, data map[string]any) {
	// Merge msg into data payload so the notification carries both.
	payload := make(map[string]any, len(data)+1)
	for k, v := range data {
		if s, ok := v.(string); ok {
			payload[k] = RedactSecrets(s)
		} else {
			payload[k] = v
		}
	}
	payload["message"] = RedactSecrets(msg)

	notification := mcp.NewLoggingMessageNotification(level, l.name, payload)

	if l.srv != nil {
		err := l.srv.SendLogMessageToClient(ctx, notification)
		if err == nil {
			return
		}
		// Fall through to slog on any error (no session, not initialized, etc.)
	}

	// Fallback: emit via slog so logs are never silently dropped.
	attrs := make([]any, 0, len(payload)*2+2)
	attrs = append(attrs, "logger", l.name)
	for k, v := range payload {
		attrs = append(attrs, k, v)
	}

	switch level {
	case mcp.LoggingLevelDebug:
		slog.DebugContext(ctx, msg, attrs...)
	case mcp.LoggingLevelInfo, mcp.LoggingLevelNotice:
		slog.InfoContext(ctx, msg, attrs...)
	case mcp.LoggingLevelWarning:
		slog.WarnContext(ctx, msg, attrs...)
	default: // error, critical, alert, emergency
		slog.ErrorContext(ctx, msg, attrs...)
	}
}

// MCPLoggingMiddleware returns a middleware that logs tool calls via MCP
// notifications. Each call emits a log message containing the tool name,
// elapsed time, and error status.
func MCPLoggingMiddleware(logger *MCPLogger) server.ToolHandlerMiddleware {
	return func(next server.ToolHandlerFunc) server.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if logger == nil {
				return next(ctx, req)
			}

			toolName := req.Params.Name
			start := time.Now()

			result, err := next(ctx, req)

			elapsed := time.Since(start)
			level := mcp.LoggingLevelInfo
			isError := false

			if err != nil {
				level = mcp.LoggingLevelError
				isError = true
			} else if result != nil && result.IsError {
				level = mcp.LoggingLevelError
				isError = true
			}

			logger.Log(ctx, level, "tool call", map[string]any{
				"tool":       toolName,
				"elapsed_ms": elapsed.Milliseconds(),
				"is_error":   isError,
			})

			return result, err
		}
	}
}
