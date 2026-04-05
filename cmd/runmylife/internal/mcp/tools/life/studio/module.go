// Package studio provides the Studio life category MCP tool.
// Covers studio session tracking, scheduling, and maintenance.
// Future: MCP federation bridge to hg-mcp for gear/project management.
package studio

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

type Module struct{}

func (m *Module) Name() string        { return "studio" }
func (m *Module) Description() string { return "Studio life: session tracking, scheduling, maintenance" }

var studioHints = map[string]string{
	"sessions/start":        "Start a studio session",
	"sessions/end":          "End current studio session",
	"sessions/list":         "List recent studio sessions",
	"sessions/stats":        "Session statistics and patterns",
	"schedule/book":         "Book studio time",
	"schedule/available":    "Check availability",
	"maintenance/list":      "List maintenance items",
	"maintenance/add":       "Add maintenance task",
	"maintenance/complete":  "Mark maintenance done",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("studio").
		Domain("sessions", common.ActionRegistry{
			"start": handleSessionStart,
			"end":   handleSessionEnd,
			"list":  handleSessionList,
			"stats": handleSessionStats,
		}).
		Domain("schedule", common.ActionRegistry{
			"book":      handleScheduleBook,
			"available": handleScheduleAvailable,
		}).
		Domain("maintenance", common.ActionRegistry{
			"list":     handleMaintenanceList,
			"add":      handleMaintenanceAdd,
			"complete": handleMaintenanceComplete,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_studio",
				mcp.WithDescription("Studio life gateway.\n\n"+
					dispatcher.DescribeActionsWithHints(studioHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: sessions, schedule, maintenance")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				// Session params
				mcp.WithString("activity_type", mcp.Description("Activity: resolume, ableton, recording, mixing, etc.")),
				mcp.WithString("tools_used", mcp.Description("Comma-separated tools/gear used")),
				mcp.WithString("project", mcp.Description("Project name")),
				mcp.WithString("notes", mcp.Description("Session notes")),
				mcp.WithNumber("session_id", mcp.Description("Session ID (for end)")),
				// Schedule params
				mcp.WithString("date", mcp.Description("Date (YYYY-MM-DD)")),
				mcp.WithString("start_time", mcp.Description("Start time (HH:MM)")),
				mcp.WithString("end_time", mcp.Description("End time (HH:MM)")),
				mcp.WithNumber("days_ahead", mcp.Description("Days to look ahead")),
				// Maintenance params
				mcp.WithNumber("maintenance_id", mcp.Description("Maintenance item ID")),
				mcp.WithString("title", mcp.Description("Maintenance title")),
				mcp.WithString("description", mcp.Description("Description")),
				mcp.WithString("priority", mcp.Description("Priority: low, normal, urgent")),
				mcp.WithString("due_date", mcp.Description("Due date (YYYY-MM-DD)")),
			),
			Handler:     tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:    "life",
			Subcategory: "studio",
			Tags:        []string{"studio", "sessions", "creative", "music", "visual"},
			Complexity:  tools.ComplexityModerate,
			IsWrite:     true,
			Timeout:     30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// --- Session Handlers ---

func handleSessionStart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	activityType := common.GetStringParam(req, "activity_type", "general")
	toolsUsed := common.GetStringParam(req, "tools_used", "")
	project := common.GetStringParam(req, "project", "")
	notes := common.GetStringParam(req, "notes", "")

	// Check for already active session
	var activeID int
	err = db.QueryRowContext(ctx, "SELECT id FROM studio_sessions WHERE ended_at IS NULL ORDER BY started_at DESC LIMIT 1").Scan(&activeID)
	if err == nil {
		return common.CodedErrorResultf(common.ErrInvalidParam, "session #%d already active — end it first", activeID), nil
	}

	now := time.Now().Format("2006-01-02T15:04:05")
	result, err := db.ExecContext(ctx,
		"INSERT INTO studio_sessions (started_at, activity_type, tools_used, project, notes) VALUES (?, ?, ?, ?, ?)",
		now, activityType, toolsUsed, project, notes)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()

	md := common.NewMarkdownBuilder().Title("Studio Session Started")
	md.KeyValue("Session", fmt.Sprintf("#%d", id))
	md.KeyValue("Activity", activityType)
	md.KeyValue("Started", now)
	if project != "" {
		md.KeyValue("Project", project)
	}
	return tools.TextResult(md.String()), nil
}

func handleSessionEnd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	// Find active session
	sessionID := int64(common.GetIntParam(req, "session_id", 0))
	var startedAt string
	var activityType string

	if sessionID > 0 {
		err = db.QueryRowContext(ctx, "SELECT started_at, activity_type FROM studio_sessions WHERE id = ? AND ended_at IS NULL", sessionID).
			Scan(&startedAt, &activityType)
	} else {
		err = db.QueryRowContext(ctx, "SELECT id, started_at, activity_type FROM studio_sessions WHERE ended_at IS NULL ORDER BY started_at DESC LIMIT 1").
			Scan(&sessionID, &startedAt, &activityType)
	}
	if err != nil {
		return common.CodedErrorResultf(common.ErrNotFound, "no active session found"), nil
	}

	now := time.Now()
	endStr := now.Format("2006-01-02T15:04:05")

	startTime, _ := time.Parse("2006-01-02T15:04:05", startedAt)
	durationMin := now.Sub(startTime).Minutes()

	notes := common.GetStringParam(req, "notes", "")
	db.ExecContext(ctx,
		"UPDATE studio_sessions SET ended_at = ?, duration_minutes = ?, notes = CASE WHEN ? = '' THEN notes ELSE notes || ' ' || ? END WHERE id = ?",
		endStr, durationMin, notes, notes, sessionID)

	md := common.NewMarkdownBuilder().Title("Studio Session Ended")
	md.KeyValue("Session", fmt.Sprintf("#%d", sessionID))
	md.KeyValue("Activity", activityType)
	md.KeyValue("Duration", fmt.Sprintf("%.0f minutes (%.1f hours)", durationMin, durationMin/60))
	return tools.TextResult(md.String()), nil
}

func handleSessionList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT id, started_at, ended_at, duration_minutes, activity_type, project
		FROM studio_sessions ORDER BY started_at DESC LIMIT 20`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Studio Sessions")
	var tableRows [][]string
	for rows.Next() {
		var id int
		var startedAt, actType, project string
		var endedAt *string
		var duration float64
		if rows.Scan(&id, &startedAt, &endedAt, &duration, &actType, &project) == nil {
			status := "Active"
			durStr := "ongoing"
			if endedAt != nil {
				status = "Done"
				durStr = fmt.Sprintf("%.0fm", duration)
			}
			// Parse just the date portion
			dateStr := startedAt
			if len(startedAt) >= 10 {
				dateStr = startedAt[:10]
			}
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), dateStr, actType, project, durStr, status,
			})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("studio sessions")
	} else {
		md.Table([]string{"ID", "Date", "Activity", "Project", "Duration", "Status"}, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleSessionStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	now := time.Now()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	md := common.NewMarkdownBuilder().Title("Studio Statistics")

	// This week
	var weekMin float64
	var weekCount int
	db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(duration_minutes), 0), COUNT(*) FROM studio_sessions WHERE started_at >= ? AND ended_at IS NOT NULL",
		weekStart.Format("2006-01-02")).Scan(&weekMin, &weekCount)
	md.KeyValue("This week", fmt.Sprintf("%.1f hours (%d sessions)", weekMin/60, weekCount))

	// This month
	var monthMin float64
	var monthCount int
	db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(duration_minutes), 0), COUNT(*) FROM studio_sessions WHERE started_at >= ? AND ended_at IS NOT NULL",
		monthStart.Format("2006-01-02")).Scan(&monthMin, &monthCount)
	md.KeyValue("This month", fmt.Sprintf("%.1f hours (%d sessions)", monthMin/60, monthCount))

	// All time
	var totalMin float64
	var totalCount int
	db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(duration_minutes), 0), COUNT(*) FROM studio_sessions WHERE ended_at IS NOT NULL").
		Scan(&totalMin, &totalCount)
	md.KeyValue("All time", fmt.Sprintf("%.1f hours (%d sessions)", totalMin/60, totalCount))

	// By activity type
	typeRows, err := db.QueryContext(ctx, `
		SELECT activity_type, COUNT(*), SUM(duration_minutes)
		FROM studio_sessions WHERE ended_at IS NOT NULL
		GROUP BY activity_type ORDER BY SUM(duration_minutes) DESC LIMIT 10`)
	if err == nil {
		defer typeRows.Close()
		var typeTable [][]string
		for typeRows.Next() {
			var aType string
			var count int
			var totalMin float64
			if typeRows.Scan(&aType, &count, &totalMin) == nil {
				typeTable = append(typeTable, []string{
					aType, fmt.Sprintf("%d", count), fmt.Sprintf("%.1fh", totalMin/60),
				})
			}
		}
		if len(typeTable) > 0 {
			md.Section("By Activity")
			md.Table([]string{"Activity", "Sessions", "Total Hours"}, typeTable)
		}
	}

	// Best day of week
	dowRows, err := db.QueryContext(ctx, `
		SELECT
			CASE CAST(strftime('%w', started_at) AS INTEGER)
				WHEN 0 THEN 'Sunday' WHEN 1 THEN 'Monday' WHEN 2 THEN 'Tuesday'
				WHEN 3 THEN 'Wednesday' WHEN 4 THEN 'Thursday' WHEN 5 THEN 'Friday'
				WHEN 6 THEN 'Saturday' END as dow,
			COUNT(*), SUM(duration_minutes)
		FROM studio_sessions WHERE ended_at IS NOT NULL
		GROUP BY strftime('%w', started_at) ORDER BY SUM(duration_minutes) DESC`)
	if err == nil {
		defer dowRows.Close()
		var dowTable [][]string
		for dowRows.Next() {
			var dow string
			var count int
			var mins float64
			if dowRows.Scan(&dow, &count, &mins) == nil {
				dowTable = append(dowTable, []string{dow, fmt.Sprintf("%d", count), fmt.Sprintf("%.1fh", mins/60)})
			}
		}
		if len(dowTable) > 0 {
			md.Section("By Day of Week")
			md.Table([]string{"Day", "Sessions", "Hours"}, dowTable)
		}
	}

	// Days since last session
	var lastSession string
	err = db.QueryRowContext(ctx, "SELECT started_at FROM studio_sessions ORDER BY started_at DESC LIMIT 1").Scan(&lastSession)
	if err == nil {
		if t, err := time.Parse("2006-01-02T15:04:05", lastSession); err == nil {
			daysSince := int(math.Floor(now.Sub(t).Hours() / 24))
			md.KeyValue("Days since last session", fmt.Sprintf("%d", daysSince))
			if daysSince > 14 {
				md.Text("It's been a while — time to get back in the studio!")
			}
		}
	}

	return tools.TextResult(md.String()), nil
}

// --- Schedule Handlers ---

func handleScheduleBook(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	date := common.GetStringParam(req, "date", "")
	if date == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "date is required (YYYY-MM-DD)"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	activityType := common.GetStringParam(req, "activity_type", "general")
	project := common.GetStringParam(req, "project", "")
	startTime := common.GetStringParam(req, "start_time", "19:00")
	notes := common.GetStringParam(req, "notes", fmt.Sprintf("Booked: %s %s", date, startTime))

	startStr := fmt.Sprintf("%sT%s:00", date, startTime)
	result, err := db.ExecContext(ctx,
		"INSERT INTO studio_sessions (started_at, activity_type, project, notes) VALUES (?, ?, ?, ?)",
		startStr, activityType, project, notes)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("Studio time booked: #%d on %s at %s (%s).", id, date, startTime, activityType)), nil
}

func handleScheduleAvailable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	daysAhead := common.GetIntParam(req, "days_ahead", 7)
	now := time.Now()

	md := common.NewMarkdownBuilder().Title("Studio Availability")

	var freeEvenings []string
	for d := 0; d < daysAhead; d++ {
		checkDate := now.AddDate(0, 0, d)
		dayStr := checkDate.Format("2006-01-02")

		// Check calendar events in evening window
		var calConflicts int
		db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM calendar_events
			WHERE start_time < ? AND end_time > ?`,
			dayStr+"T22:00:00", dayStr+"T18:00:00").Scan(&calConflicts)

		// Check booked studio sessions
		var studioConflicts int
		db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM studio_sessions
			WHERE date(started_at) = ? AND ended_at IS NULL`,
			dayStr).Scan(&studioConflicts)

		if calConflicts == 0 && studioConflicts == 0 {
			freeEvenings = append(freeEvenings, fmt.Sprintf("%s (%s)", checkDate.Format("Mon Jan 2"), checkDate.Weekday().String()))
		}
	}

	if len(freeEvenings) > 0 {
		md.KeyValue("Free evenings", fmt.Sprintf("%d of %d", len(freeEvenings), daysAhead))
		md.List(freeEvenings)
	} else {
		md.Text("No free evenings in the window.")
	}

	return tools.TextResult(md.String()), nil
}

// --- Maintenance Handlers ---

func handleMaintenanceList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT id, title, priority, status, due_date
		FROM studio_maintenance
		WHERE status != 'completed'
		ORDER BY
			CASE priority WHEN 'urgent' THEN 0 WHEN 'normal' THEN 1 ELSE 2 END,
			due_date ASC NULLS LAST`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Studio Maintenance")
	var tableRows [][]string
	for rows.Next() {
		var id int
		var title, priority, status string
		var dueDate *string
		if rows.Scan(&id, &title, &priority, &status, &dueDate) == nil {
			due := "-"
			if dueDate != nil {
				due = *dueDate
			}
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), title, priority, status, due,
			})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("maintenance items")
	} else {
		md.Table([]string{"ID", "Title", "Priority", "Status", "Due"}, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleMaintenanceAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	title := common.GetStringParam(req, "title", "")
	if title == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "title is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	desc := common.GetStringParam(req, "description", "")
	priority := common.GetStringParam(req, "priority", "normal")
	dueDate := common.GetStringParam(req, "due_date", "")

	var dueDatePtr interface{} = nil
	if dueDate != "" {
		dueDatePtr = dueDate
	}

	result, err := db.ExecContext(ctx,
		"INSERT INTO studio_maintenance (title, description, priority, due_date) VALUES (?, ?, ?, ?)",
		title, desc, priority, dueDatePtr)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("Maintenance item #%d added: %s [%s].", id, title, priority)), nil
}

func handleMaintenanceComplete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	maintID := int64(common.GetIntParam(req, "maintenance_id", 0))
	if maintID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "maintenance_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	db.ExecContext(ctx,
		"UPDATE studio_maintenance SET status = 'completed', completed_at = datetime('now') WHERE id = ?", maintID)

	return tools.TextResult(fmt.Sprintf("Maintenance #%d completed.", maintID)), nil
}
