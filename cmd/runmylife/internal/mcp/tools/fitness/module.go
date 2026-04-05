// Package fitness provides MCP tools for fitness/health tracking via Fitbit.
package fitness

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

func (m *Module) Name() string        { return "fitness" }
func (m *Module) Description() string { return "Fitness and health tracking via Fitbit" }

var fitnessHints = map[string]string{
	"activity/today":   "Today's activity summary (steps, calories, etc.)",
	"activity/history": "Recent activity log",
	"sleep/last":       "Last night's sleep data",
	"stats/daily":      "Daily stats for a specific date",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("fitness").
		Domain("activity", common.ActionRegistry{
			"today":   handleActivityToday,
			"history": handleActivityHistory,
		}).
		Domain("sleep", common.ActionRegistry{
			"last": handleSleepLast,
		}).
		Domain("stats", common.ActionRegistry{
			"daily": handleStatsDaily,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_fitness",
				mcp.WithDescription("Fitness gateway for health and activity tracking.\n\n"+dispatcher.DescribeActionsWithHints(fitnessHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: activity, sleep, stats")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("date", mcp.Description("Date YYYY-MM-DD (default: today)")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 10)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "fitness",
			Subcategory:         "gateway",
			Tags:                []string{"fitness", "health", "activity"},
			Complexity:          tools.ComplexitySimple,
			CircuitBreakerGroup: "fitbit_api",
			Timeout:             30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func fitbitClient() (*clients.FitbitClient, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	token := cfg.Credentials["fitbit"]
	if token == "" {
		return nil, fmt.Errorf("fitbit token not configured — add 'fitbit' to credentials in config.json")
	}
	return clients.NewFitbitClient(token), nil
}

func handleActivityToday(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := fitbitClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add fitbit token to config"), nil
	}
	stats, err := client.TodayStats(ctx)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Today's Activity")
	md.KeyValue("Steps", fmt.Sprintf("%d", stats.Steps))
	md.KeyValue("Calories", fmt.Sprintf("%d", stats.Calories))
	md.KeyValue("Active Minutes", fmt.Sprintf("%d", stats.ActiveMinutes))
	md.KeyValue("Distance", fmt.Sprintf("%.2f km", stats.Distance))
	if stats.RestingHeartRate > 0 {
		md.KeyValue("Resting Heart Rate", fmt.Sprintf("%d bpm", stats.RestingHeartRate))
	}
	return tools.TextResult(md.String()), nil
}

func handleActivityHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := fitbitClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add fitbit token to config"), nil
	}
	limit := common.GetLimitParam(req, 10)
	activities, err := client.RecentActivities(ctx, limit)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Recent Activities")
	headers := []string{"Type", "Duration", "Calories", "Distance", "Date"}
	var rows [][]string
	for _, a := range activities {
		dur := time.Duration(a.DurationMs) * time.Millisecond
		rows = append(rows, []string{a.Type, dur.Round(time.Minute).String(), fmt.Sprintf("%d", a.Calories), fmt.Sprintf("%.2f", a.Distance), a.StartTime})
	}
	if len(rows) == 0 {
		md.EmptyList("activities")
	} else {
		md.Table(headers, rows)
	}
	return tools.TextResult(md.String()), nil
}

func handleSleepLast(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := fitbitClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add fitbit token to config"), nil
	}
	date := common.GetStringParam(req, "date", time.Now().Format("2006-01-02"))
	sleeps, err := client.SleepLog(ctx, date)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Sleep: " + date)
	if len(sleeps) == 0 {
		md.Text("No sleep data recorded.")
	} else {
		for _, s := range sleeps {
			dur := time.Duration(s.DurationMs) * time.Millisecond
			md.KeyValue("Duration", dur.Round(time.Minute).String())
			md.KeyValue("Start", s.StartTime)
			md.KeyValue("End", s.EndTime)
			md.KeyValue("Efficiency", fmt.Sprintf("%d%%", s.Efficiency))
		}
	}
	return tools.TextResult(md.String()), nil
}

func handleStatsDaily(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := fitbitClient()
	if err != nil {
		return common.ActionableErrorResult(common.ErrConfig, err, "Add fitbit token to config"), nil
	}
	date := common.GetStringParam(req, "date", time.Now().Format("2006-01-02"))
	stats, err := client.DailyStats(ctx, date)
	if err != nil {
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}
	md := common.NewMarkdownBuilder().Title("Daily Stats: " + date)
	md.KeyValue("Steps", fmt.Sprintf("%d", stats.Steps))
	md.KeyValue("Calories", fmt.Sprintf("%d", stats.Calories))
	md.KeyValue("Active Minutes", fmt.Sprintf("%d", stats.ActiveMinutes))
	md.KeyValue("Distance", fmt.Sprintf("%.2f km", stats.Distance))
	if stats.RestingHeartRate > 0 {
		md.KeyValue("Resting Heart Rate", fmt.Sprintf("%d bpm", stats.RestingHeartRate))
	}
	return tools.TextResult(md.String()), nil
}
