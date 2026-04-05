// Package social provides the Social Health life category MCP tool.
// Covers relationship health scoring, social circles, and outreach reminders.
package social

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

func (m *Module) Name() string        { return "social" }
func (m *Module) Description() string { return "Social health: relationship scoring, circles, outreach" }

var socialHints = map[string]string{
	"health/dashboard": "Relationship health across all contacts",
	"health/contact":   "Deep dive on a single relationship",
	"health/at_risk":   "Contacts going stale",
	"health/calculate": "Recalculate health scores from interaction data",
	"circles/list":     "View social circles",
	"circles/add":      "Add contact to a circle",
	"circles/remove":   "Remove contact from circle",
	"outreach/due":     "Contacts due for outreach",
	"outreach/add":     "Set up recurring outreach reminder",
	"outreach/done":    "Mark outreach as done",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("social").
		Domain("health", common.ActionRegistry{
			"dashboard": handleHealthDashboard,
			"contact":   handleHealthContact,
			"at_risk":   handleHealthAtRisk,
			"calculate": handleHealthCalculate,
		}).
		Domain("circles", common.ActionRegistry{
			"list":   handleCirclesList,
			"add":    handleCirclesAdd,
			"remove": handleCirclesRemove,
		}).
		Domain("outreach", common.ActionRegistry{
			"due":  handleOutreachDue,
			"add":  handleOutreachAdd,
			"done": handleOutreachDone,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_social",
				mcp.WithDescription("Social health gateway.\n\n"+
					dispatcher.DescribeActionsWithHints(socialHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: health, circles, outreach")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				// Contact params
				mcp.WithString("contact_id", mcp.Description("Contact ID")),
				mcp.WithString("contact_name", mcp.Description("Contact name")),
				// Circle params
				mcp.WithString("circle", mcp.Description("Circle name")),
				mcp.WithNumber("circle_id", mcp.Description("Circle ID")),
				// Outreach params
				mcp.WithNumber("reminder_id", mcp.Description("Outreach reminder ID")),
				mcp.WithNumber("frequency_days", mcp.Description("How often to reach out (days)")),
				mcp.WithString("channel_preference", mcp.Description("Preferred channel: sms, discord, gmail, etc.")),
				mcp.WithString("notes", mcp.Description("Notes")),
			),
			Handler:     tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:    "life",
			Subcategory: "social",
			Tags:        []string{"social", "relationships", "outreach", "circles"},
			Complexity:  tools.ComplexityModerate,
			IsWrite:     true,
			Timeout:     30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// --- Health Handlers ---

func handleHealthDashboard(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	md := common.NewMarkdownBuilder().Title("Social Health Dashboard")

	// Overall stats
	var totalTracked int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM relationship_health").Scan(&totalTracked)
	md.KeyValue("Contacts tracked", fmt.Sprintf("%d", totalTracked))

	// Health distribution
	var healthy, warning, critical int
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM relationship_health WHERE health_score >= 70").Scan(&healthy)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM relationship_health WHERE health_score >= 40 AND health_score < 70").Scan(&warning)
	db.QueryRowContext(ctx, "SELECT COUNT(*) FROM relationship_health WHERE health_score < 40").Scan(&critical)
	md.KeyValue("Healthy (70+)", fmt.Sprintf("%d", healthy))
	md.KeyValue("Warning (40-69)", fmt.Sprintf("%d", warning))
	md.KeyValue("Critical (<40)", fmt.Sprintf("%d", critical))

	// Top relationships
	topRows, err := db.QueryContext(ctx, `
		SELECT contact_name, health_score, recency_score, reciprocity_score
		FROM relationship_health ORDER BY health_score DESC LIMIT 10`)
	if err == nil {
		defer topRows.Close()
		var topTable [][]string
		for topRows.Next() {
			var name string
			var score, recency, reciprocity float64
			if topRows.Scan(&name, &score, &recency, &reciprocity) == nil {
				weather := healthWeather(score)
				topTable = append(topTable, []string{
					name, fmt.Sprintf("%.0f", score), weather,
					fmt.Sprintf("%.0f", recency), fmt.Sprintf("%.0f", reciprocity),
				})
			}
		}
		if len(topTable) > 0 {
			md.Section("Top Relationships")
			md.Table([]string{"Contact", "Score", "Status", "Recency", "Reciprocity"}, topTable)
		}
	}

	// Upcoming outreach
	var dueCount int
	db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM outreach_reminders WHERE active = 1 AND next_outreach_at <= datetime('now')").
		Scan(&dueCount)
	if dueCount > 0 {
		md.Section("Outreach Due")
		md.Text(fmt.Sprintf("%d contacts due for outreach. Use `outreach/due` for details.", dueCount))
	}

	return tools.TextResult(md.String()), nil
}

func handleHealthContact(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contactID := common.GetStringParam(req, "contact_id", "")
	contactName := common.GetStringParam(req, "contact_name", "")
	if contactID == "" && contactName == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "contact_id or contact_name required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	var name string
	var score, recency, reciprocity, frequency, responsiveness, quality float64
	var lastCalc string

	query := "SELECT contact_name, health_score, recency_score, reciprocity_score, frequency_score, responsiveness_score, quality_score, last_calculated_at FROM relationship_health WHERE "
	var arg interface{}
	if contactID != "" {
		query += "contact_id = ?"
		arg = contactID
	} else {
		query += "contact_name LIKE ?"
		arg = "%" + contactName + "%"
	}

	err = db.QueryRowContext(ctx, query, arg).
		Scan(&name, &score, &recency, &reciprocity, &frequency, &responsiveness, &quality, &lastCalc)
	if err != nil {
		return common.CodedErrorResultf(common.ErrNotFound, "contact not found in health tracking"), nil
	}

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Relationship: %s", name))
	md.KeyValue("Overall Health", fmt.Sprintf("%.0f/100 %s", score, healthWeather(score)))
	md.Section("Score Breakdown")
	md.KeyValue("Recency (30pts)", fmt.Sprintf("%.0f", recency))
	md.KeyValue("Reciprocity (25pts)", fmt.Sprintf("%.0f", reciprocity))
	md.KeyValue("Frequency (20pts)", fmt.Sprintf("%.0f", frequency))
	md.KeyValue("Responsiveness (15pts)", fmt.Sprintf("%.0f", responsiveness))
	md.KeyValue("Quality (10pts)", fmt.Sprintf("%.0f", quality))
	md.KeyValue("Last calculated", lastCalc)

	// Check circles
	circleRows, err := db.QueryContext(ctx, `
		SELECT sc.name FROM social_circles sc
		JOIN contact_circles cc ON sc.id = cc.circle_id
		WHERE cc.contact_id = ?`, func() string {
		if contactID != "" {
			return contactID
		}
		return ""
	}())
	if err == nil {
		defer circleRows.Close()
		var circles []string
		for circleRows.Next() {
			var c string
			if circleRows.Scan(&c) == nil {
				circles = append(circles, c)
			}
		}
		if len(circles) > 0 {
			md.KeyValue("Circles", fmt.Sprintf("%v", circles))
		}
	}

	// Check contact_importance (reply radar)
	var tier string
	var avgReply float64
	err = db.QueryRowContext(ctx,
		"SELECT tier, avg_reply_time_minutes FROM contact_importance WHERE contact_id = ?", contactID).
		Scan(&tier, &avgReply)
	if err == nil {
		md.Section("Reply Intelligence")
		md.KeyValue("Tier", tier)
		if avgReply > 0 {
			md.KeyValue("Avg reply time", fmt.Sprintf("%.0f min", avgReply))
		}
	}

	return tools.TextResult(md.String()), nil
}

func handleHealthAtRisk(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT contact_name, health_score, recency_score, reciprocity_score
		FROM relationship_health
		WHERE health_score < 50
		ORDER BY health_score ASC LIMIT 15`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("At-Risk Relationships")
	var tableRows [][]string
	for rows.Next() {
		var name string
		var score, recency, reciprocity float64
		if rows.Scan(&name, &score, &recency, &reciprocity) == nil {
			risk := "Fading"
			if score < 25 {
				risk = "Drought"
			} else if score < 40 {
				risk = "Stormy"
			}
			tableRows = append(tableRows, []string{
				name, fmt.Sprintf("%.0f", score), risk,
				fmt.Sprintf("%.0f", recency), fmt.Sprintf("%.0f", reciprocity),
			})
		}
	}
	if len(tableRows) == 0 {
		md.Text("No at-risk relationships. All healthy!")
	} else {
		md.Table([]string{"Contact", "Score", "Status", "Recency", "Reciprocity"}, tableRows)
		md.Text("Consider reaching out to contacts in drought or stormy status.")
	}
	return tools.TextResult(md.String()), nil
}

func handleHealthCalculate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	now := time.Now()
	updated := 0

	// Calculate from contact_importance + interaction data
	rows, err := db.QueryContext(ctx, `
		SELECT ci.contact_id, COALESCE(c.name, ci.contact_id),
		       ci.last_interaction_at, ci.interaction_count, ci.avg_reply_time_minutes, ci.tier
		FROM contact_importance ci
		LEFT JOIN contacts c ON c.id = ci.contact_id`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	for rows.Next() {
		var contactID, name, tier string
		var lastInteraction *string
		var interactionCount int
		var avgReply float64
		if rows.Scan(&contactID, &name, &lastInteraction, &interactionCount, &avgReply, &tier) != nil {
			continue
		}

		// Recency score (0-30): days since last interaction
		recency := 0.0
		if lastInteraction != nil {
			if t, err := time.Parse("2006-01-02T15:04:05", *lastInteraction); err == nil {
				daysSince := now.Sub(t).Hours() / 24
				// Exponential decay: full score at 0 days, half at 14 days
				recency = 30.0 * math.Exp(-0.05*daysSince)
			}
		}

		// Frequency score (0-20): based on interaction count
		frequency := math.Min(20, float64(interactionCount)*0.5)

		// Responsiveness score (0-15): based on avg reply time
		responsiveness := 15.0
		if avgReply > 0 {
			// Full score at <=30min, decays toward 0 at 24h+
			responsiveness = 15.0 * math.Exp(-avgReply/480)
		}

		// Reciprocity estimate (0-25): tier-based proxy
		reciprocity := 12.5 // default middle
		switch tier {
		case "vip":
			reciprocity = 25
		case "close":
			reciprocity = 20
		case "normal":
			reciprocity = 12.5
		case "low":
			reciprocity = 5
		}

		// Quality score (0-10): placeholder based on tier
		quality := 5.0
		switch tier {
		case "vip":
			quality = 10
		case "close":
			quality = 8
		}

		total := recency + reciprocity + frequency + responsiveness + quality

		db.ExecContext(ctx, `
			INSERT INTO relationship_health (contact_id, contact_name, health_score, recency_score, reciprocity_score, frequency_score, responsiveness_score, quality_score, last_calculated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
			ON CONFLICT(contact_id) DO UPDATE SET
				contact_name = ?, health_score = ?, recency_score = ?, reciprocity_score = ?,
				frequency_score = ?, responsiveness_score = ?, quality_score = ?,
				last_calculated_at = datetime('now')`,
			contactID, name, total, recency, reciprocity, frequency, responsiveness, quality,
			name, total, recency, reciprocity, frequency, responsiveness, quality)
		updated++
	}

	return tools.TextResult(fmt.Sprintf("Recalculated health scores for %d contacts.", updated)), nil
}

// --- Circles Handlers ---

func handleCirclesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT sc.id, sc.name, sc.description, COUNT(cc.contact_id) as member_count
		FROM social_circles sc
		LEFT JOIN contact_circles cc ON sc.id = cc.circle_id
		GROUP BY sc.id ORDER BY sc.name`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Social Circles")
	var tableRows [][]string
	for rows.Next() {
		var id, memberCount int
		var name, desc string
		if rows.Scan(&id, &name, &desc, &memberCount) == nil {
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), name, desc, fmt.Sprintf("%d", memberCount),
			})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("social circles")
	} else {
		md.Table([]string{"ID", "Circle", "Description", "Members"}, tableRows)
	}

	// Show members of each circle
	circle := common.GetStringParam(req, "circle", "")
	if circle != "" {
		memberRows, err := db.QueryContext(ctx, `
			SELECT COALESCE(c.name, cc.contact_id)
			FROM contact_circles cc
			JOIN social_circles sc ON sc.id = cc.circle_id
			LEFT JOIN contacts c ON c.id = cc.contact_id
			WHERE sc.name = ?`, circle)
		if err == nil {
			defer memberRows.Close()
			md.Section(fmt.Sprintf("Members of '%s'", circle))
			var members []string
			for memberRows.Next() {
				var name string
				if memberRows.Scan(&name) == nil {
					members = append(members, name)
				}
			}
			if len(members) > 0 {
				md.List(members)
			} else {
				md.Text("No members yet.")
			}
		}
	}

	return tools.TextResult(md.String()), nil
}

func handleCirclesAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contactID := common.GetStringParam(req, "contact_id", "")
	circle := common.GetStringParam(req, "circle", "")
	if contactID == "" || circle == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "contact_id and circle are required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	// Find or create circle
	var circleID int
	err = db.QueryRowContext(ctx, "SELECT id FROM social_circles WHERE name = ?", circle).Scan(&circleID)
	if err != nil {
		result, err := db.ExecContext(ctx, "INSERT INTO social_circles (name) VALUES (?)", circle)
		if err != nil {
			return common.CodedErrorResult(common.ErrDBError, err), nil
		}
		id, _ := result.LastInsertId()
		circleID = int(id)
	}

	_, err = db.ExecContext(ctx,
		"INSERT OR IGNORE INTO contact_circles (contact_id, circle_id) VALUES (?, ?)",
		contactID, circleID)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("Added %s to circle '%s'.", contactID, circle)), nil
}

func handleCirclesRemove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contactID := common.GetStringParam(req, "contact_id", "")
	circle := common.GetStringParam(req, "circle", "")
	if contactID == "" || circle == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "contact_id and circle are required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	db.ExecContext(ctx, `
		DELETE FROM contact_circles WHERE contact_id = ?
		AND circle_id = (SELECT id FROM social_circles WHERE name = ?)`,
		contactID, circle)

	return tools.TextResult(fmt.Sprintf("Removed %s from circle '%s'.", contactID, circle)), nil
}

// --- Outreach Handlers ---

func handleOutreachDue(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT id, contact_name, frequency_days, last_outreach_at, next_outreach_at, channel_preference, notes
		FROM outreach_reminders
		WHERE active = 1 AND next_outreach_at <= datetime('now')
		ORDER BY next_outreach_at ASC`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Outreach Due")
	var tableRows [][]string
	for rows.Next() {
		var id, freq int
		var name, channel, notes string
		var lastOutreach, nextOutreach *string
		if rows.Scan(&id, &name, &freq, &lastOutreach, &nextOutreach, &channel, &notes) == nil {
			last := "never"
			if lastOutreach != nil {
				last = (*lastOutreach)[:10]
			}
			overdue := ""
			if nextOutreach != nil {
				if t, err := time.Parse("2006-01-02 15:04:05", *nextOutreach); err == nil {
					days := int(time.Since(t).Hours() / 24)
					if days > 0 {
						overdue = fmt.Sprintf("%dd overdue", days)
					}
				}
			}
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), name, fmt.Sprintf("%dd", freq), last, channel, overdue,
			})
		}
	}
	if len(tableRows) == 0 {
		md.Text("No outreach due. You're keeping up!")
	} else {
		md.Table([]string{"ID", "Contact", "Freq", "Last", "Channel", "Status"}, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleOutreachAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	contactID := common.GetStringParam(req, "contact_id", "")
	contactName := common.GetStringParam(req, "contact_name", "")
	if contactID == "" && contactName == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "contact_id or contact_name required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	freqDays := common.GetIntParam(req, "frequency_days", 30)
	channel := common.GetStringParam(req, "channel_preference", "")
	notes := common.GetStringParam(req, "notes", "")

	nextOutreach := time.Now().AddDate(0, 0, freqDays).Format("2006-01-02 15:04:05")

	result, err := db.ExecContext(ctx,
		`INSERT INTO outreach_reminders (contact_id, contact_name, frequency_days, next_outreach_at, channel_preference, notes)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		contactID, contactName, freqDays, nextOutreach, channel, notes)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("Outreach reminder #%d: reach out to %s every %d days.", id, contactName, freqDays)), nil
}

func handleOutreachDone(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	reminderID := int64(common.GetIntParam(req, "reminder_id", 0))
	if reminderID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "reminder_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	// Get frequency to calculate next outreach
	var freqDays int
	err = db.QueryRowContext(ctx, "SELECT frequency_days FROM outreach_reminders WHERE id = ?", reminderID).Scan(&freqDays)
	if err != nil {
		return common.CodedErrorResultf(common.ErrNotFound, "reminder %d not found", reminderID), nil
	}

	now := time.Now()
	next := now.AddDate(0, 0, freqDays).Format("2006-01-02 15:04:05")

	db.ExecContext(ctx,
		"UPDATE outreach_reminders SET last_outreach_at = datetime('now'), next_outreach_at = ? WHERE id = ?",
		next, reminderID)

	return tools.TextResult(fmt.Sprintf("Outreach #%d done. Next due in %d days.", reminderID, freqDays)), nil
}

// --- Helpers ---

func healthWeather(score float64) string {
	switch {
	case score >= 75:
		return "Sunny"
	case score >= 50:
		return "Cloudy"
	case score >= 25:
		return "Stormy"
	default:
		return "Drought"
	}
}
