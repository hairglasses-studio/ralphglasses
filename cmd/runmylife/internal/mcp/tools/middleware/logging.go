package middleware

import (
	"context"
	"database/sql"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
)

type metricsRecorder struct {
	mu   sync.Mutex
	db   *sql.DB
	open func() (*sql.DB, error)
}

var errorCodePattern = regexp.MustCompile(`^\[([A-Z_]+)\]`)

func (r *metricsRecorder) ensureDB() (*sql.DB, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.db != nil {
		return r.db, nil
	}
	db, err := r.open()
	if err != nil {
		return nil, err
	}
	r.db = db
	return r.db, nil
}

func (r *metricsRecorder) record(toolName string, durationMs int64, isError bool, errorType, errorMsg string) {
	db, err := r.ensureDB()
	if err != nil {
		return
	}
	_, err = db.Exec(`
		INSERT INTO tool_metrics (tool_name, duration_ms, is_error, error_type, error_message)
		VALUES (?, ?, ?, ?, ?)`,
		toolName, durationMs, isError, errorType, errorMsg,
	)
	if err != nil {
		log.Printf("[runmylife] record tool metrics: %v", err)
	}
}

func extractErrorInfo(result *mcp.CallToolResult) (string, string) {
	if result == nil || !result.IsError || len(result.Content) == 0 {
		return "", ""
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		return "", ""
	}
	text := tc.Text
	if m := errorCodePattern.FindStringSubmatch(text); len(m) == 2 {
		msg := strings.TrimSpace(text[len(m[0]):])
		if len(msg) > 200 {
			msg = msg[:200]
		}
		return m[1], msg
	}
	if len(text) > 200 {
		text = text[:200]
	}
	return "", text
}

// LoggingMiddleware logs tool invocations with duration and error status.
func LoggingMiddleware(dbOpener func() (*sql.DB, error)) Middleware {
	var recorder *metricsRecorder
	if dbOpener != nil {
		recorder = &metricsRecorder{open: dbOpener}
	}

	return func(name string, td tools.ToolDefinition, next tools.ToolHandlerFunc) tools.ToolHandlerFunc {
		return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			start := time.Now()
			result, err := next(ctx, req)
			duration := time.Since(start)

			isError := false
			if result != nil {
				isError = result.IsError
			}
			log.Printf("[runmylife] tool=%s duration=%dms error=%v", name, duration.Milliseconds(), isError)

			if recorder != nil {
				durationMs := duration.Milliseconds()
				errorType, errorMsg := extractErrorInfo(result)
				go func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[runmylife] metrics recorder panic: %v", r)
						}
					}()
					recorder.record(name, durationMs, isError, errorType, errorMsg)
				}()
			}

			return result, err
		}
	}
}
