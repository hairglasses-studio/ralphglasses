// Package tasks provides MCP tools for task management via Todoist.
package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for task management.
type Module struct{}

func (m *Module) Name() string        { return "tasks" }
func (m *Module) Description() string { return "Task management via Todoist integration" }

var taskHints = map[string]string{
	"manage/add":        "Create a new task (syncs to Todoist)",
	"manage/list":       "List tasks with optional filters",
	"manage/update":     "Update task title, priority, or due date",
	"manage/complete":   "Mark a task as completed",
	"manage/delete":     "Delete a task",
	"prioritize/matrix": "View tasks in Eisenhower matrix format",
	"projects/list":     "List all Todoist projects",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("tasks").
		Domain("manage", common.ActionRegistry{
			"add":      handleManageAdd,
			"list":     handleManageList,
			"update":   handleManageUpdate,
			"complete": handleManageComplete,
			"delete":   handleManageDelete,
		}).
		Domain("prioritize", common.ActionRegistry{
			"matrix": handlePrioritizeMatrix,
		}).
		Domain("projects", common.ActionRegistry{
			"list": handleProjectsList,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_tasks",
				mcp.WithDescription(
					"Task management gateway. Manages tasks synced with Todoist.\n\n"+
						dispatcher.DescribeActionsWithHints(taskHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: manage, prioritize, projects")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("title", mcp.Description("Task title (for add/update)")),
				mcp.WithString("description", mcp.Description("Task description")),
				mcp.WithNumber("priority", mcp.Description("Priority 1-4 (4=urgent, 1=low)")),
				mcp.WithString("project", mcp.Description("Project name")),
				mcp.WithString("due_date", mcp.Description("Due date (YYYY-MM-DD)")),
				mcp.WithString("task_id", mcp.Description("Task ID (for update/complete/delete)")),
				mcp.WithString("filter", mcp.Description("Filter: all, today, overdue, project:<name>")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "tasks",
			Subcategory:         "gateway",
			Tags:                []string{"tasks", "todoist", "productivity"},
			Complexity:          tools.ComplexityModerate,
			IsWrite:             true,
			ProducesRefs:        []string{"task"},
			CircuitBreakerGroup: "todoist_api",
			Timeout:             60 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func handleManageAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := common.GetStringParam(req, "title", "")
	if title == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "title is required for add"), nil
	}
	priority := common.GetIntParam(req, "priority", 1)
	project := common.GetStringParam(req, "project", "")
	dueDate := common.GetStringParam(req, "due_date", "")
	description := common.GetStringParam(req, "description", "")

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	id := fmt.Sprintf("task-%d", time.Now().UnixNano())
	_, err = database.SqlDB().ExecContext(ctx,
		`INSERT INTO tasks (id, title, description, priority, project, due_date) VALUES (?, ?, ?, ?, ?, ?)`,
		id, title, description, priority, project, dueDate,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Task Created\n\n- **ID:** %s\n- **Title:** %s\n- **Priority:** %d\n- **Project:** %s\n- **Due:** %s",
		id, title, priority, project, dueDate)), nil
}

func handleManageList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := common.GetLimitParam(req, 20)
	filter := common.GetStringParam(req, "filter", "all")

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	query := "SELECT id, title, priority, project, due_date, completed FROM tasks WHERE completed = 0"
	switch filter {
	case "today":
		query += " AND due_date = date('now')"
	case "overdue":
		query += " AND due_date < date('now') AND due_date != ''"
	}
	query += " ORDER BY priority DESC, due_date ASC LIMIT ?"

	rows, err := database.SqlDB().QueryContext(ctx, query, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Tasks")
	var count int
	headers := []string{"ID", "Title", "Priority", "Project", "Due"}
	var tableRows [][]string

	for rows.Next() {
		var id, title, project, dueDate string
		var priority, completed int
		if err := rows.Scan(&id, &title, &priority, &project, &dueDate, &completed); err != nil {
			continue
		}
		tableRows = append(tableRows, []string{id[:8], title, fmt.Sprintf("%d", priority), project, dueDate})
		count++
	}

	if count == 0 {
		md.EmptyList("tasks")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleManageUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, ok := common.RequireStringParam(req, "task_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "task_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	title := common.GetStringParam(req, "title", "")
	priority := common.GetIntParam(req, "priority", 0)
	dueDate := common.GetStringParam(req, "due_date", "")

	if title != "" {
		database.SqlDB().ExecContext(ctx, "UPDATE tasks SET title = ?, updated_at = datetime('now') WHERE id = ?", title, taskID)
	}
	if priority > 0 {
		database.SqlDB().ExecContext(ctx, "UPDATE tasks SET priority = ?, updated_at = datetime('now') WHERE id = ?", priority, taskID)
	}
	if dueDate != "" {
		database.SqlDB().ExecContext(ctx, "UPDATE tasks SET due_date = ?, updated_at = datetime('now') WHERE id = ?", dueDate, taskID)
	}

	return tools.TextResult(fmt.Sprintf("Task %s updated.", taskID)), nil
}

func handleManageComplete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, ok := common.RequireStringParam(req, "task_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "task_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	_, err = database.SqlDB().ExecContext(ctx,
		"UPDATE tasks SET completed = 1, updated_at = datetime('now') WHERE id = ?", taskID)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("Task %s completed.", taskID)), nil
}

func handleManageDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, ok := common.RequireStringParam(req, "task_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "task_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	_, err = database.SqlDB().ExecContext(ctx, "DELETE FROM tasks WHERE id = ?", taskID)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("Task %s deleted.", taskID)), nil
}

func handlePrioritizeMatrix(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT id, title, priority, due_date FROM tasks WHERE completed = 0 ORDER BY priority DESC, due_date ASC")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	type task struct {
		ID, Title, DueDate string
		Priority           int
	}
	var allTasks []task
	for rows.Next() {
		var t task
		if err := rows.Scan(&t.ID, &t.Title, &t.Priority, &t.DueDate); err != nil {
			continue
		}
		allTasks = append(allTasks, t)
	}

	md := common.NewMarkdownBuilder().Title("Eisenhower Matrix")

	// P4 = Do First, P3 = Schedule, P2 = Delegate, P1 = Eliminate
	buckets := map[string][]task{
		"Do First (P4 - Urgent & Important)":      {},
		"Schedule (P3 - Important, Not Urgent)":    {},
		"Delegate (P2 - Urgent, Not Important)":    {},
		"Eliminate (P1 - Neither)":                  {},
	}
	for _, t := range allTasks {
		switch t.Priority {
		case 4:
			buckets["Do First (P4 - Urgent & Important)"] = append(buckets["Do First (P4 - Urgent & Important)"], t)
		case 3:
			buckets["Schedule (P3 - Important, Not Urgent)"] = append(buckets["Schedule (P3 - Important, Not Urgent)"], t)
		case 2:
			buckets["Delegate (P2 - Urgent, Not Important)"] = append(buckets["Delegate (P2 - Urgent, Not Important)"], t)
		default:
			buckets["Eliminate (P1 - Neither)"] = append(buckets["Eliminate (P1 - Neither)"], t)
		}
	}

	for _, label := range []string{
		"Do First (P4 - Urgent & Important)",
		"Schedule (P3 - Important, Not Urgent)",
		"Delegate (P2 - Urgent, Not Important)",
		"Eliminate (P1 - Neither)",
	} {
		md.Section(label)
		tasks := buckets[label]
		if len(tasks) == 0 {
			md.Text("(none)")
		} else {
			var items []string
			for _, t := range tasks {
				due := ""
				if t.DueDate != "" {
					due = " (due: " + t.DueDate + ")"
				}
				items = append(items, t.Title+due)
			}
			md.List(items)
		}
	}

	return tools.TextResult(md.String()), nil
}

func handleProjectsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT DISTINCT project, COUNT(*) as cnt FROM tasks WHERE project != '' AND completed = 0 GROUP BY project ORDER BY cnt DESC")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	result := map[string]int{}
	for rows.Next() {
		var project string
		var count int
		if err := rows.Scan(&project, &count); err != nil {
			continue
		}
		result[project] = count
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return tools.TextResult(fmt.Sprintf("# Projects\n\n```json\n%s\n```", string(data))), nil
}
