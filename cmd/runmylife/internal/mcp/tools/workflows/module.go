// Package workflows provides MCP tools for user-defined multi-step workflows.
package workflows

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Workflow represents a saved workflow definition.
type Workflow struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Steps       string `json:"steps"` // JSON array of step definitions
	Schedule    string `json:"schedule"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Step represents a single step in a workflow.
type Step struct {
	Order       int    `json:"order"`
	Tool        string `json:"tool"`
	Domain      string `json:"domain"`
	Action      string `json:"action"`
	Description string `json:"description"`
}

type Module struct{}

func (m *Module) Name() string        { return "workflows" }
func (m *Module) Description() string { return "User-defined multi-step workflows" }

var workflowHints = map[string]string{
	"manage/list":   "List all saved workflows",
	"manage/get":    "Get workflow details",
	"manage/create": "Create a new workflow",
	"manage/delete": "Delete a workflow",
	"manage/toggle": "Enable/disable a workflow",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("workflows").
		Domain("manage", common.ActionRegistry{
			"list":   handleList,
			"get":    handleGet,
			"create": handleCreate,
			"delete": handleDelete,
			"toggle": handleToggle,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_workflows",
				mcp.WithDescription("Workflow management gateway.\n\n"+dispatcher.DescribeActionsWithHints(workflowHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: manage")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action: list, get, create, delete, toggle")),
				mcp.WithNumber("workflow_id", mcp.Description("Workflow ID")),
				mcp.WithString("name", mcp.Description("Workflow name")),
				mcp.WithString("description", mcp.Description("Workflow description")),
				mcp.WithString("steps", mcp.Description("JSON array of workflow steps")),
				mcp.WithString("schedule", mcp.Description("Cron-like schedule (e.g. 'daily:08:00')")),
			),
			Handler:     tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:    "workflows",
			Subcategory: "gateway",
			Tags:        []string{"workflows", "automation"},
			Complexity:  tools.ComplexityModerate,
			Timeout:     30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func ensureTable(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS workflows (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT DEFAULT '',
			steps TEXT DEFAULT '[]',
			schedule TEXT DEFAULT '',
			enabled INTEGER DEFAULT 1,
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		)
	`)
	return err
}

func handleList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	if err := ensureTable(db); err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	rows, err := db.QueryContext(ctx,
		"SELECT id, name, description, steps, schedule, enabled, created_at, updated_at FROM workflows ORDER BY name ASC")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Workflows")
	headers := []string{"ID", "Name", "Steps", "Schedule", "Enabled"}
	var tableRows [][]string

	for rows.Next() {
		var w Workflow
		var enabled int
		if rows.Scan(&w.ID, &w.Name, &w.Description, &w.Steps, &w.Schedule, &enabled, &w.CreatedAt, &w.UpdatedAt) != nil {
			continue
		}
		var steps []Step
		json.Unmarshal([]byte(w.Steps), &steps)
		enabledStr := "yes"
		if enabled == 0 {
			enabledStr = "no"
		}
		sched := w.Schedule
		if sched == "" {
			sched = "manual"
		}
		tableRows = append(tableRows, []string{
			fmt.Sprintf("%d", w.ID), w.Name, fmt.Sprintf("%d steps", len(steps)), sched, enabledStr,
		})
	}

	if len(tableRows) == 0 {
		md.EmptyList("workflows")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	wfID := int64(common.GetIntParam(req, "workflow_id", 0))
	if wfID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "workflow_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	if err := ensureTable(db); err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	var w Workflow
	var enabled int
	err = db.QueryRowContext(ctx,
		"SELECT id, name, description, steps, schedule, enabled, created_at, updated_at FROM workflows WHERE id = ?", wfID).
		Scan(&w.ID, &w.Name, &w.Description, &w.Steps, &w.Schedule, &enabled, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return common.CodedErrorResultf(common.ErrNotFound, "workflow %d not found", wfID), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Workflow: %s", w.Name))
	md.KeyValue("Description", w.Description)
	md.KeyValue("Schedule", orDefault(w.Schedule, "manual"))
	md.KeyValue("Enabled", fmt.Sprintf("%v", enabled == 1))
	md.KeyValue("Created", w.CreatedAt)
	md.KeyValue("Updated", w.UpdatedAt)

	var steps []Step
	json.Unmarshal([]byte(w.Steps), &steps)
	if len(steps) > 0 {
		md.Section("Steps")
		headers := []string{"#", "Tool", "Domain", "Action", "Description"}
		var rows [][]string
		for _, s := range steps {
			rows = append(rows, []string{
				fmt.Sprintf("%d", s.Order), s.Tool, s.Domain, s.Action, s.Description,
			})
		}
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := common.GetStringParam(req, "name", "")
	if name == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "name is required"), nil
	}
	description := common.GetStringParam(req, "description", "")
	stepsJSON := common.GetStringParam(req, "steps", "[]")
	schedule := common.GetStringParam(req, "schedule", "")

	// Validate steps JSON
	var steps []Step
	if err := json.Unmarshal([]byte(stepsJSON), &steps); err != nil {
		return common.CodedErrorResultf(common.ErrInvalidParam, "invalid steps JSON: %v", err), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	if err := ensureTable(db); err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	result, err := db.ExecContext(ctx,
		"INSERT INTO workflows (name, description, steps, schedule) VALUES (?, ?, ?, ?)",
		name, description, stepsJSON, schedule)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("# Workflow Created\n\n**%s** (ID: %d) with %d steps.", name, id, len(steps))), nil
}

func handleDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	wfID := int64(common.GetIntParam(req, "workflow_id", 0))
	if wfID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "workflow_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	if err := ensureTable(database.SqlDB()); err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	result, err := database.SqlDB().ExecContext(ctx, "DELETE FROM workflows WHERE id = ?", wfID)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return common.CodedErrorResultf(common.ErrNotFound, "workflow %d not found", wfID), nil
	}

	return tools.TextResult(fmt.Sprintf("Workflow **%d** deleted.", wfID)), nil
}

func handleToggle(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	wfID := int64(common.GetIntParam(req, "workflow_id", 0))
	if wfID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "workflow_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	if err := ensureTable(db); err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	_, err = db.ExecContext(ctx, "UPDATE workflows SET enabled = 1 - enabled, updated_at = datetime('now') WHERE id = ?", wfID)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	var enabled int
	db.QueryRowContext(ctx, "SELECT enabled FROM workflows WHERE id = ?", wfID).Scan(&enabled)
	state := "disabled"
	if enabled == 1 {
		state = "enabled"
	}

	return tools.TextResult(fmt.Sprintf("Workflow **%d** is now **%s**.", wfID, state)), nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
