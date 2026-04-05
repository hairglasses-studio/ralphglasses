// Package partner provides the Partner life category MCP tool.
// Covers relationship coordination: calendar overlap, date planning, quality time, gifts.
package partner

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

type Module struct{}

func (m *Module) Name() string        { return "partner" }
func (m *Module) Description() string { return "Partner coordination: calendar overlap, date planning, quality time" }

var partnerHints = map[string]string{
	"calendar/overlap":   "Find shared free time across both calendars",
	"calendar/suggest":   "Suggest date windows from mutual availability",
	"dates/ideas":        "Browse and generate date ideas",
	"dates/add_idea":     "Add a new date idea",
	"dates/log":          "Log a completed date",
	"dates/history":      "Past date history with ratings",
	"dates/wishlist":     "Activity bucket list",
	"dates/rate":         "Rate a past date",
	"together/log":       "Log quality time",
	"together/budget":    "Quality time tracking this week/month",
	"together/gifts":     "Gift idea tracker",
	"together/add_gift":  "Add a gift idea",
	"config/set":         "Set partner config (calendar_id, name, anniversary, etc.)",
	"config/get":         "View partner config",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("partner").
		Domain("calendar", common.ActionRegistry{
			"overlap": handleCalendarOverlap,
			"suggest": handleCalendarSuggest,
		}).
		Domain("dates", common.ActionRegistry{
			"ideas":    handleDateIdeas,
			"add_idea": handleDateAddIdea,
			"log":      handleDateLog,
			"history":  handleDateHistory,
			"wishlist": handleDateWishlist,
			"rate":     handleDateRate,
		}).
		Domain("together", common.ActionRegistry{
			"log":      handleTogetherLog,
			"budget":   handleTogetherBudget,
			"gifts":    handleTogetherGifts,
			"add_gift": handleTogetherAddGift,
		}).
		Domain("config", common.ActionRegistry{
			"set": handleConfigSet,
			"get": handleConfigGet,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_partner",
				mcp.WithDescription("Partner coordination gateway.\n\n"+
					dispatcher.DescribeActionsWithHints(partnerHints)),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: calendar, dates, together, config")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				// Calendar params
				mcp.WithString("date", mcp.Description("Date (YYYY-MM-DD) or date range start")),
				mcp.WithString("end_date", mcp.Description("Date range end")),
				mcp.WithNumber("days_ahead", mcp.Description("Number of days to look ahead (default 7)")),
				// Date idea params
				mcp.WithString("title", mcp.Description("Date idea or gift title")),
				mcp.WithString("type", mcp.Description("Date type (dinner, adventure, home, etc.)")),
				mcp.WithString("location", mcp.Description("Location")),
				mcp.WithNumber("estimated_cost", mcp.Description("Estimated cost")),
				mcp.WithString("weather_preference", mcp.Description("Weather: any, sunny, indoor, rainy_ok")),
				mcp.WithString("indoor_outdoor", mcp.Description("indoor, outdoor, or either")),
				mcp.WithString("tags", mcp.Description("Comma-separated tags")),
				// Date log params
				mcp.WithNumber("date_id", mcp.Description("Date log or idea ID")),
				mcp.WithNumber("rating", mcp.Description("Rating 1-10")),
				mcp.WithString("notes", mcp.Description("Notes")),
				mcp.WithNumber("cost", mcp.Description("Actual cost")),
				mcp.WithString("weather", mcp.Description("Weather conditions")),
				// Together params
				mcp.WithNumber("hours", mcp.Description("Hours of quality time")),
				mcp.WithString("activity_type", mcp.Description("Activity type")),
				// Gift params
				mcp.WithNumber("gift_id", mcp.Description("Gift tracker ID")),
				mcp.WithString("occasion", mcp.Description("Gift occasion")),
				mcp.WithNumber("budget", mcp.Description("Gift budget")),
				mcp.WithString("url", mcp.Description("URL for gift or idea")),
				// Config params
				mcp.WithString("key", mcp.Description("Config key")),
				mcp.WithString("value", mcp.Description("Config value")),
			),
			Handler:     tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:    "life",
			Subcategory: "partner",
			Tags:        []string{"partner", "relationship", "dates", "calendar", "gifts"},
			Complexity:  tools.ComplexityModerate,
			IsWrite:     true,
			Timeout:     30 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// --- Calendar Handlers ---

func handleCalendarOverlap(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	daysAhead := common.GetIntParam(req, "days_ahead", 7)
	now := time.Now()
	endDate := now.AddDate(0, 0, daysAhead)

	// Get partner calendar ID from config
	partnerCalID := getPartnerConfig(db, "partner_calendar_id")
	if partnerCalID == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam,
			"partner_calendar_id not configured. Use config/set to set it first."), nil
	}

	// Query both calendars for events in the window
	rows, err := db.QueryContext(ctx, `
		SELECT summary, start_time, end_time
		FROM calendar_events
		WHERE start_time >= ? AND start_time <= ?
		ORDER BY start_time`,
		now.Format("2006-01-02T15:04:05"), endDate.Format("2006-01-02T15:04:05"))
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Calendar Overlap")
	md.KeyValue("Window", fmt.Sprintf("%s → %s (%d days)", now.Format("Jan 2"), endDate.Format("Jan 2"), daysAhead))

	var busySlots [][]string
	for rows.Next() {
		var summary, start, end string
		if rows.Scan(&summary, &start, &end) == nil {
			busySlots = append(busySlots, []string{summary, start, end})
		}
	}

	if len(busySlots) == 0 {
		md.Text("No events found in this window — wide open for planning!")
	} else {
		md.Section("Your Busy Times")
		md.Table([]string{"Event", "Start", "End"}, busySlots)
		md.Text(fmt.Sprintf("Partner calendar: `%s`", partnerCalID))
		md.Text("For full overlap analysis, sync partner's calendar via Google FreeBusy API.")
	}

	// Suggest free evenings (6-10pm) in the window
	md.Section("Potential Free Evenings")
	var freeEvenings []string
	for d := 0; d < daysAhead; d++ {
		checkDate := now.AddDate(0, 0, d)
		dayStr := checkDate.Format("2006-01-02")
		eveningStart := dayStr + "T18:00:00"
		eveningEnd := dayStr + "T22:00:00"

		var conflicts int
		db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM calendar_events
			WHERE start_time < ? AND end_time > ?`,
			eveningEnd, eveningStart).Scan(&conflicts)

		if conflicts == 0 {
			freeEvenings = append(freeEvenings, fmt.Sprintf("%s (%s)", checkDate.Format("Mon Jan 2"), checkDate.Weekday().String()))
		}
	}
	if len(freeEvenings) > 0 {
		md.List(freeEvenings)
	} else {
		md.Text("No completely free evenings found.")
	}

	return tools.TextResult(md.String()), nil
}

func handleCalendarSuggest(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	daysAhead := common.GetIntParam(req, "days_ahead", 14)
	now := time.Now()

	md := common.NewMarkdownBuilder().Title("Date Window Suggestions")

	// Find weekend days and free evenings
	var suggestions [][]string
	for d := 0; d < daysAhead; d++ {
		checkDate := now.AddDate(0, 0, d)
		dayStr := checkDate.Format("2006-01-02")
		weekday := checkDate.Weekday()

		// Check for evening availability (6-10pm)
		eveningStart := dayStr + "T18:00:00"
		eveningEnd := dayStr + "T22:00:00"
		var conflicts int
		db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM calendar_events
			WHERE start_time < ? AND end_time > ?`,
			eveningEnd, eveningStart).Scan(&conflicts)

		if conflicts > 0 {
			continue
		}

		windowType := "Weeknight"
		if weekday == time.Saturday || weekday == time.Sunday {
			windowType = "Weekend"
		} else if weekday == time.Friday {
			windowType = "Friday night"
		}

		// Check weather cache
		var weatherDesc string
		db.QueryRowContext(ctx,
			"SELECT data_json FROM weather_cache WHERE location_key = 'home' AND forecast_type = 'daily' ORDER BY fetched_at DESC LIMIT 1").
			Scan(&weatherDesc)

		suggestions = append(suggestions, []string{
			checkDate.Format("Mon Jan 2"), windowType, "6-10 PM", "Free",
		})
	}

	if len(suggestions) == 0 {
		md.Text("No open windows found in the next %d days. Consider clearing some evening events.")
	} else {
		md.Table([]string{"Date", "Type", "Window", "Status"}, suggestions)

		// Suggest matching date ideas
		var topIdeas []string
		ideaRows, err := db.QueryContext(ctx,
			"SELECT title, type FROM date_ideas ORDER BY rating DESC, times_done ASC LIMIT 3")
		if err == nil {
			defer ideaRows.Close()
			for ideaRows.Next() {
				var title, dtype string
				if ideaRows.Scan(&title, &dtype) == nil {
					topIdeas = append(topIdeas, fmt.Sprintf("%s (%s)", title, dtype))
				}
			}
		}
		if len(topIdeas) > 0 {
			md.Section("Top-Rated Ideas")
			md.List(topIdeas)
		}
	}

	return tools.TextResult(md.String()), nil
}

// --- Date Handlers ---

func handleDateIdeas(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	dtype := common.GetStringParam(req, "type", "")
	query := "SELECT id, title, type, location, estimated_cost, indoor_outdoor, rating, times_done FROM date_ideas"
	var args []interface{}
	if dtype != "" {
		query += " WHERE type = ?"
		args = append(args, dtype)
	}
	query += " ORDER BY rating DESC, times_done ASC LIMIT 20"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Date Ideas")
	var tableRows [][]string
	for rows.Next() {
		var id, timesDone int
		var title, dtype, location, inOut string
		var cost, rating float64
		if rows.Scan(&id, &title, &dtype, &location, &cost, &inOut, &rating, &timesDone) == nil {
			ratingStr := "-"
			if rating > 0 {
				ratingStr = fmt.Sprintf("%.1f", rating)
			}
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), title, dtype, location,
				fmt.Sprintf("$%.0f", cost), inOut, ratingStr, fmt.Sprintf("%d", timesDone),
			})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("date ideas")
	} else {
		md.Table([]string{"ID", "Title", "Type", "Location", "Cost", "Setting", "Rating", "Done"}, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleDateAddIdea(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	dtype := common.GetStringParam(req, "type", "outing")
	location := common.GetStringParam(req, "location", "")
	cost := common.GetFloatParam(req, "estimated_cost", 0)
	weather := common.GetStringParam(req, "weather_preference", "any")
	inOut := common.GetStringParam(req, "indoor_outdoor", "either")
	tags := common.GetStringParam(req, "tags", "")

	result, err := db.ExecContext(ctx,
		`INSERT INTO date_ideas (title, type, location, estimated_cost, weather_preference, indoor_outdoor, tags)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		title, dtype, location, cost, weather, inOut, tags)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("Date idea #%d added: %s (%s, ~$%.0f).", id, title, dtype, cost)), nil
}

func handleDateLog(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	date := common.GetStringParam(req, "date", time.Now().Format("2006-01-02"))
	location := common.GetStringParam(req, "location", "")
	notes := common.GetStringParam(req, "notes", "")
	rating := common.GetIntParam(req, "rating", 0)
	weather := common.GetStringParam(req, "weather", "")
	cost := common.GetFloatParam(req, "cost", 0)

	result, err := db.ExecContext(ctx,
		`INSERT INTO date_log (title, date, location, notes, rating, weather, cost)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		title, date, location, notes, rating, weather, cost)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()

	md := common.NewMarkdownBuilder().Title("Date Logged")
	md.KeyValue("ID", fmt.Sprintf("%d", id))
	md.KeyValue("Date", date)
	md.KeyValue("What", title)
	if rating > 0 {
		md.KeyValue("Rating", fmt.Sprintf("%d/10", rating))
	}
	return tools.TextResult(md.String()), nil
}

func handleDateHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT id, title, date, location, rating, cost
		FROM date_log ORDER BY date DESC LIMIT 20`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Date History")
	var tableRows [][]string
	for rows.Next() {
		var id, rating int
		var title, date, location string
		var cost float64
		if rows.Scan(&id, &title, &date, &location, &rating, &cost) == nil {
			rStr := "-"
			if rating > 0 {
				rStr = fmt.Sprintf("%d/10", rating)
			}
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), date, title, location, rStr, fmt.Sprintf("$%.0f", cost),
			})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("logged dates")
	} else {
		md.Table([]string{"ID", "Date", "Title", "Location", "Rating", "Cost"}, tableRows)

		// Average rating
		var avgRating float64
		var ratedCount int
		db.QueryRowContext(ctx, "SELECT AVG(rating), COUNT(*) FROM date_log WHERE rating > 0").Scan(&avgRating, &ratedCount)
		if ratedCount > 0 {
			md.KeyValue("Average rating", fmt.Sprintf("%.1f/10 (%d rated)", avgRating, ratedCount))
		}
	}
	return tools.TextResult(md.String()), nil
}

func handleDateWishlist(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx,
		"SELECT id, title, type, location, estimated_cost FROM date_ideas WHERE times_done = 0 ORDER BY created_at DESC")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Date Wishlist (Never Done)")
	var tableRows [][]string
	for rows.Next() {
		var id int
		var title, dtype, location string
		var cost float64
		if rows.Scan(&id, &title, &dtype, &location, &cost) == nil {
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), title, dtype, location, fmt.Sprintf("$%.0f", cost),
			})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("wishlist items")
	} else {
		md.Table([]string{"ID", "Title", "Type", "Location", "Est. Cost"}, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleDateRate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dateID := int64(common.GetIntParam(req, "date_id", 0))
	if dateID == 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "date_id is required"), nil
	}
	rating := common.GetIntParam(req, "rating", 0)
	if rating < 1 || rating > 10 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "rating must be 1-10"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	notes := common.GetStringParam(req, "notes", "")
	db.ExecContext(ctx, "UPDATE date_log SET rating = ?, notes = CASE WHEN ? = '' THEN notes ELSE ? END WHERE id = ?",
		rating, notes, notes, dateID)

	// If linked to an idea, update average rating
	var ideaID sql.NullInt64
	db.QueryRowContext(ctx, "SELECT idea_id FROM date_log WHERE id = ?", dateID).Scan(&ideaID)
	if ideaID.Valid {
		var avg float64
		db.QueryRowContext(ctx, "SELECT AVG(rating) FROM date_log WHERE idea_id = ? AND rating > 0", ideaID.Int64).Scan(&avg)
		db.ExecContext(ctx, "UPDATE date_ideas SET rating = ? WHERE id = ?", avg, ideaID.Int64)
	}

	return tools.TextResult(fmt.Sprintf("Date #%d rated %d/10.", dateID, rating)), nil
}

// --- Together Handlers ---

func handleTogetherLog(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	hours := common.GetFloatParam(req, "hours", 0)
	if hours <= 0 {
		return common.CodedErrorResultf(common.ErrInvalidParam, "hours is required (> 0)"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	date := common.GetStringParam(req, "date", time.Now().Format("2006-01-02"))
	activityType := common.GetStringParam(req, "activity_type", "quality_time")
	qualityRating := common.GetIntParam(req, "rating", 0)
	notes := common.GetStringParam(req, "notes", "")

	result, err := db.ExecContext(ctx,
		"INSERT INTO together_time (date, hours, activity_type, quality_rating, notes) VALUES (?, ?, ?, ?, ?)",
		date, hours, activityType, qualityRating, notes)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("Logged %.1f hours together (#%d): %s.", hours, id, activityType)), nil
}

func handleTogetherBudget(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	now := time.Now()
	weekStart := now.AddDate(0, 0, -int(now.Weekday()))
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	md := common.NewMarkdownBuilder().Title("Together Time Budget")

	// This week
	var weekHours float64
	var weekEntries int
	db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(hours), 0), COUNT(*) FROM together_time WHERE date >= ?",
		weekStart.Format("2006-01-02")).Scan(&weekHours, &weekEntries)
	md.KeyValue("This week", fmt.Sprintf("%.1f hours (%d entries)", weekHours, weekEntries))

	// This month
	var monthHours float64
	var monthEntries int
	db.QueryRowContext(ctx,
		"SELECT COALESCE(SUM(hours), 0), COUNT(*) FROM together_time WHERE date >= ?",
		monthStart.Format("2006-01-02")).Scan(&monthHours, &monthEntries)
	md.KeyValue("This month", fmt.Sprintf("%.1f hours (%d entries)", monthHours, monthEntries))

	// Average quality
	var avgQuality float64
	db.QueryRowContext(ctx,
		"SELECT COALESCE(AVG(quality_rating), 0) FROM together_time WHERE quality_rating > 0 AND date >= ?",
		monthStart.Format("2006-01-02")).Scan(&avgQuality)
	if avgQuality > 0 {
		md.KeyValue("Avg quality (month)", fmt.Sprintf("%.1f/10", avgQuality))
	}

	// By activity type this month
	typeRows, err := db.QueryContext(ctx, `
		SELECT activity_type, SUM(hours), COUNT(*)
		FROM together_time WHERE date >= ?
		GROUP BY activity_type ORDER BY SUM(hours) DESC`,
		monthStart.Format("2006-01-02"))
	if err == nil {
		defer typeRows.Close()
		var typeTable [][]string
		for typeRows.Next() {
			var aType string
			var hours float64
			var count int
			if typeRows.Scan(&aType, &hours, &count) == nil {
				typeTable = append(typeTable, []string{aType, fmt.Sprintf("%.1f", hours), fmt.Sprintf("%d", count)})
			}
		}
		if len(typeTable) > 0 {
			md.Section("By Activity")
			md.Table([]string{"Type", "Hours", "Count"}, typeTable)
		}
	}

	// Anniversary countdown
	anniversary := getPartnerConfig(db, "anniversary")
	if anniversary != "" {
		if annDate, err := time.Parse("2006-01-02", anniversary); err == nil {
			nextAnn := time.Date(now.Year(), annDate.Month(), annDate.Day(), 0, 0, 0, 0, now.Location())
			if nextAnn.Before(now) {
				nextAnn = nextAnn.AddDate(1, 0, 0)
			}
			daysUntil := int(time.Until(nextAnn).Hours() / 24)
			md.Section("Anniversary")
			md.KeyValue("Date", anniversary)
			md.KeyValue("Days until", fmt.Sprintf("%d", daysUntil))
		}
	}

	return tools.TextResult(md.String()), nil
}

func handleTogetherGifts(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, `
		SELECT id, title, occasion, budget, purchased, url
		FROM gift_tracker ORDER BY purchased ASC, created_at DESC LIMIT 20`)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Gift Tracker")
	var tableRows [][]string
	for rows.Next() {
		var id, purchased int
		var title, occasion, url string
		var budget float64
		if rows.Scan(&id, &title, &occasion, &budget, &purchased, &url) == nil {
			status := "Idea"
			if purchased == 1 {
				status = "Purchased"
			}
			tableRows = append(tableRows, []string{
				fmt.Sprintf("%d", id), title, occasion, fmt.Sprintf("$%.0f", budget), status,
			})
		}
	}
	if len(tableRows) == 0 {
		md.EmptyList("gift ideas")
	} else {
		md.Table([]string{"ID", "Title", "Occasion", "Budget", "Status"}, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleTogetherAddGift(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	occasion := common.GetStringParam(req, "occasion", "")
	budget := common.GetFloatParam(req, "budget", 0)
	url := common.GetStringParam(req, "url", "")
	notes := common.GetStringParam(req, "notes", "")

	result, err := db.ExecContext(ctx,
		"INSERT INTO gift_tracker (title, occasion, budget, url, notes) VALUES (?, ?, ?, ?, ?)",
		title, occasion, budget, url, notes)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	id, _ := result.LastInsertId()
	return tools.TextResult(fmt.Sprintf("Gift idea #%d added: %s (~$%.0f).", id, title, budget)), nil
}

// --- Config Handlers ---

func handleConfigSet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key := common.GetStringParam(req, "key", "")
	value := common.GetStringParam(req, "value", "")
	if key == "" || value == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "key and value are required"), nil
	}

	validKeys := map[string]bool{
		"partner_name": true, "partner_calendar_id": true,
		"anniversary": true, "partner_phone": true,
	}
	if !validKeys[key] {
		return common.CodedErrorResultf(common.ErrInvalidParam,
			"valid keys: %s", strings.Join(sortedKeys(validKeys), ", ")), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	db.ExecContext(ctx,
		"INSERT INTO partner_config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = ?, updated_at = datetime('now')",
		key, value, value)

	return tools.TextResult(fmt.Sprintf("Partner config `%s` set.", key)), nil
}

func handleConfigGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()
	db := database.SqlDB()

	rows, err := db.QueryContext(ctx, "SELECT key, value, updated_at FROM partner_config ORDER BY key")
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Partner Config")
	var found bool
	for rows.Next() {
		var key, value, updatedAt string
		if rows.Scan(&key, &value, &updatedAt) == nil {
			md.KeyValue(key, value)
			found = true
		}
	}
	if !found {
		md.Text("No partner config set. Use `config/set` with keys: partner_name, partner_calendar_id, anniversary, partner_phone.")
	}
	return tools.TextResult(md.String()), nil
}

// --- Helpers ---

func getPartnerConfig(db *sql.DB, key string) string {
	var value string
	db.QueryRow("SELECT value FROM partner_config WHERE key = ?", key).Scan(&value)
	return value
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
