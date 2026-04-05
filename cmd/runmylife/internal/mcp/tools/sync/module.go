// Package sync provides MCP tools for data synchronization orchestration.
package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/clients"
	"github.com/hairglasses-studio/runmylife/internal/config"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

// Module implements the ToolModule interface for sync tools.
type Module struct{}

func (m *Module) Name() string        { return "sync" }
func (m *Module) Description() string { return "Data sync orchestration across external services" }

var syncHints = map[string]string{
	"run/todoist":  "Sync tasks from Todoist API",
	"run/gmail":    "Sync messages from Gmail API",
	"run/calendar": "Sync events from Google Calendar API",
	"run/messages": "Sync SMS/RCS from Google Messages",
	"run/discord":  "Cache Discord servers, channels, and messages",
	"run/drive":    "Cache Google Drive file metadata",
	"run/notion":   "Cache Notion databases and pages",
	"run/weather":  "Refresh weather cache",
	"run/contacts": "Aggregate contacts from all sources",
	"run/all":      "Sync all connected services",
	"status/check": "Check sync status for all services",
}

func (m *Module) Tools() []tools.ToolDefinition {
	dispatcher := common.NewDispatcher("sync").
		Domain("run", common.ActionRegistry{
			"todoist":  handleRunTodoist,
			"gmail":    handleRunGmail,
			"calendar": handleRunCalendar,
			"messages": handleRunMessages,
			"discord":  handleRunDiscord,
			"drive":    handleRunDrive,
			"notion":   handleRunNotion,
			"weather":  handleRunWeather,
			"contacts": handleRunContacts,
			"all":      handleRunAll,
		}).
		Domain("status", common.ActionRegistry{
			"check": handleStatusCheck,
		})

	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_sync",
				mcp.WithDescription(
					"Data sync orchestration gateway.\n\n"+
						dispatcher.DescribeActionsWithHints(syncHints),
				),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Domain: run, status")),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action within domain")),
			),
			Handler:    tools.ToolHandlerFunc(dispatcher.Handler()),
			Category:   "sync",
			Tags:       []string{"sync", "integration"},
			Complexity: tools.ComplexityComplex,
			IsWrite:    true,
			Timeout:    120 * time.Second,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

func recordSync(ctx context.Context, source, status string, count int, errMsg string) {
	database, err := common.OpenDB()
	if err != nil {
		return
	}
	defer database.Close()

	completedAt := time.Now().Format(time.RFC3339)
	database.SqlDB().ExecContext(ctx,
		"INSERT INTO sync_history (source, status, records_synced, error_message, completed_at) VALUES (?, ?, ?, ?, ?)",
		source, status, count, errMsg, completedAt,
	)
}

func handleRunTodoist(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return common.CodedErrorResult(common.ErrConfig, err), nil
	}
	token := cfg.Credentials["todoist"]
	if token == "" {
		recordSync(ctx, "todoist", "skipped", 0, "no API token configured")
		return tools.TextResult("# Todoist Sync\n\nSkipped — no Todoist API token configured in `~/.config/runmylife/config.json`."), nil
	}

	client := clients.NewTodoistClient(token)
	tasks, err := client.ListTasks(ctx, "", "")
	if err != nil {
		recordSync(ctx, "todoist", "error", 0, err.Error())
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	count := 0
	for _, t := range tasks {
		dueDate := ""
		if t.Due != nil {
			dueDate = t.Due.Date
		}
		_, err := database.SqlDB().ExecContext(ctx,
			`INSERT OR REPLACE INTO tasks (id, todoist_id, title, description, priority, project, due_date, completed)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			"todoist-"+t.ID, t.ID, t.Content, t.Description, t.Priority, t.ProjectID, dueDate, 0,
		)
		if err == nil {
			count++
		}
	}

	recordSync(ctx, "todoist", "success", count, "")
	return tools.TextResult(fmt.Sprintf("# Todoist Sync\n\nSynced **%d** tasks from Todoist.", count)), nil
}

func handleRunGmail(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := clients.NewGmailAPIClient(ctx)
	if err != nil {
		recordSync(ctx, "gmail", "skipped", 0, err.Error())
		return tools.TextResult("# Gmail Sync\n\nSkipped — " + err.Error()), nil
	}

	messages, err := client.FetchMessageHeaders(ctx, "in:inbox newer_than:7d", 100)
	if err != nil {
		recordSync(ctx, "gmail", "error", 0, err.Error())
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	count := 0
	for _, m := range messages {
		_, err := database.SqlDB().ExecContext(ctx,
			`INSERT OR REPLACE INTO gmail_messages (id, thread_id, from_addr, subject, snippet, body, timestamp, labels, triaged)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`,
			m.ID, m.ThreadID, m.From, m.Subject, m.Snippet, m.Body,
			m.Date.Format(time.RFC3339), m.Labels,
		)
		if err == nil {
			count++
		}
	}

	recordSync(ctx, "gmail", "success", count, "")
	return tools.TextResult(fmt.Sprintf("# Gmail Sync\n\nSynced **%d** messages from Gmail (last 7 days).", count)), nil
}

func handleRunCalendar(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	client, err := clients.NewCalendarAPIClient(ctx, "")
	if err != nil {
		recordSync(ctx, "calendar", "skipped", 0, err.Error())
		return tools.TextResult("# Calendar Sync\n\nSkipped — " + err.Error()), nil
	}

	now := time.Now()
	events, err := client.FetchEvents(ctx, now.AddDate(0, 0, -7), now.AddDate(0, 0, 30), 100)
	if err != nil {
		recordSync(ctx, "calendar", "error", 0, err.Error())
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	count := 0
	for _, e := range events {
		_, err := database.SqlDB().ExecContext(ctx,
			`INSERT OR REPLACE INTO calendar_events (id, summary, description, start_time, end_time, location, attendees)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			e.ID, e.Summary, e.Description,
			e.StartTime.Format(time.RFC3339), e.EndTime.Format(time.RFC3339),
			e.Location, e.Attendees,
		)
		if err == nil {
			count++
		}
	}

	recordSync(ctx, "calendar", "success", count, "")
	return tools.TextResult(fmt.Sprintf("# Calendar Sync\n\nSynced **%d** events from Google Calendar.", count)), nil
}

func handleRunMessages(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	recordSync(ctx, "messages", "skipped", 0, "Google Messages pairing not configured")
	return tools.TextResult("# Messages Sync\n\nSkipped — Google Messages pairing not configured. Run pairing flow to enable."), nil
}

func handleRunDiscord(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return common.CodedErrorResult(common.ErrConfig, err), nil
	}
	token := cfg.Credentials["discord"]
	if token == "" {
		recordSync(ctx, "discord", "skipped", 0, "no bot token configured")
		return tools.TextResult("# Discord Sync\n\nSkipped — no Discord bot token configured."), nil
	}

	client := clients.NewDiscordClient(token)
	guilds, err := client.GetGuilds(ctx)
	if err != nil {
		recordSync(ctx, "discord", "error", 0, err.Error())
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	serverCount := 0
	channelCount := 0
	for _, g := range guilds {
		_, err := database.SqlDB().ExecContext(ctx,
			`INSERT OR REPLACE INTO discord_servers (id, name, icon_url, member_count, cached_at) VALUES (?, ?, ?, ?, ?)`,
			g.ID, g.Name, g.Icon, g.MemberCount, time.Now().Format(time.RFC3339),
		)
		if err == nil {
			serverCount++
		}

		channels, err := client.GetChannels(ctx, g.ID)
		if err != nil {
			continue
		}
		for _, ch := range channels {
			_, err := database.SqlDB().ExecContext(ctx,
				`INSERT OR REPLACE INTO discord_channels (id, server_id, name, type, topic, cached_at) VALUES (?, ?, ?, ?, ?, ?)`,
				ch.ID, g.ID, ch.Name, fmt.Sprintf("%d", ch.Type), ch.Topic, time.Now().Format(time.RFC3339),
			)
			if err == nil {
				channelCount++
			}
		}
	}

	recordSync(ctx, "discord", "success", serverCount+channelCount, "")
	return tools.TextResult(fmt.Sprintf("# Discord Sync\n\nCached **%d** servers and **%d** channels.", serverCount, channelCount)), nil
}

func handleRunDrive(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	token, err := clients.LoadGoogleToken()
	if err != nil {
		recordSync(ctx, "drive", "skipped", 0, err.Error())
		return tools.TextResult("# Drive Sync\n\nSkipped — " + err.Error()), nil
	}

	client := clients.NewDriveClient(token.AccessToken)
	files, err := client.ListFiles(ctx, "", 100)
	if err != nil {
		recordSync(ctx, "drive", "error", 0, err.Error())
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	count := 0
	for _, f := range files {
		parentID := ""
		if len(f.Parents) > 0 {
			parentID = f.Parents[0]
		}
		shared := 0
		if f.Shared {
			shared = 1
		}
		_, err := database.SqlDB().ExecContext(ctx,
			`INSERT OR REPLACE INTO drive_files (id, name, mime_type, parent_id, size, modified_at, shared, web_link, cached_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.ID, f.Name, f.MimeType, parentID, f.Size, f.ModifiedAt, shared, f.WebLink, time.Now().Format(time.RFC3339),
		)
		if err == nil {
			count++
		}
	}

	recordSync(ctx, "drive", "success", count, "")
	return tools.TextResult(fmt.Sprintf("# Drive Sync\n\nCached **%d** files from Google Drive.", count)), nil
}

func handleRunNotion(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return common.CodedErrorResult(common.ErrConfig, err), nil
	}
	token := cfg.Credentials["notion"]
	if token == "" {
		recordSync(ctx, "notion", "skipped", 0, "no integration token configured")
		return tools.TextResult("# Notion Sync\n\nSkipped — no Notion integration token configured."), nil
	}

	client := clients.NewNotionClient(token)

	databases, err := client.ListDatabases(ctx)
	if err != nil {
		recordSync(ctx, "notion", "error", 0, err.Error())
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	dbCount := 0
	pageCount := 0
	for _, db := range databases {
		title := ""
		if len(db.Title) > 0 {
			title = db.Title[0].PlainText
		}
		desc := ""
		if len(db.Description) > 0 {
			desc = db.Description[0].PlainText
		}
		_, err := database.SqlDB().ExecContext(ctx,
			`INSERT OR REPLACE INTO notion_databases (id, title, description, url, cached_at) VALUES (?, ?, ?, ?, ?)`,
			db.ID, title, desc, db.URL, time.Now().Format(time.RFC3339),
		)
		if err == nil {
			dbCount++
		}

		pages, err := client.QueryDatabase(ctx, db.ID, "")
		if err != nil {
			continue
		}
		for _, p := range pages {
			pageTitle := extractNotionPageTitle(p.Properties)
			propsJSON, _ := json.Marshal(p.Properties)
			_, err := database.SqlDB().ExecContext(ctx,
				`INSERT OR REPLACE INTO notion_pages (id, database_id, title, properties_json, url, created_at, last_edited_at, cached_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				p.ID, db.ID, pageTitle, string(propsJSON), p.URL, p.CreatedTime, p.LastEditedTime, time.Now().Format(time.RFC3339),
			)
			if err == nil {
				pageCount++
			}
		}
	}

	recordSync(ctx, "notion", "success", dbCount+pageCount, "")
	return tools.TextResult(fmt.Sprintf("# Notion Sync\n\nCached **%d** databases and **%d** pages.", dbCount, pageCount)), nil
}

func extractNotionPageTitle(properties map[string]interface{}) string {
	for _, key := range []string{"Name", "Title", "name", "title"} {
		if prop, ok := properties[key]; ok {
			if propMap, ok := prop.(map[string]interface{}); ok {
				if titleArr, ok := propMap["title"].([]interface{}); ok && len(titleArr) > 0 {
					if titleObj, ok := titleArr[0].(map[string]interface{}); ok {
						if text, ok := titleObj["plain_text"].(string); ok {
							return text
						}
					}
				}
			}
		}
	}
	return "Untitled"
}

func handleRunWeather(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return common.CodedErrorResult(common.ErrConfig, err), nil
	}
	if cfg.Location == nil {
		recordSync(ctx, "weather", "skipped", 0, "no location configured")
		return tools.TextResult("# Weather Sync\n\nSkipped — no location configured."), nil
	}

	client := clients.NewWeatherClient(cfg.Location.Latitude, cfg.Location.Longitude)
	current, err := client.GetCurrent(ctx)
	if err != nil {
		recordSync(ctx, "weather", "error", 0, err.Error())
		return common.CodedErrorResult(common.ErrAPIError, err), nil
	}

	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	dataJSON, _ := json.Marshal(current)
	locationKey := fmt.Sprintf("%.4f,%.4f", cfg.Location.Latitude, cfg.Location.Longitude)

	database.SqlDB().ExecContext(ctx,
		`INSERT INTO weather_cache (location_key, data_json, forecast_type, fetched_at) VALUES (?, ?, 'current', ?)`,
		locationKey, string(dataJSON), time.Now().Format(time.RFC3339),
	)

	recordSync(ctx, "weather", "success", 1, "")
	return tools.TextResult(fmt.Sprintf("# Weather Sync\n\nCached current weather: %.1f°C, %s.",
		current.Temperature, clients.WeatherCodeDescription(current.WeatherCode))), nil
}

func handleRunContacts(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	count := 0

	// Aggregate from gmail_messages
	rows, err := database.SqlDB().QueryContext(ctx,
		`SELECT DISTINCT from_addr FROM gmail_messages WHERE from_addr != '' AND from_addr != 'me'`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var email string
			if err := rows.Scan(&email); err != nil {
				continue
			}
			id := fmt.Sprintf("gmail-%d", time.Now().UnixNano())
			_, err := database.SqlDB().ExecContext(ctx,
				`INSERT OR IGNORE INTO contacts (id, name, email, source) VALUES (?, ?, ?, 'gmail')`,
				id, email, email,
			)
			if err == nil {
				count++
			}
		}
	}

	// Aggregate from sms_conversations
	smsRows, err := database.SqlDB().QueryContext(ctx,
		`SELECT DISTINCT participant, display_name FROM sms_conversations WHERE participant != ''`)
	if err == nil {
		defer smsRows.Close()
		for smsRows.Next() {
			var phone, name string
			if err := smsRows.Scan(&phone, &name); err != nil {
				continue
			}
			if name == "" {
				name = phone
			}
			id := fmt.Sprintf("sms-%d", time.Now().UnixNano())
			_, err := database.SqlDB().ExecContext(ctx,
				`INSERT OR IGNORE INTO contacts (id, name, phone, source) VALUES (?, ?, ?, 'messages')`,
				id, name, phone,
			)
			if err == nil {
				count++
			}
		}
	}

	recordSync(ctx, "contacts", "success", count, "")
	return tools.TextResult(fmt.Sprintf("# Contacts Sync\n\nAggregated **%d** new contacts from Gmail and Messages.", count)), nil
}

func handleRunAll(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	md := common.NewMarkdownBuilder().Title("Full Sync")

	services := []struct {
		name    string
		handler common.Handler
	}{
		{"todoist", handleRunTodoist},
		{"gmail", handleRunGmail},
		{"calendar", handleRunCalendar},
		{"messages", handleRunMessages},
		{"discord", handleRunDiscord},
		{"drive", handleRunDrive},
		{"notion", handleRunNotion},
		{"weather", handleRunWeather},
		{"contacts", handleRunContacts},
	}

	for _, svc := range services {
		result, _ := svc.handler(ctx, req)
		status := "ok"
		if result != nil && result.IsError {
			status = "error"
		}
		md.KeyValue(svc.name, status)
	}

	return tools.TextResult(md.String()), nil
}

func handleStatusCheck(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	database, err := common.OpenDB()
	if err != nil {
		return common.CodedErrorResult(common.ErrClientInit, err), nil
	}
	defer database.Close()

	rows, err := database.SqlDB().QueryContext(ctx,
		`SELECT source, status, records_synced, completed_at FROM sync_history
		 WHERE id IN (SELECT MAX(id) FROM sync_history GROUP BY source)
		 ORDER BY completed_at DESC`,
	)
	if err != nil {
		return common.CodedErrorResult(common.ErrDBError, err), nil
	}
	defer rows.Close()

	md := common.NewMarkdownBuilder().Title("Sync Status")
	headers := []string{"Source", "Status", "Records", "Last Sync"}
	var tableRows [][]string

	for rows.Next() {
		var source, status, completedAt string
		var records int
		if err := rows.Scan(&source, &status, &records, &completedAt); err != nil {
			continue
		}
		tableRows = append(tableRows, []string{source, status, fmt.Sprintf("%d", records), completedAt})
	}

	if len(tableRows) == 0 {
		md.Text("No sync history found. Run `runmylife_sync(domain=run, action=all)` to start.")
	} else {
		md.Table(headers, tableRows)
	}

	return tools.TextResult(md.String()), nil
}
