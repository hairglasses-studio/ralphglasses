package api

import (
	"database/sql"
	"net/http"
	"strconv"
)

// --- GET /api/v1/tasks ---

func handleListTasks(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := intParam(r, "limit", 50)
		project := r.URL.Query().Get("project")
		completedFilter := r.URL.Query().Get("completed")

		query := `SELECT id, title, COALESCE(description,''), priority, COALESCE(due_date,''), COALESCE(project,''), completed, created_at FROM tasks WHERE 1=1`
		var args []any

		if completedFilter == "true" {
			query += " AND completed = 1"
		} else if completedFilter == "false" {
			query += " AND completed = 0"
		}
		if project != "" {
			query += " AND project = ?"
			args = append(args, project)
		}
		query += " ORDER BY created_at DESC LIMIT ?"
		args = append(args, limit)

		rows, err := db.QueryContext(r.Context(), query, args...)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to query tasks")
			return
		}
		defer rows.Close()

		var tasks []map[string]any
		for rows.Next() {
			var id, title, desc, dueDate, proj, createdAt string
			var priority, completed int
			if err := rows.Scan(&id, &title, &desc, &priority, &dueDate, &proj, &completed, &createdAt); err != nil {
				continue
			}
			tasks = append(tasks, map[string]any{
				"id": id, "title": title, "description": desc,
				"priority": priority, "due_date": dueDate, "project": proj,
				"completed": completed == 1, "created_at": createdAt,
			})
		}
		if tasks == nil {
			tasks = []map[string]any{}
		}
		WriteJSON(w, http.StatusOK, tasks)
	}
}

// --- GET /api/v1/tasks/{id} ---

func handleGetTask(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := r.PathValue("id")
		if taskID == "" {
			WriteError(w, http.StatusBadRequest, "task id required")
			return
		}

		var id, title, desc, dueDate, proj, createdAt string
		var priority, completed int
		err := db.QueryRowContext(r.Context(),
			`SELECT id, title, COALESCE(description,''), priority, COALESCE(due_date,''), COALESCE(project,''), completed, created_at
			 FROM tasks WHERE id = ?`, taskID,
		).Scan(&id, &title, &desc, &priority, &dueDate, &proj, &completed, &createdAt)
		if err != nil {
			WriteError(w, http.StatusNotFound, "task not found")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]any{
			"id": id, "title": title, "description": desc,
			"priority": priority, "due_date": dueDate, "project": proj,
			"completed": completed == 1, "created_at": createdAt,
		})
	}
}

// --- GET /api/v1/habits ---

func handleListHabits(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rows, err := db.QueryContext(r.Context(),
			`SELECT h.id, h.name, h.frequency, h.created_at,
			        COUNT(hc.id) AS completions
			 FROM habits h
			 LEFT JOIN habit_completions hc ON hc.habit_id = h.id
			   AND hc.completed_at >= datetime('now', '-7 days')
			 GROUP BY h.id
			 ORDER BY h.name`)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to query habits")
			return
		}
		defer rows.Close()

		var habits []map[string]any
		for rows.Next() {
			var id, name, freq, createdAt string
			var completions int
			if rows.Scan(&id, &name, &freq, &createdAt, &completions) != nil {
				continue
			}
			habits = append(habits, map[string]any{
				"id": id, "name": name, "frequency": freq,
				"created_at": createdAt, "completions_7d": completions,
			})
		}
		if habits == nil {
			habits = []map[string]any{}
		}
		WriteJSON(w, http.StatusOK, habits)
	}
}

// --- GET /api/v1/habits/{id} ---

func handleGetHabit(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		habitID := r.PathValue("id")
		if habitID == "" {
			WriteError(w, http.StatusBadRequest, "habit id required")
			return
		}

		var id, name, freq, createdAt string
		err := db.QueryRowContext(r.Context(),
			"SELECT id, name, frequency, created_at FROM habits WHERE id = ?", habitID,
		).Scan(&id, &name, &freq, &createdAt)
		if err != nil {
			WriteError(w, http.StatusNotFound, "habit not found")
			return
		}

		// Recent completions
		compRows, _ := db.QueryContext(r.Context(),
			`SELECT completed_at FROM habit_completions WHERE habit_id = ?
			 ORDER BY completed_at DESC LIMIT 10`, habitID)
		var completions []string
		if compRows != nil {
			defer compRows.Close()
			for compRows.Next() {
				var at string
				if compRows.Scan(&at) == nil {
					completions = append(completions, at)
				}
			}
		}

		WriteJSON(w, http.StatusOK, map[string]any{
			"id": id, "name": name, "frequency": freq,
			"created_at": createdAt, "recent_completions": completions,
		})
	}
}

// --- GET /api/v1/mood ---

func handleListMood(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		days := intParam(r, "days", 7)

		rows, err := db.QueryContext(r.Context(),
			`SELECT date, mood_score, COALESCE(energy_level,0), COALESCE(sleep_hours,0),
			        COALESCE(exercise_done,0), COALESCE(notes,'')
			 FROM mood_log
			 WHERE date >= date('now', '-'||? ||' days')
			 ORDER BY date DESC`, days)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to query mood log")
			return
		}
		defer rows.Close()

		var entries []map[string]any
		for rows.Next() {
			var date, notes string
			var score, energy, exercise int
			var sleep float64
			if rows.Scan(&date, &score, &energy, &sleep, &exercise, &notes) != nil {
				continue
			}
			entries = append(entries, map[string]any{
				"date": date, "mood_score": score, "energy_level": energy,
				"sleep_hours": sleep, "exercise_done": exercise == 1, "notes": notes,
			})
		}
		if entries == nil {
			entries = []map[string]any{}
		}
		WriteJSON(w, http.StatusOK, entries)
	}
}

// --- GET /api/v1/focus/sessions ---

func handleListFocusSessions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		days := intParam(r, "days", 7)

		rows, err := db.QueryContext(r.Context(),
			`SELECT id, category, started_at, COALESCE(ended_at,''), planned_minutes, COALESCE(actual_minutes,0)
			 FROM focus_sessions
			 WHERE started_at >= datetime('now', '-'||?||' days')
			 ORDER BY started_at DESC`, days)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to query focus sessions")
			return
		}
		defer rows.Close()

		var sessions []map[string]any
		for rows.Next() {
			var id int64
			var category, startedAt, endedAt string
			var planned, actual int
			if rows.Scan(&id, &category, &startedAt, &endedAt, &planned, &actual) != nil {
				continue
			}
			sessions = append(sessions, map[string]any{
				"id": id, "category": category, "started_at": startedAt,
				"ended_at": endedAt, "planned_minutes": planned, "actual_minutes": actual,
			})
		}
		if sessions == nil {
			sessions = []map[string]any{}
		}
		WriteJSON(w, http.StatusOK, sessions)
	}
}

// --- GET /api/v1/notifications ---

func handleListNotifications(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		days := intParam(r, "days", 7)
		urgency := r.URL.Query().Get("urgency")

		query := `SELECT title, message, urgency, COALESCE(source,''), COALESCE(channels,''), sent_at
		          FROM notification_log
		          WHERE sent_at >= datetime('now', '-'||?||' days')`
		args := []any{days}

		if urgency != "" {
			query += " AND urgency = ?"
			args = append(args, urgency)
		}
		query += " ORDER BY sent_at DESC LIMIT 100"

		rows, err := db.QueryContext(r.Context(), query, args...)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to query notifications")
			return
		}
		defer rows.Close()

		var notifs []map[string]any
		for rows.Next() {
			var title, message, urg, source, channels, sentAt string
			if rows.Scan(&title, &message, &urg, &source, &channels, &sentAt) != nil {
				continue
			}
			notifs = append(notifs, map[string]any{
				"title": title, "message": message, "urgency": urg,
				"source": source, "channels": channels, "sent_at": sentAt,
			})
		}
		if notifs == nil {
			notifs = []map[string]any{}
		}
		WriteJSON(w, http.StatusOK, notifs)
	}
}

// intParam reads an integer query param with a default.
func intParam(r *http.Request, key string, def int) int {
	s := r.URL.Query().Get(key)
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return def
	}
	return v
}
