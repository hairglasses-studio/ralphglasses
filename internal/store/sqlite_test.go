package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// ---------- WAL mode ----------

func TestWALModeEnabled(t *testing.T) {
	s := newTestStore(t)

	var mode string
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	// In-memory databases may report "memory" instead of "wal" since WAL
	// requires a file on disk. Accept both.
	if mode != "wal" && mode != "memory" {
		t.Errorf("expected journal_mode wal or memory, got %q", mode)
	}
}

func TestWALModeOnDisk(t *testing.T) {
	path := t.TempDir() + "/test.db"
	s, err := New(path)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	var mode string
	if err := s.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("query journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Errorf("expected journal_mode wal, got %q", mode)
	}
}

// ---------- Session CRUD ----------

func TestSaveAndGetSession(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	data := json.RawMessage(`{"prompt":"build feature X"}`)
	row := &SessionRow{
		ID:       "sess-001",
		Repo:     "ralphglasses",
		Status:   "running",
		Provider: "claude",
		Data:     data,
	}

	if err := s.SaveSession(ctx, row); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetSession(ctx, "sess-001")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.ID != "sess-001" {
		t.Errorf("ID = %q, want sess-001", got.ID)
	}
	if got.Repo != "ralphglasses" {
		t.Errorf("Repo = %q, want ralphglasses", got.Repo)
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want running", got.Status)
	}
	if got.Provider != "claude" {
		t.Errorf("Provider = %q, want claude", got.Provider)
	}
	if string(got.Data) != `{"prompt":"build feature X"}` {
		t.Errorf("Data = %s, want %s", got.Data, data)
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
}

func TestSaveSessionUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	row := &SessionRow{
		ID:       "sess-002",
		Repo:     "mesmer",
		Status:   "running",
		Provider: "gemini",
	}
	if err := s.SaveSession(ctx, row); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Update status.
	row.Status = "completed"
	if err := s.SaveSession(ctx, row); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := s.GetSession(ctx, "sess-002")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("Status = %q after upsert, want completed", got.Status)
	}
}

func TestSaveSessionNilError(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SaveSession(ctx, nil); !errors.Is(err, ErrNilValue) {
		t.Errorf("expected ErrNilValue, got %v", err)
	}
	if err := s.SaveSession(ctx, &SessionRow{}); !errors.Is(err, ErrNilValue) {
		t.Errorf("expected ErrNilValue for empty ID, got %v", err)
	}
}

func TestGetSessionNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetSession(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListSessions(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for i := range 5 {
		repo := "repo-a"
		status := "running"
		if i >= 3 {
			repo = "repo-b"
		}
		if i == 4 {
			status = "completed"
		}
		row := &SessionRow{
			ID:       fmt.Sprintf("sess-%03d", i),
			Repo:     repo,
			Status:   status,
			Provider: "claude",
		}
		if err := s.SaveSession(ctx, row); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	// All sessions.
	all, err := s.ListSessions(ctx, "", "")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("list all: got %d, want 5", len(all))
	}

	// Filter by repo.
	repoA, err := s.ListSessions(ctx, "repo-a", "")
	if err != nil {
		t.Fatalf("list repo-a: %v", err)
	}
	if len(repoA) != 3 {
		t.Errorf("list repo-a: got %d, want 3", len(repoA))
	}

	// Filter by status.
	completed, err := s.ListSessions(ctx, "", "completed")
	if err != nil {
		t.Fatalf("list completed: %v", err)
	}
	if len(completed) != 1 {
		t.Errorf("list completed: got %d, want 1", len(completed))
	}

	// Filter by both.
	repoB, err := s.ListSessions(ctx, "repo-b", "running")
	if err != nil {
		t.Fatalf("list repo-b running: %v", err)
	}
	if len(repoB) != 1 {
		t.Errorf("list repo-b running: got %d, want 1", len(repoB))
	}
}

// ---------- Observation CRUD ----------

func TestSaveAndQueryObservations(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	obs := &ObservationRow{
		ID:        "obs-001",
		SessionID: "sess-001",
		Type:      "metric",
		Data:      json.RawMessage(`{"value":42}`),
	}
	if err := s.SaveObservation(ctx, obs); err != nil {
		t.Fatalf("save: %v", err)
	}

	obs2 := &ObservationRow{
		ID:        "obs-002",
		SessionID: "sess-001",
		Type:      "error",
		Data:      json.RawMessage(`{"msg":"timeout"}`),
	}
	if err := s.SaveObservation(ctx, obs2); err != nil {
		t.Fatalf("save 2: %v", err)
	}

	obs3 := &ObservationRow{
		ID:        "obs-003",
		SessionID: "sess-002",
		Type:      "metric",
	}
	if err := s.SaveObservation(ctx, obs3); err != nil {
		t.Fatalf("save 3: %v", err)
	}

	// Query by session_id.
	bySession, err := s.QueryObservations(ctx, "sess-001", "")
	if err != nil {
		t.Fatalf("query by session: %v", err)
	}
	if len(bySession) != 2 {
		t.Errorf("query by session: got %d, want 2", len(bySession))
	}

	// Query by type.
	byType, err := s.QueryObservations(ctx, "", "metric")
	if err != nil {
		t.Fatalf("query by type: %v", err)
	}
	if len(byType) != 2 {
		t.Errorf("query by type: got %d, want 2", len(byType))
	}

	// Query by session + type.
	specific, err := s.QueryObservations(ctx, "sess-001", "error")
	if err != nil {
		t.Fatalf("query specific: %v", err)
	}
	if len(specific) != 1 {
		t.Errorf("query specific: got %d, want 1", len(specific))
	}
	if string(specific[0].Data) != `{"msg":"timeout"}` {
		t.Errorf("Data = %s, want {\"msg\":\"timeout\"}", specific[0].Data)
	}

	// All observations.
	allObs, err := s.QueryObservations(ctx, "", "")
	if err != nil {
		t.Fatalf("query all: %v", err)
	}
	if len(allObs) != 3 {
		t.Errorf("query all: got %d, want 3", len(allObs))
	}
}

func TestSaveObservationNilError(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SaveObservation(ctx, nil); !errors.Is(err, ErrNilValue) {
		t.Errorf("expected ErrNilValue, got %v", err)
	}
}

func TestSaveObservationUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	obs := &ObservationRow{
		ID:        "obs-upsert",
		SessionID: "sess-x",
		Type:      "metric",
		Data:      json.RawMessage(`{"v":1}`),
	}
	if err := s.SaveObservation(ctx, obs); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Update data via upsert.
	obs.Data = json.RawMessage(`{"v":2}`)
	if err := s.SaveObservation(ctx, obs); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	results, err := s.QueryObservations(ctx, "sess-x", "")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 observation after upsert, got %d", len(results))
	}
	if string(results[0].Data) != `{"v":2}` {
		t.Errorf("Data = %s after upsert, want {\"v\":2}", results[0].Data)
	}
}

// ---------- Fleet State CRUD ----------

func TestSetAndGetFleetState(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	value := json.RawMessage(`{"workers":3,"budget_usd":100}`)
	if err := s.SetFleetState(ctx, "fleet_status", value); err != nil {
		t.Fatalf("set: %v", err)
	}

	got, err := s.GetFleetState(ctx, "fleet_status")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.Key != "fleet_status" {
		t.Errorf("Key = %q, want fleet_status", got.Key)
	}
	if string(got.Value) != `{"workers":3,"budget_usd":100}` {
		t.Errorf("Value = %s, want original JSON", got.Value)
	}
	if got.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero")
	}
}

func TestSetFleetStateUpsert(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SetFleetState(ctx, "budget", json.RawMessage(`{"total":100}`)); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Overwrite.
	if err := s.SetFleetState(ctx, "budget", json.RawMessage(`{"total":200}`)); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	got, err := s.GetFleetState(ctx, "budget")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(got.Value) != `{"total":200}` {
		t.Errorf("Value = %s after upsert, want {\"total\":200}", got.Value)
	}
}

func TestSetFleetStateEmptyKey(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if err := s.SetFleetState(ctx, "", nil); !errors.Is(err, ErrNilValue) {
		t.Errorf("expected ErrNilValue for empty key, got %v", err)
	}
}

func TestGetFleetStateNotFound(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_, err := s.GetFleetState(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ---------- Concurrent writes ----------

func TestConcurrentWrites(t *testing.T) {
	// Use an on-disk database so WAL actually kicks in.
	path := t.TempDir() + "/concurrent.db"
	s, err := New(path)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	const n = 50
	var wg sync.WaitGroup

	// Concurrent session writes.
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			row := &SessionRow{
				ID:       fmt.Sprintf("concurrent-sess-%03d", i),
				Repo:     "test-repo",
				Status:   "running",
				Provider: "claude",
				Data:     json.RawMessage(fmt.Sprintf(`{"i":%d}`, i)),
			}
			if err := s.SaveSession(ctx, row); err != nil {
				t.Errorf("concurrent save session %d: %v", i, err)
			}
		}(i)
	}

	// Concurrent observation writes.
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			obs := &ObservationRow{
				ID:        fmt.Sprintf("concurrent-obs-%03d", i),
				SessionID: fmt.Sprintf("concurrent-sess-%03d", i%10),
				Type:      "metric",
				Data:      json.RawMessage(fmt.Sprintf(`{"val":%d}`, i)),
			}
			if err := s.SaveObservation(ctx, obs); err != nil {
				t.Errorf("concurrent save observation %d: %v", i, err)
			}
		}(i)
	}

	// Concurrent fleet state writes.
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%03d", i%10) // deliberate key collisions for upsert
			val := json.RawMessage(fmt.Sprintf(`{"i":%d}`, i))
			if err := s.SetFleetState(ctx, key, val); err != nil {
				t.Errorf("concurrent set fleet state %d: %v", i, err)
			}
		}(i)
	}

	wg.Wait()

	// Verify counts.
	sessions, err := s.ListSessions(ctx, "", "")
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != n {
		t.Errorf("sessions: got %d, want %d", len(sessions), n)
	}

	obs, err := s.QueryObservations(ctx, "", "")
	if err != nil {
		t.Fatalf("query observations: %v", err)
	}
	if len(obs) != n {
		t.Errorf("observations: got %d, want %d", len(obs), n)
	}
}

// ---------- Null / empty data handling ----------

func TestNullDataHandling(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	// Session with nil data.
	row := &SessionRow{
		ID:       "null-data-sess",
		Repo:     "test",
		Status:   "running",
		Provider: "claude",
	}
	if err := s.SaveSession(ctx, row); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetSession(ctx, "null-data-sess")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	// Data should be nil or "null" — either is acceptable.
	if got.Data != nil && string(got.Data) != "null" {
		t.Errorf("Data = %s, want nil or null", got.Data)
	}

	// Fleet state with nil value.
	if err := s.SetFleetState(ctx, "empty-val", nil); err != nil {
		t.Fatalf("set nil fleet state: %v", err)
	}
	fs, err := s.GetFleetState(ctx, "empty-val")
	if err != nil {
		t.Fatalf("get fleet state: %v", err)
	}
	if fs.Value != nil && string(fs.Value) != "null" {
		t.Errorf("fleet state Value = %s, want nil or null", fs.Value)
	}
}

// ---------- Close ----------

func TestCloseIdempotent(t *testing.T) {
	s, err := New(":memory:")
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// Close twice should not panic.
	if err := s.Close(); err != nil {
		t.Errorf("first close: %v", err)
	}
	// Second close may return an error, but must not panic.
	_ = s.Close()
}

// ---------- Timestamps ----------

func TestTimestampsSet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	before := time.Now().UTC().Add(-time.Second)

	row := &SessionRow{
		ID:       "ts-sess",
		Repo:     "test",
		Status:   "running",
		Provider: "claude",
	}
	if err := s.SaveSession(ctx, row); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := s.GetSession(ctx, "ts-sess")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.CreatedAt.Before(before) {
		t.Errorf("CreatedAt %v is before test start %v", got.CreatedAt, before)
	}
	if got.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt %v is before test start %v", got.UpdatedAt, before)
	}
}

func TestUpsertUpdatesTimestamp(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	row := &SessionRow{
		ID:       "upsert-ts",
		Repo:     "test",
		Status:   "running",
		Provider: "claude",
	}
	if err := s.SaveSession(ctx, row); err != nil {
		t.Fatalf("save: %v", err)
	}

	first, _ := s.GetSession(ctx, "upsert-ts")

	// Small sleep to ensure time moves forward.
	time.Sleep(10 * time.Millisecond)

	row.Status = "completed"
	row.UpdatedAt = time.Time{} // reset so SaveSession auto-sets to now
	if err := s.SaveSession(ctx, row); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	second, _ := s.GetSession(ctx, "upsert-ts")
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Errorf("UpdatedAt did not advance: first=%v second=%v", first.UpdatedAt, second.UpdatedAt)
	}
}
