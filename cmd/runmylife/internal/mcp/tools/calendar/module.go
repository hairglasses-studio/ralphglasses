// Package calendar provides MCP tools for Google Calendar integration.
package calendar

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for calendar tools.
type Module struct{}

func (m *Module) Name() string        { return "calendar" }
func (m *Module) Description() string { return "Google Calendar integration for schedule management" }

var calendarHints = map[string]string{
	"events/list":       "List upcoming calendar events (live API or DB)",
	"events/get":        "Get details for a specific event",
	"events/create":     "Create a new calendar event (live via Google Calendar API)",
	"events/delete":     "Delete a calendar event",
	"calendars/list":    "List all calendars for this account",
	"schedule/free":     "Find free time slots via FreeBusy API",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("calendar").
		Domain("events", common.ActionRegistry{
			"list":   handleEventsList,
			"get":    handleEventsGet,
			"create": handleEventsCreate,
			"delete": handleEventsDelete,
		}).
		Domain("calendars", common.ActionRegistry{
			"list": handleCalendarsList,
		}).
		Domain("schedule", common.ActionRegistry{
			"free": handleScheduleFree,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_calendar",
				mcp.WithDescription(
					"Google Calendar gateway.\n\n"+
						dispatcher.DescribeActionsWithHints(calendarHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: events, calendars, schedule")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
				mcp.WithString("event_id", mcp.Description("Event ID (for get/delete)")),
				mcp.WithString("summary", mcp.Description("Event title (for create)")),
				mcp.WithString("description", mcp.Description("Event description (for create)")),
				mcp.WithString("start_time", mcp.Description("Start time ISO8601 (for create)")),
				mcp.WithString("end_time", mcp.Description("End time ISO8601 (for create)")),
				mcp.WithString("location", mcp.Description("Event location")),
				mcp.WithString("attendees", mcp.Description("Comma-separated attendee emails")),
				mcp.WithString("calendar_id", mcp.Description("Calendar ID (default: primary)")),
				mcp.WithString("account", mcp.Description("Google account (default: personal)")),
				mcp.WithString("date_from", mcp.Description("Range start YYYY-MM-DD")),
				mcp.WithString("date_to", mcp.Description("Range end YYYY-MM-DD")),
				mcp.WithBoolean("add_meet", mcp.Description("Add Google Meet link (for create)")),
				mcp.WithNumber("days", mcp.Description("Days ahead to look (default 7)")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 20)")),
			),
			Handler:             tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:            "calendar",
			Subcategory:         "gateway",
			Tags:                []string{"calendar", "schedule", "events"},
			Complexity:          tools.ComplexityModerate,
			IsWrite:             true,
			ProducesRefs:        []string{"calendar_event"},
			CircuitBreakerGroup: "calendar_api",
			Timeout:             60 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// calendarClient returns a live Calendar API client, or nil if credentials are unavailable.
func calendarClient(ctx context.Context, req mcp.CallToolRequest) *clients.CalendarAPIClient {
	account := common.GetStringParam(req, "account", "personal")
	client, err := clients.NewCalendarAPIClient(ctx, account)
	if err != nil {
		return nil
	}
	return client
}

func handleEventsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := common.GetLimitParam(req, 20)
	days := common.GetIntParam(req, "days", 7)

	// Try live API first.
	if client := calendarClient(ctx, req); client != nil {
		now := time.Now()
		timeMax := now.AddDate(0, 0, days)
		events, err := client.FetchEvents(ctx, now, timeMax, int64(limit))
		if err != nil {
			return common.ActionableErrorResult(common.ErrAPIError, err, "Check Google OAuth credentials"), nil
		}

		md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Calendar — Next %d Days", days))
		headers := []string{"Summary", "Start", "End", "Location"}
		var tableRows [][]string
		for _, ev := range events {
			tableRows = append(tableRows, []string{
				ev.Summary,
				ev.StartTime.Format("2006-01-02 15:04"),
				ev.EndTime.Format("2006-01-02 15:04"),
				ev.Location,
			})
		}
		if len(tableRows) == 0 {
			md.EmptyList("events")
		} else {
			md.Table(headers, tableRows)
		}
		return tools.TextResult(md.String()), nil
	}

	// Fallback to DB.
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	cutoff := time.Now().AddDate(0, 0, days).Format("2006-01-02T15:04:05")
	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT id, summary, start_time, end_time, location FROM calendar_events WHERE start_time <= ? ORDER BY start_time ASC LIMIT ?",
		cutoff, limit,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Calendar (cached) — Next %d Days", days))
	headers := []string{"Summary", "Start", "End", "Location"}
	var tableRows [][]string
	for rows.Next() {
		var id, summary, start, end, location string
		if err := rows.Scan(&id, &summary, &start, &end, &location); err != nil {
			continue
		}
		tableRows = append(tableRows, []string{summary, start, end, location})
	}
	if len(tableRows) == 0 {
		md.EmptyList("events")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleEventsGet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	eventID, ok := common.RequireStringParam(req, "event_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "event_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	var summary, description, start, end, location, attendees string
	err = database.SqlDB().QueryRowContext(ctx,
		"SELECT summary, description, start_time, end_time, location, attendees FROM calendar_events WHERE id = ?",
		eventID,
	).Scan(&summary, &description, &start, &end, &location, &attendees)
	if err != nil {
		return common.CodedErrorResultf(common.ErrNotFound, "event %s not found", eventID), nil
	}

	md := common.NewMarkdownBuilder().Title(summary)
	md.KeyValue("Start", start)
	md.KeyValue("End", end)
	if location != "" {
		md.KeyValue("Location", location)
	}
	if description != "" {
		md.Section("Description").Text(description)
	}
	if attendees != "" {
		md.KeyValue("Attendees", attendees)
	}

	return tools.TextResult(md.String()), nil
}

func handleEventsCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	summary := common.GetStringParam(req, "summary", "")
	if summary == "" {
		return common.CodedErrorResultf(common.ErrInvalidParam, "summary is required"), nil
	}
	startTimeStr := common.GetStringParam(req, "start_time", "")
	endTimeStr := common.GetStringParam(req, "end_time", "")
	location := common.GetStringParam(req, "location", "")
	description := common.GetStringParam(req, "description", "")
	attendeesStr := common.GetStringParam(req, "attendees", "")
	calendarID := common.GetStringParam(req, "calendar_id", "primary")
	addMeet := common.GetBoolParam(req, "add_meet", false)

	// Try live API first.
	if client := calendarClient(ctx, req); client != nil {
		startTime, err := time.Parse(time.RFC3339, startTimeStr)
		if err != nil {
			return common.CodedErrorResultf(common.ErrInvalidParam, "start_time must be RFC3339 (e.g. 2026-04-05T10:00:00-07:00): %v", err), nil
		}
		endTime, err := time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			return common.CodedErrorResultf(common.ErrInvalidParam, "end_time must be RFC3339 (e.g. 2026-04-05T11:00:00-07:00): %v", err), nil
		}

		var attendees []string
		if attendeesStr != "" {
			for _, email := range strings.Split(attendeesStr, ",") {
				email = strings.TrimSpace(email)
				if email != "" {
					attendees = append(attendees, email)
				}
			}
		}

		ev, err := client.CreateEvent(ctx, calendarID, summary, description, location, startTime, endTime, attendees, addMeet)
		if err != nil {
			return common.ActionableErrorResult(common.ErrAPIError, err, "Check calendar permissions"), nil
		}

		md := common.NewMarkdownBuilder().Title("Event Created (Google Calendar)")
		md.KeyValue("ID", ev.ID)
		md.KeyValue("Summary", ev.Summary)
		md.KeyValue("Start", ev.StartTime.Format("2006-01-02 15:04"))
		md.KeyValue("End", ev.EndTime.Format("2006-01-02 15:04"))
		if ev.Location != "" {
			md.KeyValue("Location", ev.Location)
		}
		if ev.ConferenceLink != "" {
			md.KeyValue("Meet Link", ev.ConferenceLink)
		}
		return tools.TextResult(md.String()), nil
	}

	// Fallback to local DB.
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	id := fmt.Sprintf("evt-%d", time.Now().UnixNano())
	_, err = database.SqlDB().ExecContext(ctx,
		"INSERT INTO calendar_events (id, summary, start_time, end_time, location) VALUES (?, ?, ?, ?, ?)",
		id, summary, startTimeStr, endTimeStr, location,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("# Event Created (local)\n\n- **ID:** %s\n- **Summary:** %s\n\n⚠️ Stored locally only — configure Google OAuth for live calendar events.",
		id, summary)), nil
}

func handleEventsDelete(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	eventID, ok := common.RequireStringParam(req, "event_id")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "event_id is required"), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	_, err = database.SqlDB().ExecContext(ctx, "DELETE FROM calendar_events WHERE id = ?", eventID)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}

	return tools.TextResult(fmt.Sprintf("Event %s deleted.", eventID)), nil
}

func handleCalendarsList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client := calendarClient(ctx, req)
	if client == nil {
		return common.ActionableErrorResult(common.ErrConfig, fmt.Errorf("Google OAuth not configured"), "Run 'runmylife google-auth' to authenticate"), nil
	}

	cals, err := client.ListCalendars(ctx)
	if err != nil {
		return common.ActionableErrorResult(common.ErrAPIError, err, "Check Google OAuth credentials"), nil
	}

	md := common.NewMarkdownBuilder().Title("Calendars")
	headers := []string{"ID", "Name", "Primary", "Access"}
	var tableRows [][]string
	for _, c := range cals {
		primary := ""
		if c.Primary {
			primary = "yes"
		}
		tableRows = append(tableRows, []string{c.ID, c.Summary, primary, c.AccessRole})
	}
	if len(tableRows) == 0 {
		md.EmptyList("calendars")
	} else {
		md.Table(headers, tableRows)
	}
	return tools.TextResult(md.String()), nil
}

func handleScheduleFree(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	days := common.GetIntParam(req, "days", 7)
	now := time.Now()
	timeMax := now.AddDate(0, 0, days)

	// Try live FreeBusy API.
	if client := calendarClient(ctx, req); client != nil {
		calendarID := common.GetStringParam(req, "calendar_id", "primary")
		busy, err := client.FindAvailability(ctx, []string{calendarID}, now, timeMax)
		if err != nil {
			return common.ActionableErrorResult(common.ErrAPIError, err, "Check Google OAuth credentials"), nil
		}

		md := common.NewMarkdownBuilder().Title(fmt.Sprintf("Availability — Next %d Days", days))
		md.KeyValue("Period", fmt.Sprintf("%s to %s", now.Format("2006-01-02"), timeMax.Format("2006-01-02")))

		if len(busy) == 0 {
			md.Text("No busy slots found — you're completely free!")
		} else {
			headers := []string{"Start", "End", "Duration"}
			var tableRows [][]string
			for _, slot := range busy {
				duration := slot.End.Sub(slot.Start).Round(time.Minute)
				tableRows = append(tableRows, []string{
					slot.Start.Format("2006-01-02 15:04"),
					slot.End.Format("2006-01-02 15:04"),
					duration.String(),
				})
			}
			md.KeyValue("Busy slots", fmt.Sprintf("%d", len(busy)))
			md.Table(headers, tableRows)
		}
		return tools.TextResult(md.String()), nil
	}

	// Fallback to DB.
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		"SELECT start_time, end_time FROM calendar_events WHERE start_time >= ? AND start_time <= ? ORDER BY start_time",
		now.Format("2006-01-02T15:04:05"), timeMax.Format("2006-01-02T15:04:05"),
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	var busyCount int
	for rows.Next() {
		busyCount++
	}

	md := common.NewMarkdownBuilder().Title("Free Time Analysis (cached)")
	md.Bold("Period", fmt.Sprintf("%s to %s", now.Format("2006-01-02"), timeMax.Format("2006-01-02")))
	md.Bold("Busy slots", fmt.Sprintf("%d events scheduled", busyCount))
	md.Text("Configure Google OAuth for real-time FreeBusy availability data.")

	return tools.TextResult(md.String()), nil
}
