package mcp

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// ConfigureHooks sets up observability hooks on the MCP server.
func ConfigureHooks() *server.Hooks {
	tracker := &metricsTracker{
		starts: &sync.Map{},
	}

	hooks := &server.Hooks{}

	hooks.AddBeforeCallTool(func(ctx context.Context, id any, message *mcp.CallToolRequest) {
		toolName := message.Params.Name
		tracker.starts.Store(id, time.Now())
		log.Printf("[runmylife-mcp] CALL tool=%s", toolName)
	})

	hooks.AddAfterCallTool(func(ctx context.Context, id any, message *mcp.CallToolRequest, result any) {
		toolName := message.Params.Name
		var durationMs int64
		if startVal, ok := tracker.starts.LoadAndDelete(id); ok {
			durationMs = time.Since(startVal.(time.Time)).Milliseconds()
		}

		var isError bool
		var errorType, errorMessage string
		if r, ok := result.(*mcp.CallToolResult); ok && r != nil {
			isError = r.IsError
			if r.IsError {
				for _, content := range r.Content {
					if tc, ok := content.(mcp.TextContent); ok {
						text := tc.Text
						if strings.HasPrefix(text, "[") {
							if idx := strings.Index(text, "]"); idx > 0 {
								errorType = text[1:idx]
								errorMessage = strings.TrimSpace(text[idx+1:])
							}
						} else {
							errorMessage = text
						}
						break
					}
				}
			}
		}
		log.Printf("[runmylife-mcp] DONE tool=%s duration=%dms error=%v", toolName, durationMs, isError)

		go recordToolMetric(toolName, durationMs, isError, errorType, errorMessage)
	})

	hooks.AddOnError(func(ctx context.Context, id any, method mcp.MCPMethod, message any, err error) {
		log.Printf("[runmylife-mcp] ERROR method=%s err=%v", method, err)
	})

	return hooks
}

type metricsTracker struct {
	starts *sync.Map
}

var (
	metricsDB     *sql.DB
	metricsDBOnce sync.Once
)

func getMetricsDB() *sql.DB {
	metricsDBOnce.Do(func() {
		database, err := common.OpenDB()
		if err != nil {
			log.Printf("[runmylife-mcp] metrics DB open error: %v", err)
			return
		}
		metricsDB = database.SqlDB()
	})
	return metricsDB
}

func recordToolMetric(toolName string, durationMs int64, isError bool, errorType, errorMessage string) {
	db := getMetricsDB()
	if db == nil {
		return
	}
	_, err := db.Exec(
		`INSERT INTO tool_metrics (tool_name, duration_ms, is_error, error_type, error_message, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		toolName, durationMs, isError, errorType, errorMessage, time.Now().UTC().Format(time.RFC3339),
	)
	if err != nil {
		log.Printf("[runmylife-mcp] record tool metric: %v", err)
	}
}
