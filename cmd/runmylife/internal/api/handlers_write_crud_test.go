package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- Update task tests ---

func TestUpdateTask(t *testing.T) {
	wc := testWriteContext(t)
	wc.DB.Exec(`INSERT INTO tasks (id, title, priority, completed, created_at) VALUES ('t1', 'Original', 2, 0, datetime('now'))`)

	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/tasks/{id}", handleUpdateTask(wc))

	body, _ := json.Marshal(updateTaskReq{
		Title:    strPtr("Updated"),
		Priority: intPtr(3),
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/t1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var title string
	var priority int
	wc.DB.QueryRow("SELECT title, priority FROM tasks WHERE id = 't1'").Scan(&title, &priority)
	if title != "Updated" {
		t.Errorf("title = %q, want Updated", title)
	}
	if priority != 3 {
		t.Errorf("priority = %d, want 3", priority)
	}
}

func TestUpdateTask_NotFound(t *testing.T) {
	wc := testWriteContext(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/tasks/{id}", handleUpdateTask(wc))

	body, _ := json.Marshal(updateTaskReq{Title: strPtr("Nope")})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/nonexistent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestUpdateTask_PartialFields(t *testing.T) {
	wc := testWriteContext(t)
	wc.DB.Exec(`INSERT INTO tasks (id, title, description, priority, completed, project, created_at) VALUES ('t1', 'Original', 'desc', 2, 0, 'work', datetime('now'))`)

	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/tasks/{id}", handleUpdateTask(wc))

	// Only update project, leave everything else
	body, _ := json.Marshal(updateTaskReq{Project: strPtr("personal")})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/t1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var title, project string
	var priority int
	wc.DB.QueryRow("SELECT title, priority, COALESCE(project,'') FROM tasks WHERE id = 't1'").Scan(&title, &priority, &project)
	if title != "Original" {
		t.Errorf("title = %q, want Original (unchanged)", title)
	}
	if priority != 2 {
		t.Errorf("priority = %d, want 2 (unchanged)", priority)
	}
	if project != "personal" {
		t.Errorf("project = %q, want personal", project)
	}
}

func TestUpdateTask_PriorityClamped(t *testing.T) {
	wc := testWriteContext(t)
	wc.DB.Exec(`INSERT INTO tasks (id, title, priority, completed, created_at) VALUES ('t1', 'Test', 2, 0, datetime('now'))`)

	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/tasks/{id}", handleUpdateTask(wc))

	// Priority 99 is out of range, should clamp to 2
	body, _ := json.Marshal(updateTaskReq{Priority: intPtr(99)})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/t1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var priority int
	wc.DB.QueryRow("SELECT priority FROM tasks WHERE id = 't1'").Scan(&priority)
	if priority != 2 {
		t.Errorf("priority = %d, want 2 (clamped)", priority)
	}
}

// --- Delete task tests ---

func TestDeleteTask(t *testing.T) {
	wc := testWriteContext(t)
	wc.DB.Exec(`INSERT INTO tasks (id, title, priority, completed, created_at) VALUES ('t1', 'Delete me', 2, 0, datetime('now'))`)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/tasks/{id}", handleDeleteTask(wc))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/t1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var count int
	wc.DB.QueryRow("SELECT COUNT(*) FROM tasks WHERE id = 't1'").Scan(&count)
	if count != 0 {
		t.Errorf("task still exists after delete")
	}
}

func TestDeleteTask_NotFound(t *testing.T) {
	wc := testWriteContext(t)

	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /api/v1/tasks/{id}", handleDeleteTask(wc))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/tasks/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// --- Create habit tests ---

func TestCreateHabit(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleCreateHabit(wc), createHabitReq{
		Name:      "Exercise",
		Frequency: "daily",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	data := decodeData(t, rec)
	if data["name"] != "Exercise" {
		t.Errorf("name = %v, want Exercise", data["name"])
	}
	if data["id"] == nil || data["id"] == "" {
		t.Error("expected non-empty id")
	}

	var count int
	wc.DB.QueryRow("SELECT COUNT(*) FROM habits").Scan(&count)
	if count != 1 {
		t.Errorf("habits rows = %d, want 1", count)
	}
}

func TestCreateHabit_MissingName(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleCreateHabit(wc), createHabitReq{Frequency: "weekly"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestCreateHabit_DefaultFrequency(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleCreateHabit(wc), createHabitReq{Name: "Read"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	var freq string
	wc.DB.QueryRow("SELECT frequency FROM habits LIMIT 1").Scan(&freq)
	if freq != "daily" {
		t.Errorf("frequency = %q, want daily (default)", freq)
	}
}

// --- Update habit tests ---

func TestUpdateHabit(t *testing.T) {
	wc := testWriteContext(t)
	wc.DB.Exec(`INSERT INTO habits (id, name, frequency, created_at) VALUES ('h1', 'Read', 'daily', datetime('now'))`)

	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/habits/{id}", handleUpdateHabit(wc))

	body, _ := json.Marshal(updateHabitReq{
		Name:      strPtr("Read Books"),
		Frequency: strPtr("weekly"),
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/habits/h1", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var name, freq string
	wc.DB.QueryRow("SELECT name, frequency FROM habits WHERE id = 'h1'").Scan(&name, &freq)
	if name != "Read Books" {
		t.Errorf("name = %q, want 'Read Books'", name)
	}
	if freq != "weekly" {
		t.Errorf("frequency = %q, want weekly", freq)
	}
}

func TestUpdateHabit_NotFound(t *testing.T) {
	wc := testWriteContext(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/habits/{id}", handleUpdateHabit(wc))

	body, _ := json.Marshal(updateHabitReq{Name: strPtr("Nope")})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/habits/nonexistent", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// Helpers for pointer fields

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }
