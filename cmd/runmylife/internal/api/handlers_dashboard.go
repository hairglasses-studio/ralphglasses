package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/adhd"
	"github.com/hairglasses-studio/runmylife/internal/intelligence"
	"github.com/hairglasses-studio/runmylife/internal/timecontext"
)

func handleDashboardToday(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		now := time.Now()
		today := now.Format("2006-01-02")
		block := timecontext.CurrentBlock()

		data := map[string]any{
			"date":       today,
			"time_block": block.Label(),
			"priorities": block.Priorities(),
		}

		// Calendar events today
		data["calendar_events"] = queryCalendarEvents(ctx, db, today)

		// Top tasks
		data["top_tasks"] = queryTopTasks(ctx, db)

		// Pending replies
		data["pending_replies"] = countPendingReplies(ctx, db)

		// Energy level
		curve := adhd.BuildEnergyCurve(ctx, db, today)
		if curve != nil {
			data["energy_level"] = adhd.CurrentEnergyFromCurve(curve)
		}

		// Weather
		data["weather"] = queryWeather(ctx, db)

		// Intelligence suggestions
		suggestions := intelligence.QuerySuggestions(ctx, db, 5)
		sugList := make([]map[string]any, 0, len(suggestions))
		for _, s := range suggestions {
			sugList = append(sugList, map[string]any{
				"category":    s.Category,
				"priority":    s.Priority,
				"title":       s.Title,
				"description": s.Description,
				"action_hint": s.ActionHint,
			})
		}
		data["suggestions"] = sugList

		WriteJSON(w, http.StatusOK, data)
	}
}

func handleDashboardADHD(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		now := time.Now()
		today := now.Format("2006-01-02")

		data := map[string]any{}

		// Overwhelm score
		score, _ := adhd.CheckOverwhelm(ctx, db)
		if score != nil {
			data["overwhelm_score"] = score.CompositeScore
			data["triage_activated"] = score.TriageActivated
		}

		// Active focus session + hyperfocus check
		alert, _ := adhd.DetectHyperfocus(ctx, db)
		if alert != nil {
			data["focus_session"] = map[string]any{
				"category":    alert.Category,
				"minutes":     alert.Minutes,
				"should_break": alert.ShouldBreak,
				"nudge":       alert.GentleNudge,
			}
		}

		// Recent achievements
		celebrations := adhd.GetRecentCelebrations(ctx, db, 7)
		achList := make([]map[string]any, 0, len(celebrations))
		for _, c := range celebrations {
			achList = append(achList, map[string]any{
				"type":        c.Type,
				"title":       c.Title,
				"description": c.Description,
				"achieved_at": c.AchievedAt,
			})
		}
		data["recent_achievements"] = achList

		// Context switch stats today
		stats, _ := adhd.GetSwitchStats(ctx, db, today)
		if stats != nil {
			data["switch_stats"] = map[string]any{
				"total_switches":  stats.TotalSwitches,
				"total_cost_min":  stats.TotalCostMinutes,
				"avg_cost_min":    stats.AvgCostMinutes,
				"most_frequent":   stats.MostFrequentPair,
			}
		}

		WriteJSON(w, http.StatusOK, data)
	}
}

// --- DB query helpers ---

func queryCalendarEvents(ctx context.Context, db *sql.DB, date string) []map[string]any {
	rows, err := db.QueryContext(ctx,
		`SELECT summary, start_time, end_time FROM calendar_events
		 WHERE date(start_time) = ? ORDER BY start_time LIMIT 10`, date)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var events []map[string]any
	for rows.Next() {
		var summary, start, end string
		if rows.Scan(&summary, &start, &end) == nil {
			events = append(events, map[string]any{
				"summary": summary, "start": start, "end": end,
			})
		}
	}
	return events
}

func queryTopTasks(ctx context.Context, db *sql.DB) []map[string]any {
	rows, err := db.QueryContext(ctx,
		`SELECT id, title, priority, due_date FROM tasks
		 WHERE completed = 0 ORDER BY priority DESC, due_date ASC LIMIT 5`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var tasks []map[string]any
	for rows.Next() {
		var id, title string
		var priority int
		var dueDate sql.NullString
		if rows.Scan(&id, &title, &priority, &dueDate) == nil {
			t := map[string]any{"id": id, "title": title, "priority": priority}
			if dueDate.Valid {
				t["due_date"] = dueDate.String
			}
			tasks = append(tasks, t)
		}
	}
	return tasks
}

func countPendingReplies(ctx context.Context, db *sql.DB) int {
	var count int
	db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM reply_tracker WHERE replied = 0`).Scan(&count)
	return count
}

func queryWeather(ctx context.Context, db *sql.DB) map[string]any {
	var dataJSON string
	err := db.QueryRowContext(ctx,
		`SELECT data_json FROM weather_cache
		 WHERE forecast_type = 'current'
		 ORDER BY fetched_at DESC LIMIT 1`).Scan(&dataJSON)
	if err != nil {
		return nil
	}
	var result map[string]any
	if json.Unmarshal([]byte(dataJSON), &result) != nil {
		return nil
	}
	return result
}
