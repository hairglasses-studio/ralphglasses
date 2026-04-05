package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/db"
)

func testReadDB(t *testing.T) *WriteContext {
	t.Helper()
	database, err := db.OpenMemory()
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return &WriteContext{DB: database.DB, Emitter: nil}
}

func getJSON(t *testing.T, handler http.HandlerFunc, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeList(t *testing.T, rec *httptest.ResponseRecorder) []any {
	t.Helper()
	var resp Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %s", *resp.Error)
	}
	list, ok := resp.Data.([]any)
	if !ok {
		t.Fatalf("data is not list: %T", resp.Data)
	}
	return list
}

// --- Task list tests ---

func TestListTasks_Empty(t *testing.T) {
	wc := testReadDB(t)
	rec := getJSON(t, handleListTasks(wc.DB), "/api/v1/tasks")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	list := decodeList(t, rec)
	if len(list) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(list))
	}
}

func TestListTasks_WithData(t *testing.T) {
	wc := testReadDB(t)
	wc.DB.Exec(`INSERT INTO tasks (id, title, priority, completed, created_at) VALUES ('t1', 'Task A', 1, 0, datetime('now'))`)
	wc.DB.Exec(`INSERT INTO tasks (id, title, priority, completed, created_at) VALUES ('t2', 'Task B', 2, 1, datetime('now'))`)

	rec := getJSON(t, handleListTasks(wc.DB), "/api/v1/tasks")
	list := decodeList(t, rec)
	if len(list) != 2 {
		t.Errorf("expected 2 tasks, got %d", len(list))
	}
}

func TestListTasks_FilterCompleted(t *testing.T) {
	wc := testReadDB(t)
	wc.DB.Exec(`INSERT INTO tasks (id, title, priority, completed, created_at) VALUES ('t1', 'Open', 1, 0, datetime('now'))`)
	wc.DB.Exec(`INSERT INTO tasks (id, title, priority, completed, created_at) VALUES ('t2', 'Done', 2, 1, datetime('now'))`)

	rec := getJSON(t, handleListTasks(wc.DB), "/api/v1/tasks?completed=false")
	list := decodeList(t, rec)
	if len(list) != 1 {
		t.Errorf("expected 1 open task, got %d", len(list))
	}
}

func TestListTasks_FilterProject(t *testing.T) {
	wc := testReadDB(t)
	wc.DB.Exec(`INSERT INTO tasks (id, title, priority, completed, project, created_at) VALUES ('t1', 'A', 1, 0, 'work', datetime('now'))`)
	wc.DB.Exec(`INSERT INTO tasks (id, title, priority, completed, project, created_at) VALUES ('t2', 'B', 1, 0, 'personal', datetime('now'))`)

	rec := getJSON(t, handleListTasks(wc.DB), "/api/v1/tasks?project=work")
	list := decodeList(t, rec)
	if len(list) != 1 {
		t.Errorf("expected 1 work task, got %d", len(list))
	}
}

// --- Get task tests ---

func TestGetTask_Found(t *testing.T) {
	wc := testReadDB(t)
	wc.DB.Exec(`INSERT INTO tasks (id, title, priority, completed, created_at) VALUES ('t1', 'Found', 2, 0, datetime('now'))`)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/tasks/{id}", handleGetTask(wc.DB))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/t1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	data := decodeData(t, rec)
	if data["title"] != "Found" {
		t.Errorf("title = %v, want Found", data["title"])
	}
}

func TestGetTask_NotFound(t *testing.T) {
	wc := testReadDB(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/tasks/{id}", handleGetTask(wc.DB))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks/nonexistent", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// --- Habit list tests ---

func TestListHabits_WithStreaks(t *testing.T) {
	wc := testReadDB(t)
	wc.DB.Exec(`INSERT INTO habits (id, name, frequency, created_at) VALUES ('h1', 'Exercise', 'daily', datetime('now'))`)
	wc.DB.Exec(`INSERT INTO habit_completions (habit_id, completed_at) VALUES ('h1', datetime('now'))`)
	wc.DB.Exec(`INSERT INTO habit_completions (habit_id, completed_at) VALUES ('h1', datetime('now', '-1 day'))`)

	rec := getJSON(t, handleListHabits(wc.DB), "/api/v1/habits")
	list := decodeList(t, rec)
	if len(list) != 1 {
		t.Fatalf("expected 1 habit, got %d", len(list))
	}
	habit := list[0].(map[string]any)
	completions := int(habit["completions_7d"].(float64))
	if completions != 2 {
		t.Errorf("completions_7d = %d, want 2", completions)
	}
}

func TestListHabits_Empty(t *testing.T) {
	wc := testReadDB(t)
	rec := getJSON(t, handleListHabits(wc.DB), "/api/v1/habits")
	list := decodeList(t, rec)
	if len(list) != 0 {
		t.Errorf("expected 0 habits, got %d", len(list))
	}
}

// --- Get habit tests ---

func TestGetHabit_Found(t *testing.T) {
	wc := testReadDB(t)
	wc.DB.Exec(`INSERT INTO habits (id, name, frequency, created_at) VALUES ('h1', 'Read', 'daily', datetime('now'))`)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/habits/{id}", handleGetHabit(wc.DB))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/habits/h1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	data := decodeData(t, rec)
	if data["name"] != "Read" {
		t.Errorf("name = %v, want Read", data["name"])
	}
}

func TestGetHabit_NotFound(t *testing.T) {
	wc := testReadDB(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/habits/{id}", handleGetHabit(wc.DB))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/habits/nope", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

// --- Mood list tests ---

func TestListMood_DefaultDays(t *testing.T) {
	wc := testReadDB(t)
	today := time.Now().Format("2006-01-02")
	wc.DB.Exec(`INSERT INTO mood_log (date, mood_score, energy_level, sleep_hours) VALUES (?, 7, 6, 8)`, today)

	rec := getJSON(t, handleListMood(wc.DB), "/api/v1/mood")
	list := decodeList(t, rec)
	if len(list) != 1 {
		t.Errorf("expected 1 mood entry, got %d", len(list))
	}
}

func TestListMood_Empty(t *testing.T) {
	wc := testReadDB(t)
	rec := getJSON(t, handleListMood(wc.DB), "/api/v1/mood")
	list := decodeList(t, rec)
	if len(list) != 0 {
		t.Errorf("expected 0 mood entries, got %d", len(list))
	}
}

// --- Focus sessions tests ---

func TestListFocusSessions_WithData(t *testing.T) {
	wc := testReadDB(t)
	wc.DB.Exec(`INSERT INTO focus_sessions (category, started_at, planned_minutes) VALUES ('coding', datetime('now'), 45)`)

	rec := getJSON(t, handleListFocusSessions(wc.DB), "/api/v1/focus/sessions")
	list := decodeList(t, rec)
	if len(list) != 1 {
		t.Errorf("expected 1 session, got %d", len(list))
	}
}

func TestListFocusSessions_Empty(t *testing.T) {
	wc := testReadDB(t)
	rec := getJSON(t, handleListFocusSessions(wc.DB), "/api/v1/focus/sessions")
	list := decodeList(t, rec)
	if len(list) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(list))
	}
}

// --- Notification list tests ---

func TestListNotifications_WithData(t *testing.T) {
	wc := testReadDB(t)
	now := time.Now().Format(time.RFC3339)
	wc.DB.Exec(
		`INSERT INTO notification_log (title, message, urgency, source, channels, sent_at)
		 VALUES ('Alert', 'msg', 'high', 'test', 'discord_dm', ?)`, now)

	rec := getJSON(t, handleListNotifications(wc.DB), "/api/v1/notifications")
	list := decodeList(t, rec)
	if len(list) != 1 {
		t.Errorf("expected 1 notification, got %d", len(list))
	}
}

func TestListNotifications_FilterUrgency(t *testing.T) {
	wc := testReadDB(t)
	now := time.Now().Format(time.RFC3339)
	wc.DB.Exec(`INSERT INTO notification_log (title, message, urgency, source, channels, sent_at) VALUES ('Hi', 'msg', 'high', 'test', 'discord', ?)`, now)
	wc.DB.Exec(`INSERT INTO notification_log (title, message, urgency, source, channels, sent_at) VALUES ('Lo', 'msg', 'low', 'test', 'log', ?)`, now)

	rec := getJSON(t, handleListNotifications(wc.DB), "/api/v1/notifications?urgency=low")
	list := decodeList(t, rec)
	if len(list) != 1 {
		t.Errorf("expected 1 low-urgency notification, got %d", len(list))
	}
}

func TestListNotifications_Empty(t *testing.T) {
	wc := testReadDB(t)
	rec := getJSON(t, handleListNotifications(wc.DB), "/api/v1/notifications")
	list := decodeList(t, rec)
	if len(list) != 0 {
		t.Errorf("expected 0 notifications, got %d", len(list))
	}
}
