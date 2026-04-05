// Package analytics provides MCP tools for cross-module reporting and insights.
package analytics

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

type Module struct{}

func (m *Module) Name() string        { return "analytics" }
func (m *Module) Description() string { return "Cross-module analytics and reporting" }

var analyticsHints = map[string]string{
	"daily/summary":       "Today's cross-module summary",
	"daily/snapshot":      "Generate a daily stats snapshot",
	"weekly/review":       "Weekly review with trends",
	"trends/productivity": "Task completion and habit streaks",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("analytics").
		Domain("daily", common.ActionRegistry{
			"summary":  handleDailySummary,
			"snapshot": handleDailySnapshot,
		}).
		Domain("weekly", common.ActionRegistry{
			"review": handleWeeklyReview,
		}).
		Domain("trends", common.ActionRegistry{
			"productivity": handleTrendsProductivity,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_analytics",
				mcp.WithDescription("Analytics gateway for cross-module insights.\n\n"+dispatcher.DescribeActionsWithHints(analyticsHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: daily, weekly, trends")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("date", mcp.Description("Date YYYY-MM-DD (default: today)")),
				mcp.WithNumber("days", mcp.Description("Number of days to analyze (default 7)")),
			),
			Handler:     tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:    "analytics",
			Subcategory: "gateway",
			Tags:        []string{"analytics", "reporting", "insights"},
			Complexity:  tools.ComplexityModerate,
			Timeout:     60 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func handleDailySummary(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	today := time.Now().Format("2006-01-02")
	md := common.NewMarkdownBuilder().Title("Daily Summary: " + today)

	// Tasks
	var totalTasks, completedTasks, overdueTasks int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0").Scan(&totalTasks)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 1 AND updated_at >= ?", today).Scan(&completedTasks)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0 AND due_date < ? AND due_date != ''", today).Scan(&overdueTasks)
	md.Section("Tasks")
	md.KeyValue("Open", fmt.Sprintf("%d", totalTasks))
	md.KeyValue("Completed today", fmt.Sprintf("%d", completedTasks))
	md.KeyValue("Overdue", fmt.Sprintf("%d", overdueTasks))

	// Calendar
	var eventsToday int
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM calendar_events WHERE start_time >= ? AND start_time < ?", today, tomorrow).Scan(&eventsToday)
	md.Section("Calendar")
	md.KeyValue("Events today", fmt.Sprintf("%d", eventsToday))

	// Gmail
	var unreadEmails int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gmail_messages WHERE triaged = 0").Scan(&unreadEmails)
	md.Section("Email")
	md.KeyValue("Untriaged", fmt.Sprintf("%d", unreadEmails))

	// Habits
	var habitsCompleted int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habit_completions WHERE completed_at >= ?", today).Scan(&habitsCompleted)
	var totalHabits int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habits WHERE archived = 0").Scan(&totalHabits)
	md.Section("Habits")
	md.KeyValue("Completed today", fmt.Sprintf("%d / %d", habitsCompleted, totalHabits))

	// Finances
	var todaySpend float64
	db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE date = ? AND type = 'expense'", today).Scan(&todaySpend)
	md.Section("Finances")
	md.KeyValue("Spent today", fmt.Sprintf("$%.2f", todaySpend))

	return tools.TextResult(md.String()), nil
}

func handleDailySnapshot(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	// Ensure daily_snapshots table.
	db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS daily_snapshots (
		date TEXT PRIMARY KEY,
		open_tasks INTEGER DEFAULT 0,
		completed_tasks INTEGER DEFAULT 0,
		events INTEGER DEFAULT 0,
		emails_untriaged INTEGER DEFAULT 0,
		habits_completed INTEGER DEFAULT 0,
		spend REAL DEFAULT 0,
		created_at TEXT DEFAULT (datetime('now'))
	)`)

	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")

	var openTasks, completedTasks, events, untriaged, habits int
	var spend float64
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0").Scan(&openTasks)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 1 AND updated_at >= ?", today).Scan(&completedTasks)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM calendar_events WHERE start_time >= ? AND start_time < ?", today, tomorrow).Scan(&events)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gmail_messages WHERE triaged = 0").Scan(&untriaged)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habit_completions WHERE completed_at >= ?", today).Scan(&habits)
	db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE date = ? AND type = 'expense'", today).Scan(&spend)

	db.ExecContext(ctx,
		`INSERT OR REPLACE INTO daily_snapshots (date, open_tasks, completed_tasks, events, emails_untriaged, habits_completed, spend)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		today, openTasks, completedTasks, events, untriaged, habits, spend,
	)

	return tools.TextResult(fmt.Sprintf("# Snapshot Saved\n\nDaily snapshot for **%s** saved successfully.", today)), nil
}

func handleWeeklyReview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	days := common.GetIntParam(req, "days", 7)
	cutoff := time.Now().AddDate(0, 0, -days).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Weekly Review: %s to %s", cutoff, today))

	// Tasks completed this week.
	var tasksCompleted int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 1 AND updated_at >= ?", cutoff).Scan(&tasksCompleted)
	md.Section("Tasks")
	md.KeyValue("Completed this week", fmt.Sprintf("%d", tasksCompleted))

	// Events this week.
	var eventsThisWeek int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM calendar_events WHERE start_time >= ? AND start_time <= ?", cutoff, today+"T23:59:59").Scan(&eventsThisWeek)
	md.Section("Calendar")
	md.KeyValue("Events this week", fmt.Sprintf("%d", eventsThisWeek))

	// Emails received this week.
	var emailsReceived int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gmail_messages WHERE timestamp >= ?", cutoff).Scan(&emailsReceived)
	md.Section("Email")
	md.KeyValue("Received this week", fmt.Sprintf("%d", emailsReceived))

	// Habit completions this week.
	var habitCompletions int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habit_completions WHERE completed_at >= ?", cutoff).Scan(&habitCompletions)
	md.Section("Habits")
	md.KeyValue("Total completions", fmt.Sprintf("%d", habitCompletions))

	// Spending this week.
	var weekSpend float64
	db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE date >= ? AND type = 'expense'", cutoff).Scan(&weekSpend)
	md.Section("Finances")
	md.KeyValue("Total spent", fmt.Sprintf("$%.2f", weekSpend))

	return tools.TextResult(md.String()), nil
}

func handleTrendsProductivity(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	days := common.GetIntParam(req, "days", 7)
	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Productivity Trends — Last %d Days", days))

	// Daily task completion trend.
	rows, err := db.QueryContext(ctx,
		`SELECT date(updated_at) as d, COUNT(*) FROM tasks
		 WHERE completed = 1 AND updated_at >= date('now', ?)
		 GROUP BY d ORDER BY d`,
		fmt.Sprintf("-%d days", days),
	)
	if err == nil {
		defer rows.Close()
		headers := []string{"Date", "Tasks Completed"}
		var tableRows [][]string
		for rows.Next() {
			var date string
			var count int
			if rows.Scan(&date, &count) == nil {
				tableRows = append(tableRows, []string{date, fmt.Sprintf("%d", count)})
			}
		}
		if len(tableRows) > 0 {
			md.Section("Task Completions by Day")
			md.Table(headers, tableRows)
		}
	}

	// Habit streak info.
	habitRows, err := db.QueryContext(ctx,
		`SELECT h.name, COUNT(hc.id) as completions
		 FROM habits h LEFT JOIN habit_completions hc ON h.id = hc.habit_id AND hc.completed_at >= date('now', ?)
		 WHERE h.archived = 0
		 GROUP BY h.id ORDER BY completions DESC`,
		fmt.Sprintf("-%d days", days),
	)
	if err == nil {
		defer habitRows.Close()
		headers := []string{"Habit", "Completions", "Rate"}
		var tableRows [][]string
		for habitRows.Next() {
			var name string
			var completions int
			if habitRows.Scan(&name, &completions) == nil {
				rate := float64(completions) / float64(days) * 100
				tableRows = append(tableRows, []string{name, fmt.Sprintf("%d", completions), fmt.Sprintf("%.0f%%", rate)})
			}
		}
		if len(tableRows) > 0 {
			md.Section("Habit Consistency")
			md.Table(headers, tableRows)
		}
	}

	return tools.TextResult(md.String()), nil
}
