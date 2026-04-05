package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/events"
	"github.com/hairglasses-studio/runmylife/internal/jobs"
)

// WriteContext holds dependencies for write endpoints.
type WriteContext struct {
	DB      *sql.DB
	Emitter *events.Emitter
}

// --- POST /api/v1/tasks ---

type createTaskReq struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
	DueDate     string `json:"due_date"`
	Project     string `json:"project"`
}

func handleCreateTask(wc *WriteContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createTaskReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Title == "" {
			WriteError(w, http.StatusBadRequest, "title is required")
			return
		}
		if req.Priority < 1 || req.Priority > 4 {
			req.Priority = 2
		}

		id := fmt.Sprintf("api-%d", time.Now().UnixNano())
		_, err := wc.DB.ExecContext(r.Context(),
			`INSERT INTO tasks (id, title, description, priority, due_date, project, completed, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, 0, datetime('now'))`,
			id, req.Title, req.Description, req.Priority, req.DueDate, req.Project,
		)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to create task")
			return
		}

		WriteJSON(w, http.StatusCreated, map[string]string{"id": id, "title": req.Title})
	}
}

// --- PATCH /api/v1/tasks/{id}/complete ---

func handleCompleteTask(wc *WriteContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := r.PathValue("id")
		if taskID == "" {
			WriteError(w, http.StatusBadRequest, "task id required")
			return
		}

		var title string
		err := wc.DB.QueryRowContext(r.Context(),
			"SELECT title FROM tasks WHERE id = ?", taskID).Scan(&title)
		if err != nil {
			WriteError(w, http.StatusNotFound, "task not found")
			return
		}

		_, err = wc.DB.ExecContext(r.Context(),
			"UPDATE tasks SET completed = 1, updated_at = datetime('now') WHERE id = ?", taskID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to complete task")
			return
		}

		if wc.Emitter != nil {
			wc.Emitter.TaskCompleted(r.Context(), taskID, title)
		}

		WriteJSON(w, http.StatusOK, map[string]string{"id": taskID, "status": "completed"})
	}
}

// --- POST /api/v1/mood ---

type logMoodReq struct {
	Score        int     `json:"score"`
	EnergyLevel  int     `json:"energy_level"`
	SleepHours   float64 `json:"sleep_hours"`
	ExerciseDone bool    `json:"exercise_done"`
	Notes        string  `json:"notes"`
}

func handleLogMood(wc *WriteContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req logMoodReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Score < 1 || req.Score > 10 {
			WriteError(w, http.StatusBadRequest, "score must be 1-10")
			return
		}
		if req.EnergyLevel < 1 || req.EnergyLevel > 10 {
			req.EnergyLevel = 5
		}

		exerciseInt := 0
		if req.ExerciseDone {
			exerciseInt = 1
		}

		today := time.Now().Format("2006-01-02")
		_, err := wc.DB.ExecContext(r.Context(),
			`INSERT INTO mood_log (date, mood_score, energy_level, sleep_hours, exercise_done, notes)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			today, req.Score, req.EnergyLevel, req.SleepHours, exerciseInt, req.Notes,
		)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to log mood")
			return
		}

		if wc.Emitter != nil {
			wc.Emitter.MoodLogged(r.Context(), req.Score, req.Notes)
		}

		WriteJSON(w, http.StatusCreated, map[string]any{"date": today, "score": req.Score})
	}
}

// --- POST /api/v1/habits/{id}/complete ---

func handleCompleteHabit(wc *WriteContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		habitID := r.PathValue("id")
		if habitID == "" {
			WriteError(w, http.StatusBadRequest, "habit id required")
			return
		}

		var name string
		err := wc.DB.QueryRowContext(r.Context(),
			"SELECT name FROM habits WHERE id = ?", habitID).Scan(&name)
		if err != nil {
			WriteError(w, http.StatusNotFound, "habit not found")
			return
		}

		_, err = wc.DB.ExecContext(r.Context(),
			`INSERT INTO habit_completions (habit_id, completed_at) VALUES (?, datetime('now'))`,
			habitID,
		)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to complete habit")
			return
		}

		if wc.Emitter != nil {
			wc.Emitter.HabitCompleted(r.Context(), habitID, name)
		}

		WriteJSON(w, http.StatusCreated, map[string]string{"habit_id": habitID, "name": name})
	}
}

// --- POST /api/v1/focus/start ---

type startFocusReq struct {
	Category       string `json:"category"`
	PlannedMinutes int    `json:"planned_minutes"`
}

func handleStartFocus(wc *WriteContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req startFocusReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Category == "" {
			req.Category = "general"
		}
		if req.PlannedMinutes <= 0 {
			req.PlannedMinutes = 25
		}

		result, err := wc.DB.ExecContext(r.Context(),
			`INSERT INTO focus_sessions (category, started_at, planned_minutes)
			 VALUES (?, datetime('now'), ?)`,
			req.Category, req.PlannedMinutes,
		)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to start focus session")
			return
		}

		id, _ := result.LastInsertId()

		if wc.Emitter != nil {
			wc.Emitter.FocusStarted(r.Context(), req.Category)
		}

		WriteJSON(w, http.StatusCreated, map[string]any{
			"session_id":      id,
			"category":        req.Category,
			"planned_minutes": req.PlannedMinutes,
		})
	}
}

// --- POST /api/v1/focus/end ---

type endFocusReq struct {
	SessionID int64 `json:"session_id"`
}

func handleEndFocus(wc *WriteContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req endFocusReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.SessionID <= 0 {
			WriteError(w, http.StatusBadRequest, "session_id is required")
			return
		}

		var category, startedAt string
		err := wc.DB.QueryRowContext(r.Context(),
			"SELECT category, started_at FROM focus_sessions WHERE id = ? AND ended_at IS NULL",
			req.SessionID,
		).Scan(&category, &startedAt)
		if err != nil {
			WriteError(w, http.StatusNotFound, "active session not found")
			return
		}

		_, err = wc.DB.ExecContext(r.Context(),
			`UPDATE focus_sessions SET ended_at = datetime('now'),
			 actual_minutes = CAST((julianday('now') - julianday(started_at)) * 1440 AS INTEGER)
			 WHERE id = ?`, req.SessionID,
		)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to end session")
			return
		}

		var minutes int
		wc.DB.QueryRowContext(r.Context(),
			"SELECT actual_minutes FROM focus_sessions WHERE id = ?", req.SessionID).Scan(&minutes)

		if wc.Emitter != nil {
			wc.Emitter.FocusEnded(r.Context(), category, minutes)
		}

		WriteJSON(w, http.StatusOK, map[string]any{
			"session_id": req.SessionID,
			"category":   category,
			"minutes":    minutes,
		})
	}
}

// --- POST /api/v1/jobs ---

type enqueueJobReq struct {
	Type     string `json:"type"`
	Payload  string `json:"payload"`
	Priority int    `json:"priority"`
}

func handleEnqueueJob(wc *WriteContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req enqueueJobReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Type == "" {
			WriteError(w, http.StatusBadRequest, "type is required")
			return
		}
		if req.Priority <= 0 {
			req.Priority = 5
		}

		_, err := jobs.Enqueue(r.Context(), wc.DB, req.Type, req.Payload, req.Priority)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to enqueue job")
			return
		}

		WriteJSON(w, http.StatusCreated, map[string]string{"type": req.Type, "status": "queued"})
	}
}
