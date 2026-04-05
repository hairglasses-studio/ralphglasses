// Package gtasks provides MCP tools for Google Tasks integration.
package gtasks

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

type Module struct{}

func (m *Module) Name() string        { return "gtasks" }
func (m *Module) Description() string { return "Google Tasks integration" }

var gtasksHints = map[string]string{
	"lists/list":    "List all task lists",
	"tasks/list":    "List tasks in a list",
	"tasks/add":     "Create a new task",
	"tasks/complete": "Complete a task",
	"tasks/move":    "Move task between lists",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("gtasks").
		Domain("lists", common.ActionRegistry{
			"list": handleListsList,
		}).
		Domain("tasks", common.ActionRegistry{
			"list":     handleTasksList,
			"add":      handleTasksAdd,
			"complete": handleTasksComplete,
			"move":     handleTasksMove,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_gtasks",
				mcp.WithDescription("Google Tasks gateway.\n\n"+dispatcher.DescribeActionsWithHints(gtasksHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: lists, tasks")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("list_id", mcp.Description("Task list ID (default: @default)")),
				mcp.WithString("task_id", mcp.Description("Task ID")),
				mcp.WithString("title", mcp.Description("Task title")),
				mcp.WithString("notes", mcp.Description("Task notes")),
				mcp.WithString("due", mcp.Description("Due date YYYY-MM-DD")),
				mcp.WithString("to_list_id", mcp.Description("Target list ID (for move)")),
				mcp.WithString("account", mcp.Description("Google account (default: personal)")),
				mcp.WithBoolean("show_completed", mcp.Description("Include completed tasks")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "gtasks",
			Subcategory:         "gateway",
			Tags:                []string{"google", "tasks", "productivity"},
			Complexity:          tools.ComplexitySimple,
			IsWrite:             true,
			CircuitBreakerGroup: "gtasks_api",
			Timeout:             30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func gtasksClient(ctx context.Context, req mcp.CallToolRequest) (*clients.GTasksClient, error) {
	account := common.GetStringParam(req, "account", "personal")
	return clients.NewGTasksClient(ctx, account)
}

func handleListsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := gtasksClient(ctx, req)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Run 'runmylife google-auth' to authenticate"), nil
	}
	lists, err := client.ListTaskLists(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Task Lists")
	headers := []string{"ID", "Title", "Updated"}
	var rows [][]string
	for _, l := range lists {
		rows = append(rows, []string{l.ID, l.Title, l.UpdatedAt})
	}
	if len(rows) == 0 {
		md.EmptyList("task lists")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleTasksList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := gtasksClient(ctx, req)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Run 'runmylife google-auth' to authenticate"), nil
	}
	listID := common.GetStringParam(req, "list_id", "@default")
	showCompleted := common.GetBoolParam(req, "show_completed", false)
	tasks, err := client.ListTasks(ctx, listID, showCompleted)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Tasks")
	headers := []string{"ID", "Title", "Status", "Due"}
	var rows [][]string
	for _, t := range tasks {
		rows = append(rows, []string{t.ID, t.Title, t.Status, t.Due})
	}
	if len(rows) == 0 {
		md.EmptyList("tasks")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleTasksAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := common.GetStringParam(req, "title", "")
	if title == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "title is required"), nil
	}
	client, err := gtasksClient(ctx, req)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Run 'runmylife google-auth' to authenticate"), nil
	}
	listID := common.GetStringParam(req, "list_id", "@default")
	notes := common.GetStringParam(req, "notes", "")
	due := common.GetStringParam(req, "due", "")
	task, err := client.CreateTask(ctx, listID, title, notes, due)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return tools.TextResult(fmt.Sprintf("# Task Created\n\n- **ID:** %s\n- **Title:** %s\n- **Due:** %s", task.ID, task.Title, task.Due)), nil
}

func handleTasksComplete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, ok := common.RequireStringParam(req, "task_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "task_id is required"), nil
	}
	client, err := gtasksClient(ctx, req)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Run 'runmylife google-auth' to authenticate"), nil
	}
	listID := common.GetStringParam(req, "list_id", "@default")
	if err := client.CompleteTask(ctx, listID, taskID); err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return tools.TextResult(fmt.Sprintf("Task %s completed.", taskID)), nil
}

func handleTasksMove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	taskID, ok := common.RequireStringParam(req, "task_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "task_id is required"), nil
	}
	toListID := common.GetStringParam(req, "to_list_id", "")
	if toListID == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "to_list_id is required"), nil
	}
	client, err := gtasksClient(ctx, req)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Run 'runmylife google-auth' to authenticate"), nil
	}
	fromListID := common.GetStringParam(req, "list_id", "@default")
	task, err := client.MoveTask(ctx, fromListID, taskID, toListID)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	return tools.TextResult(fmt.Sprintf("Task moved to list %s. New ID: %s", toListID, task.ID)), nil
}
