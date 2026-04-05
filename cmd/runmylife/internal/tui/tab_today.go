package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/evertras/bubble-table/table"

	"github.com/hairglasses-studio/runmylife/internal/intelligence"
	"github.com/hairglasses-studio/runmylife/internal/timecontext"
	"github.com/hairglasses-studio/runmylife/internal/tui/components"
)

type todayData struct {
	Date        string
	TimeBlock   string
	Priorities  []string
	Events      []calEvent
	Tasks       []taskItem
	Replies     int
	Weather     string
	Suggestions []intelligence.Suggestion
}

type calEvent struct {
	Summary  string
	Start    string
	Duration string
}

type taskItem struct {
	Title    string
	Priority int
	DueDate  string
}

func loadTodayData(db *sql.DB) todayData {
	ctx := context.Background()
	now := time.Now()
	block := timecontext.CurrentBlock()

	d := todayData{
		Date:       now.Format("Monday, January 2"),
		TimeBlock:  block.Label(),
		Priorities: block.Priorities(),
	}

	// Calendar events
	today := now.Format("2006-01-02")
	rows, err := db.QueryContext(ctx,
		`SELECT summary, start_time, end_time FROM calendar_events
		 WHERE date(start_time) = ? ORDER BY start_time LIMIT 8`, today)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var summary, startStr, endStr string
			if rows.Scan(&summary, &startStr, &endStr) == nil {
				e := calEvent{Summary: summary}
				if t, err := time.Parse(time.RFC3339, startStr); err == nil {
					e.Start = t.Format("3:04 PM")
					if t2, err := time.Parse(time.RFC3339, endStr); err == nil {
						dur := t2.Sub(t)
						if dur >= time.Hour {
							e.Duration = fmt.Sprintf("%.0fh", dur.Hours())
						} else {
							e.Duration = fmt.Sprintf("%dm", int(dur.Minutes()))
						}
					}
				}
				d.Events = append(d.Events, e)
			}
		}
	}

	// Top tasks
	rows, err = db.QueryContext(ctx,
		`SELECT title, priority, COALESCE(due_date, '') FROM tasks
		 WHERE completed = 0 ORDER BY priority DESC, due_date ASC LIMIT 8`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var t taskItem
			if rows.Scan(&t.Title, &t.Priority, &t.DueDate) == nil {
				d.Tasks = append(d.Tasks, t)
			}
		}
	}

	// Pending replies
	db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reply_tracker WHERE replied = 0`).Scan(&d.Replies)

	// Weather
	var dataJSON string
	err = db.QueryRowContext(ctx,
		`SELECT data_json FROM weather_cache
		 WHERE forecast_type = 'current' ORDER BY fetched_at DESC LIMIT 1`).Scan(&dataJSON)
	if err == nil {
		d.Weather = summarizeWeather(dataJSON)
	}

	// Intelligence suggestions
	d.Suggestions = intelligence.QuerySuggestions(ctx, db, 3)

	return d
}

func summarizeWeather(dataJSON string) string {
	if dataJSON == "" {
		return ""
	}
	if len(dataJSON) > 100 {
		return dataJSON[:100] + "..."
	}
	return dataJSON
}

func renderToday(d todayData, width int) string {
	var b strings.Builder

	// Date header with time block badge
	b.WriteString(subtitleStyle.Render(d.Date))
	b.WriteString("  ")
	blockStyle := lipgloss.NewStyle().
		Background(colorPrimary).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(0, 1).
		Bold(true)
	b.WriteString(blockStyle.Render(d.TimeBlock))
	b.WriteString("\n\n")

	// Calendar table
	cal := components.StyledIcon(components.IconCalendar, colorSecondary) + subtitleStyle.Render("Calendar") + "\n"
	if len(d.Events) == 0 {
		cal += mutedStyle.Render("  No events today")
	} else {
		cols := []table.Column{
			table.NewColumn("time", "Time", 10),
			table.NewFlexColumn("summary", "Event", 1),
			table.NewColumn("dur", "Duration", 10),
		}
		var rows []table.Row
		for _, e := range d.Events {
			rows = append(rows, table.NewRow(table.RowData{
				"time":    e.Start,
				"summary": e.Summary,
				"dur":     e.Duration,
			}))
		}
		t := components.SimpleTable(cols, rows, width-4)
		cal += t.View()
	}
	b.WriteString(cardStyle.Width(width).Render(cal))
	b.WriteString("\n")

	// Tasks table
	tasks := components.StyledIcon(components.IconTask, colorWarning) + subtitleStyle.Render("Top Tasks") + "\n"
	if len(d.Tasks) == 0 {
		tasks += successStyle.Render("  All clear!")
	} else {
		cols := []table.Column{
			table.NewColumn("pri", "P", 3),
			table.NewFlexColumn("title", "Task", 1),
			table.NewColumn("due", "Due", 12),
		}
		var rows []table.Row
		for _, t := range d.Tasks {
			pri := ""
			switch {
			case t.Priority >= 4:
				pri = alertStyle.Render("!!!")
			case t.Priority >= 3:
				pri = warningStyle.Render("!!")
			case t.Priority >= 2:
				pri = mutedStyle.Render("!")
			}
			rows = append(rows, table.NewRow(table.RowData{
				"pri":   pri,
				"title": t.Title,
				"due":   t.DueDate,
			}))
		}
		t := components.SimpleTable(cols, rows, width-4)
		tasks += t.View()
	}
	b.WriteString(cardStyle.Width(width).Render(tasks))
	b.WriteString("\n")

	// Suggestions card
	if len(d.Suggestions) > 0 {
		sug := components.StyledIcon(components.IconStar, colorAccent) + subtitleStyle.Render("Suggestions") + "\n"
		for _, s := range d.Suggestions {
			icon := components.IconArrowR
			sug += fmt.Sprintf("  %s %s\n", components.StyledIcon(icon, colorSeries1), s.Title)
			if s.ActionHint != "" {
				sug += fmt.Sprintf("    %s\n", mutedStyle.Render(s.ActionHint))
			}
		}
		b.WriteString(cardStyle.Width(width).Render(sug))
		b.WriteString("\n")
	}

	// Status line: replies + weather
	var statusParts []string
	replyLabel := fmt.Sprintf("%s%d pending", components.IconReply, d.Replies)
	if d.Replies > 5 {
		statusParts = append(statusParts, warningStyle.Render(replyLabel))
	} else {
		statusParts = append(statusParts, mutedStyle.Render(replyLabel))
	}
	if d.Weather != "" {
		statusParts = append(statusParts, components.StyledIcon(components.IconSun, colorWarning)+d.Weather)
	}
	b.WriteString(mutedStyle.Render(strings.Join(statusParts, "  |  ")))

	return b.String()
}
