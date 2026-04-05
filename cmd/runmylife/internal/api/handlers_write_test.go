package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hairglasses-studio/runmylife/internal/db"
	"github.com/hairglasses-studio/runmylife/internal/jobs"
)

func testWriteContext(t *testing.T) *WriteContext {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if err := jobs.EnsureTable(database.DB); err != nil {
		t.Fatalf("ensure job_queue: %v", err)
	}

	return &WriteContext{DB: database.DB, Emitter: nil}
}

func postJSON(t *testing.T, handler http.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func patchJSON(t *testing.T, handler http.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPatch, "/", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", *resp.Error)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("data is not map: %T", resp.Data)
	}
	return data
}

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error, got nil")
	}
	return *resp.Error
}

// --- Task tests ---

func TestCreateTask(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleCreateTask(wc), createTaskReq{
		Title:    "Buy groceries",
		Priority: 1,
		Project:  "personal",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	data := decodeData(t, rec)
	if data["title"] != "Buy groceries" {
		t.Errorf("title = %v, want Buy groceries", data["title"])
	}
	if data["id"] == nil || data["id"] == "" {
		t.Error("expected non-empty id")
	}
}

func TestCreateTask_MissingTitle(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleCreateTask(wc), createTaskReq{Priority: 1})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	msg := decodeError(t, rec)
	if msg != "title is required" {
		t.Errorf("error = %q, want 'title is required'", msg)
	}
}

func TestCreateTask_DefaultPriority(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleCreateTask(wc), createTaskReq{Title: "Test", Priority: 99})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	// Priority 99 is out of range, should default to 2
	var priority int
	err := wc.DB.QueryRow("SELECT priority FROM tasks ORDER BY created_at DESC LIMIT 1").Scan(&priority)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if priority != 2 {
		t.Errorf("priority = %d, want 2 (default)", priority)
	}
}

// --- Complete task tests ---

func TestCompleteTask(t *testing.T) {
	wc := testWriteContext(t)
	// Insert a task first
	_, err := wc.DB.Exec(
		`INSERT INTO tasks (id, title, priority, completed, created_at) VALUES ('t1', 'Test task', 2, 0, datetime('now'))`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Use a mux to handle path params
	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/tasks/{id}/complete", handleCompleteTask(wc))

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/t1/complete", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var completed int
	wc.DB.QueryRow("SELECT completed FROM tasks WHERE id = 't1'").Scan(&completed)
	if completed != 1 {
		t.Errorf("completed = %d, want 1", completed)
	}
}

func TestCompleteTask_NotFound(t *testing.T) {
	wc := testWriteContext(t)

	mux := http.NewServeMux()
	mux.HandleFunc("PATCH /api/v1/tasks/{id}/complete", handleCompleteTask(wc))

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/tasks/nonexistent/complete", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// --- Mood tests ---

func TestLogMood(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleLogMood(wc), logMoodReq{
		Score:       7,
		EnergyLevel: 6,
		SleepHours:  7.5,
		Notes:       "good day",
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	data := decodeData(t, rec)
	if data["score"].(float64) != 7 {
		t.Errorf("score = %v, want 7", data["score"])
	}

	var count int
	wc.DB.QueryRow("SELECT COUNT(*) FROM mood_log").Scan(&count)
	if count != 1 {
		t.Errorf("mood_log rows = %d, want 1", count)
	}
}

func TestLogMood_InvalidScore(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleLogMood(wc), logMoodReq{Score: 0})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	msg := decodeError(t, rec)
	if msg != "score must be 1-10" {
		t.Errorf("error = %q", msg)
	}
}

func TestLogMood_DefaultEnergy(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleLogMood(wc), logMoodReq{Score: 5, EnergyLevel: 0})
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	var energy int
	wc.DB.QueryRow("SELECT energy_level FROM mood_log LIMIT 1").Scan(&energy)
	if energy != 5 {
		t.Errorf("energy_level = %d, want 5 (default)", energy)
	}
}

// --- Habit tests ---

func TestCompleteHabit(t *testing.T) {
	wc := testWriteContext(t)
	_, err := wc.DB.Exec(
		`INSERT INTO habits (id, name, frequency, created_at) VALUES ('h1', 'Exercise', 'daily', datetime('now'))`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/habits/{id}/complete", handleCompleteHabit(wc))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/habits/h1/complete", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}

	var count int
	wc.DB.QueryRow("SELECT COUNT(*) FROM habit_completions WHERE habit_id = 'h1'").Scan(&count)
	if count != 1 {
		t.Errorf("habit_completions = %d, want 1", count)
	}
}

func TestCompleteHabit_NotFound(t *testing.T) {
	wc := testWriteContext(t)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/habits/{id}/complete", handleCompleteHabit(wc))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/habits/nonexistent/complete", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// --- Focus tests ---

func TestStartFocus(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleStartFocus(wc), startFocusReq{
		Category:       "deep-work",
		PlannedMinutes: 45,
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	data := decodeData(t, rec)
	if data["category"] != "deep-work" {
		t.Errorf("category = %v", data["category"])
	}
	if data["planned_minutes"].(float64) != 45 {
		t.Errorf("planned_minutes = %v", data["planned_minutes"])
	}
}

func TestStartFocus_Defaults(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleStartFocus(wc), startFocusReq{})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	data := decodeData(t, rec)
	if data["category"] != "general" {
		t.Errorf("category = %v, want general", data["category"])
	}
	if data["planned_minutes"].(float64) != 25 {
		t.Errorf("planned_minutes = %v, want 25", data["planned_minutes"])
	}
}

func TestEndFocus(t *testing.T) {
	wc := testWriteContext(t)

	// Start a session first
	_, err := wc.DB.Exec(
		`INSERT INTO focus_sessions (id, category, started_at, planned_minutes)
		 VALUES (1, 'coding', datetime('now', '-30 minutes'), 45)`)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	rec := postJSON(t, handleEndFocus(wc), endFocusReq{SessionID: 1})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	data := decodeData(t, rec)
	if data["category"] != "coding" {
		t.Errorf("category = %v", data["category"])
	}
}

func TestEndFocus_NotFound(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleEndFocus(wc), endFocusReq{SessionID: 999})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestEndFocus_MissingID(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleEndFocus(wc), endFocusReq{SessionID: 0})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// --- Job tests ---

func TestEnqueueJob(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleEnqueueJob(wc), enqueueJobReq{
		Type:    "morning_briefing",
		Payload: `{"force": true}`,
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	data := decodeData(t, rec)
	if data["type"] != "morning_briefing" {
		t.Errorf("type = %v", data["type"])
	}
	if data["status"] != "queued" {
		t.Errorf("status = %v", data["status"])
	}
}

func TestEnqueueJob_MissingType(t *testing.T) {
	wc := testWriteContext(t)
	rec := postJSON(t, handleEnqueueJob(wc), enqueueJobReq{Payload: "{}"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// --- Auth middleware tests ---

func TestAuthMiddleware_ValidToken(t *testing.T) {
	mw := AuthMiddleware("secret123")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	mw := AuthMiddleware("secret123")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	mw := AuthMiddleware("secret123")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuthMiddleware_NoTokenConfigured(t *testing.T) {
	mw := AuthMiddleware("")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// No auth header — should still pass through
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (no token configured = no auth)", rec.Code)
	}
}

func TestAuthMiddleware_CaseInsensitiveBearer(t *testing.T) {
	mw := AuthMiddleware("tok")
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "bearer tok")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (bearer is case-insensitive)", rec.Code)
	}
}
