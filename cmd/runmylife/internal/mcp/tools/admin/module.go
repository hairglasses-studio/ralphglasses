// Package admin provides MCP tools for database administration.
package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/jobs"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for admin tools.
type Module struct{}

func (m *Module) Name() string        { return "admin" }
func (m *Module) Description() string { return "Database administration and system health" }

var adminHints = map[string]string{
	"db/status":       "Show table row counts and database health",
	"db/backup":       "Export full database backup to JSON",
	"config/show":     "Show current configuration",
	"config/validate": "Validate configuration and report warnings",
	"health/check":    "Run system health check",
	"jobs/list":       "List queued jobs by status",
	"jobs/stats":      "Job queue statistics",
	"jobs/retry":      "Retry a failed or dead job",
	"jobs/clear":      "Clear completed jobs older than N days",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("admin").
		Domain("db", common.ActionRegistry{
			"status": handleDBStatus,
			"backup": handleDBBackup,
		}).
		Domain("config", common.ActionRegistry{
			"show":     handleConfigShow,
			"validate": handleConfigValidate,
		}).
		Domain("health", common.ActionRegistry{
			"check": handleHealthCheck,
		}).
		Domain("jobs", common.ActionRegistry{
			"list":  handleJobsList,
			"stats": handleJobsStats,
			"retry": handleJobsRetry,
			"clear": handleJobsClear,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_admin",
				mcp.WithDescription(
					"Administration gateway.\n\n"+
						dispatcher.DescribeActionsWithHints(adminHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: db, config, health, jobs")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("status", mcp.Description("Job status filter: pending, running, completed, failed, dead")),
				mcp.WithNumber("job_id", mcp.Description("Job ID for retry")),
				mcp.WithNumber("days", mcp.Description("Clear completed jobs older than N days (default 7)")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 50)")),
			),
			Handler:    tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:   "admin",
			Tags:       []string{"admin", "maintenance", "health"},
			Complexity: tools.ComplexityModerate,
			Timeout:    120 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func handleDBStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	tables := []string{
		"tasks", "gmail_messages", "calendar_events", "tool_usage", "tool_metrics", "sync_history",
		"contacts", "habits", "habit_completions", "transactions", "budgets",
		"sms_conversations", "sms_messages", "discord_servers", "discord_channels", "discord_messages",
		"drive_files", "notion_databases", "notion_pages", "weather_cache",
		"entity_links", "daily_snapshots", "job_queue",
		"spotify_tracks", "spotify_playlists", "reddit_saved", "reddit_subscriptions",
		"ha_entities", "ha_automations", "fitness_activities", "fitness_daily_stats", "fitness_sleep",
		"readwise_books", "readwise_highlights", "bluesky_posts", "bluesky_follows",
		"google_task_lists", "google_tasks", "clockify_projects", "clockify_entries",
	}
	counts := make(map[string]int)

	for _, table := range tables {
		var count int
		err := database.SqlDB().QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
		if err != nil {
			counts[table] = -1
		} else {
			counts[table] = count
		}
	}

	data, _ := json.MarshalIndent(counts, "", "  ")
	md := common.NewMarkdownBuilder().Title("Database Status")
	md.Text(fmt.Sprintf("```json\n%s\n```", string(data)))

	return tools.TextResult(md.String()), nil
}

func handleDBBackup(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	tables := []string{"tasks", "gmail_messages", "calendar_events", "sync_history"}
	backup := make(map[string]int)

	for _, table := range tables {
		var count int
		database.SqlDB().QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&count)
		backup[table] = count
	}

	data, _ := json.MarshalIndent(map[string]interface{}{
		"status":    "backup_ready",
		"tables":    backup,
		"timestamp": time.Now().Format(time.RFC3339),
	}, "", "  ")

	return tools.TextResult(string(data)), nil
}

func handleConfigShow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return common.CodedErrorResult(common.ErrConfig, err), nil
	}

	// Redact credential values
	safe := map[string]interface{}{
		"db_path":  cfg.DBPath,
		"location": cfg.Location,
		"credentials": func() map[string]string {
			redacted := make(map[string]string)
			for k := range cfg.Credentials {
				redacted[k] = "***"
			}
			return redacted
		}(),
	}

	data, _ := json.MarshalIndent(safe, "", "  ")
	return tools.TextResult(string(data)), nil
}

func handleConfigValidate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return common.CodedErrorResult(common.ErrConfig, err), nil
	}

	warnings := config.Validate(cfg)
	if len(warnings) == 0 {
		return tools.TextResult("Configuration is valid. No warnings."), nil
	}

	md := common.NewMarkdownBuilder().Title("Configuration Warnings")
	md.List(warnings)
	return tools.TextResult(md.String()), nil
}

func handleHealthCheck(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	md := common.NewMarkdownBuilder().Title("System Health")

	// DB check
	database, err := common.OpenDB()
	if err != nil {
		md.KeyValue("Database", "ERROR: "+err.Error())
	} else {
		defer database.Close()
		var count int
		database.SqlDB().QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
		md.KeyValue("Database", fmt.Sprintf("OK (%d migrations applied)", count))
	}

	// Config check
	cfg, err := config.Load()
	if err != nil {
		md.KeyValue("Config", "ERROR: "+err.Error())
	} else {
		warnings := config.Validate(cfg)
		if len(warnings) == 0 {
			md.KeyValue("Config", "OK")
		} else {
			md.KeyValue("Config", fmt.Sprintf("%d warnings", len(warnings)))
		}
	}

	// Registry check
	registry := tools.GetRegistry()
	stats := registry.GetToolStats()
	md.KeyValue("Tools", fmt.Sprintf("%d tools across %d modules", stats.TotalTools, stats.ModuleCount))

	return tools.TextResult(md.String()), nil
}

func handleJobsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	if err := jobs.EnsureTable(db); err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	status := common.GetStringParam(req, "status", "")
	limit := common.GetLimitParam(req, 50)

	jobList, err := jobs.ListAll(ctx, db, status, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	md := common.NewMarkdownBuilder().Title("Job Queue")
	if len(jobList) == 0 {
		md.EmptyList("jobs")
	} else {
		headers := []string{"ID", "Type", "Status", "Priority", "Attempts", "Next Run", "Error"}
		var rows [][]string
		for _, j := range jobList {
			errMsg := j.ErrorMessage
			if len(errMsg) > 50 {
				errMsg = errMsg[:50] + "..."
			}
			rows = append(rows, []string{
				fmt.Sprintf("%d", j.ID), j.Type, j.Status,
				fmt.Sprintf("%d", j.Priority), fmt.Sprintf("%d/%d", j.Attempts, j.MaxAttempts),
				j.NextRunAt, errMsg,
			})
		}
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleJobsStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	if err := jobs.EnsureTable(db); err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	pending, running, completed, failed, dead, err := jobs.GetStats(ctx, db)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	md := common.NewMarkdownBuilder().Title("Job Queue Stats")
	md.KeyValue("Pending", fmt.Sprintf("%d", pending))
	md.KeyValue("Running", fmt.Sprintf("%d", running))
	md.KeyValue("Completed", fmt.Sprintf("%d", completed))
	md.KeyValue("Failed", fmt.Sprintf("%d", failed))
	md.KeyValue("Dead", fmt.Sprintf("%d", dead))
	md.KeyValue("Total", fmt.Sprintf("%d", pending+running+completed+failed+dead))
	return tools.TextResult(md.String()), nil
}

func handleJobsRetry(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	jobID := int64(common.GetIntParam(req, "job_id", 0))
	if jobID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "job_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	if err := jobs.RetryJob(ctx, database.SqlDB(), jobID); err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("Job **%d** reset to pending.", jobID)), nil
}

func handleJobsClear(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	days := common.GetIntParam(req, "days", 7)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	removed, err := jobs.ClearCompleted(ctx, database.SqlDB(), time.Duration(days)*24*time.Hour)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("Cleared **%d** completed jobs older than %d days.", removed, days)), nil
}
