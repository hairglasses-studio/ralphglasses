// Package clockify provides MCP tools for Clockify time tracking.
package clockify

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

type Module struct{}

func (m *Module) Name() string        { return "clockify" }
func (m *Module) Description() string { return "Clockify time tracking integration" }

var clockifyHints = map[string]string{
	"timer/start":    "Start a time entry",
	"timer/stop":     "Stop the current timer",
	"timer/current":  "Get the running timer",
	"entries/list":   "Recent time entries",
	"projects/list":  "List projects",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("clockify").
		Domain("timer", common.ActionRegistry{
			"start":   handleTimerStart,
			"stop":    handleTimerStop,
			"current": handleTimerCurrent,
		}).
		Domain("entries", common.ActionRegistry{
			"list": handleEntriesList,
		}).
		Domain("projects", common.ActionRegistry{
			"list": handleProjectsList,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_clockify",
				mcp.WithDescription("Clockify gateway for time tracking.\n\n"+dispatcher.DescribeActionsWithHints(clockifyHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: timer, entries, projects")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("description", mcp.Description("Time entry description")),
				mcp.WithString("project_id", mcp.Description("Project ID")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "clockify",
			Subcategory:         "gateway",
			Tags:                []string{"clockify", "time", "tracking"},
			Complexity:          tools.ComplexitySimple,
			IsWrite:             true,
			CircuitBreakerGroup: "clockify_api",
			Timeout:             30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func clockifyClient(ctx context.Context) (*clients.ClockifyClient, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	apiKey := cfg.Credentials["clockify"]
	if apiKey == "" {
		return nil, fmt.Errorf("clockify API key not configured — add 'clockify' to credentials in config.json")
	}
	return clients.NewClockifyClient(ctx, apiKey)
}

func handleTimerStart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := clockifyClient(ctx)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add clockify API key to config"), nil
	}
	desc := common.GetStringParam(req, "description", "")
	projectID := common.GetStringParam(req, "project_id", "")
	entry, err := client.StartTimer(ctx, desc, projectID)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Timer Started")
	md.KeyValue("ID", entry.ID)
	md.KeyValue("Description", entry.Description)
	md.KeyValue("Started", entry.Start)
	return tools.TextResult(md.String()), nil
}

func handleTimerStop(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := clockifyClient(ctx)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add clockify API key to config"), nil
	}
	entry, err := client.StopTimer(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	dur := time.Duration(entry.Duration) * time.Second
	md := common.NewMarkdownBuilder().Title("Timer Stopped")
	md.KeyValue("Description", entry.Description)
	md.KeyValue("Duration", dur.Round(time.Minute).String())
	md.KeyValue("Start", entry.Start)
	md.KeyValue("End", entry.End)
	return tools.TextResult(md.String()), nil
}

func handleTimerCurrent(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := clockifyClient(ctx)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add clockify API key to config"), nil
	}
	entry, err := client.CurrentEntry(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Current Timer")
	if entry == nil {
		md.Text("No timer is currently running.")
	} else {
		md.KeyValue("Description", entry.Description)
		md.KeyValue("Started", entry.Start)
		md.KeyValue("Project", entry.ProjectID)
	}
	return tools.TextResult(md.String()), nil
}

func handleEntriesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := clockifyClient(ctx)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add clockify API key to config"), nil
	}
	limit := common.GetLimitParam(req, 20)
	entries, err := client.RecentEntries(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Recent Time Entries")
	headers := []string{"Description", "Start", "Duration", "Billable"}
	var rows [][]string
	for _, e := range entries {
		dur := time.Duration(e.Duration) * time.Second
		billable := "no"
		if e.Billable {
			billable = "yes"
		}
		rows = append(rows, []string{e.Description, e.Start, dur.Round(time.Minute).String(), billable})
	}
	if len(rows) == 0 {
		md.EmptyList("time entries")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleProjectsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := clockifyClient(ctx)
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add clockify API key to config"), nil
	}
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Projects")
	headers := []string{"ID", "Name", "Client", "Archived"}
	var rows [][]string
	for _, p := range projects {
		archived := "no"
		if p.Archived {
			archived = "yes"
		}
		rows = append(rows, []string{p.ID, p.Name, p.ClientName, archived})
	}
	if len(rows) == 0 {
		md.EmptyList("projects")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}
