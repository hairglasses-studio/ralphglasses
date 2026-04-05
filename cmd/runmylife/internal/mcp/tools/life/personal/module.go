// Package personal provides the personal life category MCP tool.
// Covers reply intelligence (reply radar), daily planning, and routines.
package personal

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/comms"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
	"github.com/hairglasses-studio/runmylife/internal/scoring"
)

type Module struct{}

func (m *Module) Name() string        { return "personal" }
func (m *Module) Description() string { return "Personal life: reply intelligence, daily planning, routines" }

var personalHints = map[string]string{
	"reply_radar/scan":     "Scan all channels for unreplied messages with urgency scores",
	"reply_radar/status":   "Current reply debt dashboard",
	"reply_radar/snooze":   "Snooze a reply reminder for N hours",
	"reply_radar/resolve":  "Mark a tracked message as handled",
	"reply_radar/settings": "View/update contact tiers and reply windows",
	"reply_radar/quality":  "Score a draft reply for quality before sending",
	"reply_radar/batch":    "Get the current batch of replies to process (time-window aware)",
	"daily/plan":           "Generate today's plan from calendar + tasks + reply debt",
	"daily/review":         "End-of-day accomplishments summary",
	"daily/focus":          "Context-aware 'what should I do now?' suggestion",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("personal").
		Domain("reply_radar", common.ActionRegistry{
			"scan":     handleRadarScan,
			"status":   handleRadarStatus,
			"snooze":   handleRadarSnooze,
			"resolve":  handleRadarResolve,
			"settings": handleRadarSettings,
			"quality":  handleRadarQuality,
			"batch":    handleRadarBatch,
		}).
		Domain("daily", common.ActionRegistry{
			"plan":   handleDailyPlan,
			"review": handleDailyReview,
			"focus":  handleDailyFocus,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_personal",
				mcp.WithDescription("Personal life gateway: reply intelligence, daily planning, routines.\n\n"+
					dispatcher.DescribeActionsWithHints(personalHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: reply_radar, daily")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithNumber("tracker_id", mcp.Description("Reply tracker ID (for snooze/resolve)")),
				mcp.WithNumber("snooze_hours", mcp.Description("Hours to snooze (default 4)")),
				mcp.WithString("contact_id", mcp.Description("Contact ID (for settings)")),
				mcp.WithString("tier", mcp.Description("Contact tier: vip, close, normal, low")),
				mcp.WithNumber("reply_window_hours", mcp.Description("Default reply window in hours")),
				mcp.WithString("relationship_type", mcp.Description("Relationship type label")),
			mcp.WithString("text", mcp.Description("Draft reply text (for quality scoring)")),
			),
			Handler:     tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:    "life",
			Subcategory: "personal",
			Tags:        []string{"reply", "radar", "daily", "planning", "personal"},
			Complexity:  tools.ComplexityComplex,
			Timeout:     60 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// --- Reply Radar Handlers ---

func handleRadarScan(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	ensureReplyTables(db)

	// Scan all channels for unreplied messages
	allMsgs, err := comms.ScanAll(ctx, db, "", "") // discord/bluesky IDs from config if available
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	// Load contact importance tiers
	tiers := loadContactTiers(ctx, db)

	// Score each message and upsert into reply_tracker
	var results []comms.ScanResult
	for _, msg := range allMsgs {
		tier := comms.TierNormal
		replyWindow := 24.0
		if imp, ok := tiers[msg.ContactID]; ok {
			tier = imp.tier
			replyWindow = imp.replyWindow
		}

		unrepliedCount := countUnreplied(ctx, db, msg.Channel, msg.ContactID)
		reciprocity := getReciprocityRatio(ctx, db, msg.Channel, msg.ContactID)

		urgency := comms.ScoreUrgency(msg, tier, replyWindow, unrepliedCount, reciprocity)

		// Upsert into reply_tracker
		upsertTracker(ctx, db, msg, urgency)

		results = append(results, comms.ScanResult{Message: msg, Urgency: urgency})
	}

	// Sort by urgency descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Urgency.Total > results[j].Urgency.Total
	})

	// Build output
	md := common.NewMarkdownBuilder().Title("Reply Radar Scan")

	if len(results) == 0 {
		md.Text("All clear! No unreplied messages detected across any channel.")
		return tools.TextResult(md.String()), nil
	}

	// Reply debt score (0-100, lower is better)
	totalUrgency := 0.0
	for _, r := range results {
		totalUrgency += r.Urgency.Total
	}
	debtScore := math.Min(100, totalUrgency*20)
	md.KeyValue("Reply Debt Score", fmt.Sprintf("%.0f/100", debtScore))
	md.KeyValue("Unreplied Messages", fmt.Sprintf("%d", len(results)))

	// Channel breakdown
	channelCounts := make(map[comms.Channel]int)
	for _, r := range results {
		channelCounts[r.Message.Channel]++
	}
	var breakdown []string
	for ch, count := range channelCounts {
		breakdown = append(breakdown, fmt.Sprintf("%s: %d", ch, count))
	}
	md.KeyValue("Channels", strings.Join(breakdown, ", "))

	// Top items table
	md.Section("Priority Queue")
	headers := []string{"#", "Contact", "Channel", "Preview", "Age", "Urgency", "Reason"}
	var rows [][]string
	limit := 15
	if len(results) < limit {
		limit = len(results)
	}
	for i, r := range results[:limit] {
		age := formatAge(r.Message.ReceivedAt)
		urgencyLabel := urgencyLabel(r.Urgency.Total)
		rows = append(rows, []string{
			fmt.Sprintf("%d", i+1),
			r.Message.ContactName,
			string(r.Message.Channel),
			common.TruncateWords(r.Message.Preview, 50),
			age,
			fmt.Sprintf("%.0f%% %s", r.Urgency.Total*100, urgencyLabel),
			r.Urgency.Reason,
		})
	}
	md.Table(headers, rows)

	return tools.TextResult(md.String()), nil
}

func handleRadarStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	ensureReplyTables(db)

	md := common.NewMarkdownBuilder().Title("Reply Radar Status")

	// Pending items
	var pending, snoozed, resolvedToday int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending'").Scan(&pending)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'snoozed' AND snoozed_until > datetime('now')").Scan(&snoozed)
	today := time.Now().Format("2006-01-02")
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'resolved' AND resolved_at >= ?", today).Scan(&resolvedToday)

	md.KeyValue("Pending replies", fmt.Sprintf("%d", pending))
	md.KeyValue("Snoozed", fmt.Sprintf("%d", snoozed))
	md.KeyValue("Resolved today", fmt.Sprintf("%d", resolvedToday))

	// Ghost probability for top contacts
	rows, err := db.QueryContext(ctx, `
		SELECT rt.contact_id, rt.contact_name, rt.channel, rt.urgency_score, rt.received_at,
		       COALESCE(ci.avg_reply_time_minutes, 0), COALESCE(ci.ghost_count, 0), COALESCE(ci.tier, 'normal')
		FROM reply_tracker rt
		LEFT JOIN contact_importance ci ON ci.contact_id = rt.contact_id
		WHERE rt.status = 'pending'
		ORDER BY rt.urgency_score DESC LIMIT 10
	`)
	if err == nil {
		defer rows.Close()
		md.Section("Top Pending")
		headers := []string{"Contact", "Channel", "Urgency", "Ghost %", "Weather"}
		var tableRows [][]string
		for rows.Next() {
			var contactID, contactName, channel, receivedAt, tier string
			var urgencyScore, avgReplyMin float64
			var ghostCount int
			if rows.Scan(&contactID, &contactName, &channel, &urgencyScore, &receivedAt, &avgReplyMin, &ghostCount, &tier) != nil {
				continue
			}
			parsed, _ := time.Parse("2006-01-02T15:04:05-07:00", receivedAt)
			if parsed.IsZero() {
				parsed, _ = time.Parse("2006-01-02 15:04:05", receivedAt)
			}
			hoursSince := time.Since(parsed).Hours()
			ghostProb := comms.GhostProbability(avgReplyMin, hoursSince)
			weather := comms.RelationshipWeather(hoursSince/24, 1.0)

			tableRows = append(tableRows, []string{
				contactName,
				channel,
				fmt.Sprintf("%.0f%%", urgencyScore*100),
				fmt.Sprintf("%.0f%%", ghostProb*100),
				comms.WeatherEmoji(weather) + " " + weather,
			})
		}
		if len(tableRows) > 0 {
			md.Table(headers, tableRows)
		} else {
			md.Text("No pending replies.")
		}
	}

	return tools.TextResult(md.String()), nil
}

func handleRadarSnooze(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	trackerID := int64(common.GetIntParam(req, "tracker_id", 0))
	if trackerID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "tracker_id is required"), nil
	}
	hours := common.GetFloatParam(req, "snooze_hours", 4)

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	snoozedUntil := time.Now().Add(time.Duration(hours * float64(time.Hour))).Format("2006-01-02T15:04:05")
	result, err := db.ExecContext(ctx,
		"UPDATE reply_tracker SET status = 'snoozed', snoozed_until = ?, updated_at = datetime('now') WHERE id = ?",
		snoozedUntil, trackerID)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return common.CodedErrorResultf(common.ErrNotFound, "tracker ID %d not found", trackerID), nil
	}

	return tools.TextResult(fmt.Sprintf("Snoozed reply #%d until %s (%.0f hours).", trackerID, snoozedUntil, hours)), nil
}

func handleRadarResolve(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	trackerID := int64(common.GetIntParam(req, "tracker_id", 0))
	if trackerID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "tracker_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	result, err := db.ExecContext(ctx,
		"UPDATE reply_tracker SET status = 'resolved', resolved_at = datetime('now'), updated_at = datetime('now') WHERE id = ?",
		trackerID)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return common.CodedErrorResultf(common.ErrNotFound, "tracker ID %d not found", trackerID), nil
	}

	return tools.TextResult(fmt.Sprintf("Reply #%d marked as resolved.", trackerID)), nil
}

func handleRadarSettings(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	ensureReplyTables(db)

	contactID := common.GetStringParam(req, "contact_id", "")
	tier := common.GetStringParam(req, "tier", "")
	replyWindow := common.GetFloatParam(req, "reply_window_hours", 0)
	relType := common.GetStringParam(req, "relationship_type", "")

	// If updating a specific contact
	if contactID != "" && (tier != "" || replyWindow > 0 || relType != "") {
		if tier != "" {
			validTiers := map[string]bool{"vip": true, "close": true, "normal": true, "low": true}
			if !validTiers[tier] {
				return common.CodedErrorResultf(common.ErrInvalidParam, "invalid tier: %s (use vip, close, normal, low)", tier), nil
			}
		}

		// Upsert contact_importance
		_, err := db.ExecContext(ctx, `
			INSERT INTO contact_importance (contact_id, tier, default_reply_window_hours, relationship_type)
			VALUES (?, COALESCE(NULLIF(?, ''), 'normal'), CASE WHEN ? > 0 THEN ? ELSE 24 END, COALESCE(NULLIF(?, ''), ''))
			ON CONFLICT(contact_id) DO UPDATE SET
				tier = CASE WHEN ? != '' THEN ? ELSE tier END,
				default_reply_window_hours = CASE WHEN ? > 0 THEN ? ELSE default_reply_window_hours END,
				relationship_type = CASE WHEN ? != '' THEN ? ELSE relationship_type END,
				updated_at = datetime('now')
		`, contactID, tier, replyWindow, replyWindow, relType,
			tier, tier, replyWindow, replyWindow, relType, relType)
		if err != nil {
			return common.CodedErrorResult(common.ErrDBError, err), nil
		}

		return tools.TextResult(fmt.Sprintf("Updated settings for contact %s.", contactID)), nil
	}

	// List all contact importance settings
	rows, err := db.QueryContext(ctx,
		"SELECT contact_id, tier, default_reply_window_hours, relationship_type, interaction_count, ghost_count FROM contact_importance ORDER BY tier, contact_id")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Reply Radar Settings")
	headers := []string{"Contact", "Tier", "Reply Window", "Type", "Interactions", "Ghosts"}
	var tableRows [][]string

	for rows.Next() {
		var cID, cTier, relationType string
		var replyWin float64
		var interactions, ghosts int
		if rows.Scan(&cID, &cTier, &replyWin, &relationType, &interactions, &ghosts) != nil {
			continue
		}
		tableRows = append(tableRows, []string{
			cID, cTier, fmt.Sprintf("%.0fh", replyWin), relationType,
			fmt.Sprintf("%d", interactions), fmt.Sprintf("%d", ghosts),
		})
	}

	if len(tableRows) == 0 {
		md.Text("No contact importance settings configured yet. Use `contact_id` + `tier` to set one.")
	} else {
		md.Table(headers, tableRows)
	}
	md.Section("Tiers")
	md.List([]string{
		"**vip** — family, partner, closest friends (reply within 2-4h)",
		"**close** — good friends, important colleagues (reply within 8-12h)",
		"**normal** — regular contacts (reply within 24h)",
		"**low** — newsletters, acquaintances (reply when convenient)",
	})

	return tools.TextResult(md.String()), nil
}

// --- Daily Handlers ---

func handleDailyPlan(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	today := time.Now().Format("2006-01-02")
	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	dow := time.Now().Format("Monday")

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Daily Plan — %s, %s", dow, today))

	// Reply debt
	var pendingReplies int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending'").Scan(&pendingReplies)
	if pendingReplies > 0 {
		md.Section("Reply Radar")
		md.KeyValue("Pending replies", fmt.Sprintf("%d", pendingReplies))

		// Top 3 urgent
		urgentRows, err := db.QueryContext(ctx,
			"SELECT contact_name, channel, urgency_score FROM reply_tracker WHERE status = 'pending' ORDER BY urgency_score DESC LIMIT 3")
		if err == nil {
			defer urgentRows.Close()
			var urgentList []string
			for urgentRows.Next() {
				var name, ch string
				var score float64
				if urgentRows.Scan(&name, &ch, &score) == nil {
					urgentList = append(urgentList, fmt.Sprintf("%s (%s) — %.0f%% urgency", name, ch, score*100))
				}
			}
			if len(urgentList) > 0 {
				md.List(urgentList)
			}
		}
	}

	// Calendar
	calRows, err := db.QueryContext(ctx,
		"SELECT summary, start_time, end_time, location FROM calendar_events WHERE start_time >= ? AND start_time < ? ORDER BY start_time ASC",
		today, tomorrow)
	if err == nil {
		defer calRows.Close()
		md.Section("Calendar")
		var events [][]string
		for calRows.Next() {
			var summary, start, end, location string
			if calRows.Scan(&summary, &start, &end, &location) == nil {
				loc := ""
				if location != "" {
					loc = " @ " + location
				}
				events = append(events, []string{formatTime(start), formatTime(end), summary + loc})
			}
		}
		if len(events) == 0 {
			md.Text("No events scheduled.")
		} else {
			md.Table([]string{"Start", "End", "Event"}, events)
		}
	}

	// Tasks due today + overdue
	var overdue, dueToday, openTasks int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0 AND due_date < ? AND due_date != ''", today).Scan(&overdue)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0 AND due_date = ?", today).Scan(&dueToday)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0").Scan(&openTasks)

	md.Section("Tasks")
	md.KeyValue("Open", fmt.Sprintf("%d", openTasks))
	if overdue > 0 {
		md.KeyValue("OVERDUE", fmt.Sprintf("%d", overdue))
	}
	md.KeyValue("Due today", fmt.Sprintf("%d", dueToday))

	taskRows, err := db.QueryContext(ctx,
		"SELECT title, due_date, priority FROM tasks WHERE completed = 0 AND due_date <= ? AND due_date != '' ORDER BY due_date ASC, priority DESC LIMIT 7",
		today)
	if err == nil {
		defer taskRows.Close()
		var taskList [][]string
		for taskRows.Next() {
			var title, dueDate string
			var priority int
			if taskRows.Scan(&title, &dueDate, &priority) == nil {
				taskList = append(taskList, []string{title, dueDate, fmt.Sprintf("P%d", priority)})
			}
		}
		if len(taskList) > 0 {
			md.Table([]string{"Task", "Due", "Priority"}, taskList)
		}
	}

	// Email
	var untriaged int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gmail_messages WHERE triaged = 0").Scan(&untriaged)
	if untriaged > 0 {
		md.Section("Email")
		md.KeyValue("Untriaged", fmt.Sprintf("%d", untriaged))
	}

	// Habits
	var habitsCompleted, totalHabits int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habit_completions WHERE completed_at >= ?", today).Scan(&habitsCompleted)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habits WHERE archived = 0").Scan(&totalHabits)
	if totalHabits > 0 {
		md.Section("Habits")
		md.KeyValue("Progress", fmt.Sprintf("%d / %d", habitsCompleted, totalHabits))
	}

	return tools.TextResult(md.String()), nil
}

func handleDailyReview(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	today := time.Now().Format("2006-01-02")
	dow := time.Now().Format("Monday")

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Daily Review — %s, %s", dow, today))

	// Tasks completed today
	var tasksCompleted int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 1 AND updated_at >= ?", today).Scan(&tasksCompleted)
	md.Section("Accomplishments")
	md.KeyValue("Tasks completed", fmt.Sprintf("%d", tasksCompleted))

	// Habits
	var habitsCompleted, totalHabits int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habit_completions WHERE completed_at >= ?", today).Scan(&habitsCompleted)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM habits WHERE archived = 0").Scan(&totalHabits)
	md.KeyValue("Habits", fmt.Sprintf("%d / %d", habitsCompleted, totalHabits))

	// Emails triaged
	var emailsTriaged int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gmail_messages WHERE triaged = 1 AND updated_at >= ?", today).Scan(&emailsTriaged)
	md.KeyValue("Emails triaged", fmt.Sprintf("%d", emailsTriaged))

	// Replies handled today
	var repliesResolved int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'resolved' AND resolved_at >= ?", today).Scan(&repliesResolved)
	md.KeyValue("Replies sent", fmt.Sprintf("%d", repliesResolved))

	// Spending
	var todaySpend float64
	db.QueryRowContext(ctx, "SELECT COALESCE(SUM(amount), 0) FROM transactions WHERE date = ? AND type = 'expense'", today).Scan(&todaySpend)
	md.KeyValue("Spent today", fmt.Sprintf("$%.2f", todaySpend))

	// Still pending
	md.Section("Still Outstanding")
	var pendingReplies, openTasks int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending'").Scan(&pendingReplies)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0").Scan(&openTasks)
	md.KeyValue("Pending replies", fmt.Sprintf("%d", pendingReplies))
	md.KeyValue("Open tasks", fmt.Sprintf("%d", openTasks))

	return tools.TextResult(md.String()), nil
}

func handleDailyFocus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	now := time.Now()
	hour := now.Hour()
	today := now.Format("2006-01-02")

	md := common.NewMarkdownBuilder().Title("What Should I Do Now?")

	var timeBlock string
	switch {
	case hour >= 6 && hour < 9:
		timeBlock = "morning"
		md.KeyValue("Time context", "Morning (6-9am) — briefing, planning, quick replies")
	case hour >= 9 && hour < 18:
		timeBlock = "work"
		md.KeyValue("Time context", "Work hours (9am-6pm) — deep work, tasks, professional replies")
	case hour >= 18 && hour < 22:
		timeBlock = "evening"
		md.KeyValue("Time context", "Evening (6-10pm) — personal, social, partner time")
	default:
		timeBlock = "night"
		md.KeyValue("Time context", "Night (10pm+) — wind down, light tasks, tomorrow prep")
	}

	var suggestions []string

	// Check urgent replies
	var urgentReplies int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending' AND urgency_score > 0.5").Scan(&urgentReplies)
	if urgentReplies > 0 {
		suggestions = append(suggestions, fmt.Sprintf("Reply to %d urgent messages (reply_radar/scan for details)", urgentReplies))
	}

	// Check overdue tasks
	var overdueTasks int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0 AND due_date < ? AND due_date != ''", today).Scan(&overdueTasks)
	if overdueTasks > 0 {
		suggestions = append(suggestions, fmt.Sprintf("Address %d overdue tasks", overdueTasks))
	}

	// Time-block specific suggestions
	switch timeBlock {
	case "morning":
		var untriaged int
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM gmail_messages WHERE triaged = 0").Scan(&untriaged)
		if untriaged > 5 {
			suggestions = append(suggestions, fmt.Sprintf("Triage %d emails", untriaged))
		}
		var habitsRemaining int
		db.QueryRowContext(ctx, `SELECT COUNT(*) FROM habits WHERE archived = 0 AND id NOT IN (SELECT habit_id FROM habit_completions WHERE completed_at >= ?)`, today).Scan(&habitsRemaining)
		if habitsRemaining > 0 {
			suggestions = append(suggestions, fmt.Sprintf("Complete %d morning habits", habitsRemaining))
		}
	case "work":
		var dueToday int
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tasks WHERE completed = 0 AND due_date = ?", today).Scan(&dueToday)
		if dueToday > 0 {
			suggestions = append(suggestions, fmt.Sprintf("Complete %d tasks due today", dueToday))
		}
	case "evening":
		var pendingReplies int
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM reply_tracker WHERE status = 'pending'").Scan(&pendingReplies)
		if pendingReplies > 0 {
			suggestions = append(suggestions, fmt.Sprintf("Clear %d pending personal replies", pendingReplies))
		}
	case "night":
		suggestions = append(suggestions, "Review tomorrow's calendar")
		suggestions = append(suggestions, "Run daily/review to see today's accomplishments")
	}

	if len(suggestions) == 0 {
		md.Text("You're all caught up! Consider reviewing your goals or doing something creative.")
	} else {
		md.Section("Suggestions")
		md.List(suggestions)
	}

	return tools.TextResult(md.String()), nil
}

// --- Reply Quality Gate ---

func handleRadarQuality(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, ok := common.RequireStringParam(req, "text")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "text required: provide draft reply text to score"), nil
	}

	score := scoring.ScoreText(text)

	md := common.NewMarkdownBuilder().Title("Reply Quality Gate")

	// Traffic light verdict
	switch {
	case score.Overall >= 70:
		md.KeyValue("Verdict", "PASS — good to send")
	case score.Overall >= 50:
		md.KeyValue("Verdict", "REVIEW — consider improvements below")
	default:
		md.KeyValue("Verdict", "REVISE — quality too low for sending")
	}

	md.Text(scoring.FormatScore(score))
	return tools.TextResult(md.String()), nil
}

// --- Batch Reply Window ---

func handleRadarBatch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	now := time.Now()
	hour := now.Hour()
	dayName := now.Weekday().String()[:3]

	// Check if we're in a reply batch window
	var windowName string
	err = db.QueryRowContext(ctx,
		`SELECT window_name FROM reply_batch_windows
		 WHERE enabled = 1 AND start_hour <= ? AND end_hour > ?
		   AND days_of_week LIKE ?`,
		hour, hour, "%"+dayName+"%",
	).Scan(&windowName)

	md := common.NewMarkdownBuilder()

	if err != nil || windowName == "" {
		// Not in a batch window — show next window
		var nextWindow string
		var startHour int
		_ = db.QueryRowContext(ctx,
			`SELECT window_name, start_hour FROM reply_batch_windows
			 WHERE enabled = 1 AND start_hour > ? AND days_of_week LIKE ?
			 ORDER BY start_hour ASC LIMIT 1`,
			hour, "%"+dayName+"%",
		).Scan(&nextWindow, &startHour)

		md.Title("Reply Batch — Not in Window")
		if nextWindow != "" {
			md.KeyValue("Next window", fmt.Sprintf("%s at %d:00", nextWindow, startHour))
		} else {
			md.Text("No more reply windows today.")
		}

		// Still show high-urgency items that shouldn't wait
		rows, _ := db.QueryContext(ctx,
			`SELECT contact_name, channel, message_preview, urgency_score
			 FROM reply_tracker WHERE status = 'pending' AND urgency_score >= 0.6
			 ORDER BY urgency_score DESC LIMIT 5`)
		if rows != nil {
			defer rows.Close()
			var urgentItems []string
			for rows.Next() {
				var name, ch, preview string
				var score float64
				if rows.Scan(&name, &ch, &preview, &score) == nil {
					if len(preview) > 40 {
						preview = preview[:37] + "..."
					}
					urgentItems = append(urgentItems, fmt.Sprintf("[%.0f%%] %s (%s): %s", score*100, name, ch, preview))
				}
			}
			if len(urgentItems) > 0 {
				md.Section("Urgent (can't wait for window)")
				md.List(urgentItems)
			}
		}

		return tools.TextResult(md.String()), nil
	}

	// We're in a batch window — load pending replies sorted by urgency
	md.Title(fmt.Sprintf("Reply Batch — %s Window", windowName))

	rows, err := db.QueryContext(ctx,
		`SELECT id, contact_name, channel, message_preview, urgency_score, urgency_reason, received_at
		 FROM reply_tracker WHERE status = 'pending'
		   AND (snoozed_until IS NULL OR snoozed_until < datetime('now'))
		 ORDER BY urgency_score DESC`)
	if err != nil {
		return common.CodedErrorResultf(common.ErrDBError, "query: %v", err), nil
	}
	defer rows.Close()

	headers := []string{"#", "Contact", "Channel", "Preview", "Urgency", "Age"}
	var tableRows [][]string
	i := 0
	for rows.Next() {
		var id int
		var name, ch, preview, reason, receivedAt string
		var score float64
		if rows.Scan(&id, &name, &ch, &preview, &score, &reason, &receivedAt) != nil {
			continue
		}
		i++
		if len(preview) > 40 {
			preview = preview[:37] + "..."
		}
		recvTime, _ := time.Parse("2006-01-02T15:04:05-07:00", receivedAt)
		age := formatAge(recvTime)
		tableRows = append(tableRows, []string{
			fmt.Sprintf("%d (id:%d)", i, id),
			name, ch, preview,
			fmt.Sprintf("%.0f%%", score*100), age,
		})
	}

	if len(tableRows) == 0 {
		md.Text("No pending replies! You're all caught up.")
	} else {
		md.Table(headers, tableRows)
		md.Text(fmt.Sprintf("\n%d replies to process. Use `reply_radar/quality` to score drafts before sending.", len(tableRows)))
	}

	return tools.TextResult(md.String()), nil
}

// --- Helper functions ---

type contactImportance struct {
	tier        comms.ContactTier
	replyWindow float64
}

func loadContactTiers(ctx context.Context, db *sql.DB) map[string]contactImportance {
	tiers := make(map[string]contactImportance)
	rows, err := db.QueryContext(ctx, "SELECT contact_id, tier, default_reply_window_hours FROM contact_importance")
	if err != nil {
		return tiers
	}
	defer rows.Close()
	for rows.Next() {
		var id, tier string
		var window float64
		if rows.Scan(&id, &tier, &window) == nil {
			tiers[id] = contactImportance{
				tier:        comms.ContactTier(tier),
				replyWindow: window,
			}
		}
	}
	return tiers
}

func countUnreplied(ctx context.Context, db *sql.DB, channel comms.Channel, contactID string) int {
	var count int
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM reply_tracker WHERE channel = ? AND contact_id = ? AND status = 'pending'",
		string(channel), contactID).Scan(&count)
	return count
}

func getReciprocityRatio(ctx context.Context, db *sql.DB, channel comms.Channel, contactID string) float64 {
	switch channel {
	case comms.ChannelSMS:
		var incoming, outgoing int
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sms_messages WHERE conversation_id IN (SELECT id FROM sms_conversations WHERE participant = ?) AND direction = 'incoming'", contactID).Scan(&incoming)
		db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sms_messages WHERE conversation_id IN (SELECT id FROM sms_conversations WHERE participant = ?) AND direction = 'outgoing'", contactID).Scan(&outgoing)
		if outgoing == 0 {
			return float64(incoming)
		}
		return float64(incoming) / float64(outgoing)
	default:
		return 1.0 // neutral if we can't determine
	}
}

func upsertTracker(ctx context.Context, db *sql.DB, msg comms.UnifiedMessage, urgency comms.UrgencyFactors) {
	db.ExecContext(ctx, `
		INSERT INTO reply_tracker (channel, channel_message_id, contact_id, contact_name, message_preview, received_at, urgency_score, urgency_reason, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending')
		ON CONFLICT(channel, channel_message_id) DO UPDATE SET
			urgency_score = ?,
			urgency_reason = ?,
			updated_at = datetime('now')
	`, string(msg.Channel), msg.ChannelMessageID, msg.ContactID, msg.ContactName,
		msg.Preview, msg.ReceivedAt.Format("2006-01-02T15:04:05-07:00"),
		urgency.Total, urgency.Reason,
		urgency.Total, urgency.Reason)
}

func ensureReplyTables(db *sql.DB) {
	db.Exec(`CREATE TABLE IF NOT EXISTS reply_tracker (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		channel TEXT NOT NULL,
		channel_message_id TEXT NOT NULL,
		contact_id TEXT NOT NULL,
		contact_name TEXT NOT NULL DEFAULT '',
		message_preview TEXT DEFAULT '',
		received_at TEXT NOT NULL,
		urgency_score REAL DEFAULT 0,
		urgency_reason TEXT DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		snoozed_until TEXT,
		resolved_at TEXT,
		created_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now')),
		UNIQUE(channel, channel_message_id)
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS contact_importance (
		contact_id TEXT PRIMARY KEY,
		tier TEXT NOT NULL DEFAULT 'normal',
		default_reply_window_hours REAL DEFAULT 24,
		relationship_type TEXT DEFAULT '',
		last_interaction_at TEXT,
		interaction_count INTEGER DEFAULT 0,
		avg_reply_time_minutes REAL DEFAULT 0,
		ghost_count INTEGER DEFAULT 0,
		created_at TEXT DEFAULT (datetime('now')),
		updated_at TEXT DEFAULT (datetime('now'))
	)`)
}

func formatTime(ts string) string {
	for _, layout := range []string{
		"2006-01-02T15:04:05-07:00",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.Format("3:04 PM")
		}
	}
	return ts
}

func formatAge(t time.Time) string {
	if t.IsZero() {
		return "unknown"
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func urgencyLabel(score float64) string {
	switch {
	case score >= 0.7:
		return "[URGENT]"
	case score >= 0.4:
		return "[HIGH]"
	case score >= 0.2:
		return "[MEDIUM]"
	default:
		return "[LOW]"
	}
}
