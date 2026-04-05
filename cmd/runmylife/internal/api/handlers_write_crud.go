package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// --- PATCH /api/v1/tasks/{id} ---

type updateTaskReq struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	Priority    *int    `json:"priority"`
	DueDate     *string `json:"due_date"`
	Project     *string `json:"project"`
}

func handleUpdateTask(wc *WriteContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := r.PathValue("id")
		if taskID == "" {
			WriteError(w, http.StatusBadRequest, "task id required")
			return
		}

		var req updateTaskReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		// Check task exists
		var exists int
		err := wc.DB.QueryRowContext(r.Context(),
			"SELECT 1 FROM tasks WHERE id = ?", taskID).Scan(&exists)
		if err != nil {
			WriteError(w, http.StatusNotFound, "task not found")
			return
		}

		// Build dynamic update
		sets := "updated_at = datetime('now')"
		var args []any
		if req.Title != nil {
			sets += ", title = ?"
			args = append(args, *req.Title)
		}
		if req.Description != nil {
			sets += ", description = ?"
			args = append(args, *req.Description)
		}
		if req.Priority != nil {
			p := *req.Priority
			if p < 1 || p > 4 {
				p = 2
			}
			sets += ", priority = ?"
			args = append(args, p)
		}
		if req.DueDate != nil {
			sets += ", due_date = ?"
			args = append(args, *req.DueDate)
		}
		if req.Project != nil {
			sets += ", project = ?"
			args = append(args, *req.Project)
		}
		args = append(args, taskID)

		_, err = wc.DB.ExecContext(r.Context(),
			fmt.Sprintf("UPDATE tasks SET %s WHERE id = ?", sets), args...)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to update task")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"id": taskID, "status": "updated"})
	}
}

// --- DELETE /api/v1/tasks/{id} ---

func handleDeleteTask(wc *WriteContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskID := r.PathValue("id")
		if taskID == "" {
			WriteError(w, http.StatusBadRequest, "task id required")
			return
		}

		result, err := wc.DB.ExecContext(r.Context(),
			"DELETE FROM tasks WHERE id = ?", taskID)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to delete task")
			return
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			WriteError(w, http.StatusNotFound, "task not found")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"id": taskID, "status": "deleted"})
	}
}

// --- POST /api/v1/habits ---

type createHabitReq struct {
	Name      string `json:"name"`
	Frequency string `json:"frequency"`
}

func handleCreateHabit(wc *WriteContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createHabitReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.Name == "" {
			WriteError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.Frequency == "" {
			req.Frequency = "daily"
		}

		id := fmt.Sprintf("api-%d", time.Now().UnixNano())
		_, err := wc.DB.ExecContext(r.Context(),
			`INSERT INTO habits (id, name, frequency, created_at) VALUES (?, ?, ?, datetime('now'))`,
			id, req.Name, req.Frequency)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to create habit")
			return
		}

		WriteJSON(w, http.StatusCreated, map[string]string{"id": id, "name": req.Name})
	}
}

// --- PATCH /api/v1/habits/{id} ---

type updateHabitReq struct {
	Name      *string `json:"name"`
	Frequency *string `json:"frequency"`
}

func handleUpdateHabit(wc *WriteContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		habitID := r.PathValue("id")
		if habitID == "" {
			WriteError(w, http.StatusBadRequest, "habit id required")
			return
		}

		var req updateHabitReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		var exists int
		err := wc.DB.QueryRowContext(r.Context(),
			"SELECT 1 FROM habits WHERE id = ?", habitID).Scan(&exists)
		if err != nil {
			WriteError(w, http.StatusNotFound, "habit not found")
			return
		}

		sets := "id = id" // no-op base for SET clause
		var args []any
		if req.Name != nil {
			sets += ", name = ?"
			args = append(args, *req.Name)
		}
		if req.Frequency != nil {
			sets += ", frequency = ?"
			args = append(args, *req.Frequency)
		}
		args = append(args, habitID)

		_, err = wc.DB.ExecContext(r.Context(),
			fmt.Sprintf("UPDATE habits SET %s WHERE id = ?", sets), args...)
		if err != nil {
			WriteError(w, http.StatusInternalServerError, "failed to update habit")
			return
		}

		WriteJSON(w, http.StatusOK, map[string]string{"id": habitID, "status": "updated"})
	}
}
