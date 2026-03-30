package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestDB_ReturnsUnderlyingDB(t *testing.T) {
	s := newTestStore(t)
	db := s.DB()
	if db == nil {
		t.Fatal("DB() returned nil")
	}
	// Verify we can execute a query.
	var result int
	if err := db.QueryRow("SELECT 1").Scan(&result); err != nil {
		t.Fatalf("query via DB(): %v", err)
	}
	if result != 1 {
		t.Errorf("SELECT 1 = %d, want 1", result)
	}
}

func TestParseTime_RFC3339Nano(t *testing.T) {
	t.Parallel()
	input := "2025-03-15T10:30:00.123456789Z"
	got := parseTime(input)
	if got.IsZero() {
		t.Error("parseTime returned zero for valid RFC3339Nano")
	}
	if got.Nanosecond() == 0 {
		t.Error("nanoseconds should be preserved")
	}
}

func TestParseTime_RFC3339(t *testing.T) {
	t.Parallel()
	input := "2025-03-15T10:30:00Z"
	got := parseTime(input)
	if got.IsZero() {
		t.Error("parseTime returned zero for valid RFC3339")
	}
}

func TestParseTime_InvalidFormat(t *testing.T) {
	t.Parallel()
	got := parseTime("not-a-timestamp")
	if !got.IsZero() {
		t.Errorf("parseTime(invalid) should return zero time, got %v", got)
	}
}

func TestParseTime_EmptyString(t *testing.T) {
	t.Parallel()
	got := parseTime("")
	if !got.IsZero() {
		t.Errorf("parseTime(\"\") should return zero time, got %v", got)
	}
}

func TestNormalizeJSON_NilInput(t *testing.T) {
	t.Parallel()
	got := normalizeJSON(nil)
	if string(got) != "null" {
		t.Errorf("normalizeJSON(nil) = %q, want \"null\"", got)
	}
}

func TestNormalizeJSON_EmptyInput(t *testing.T) {
	t.Parallel()
	got := normalizeJSON(json.RawMessage{})
	if string(got) != "null" {
		t.Errorf("normalizeJSON(empty) = %q, want \"null\"", got)
	}
}

func TestNormalizeJSON_ValidInput(t *testing.T) {
	t.Parallel()
	input := json.RawMessage(`{"key":"value"}`)
	got := normalizeJSON(input)
	if string(got) != `{"key":"value"}` {
		t.Errorf("normalizeJSON = %q, want original", got)
	}
}

func TestSaveSession_WithExistingCreatedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fixedTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	row := &SessionRow{
		ID:        "ts-preserved",
		Repo:      "test",
		Status:    "running",
		Provider:  "claude",
		CreatedAt: fixedTime,
	}
	if err := s.SaveSession(ctx, row); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetSession(ctx, "ts-preserved")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// CreatedAt should be the fixed time, not auto-set.
	if got.CreatedAt.Year() != 2025 || got.CreatedAt.Month() != 1 {
		t.Errorf("CreatedAt not preserved: %v", got.CreatedAt)
	}
}

func TestSaveObservation_EmptyID(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.SaveObservation(ctx, &ObservationRow{})
	if !errors.Is(err, ErrNilValue) {
		t.Errorf("expected ErrNilValue for empty observation ID, got %v", err)
	}
}

func TestSaveObservation_WithExistingCreatedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	fixedTime := time.Date(2024, 6, 15, 8, 0, 0, 0, time.UTC)
	obs := &ObservationRow{
		ID:        "obs-ts-preserved",
		SessionID: "sess-1",
		Type:      "metric",
		CreatedAt: fixedTime,
	}
	if err := s.SaveObservation(ctx, obs); err != nil {
		t.Fatalf("save: %v", err)
	}

	results, err := s.QueryObservations(ctx, "sess-1", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].CreatedAt.Year() != 2024 {
		t.Errorf("CreatedAt not preserved: %v", results[0].CreatedAt)
	}
}

func TestQueryObservations_NoMatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	results, err := s.QueryObservations(ctx, "no-session", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for non-existent session, got %d", len(results))
	}
}

func TestListSessions_Empty(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	sessions, err := s.ListSessions(ctx, "", "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions in empty store, got %d", len(sessions))
	}
}

func TestSetFleetState_NullValue(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Explicit null JSON.
	if err := s.SetFleetState(ctx, "null-key", json.RawMessage("null")); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := s.GetFleetState(ctx, "null-key")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Key != "null-key" {
		t.Errorf("key = %q", got.Key)
	}
}

func TestNewStore_OnDiskSubdirectoryCreation(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/sub/dir/test.db"
	s, err := New(path)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer s.Close()

	// Verify the store is functional.
	ctx := context.Background()
	if err := s.SetFleetState(ctx, "test", json.RawMessage(`"ok"`)); err != nil {
		t.Fatalf("set: %v", err)
	}
	got, err := s.GetFleetState(ctx, "test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got.Value) != `"ok"` {
		t.Errorf("value = %s", got.Value)
	}
}

func TestCloseWithNilStatements(t *testing.T) {
	// Simulate a store with nil prepared statements (partial init).
	s := &Store{}
	// Should not panic.
	err := s.Close()
	if err != nil {
		t.Errorf("close nil store: %v", err)
	}
}
