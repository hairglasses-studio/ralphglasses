// Package habits provides MCP tools for habit tracking.
package habits

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for habit tracking.
type Module struct{}

func (m *Module) Name() string        { return "habits" }
func (m *Module) Description() string { return "Habit tracking with streaks and completion history" }

var habitsHints = map[string]string{
	"manage/list":     "List all active habits",
	"manage/add":      "Create a new habit to track",
	"manage/complete": "Record today's completion of a habit",
	"manage/delete":   "Archive a habit (soft delete)",
	"stats/streaks":   "View current streaks for all habits",
	"stats/history":   "View completion history for a habit",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("habits").
		Domain("manage", common.ActionRegistry{
			"list":     handleManageList,
			"add":      handleManageAdd,
			"complete": handleManageComplete,
			"delete":   handleManageDelete,
		}).
		Domain("stats", common.ActionRegistry{
			"streaks": handleStatsStreaks,
			"history": handleStatsHistory,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_habits",
				mcp.WithDescription(
					"Habit tracking gateway. Track daily/weekly habits with streaks and history.\n\n"+
						dispatcher.DescribeActionsWithHints(habitsHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: manage, stats")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("habit_id", mcp.Description("Habit ID")),
				mcp.WithString("name", mcp.Description("Habit name (for add)")),
				mcp.WithString("description", mcp.Description("Habit description")),
				mcp.WithString("frequency", mcp.Description("Frequency: daily or weekly (default daily)")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:    tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:   "habits",
			Tags:       []string{"habits", "wellness", "tracking"},
			Complexity: tools.ComplexityModerate,
			IsWrite:    true,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func handleManageList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT id, name, description, frequency FROM habits WHERE archived = 0 ORDER BY created_at DESC")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Habits")
	headers := []string{"ID", "Name", "Description", "Frequency"}
	var tableRows [][]string

	for rows.Next() {
		var id, name, description, frequency string
		if err := rows.Scan(&id, &name, &description, &frequency); err != nil {
			continue
		}
		shortID := id
		if len(id) > 8 {
			shortID = id[:8]
		}
		tableRows = append(tableRows, []string{shortID, name, description, frequency})
	}

	if len(tableRows) == 0 {
		md.EmptyList("habits")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleManageAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := common.GetStringParam(req, "name", "")
	if name == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "name is required for manage/add"), nil
	}
	description := common.GetStringParam(req, "description", "")
	frequency := common.GetStringParam(req, "frequency", "daily")

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	id := fmt.Sprintf("habit-%d", time.Now().UnixNano())
	_, err = database.SqlDB().ExecContext(ctx,
		`INSERT INTO habits (id, name, description, frequency, archived, created_at) VALUES (?, ?, ?, ?, 0, datetime('now'))`,
		id, name, description, frequency,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Habit Created\n\n- **ID:** %s\n- **Name:** %s\n- **Frequency:** %s",
		id, name, frequency)), nil
}

func handleManageComplete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	habitID, ok := common.RequireStringParam(req, "habit_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "habit_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	// Check if already completed today
	var count int
	err = database.SqlDB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM habit_completions WHERE habit_id = ? AND date(completed_at) = date('now')",
		habitID,
	).Scan(&count)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	if count > 0 {
		return tools.TextResult(fmt.Sprintf("Habit %s already completed today.", habitID)), nil
	}

	id := fmt.Sprintf("hc-%d", time.Now().UnixNano())
	_, err = database.SqlDB().ExecContext(ctx,
		`INSERT INTO habit_completions (id, habit_id, completed_at) VALUES (?, ?, datetime('now'))`,
		id, habitID,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Habit Completed\n\n- **Habit ID:** %s\n- **Date:** %s",
		habitID, time.Now().Format("2006-01-02"))), nil
}

func handleManageDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	habitID, ok := common.RequireStringParam(req, "habit_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "habit_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	_, err = database.SqlDB().ExecContext(ctx,
		"UPDATE habits SET archived = 1 WHERE id = ?", habitID)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("Habit %s archived.", habitID)), nil
}

func handleStatsStreaks(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	// Get all active habits
	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT id, name FROM habits WHERE archived = 0 ORDER BY name ASC")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	type habit struct {
		ID   string
		Name string
	}
	var habits []habit
	for rows.Next() {
		var h habit
		if err := rows.Scan(&h.ID, &h.Name); err != nil {
			continue
		}
		habits = append(habits, h)
	}
	rows.Close()

	md := common.NewMarkdownBuilder().Title("Habit Streaks")
	headers := []string{"Habit", "Current Streak", "Last Completed"}
	var tableRows [][]string

	for _, h := range habits {
		// Get completion dates ordered descending
		compRows, err := database.SqlDB().QueryContext(ctx,
			"SELECT DISTINCT date(completed_at) as d FROM habit_completions WHERE habit_id = ? ORDER BY d DESC",
			h.ID,
		)
		if err != nil {
			continue
		}

		var dates []string
		for compRows.Next() {
			var d string
			if err := compRows.Scan(&d); err != nil {
				continue
			}
			dates = append(dates, d)
		}
		compRows.Close()

		streak := 0
		lastCompleted := "never"
		if len(dates) > 0 {
			lastCompleted = dates[0]
			// Calculate streak: count consecutive days backward from today
			today := time.Now().Truncate(24 * time.Hour)
			for i, d := range dates {
				t, err := time.Parse("2006-01-02", d)
				if err != nil {
					break
				}
				expected := today.AddDate(0, 0, -i)
				if t.Truncate(24 * time.Hour).Equal(expected) {
					streak++
				} else {
					break
				}
			}
		}

		tableRows = append(tableRows, []string{h.Name, fmt.Sprintf("%d days", streak), lastCompleted})
	}

	if len(tableRows) == 0 {
		md.EmptyList("habits")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleStatsHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	habitID, ok := common.RequireStringParam(req, "habit_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "habit_id is required"), nil
	}
	limit := common.GetLimitParam(req, 20)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT completed_at, COALESCE(notes, '') FROM habit_completions WHERE habit_id = ? ORDER BY completed_at DESC LIMIT ?",
		habitID, limit,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Habit History")
	headers := []string{"Completed At", "Notes"}
	var tableRows [][]string

	for rows.Next() {
		var completedAt, notes string
		if err := rows.Scan(&completedAt, &notes); err != nil {
			continue
		}
		tableRows = append(tableRows, []string{completedAt, notes})
	}

	if len(tableRows) == 0 {
		md.EmptyList("completions")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}
