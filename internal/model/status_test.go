package model

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupRalphDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadStatus_Valid(t *testing.T) {
	dir := setupRalphDir(t)
	now := time.Now().Truncate(time.Second)

	status := LoopStatus{
		Timestamp:       now,
		LoopCount:       42,
		CallsMadeThisHr: 15,
		MaxCallsPerHour: 100,
		LastAction:      "edit_file",
		Status:          "running",
		Model:           "claude-sonnet-4-20250514",
		SessionSpendUSD: 1.23,
		BudgetStatus:    "ok",
	}
	writeJSON(t, filepath.Join(dir, ".ralph", "status.json"), status)

	loaded, err := LoadStatus(dir)
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}

	if loaded.LoopCount != 42 {
		t.Errorf("LoopCount = %d, want 42", loaded.LoopCount)
	}
	if loaded.Status != "running" {
		t.Errorf("Status = %q, want %q", loaded.Status, "running")
	}
	if loaded.CallsMadeThisHr != 15 {
		t.Errorf("CallsMadeThisHr = %d, want 15", loaded.CallsMadeThisHr)
	}
	if loaded.SessionSpendUSD != 1.23 {
		t.Errorf("SessionSpendUSD = %f, want 1.23", loaded.SessionSpendUSD)
	}
}

func TestLoadStatus_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadStatus(dir)
	if err == nil {
		t.Fatal("expected error for missing status.json")
	}
}

func TestLoadStatus_InvalidJSON(t *testing.T) {
	dir := setupRalphDir(t)
	if err := os.WriteFile(filepath.Join(dir, ".ralph", "status.json"), []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadStatus(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadCircuitBreaker_Valid(t *testing.T) {
	dir := setupRalphDir(t)

	cb := CircuitBreakerState{
		State:                 "CLOSED",
		ConsecutiveNoProgress: 0,
		ConsecutiveSameError:  0,
		TotalOpens:            2,
		Reason:                "",
		CurrentLoop:           10,
	}
	writeJSON(t, filepath.Join(dir, ".ralph", ".circuit_breaker_state"), cb)

	loaded, err := LoadCircuitBreaker(dir)
	if err != nil {
		t.Fatalf("LoadCircuitBreaker: %v", err)
	}

	if loaded.State != "CLOSED" {
		t.Errorf("State = %q, want %q", loaded.State, "CLOSED")
	}
	if loaded.TotalOpens != 2 {
		t.Errorf("TotalOpens = %d, want 2", loaded.TotalOpens)
	}
	if loaded.CurrentLoop != 10 {
		t.Errorf("CurrentLoop = %d, want 10", loaded.CurrentLoop)
	}
}

func TestLoadCircuitBreaker_OpenState(t *testing.T) {
	dir := setupRalphDir(t)
	now := time.Now().Truncate(time.Second)

	cb := CircuitBreakerState{
		State:                        "OPEN",
		ConsecutiveNoProgress:        5,
		ConsecutiveSameError:         3,
		ConsecutivePermissionDenials: 1,
		TotalOpens:                   4,
		Reason:                       "too many errors",
		OpenedAt:                     &now,
	}
	writeJSON(t, filepath.Join(dir, ".ralph", ".circuit_breaker_state"), cb)

	loaded, err := LoadCircuitBreaker(dir)
	if err != nil {
		t.Fatalf("LoadCircuitBreaker: %v", err)
	}

	if loaded.State != "OPEN" {
		t.Errorf("State = %q, want %q", loaded.State, "OPEN")
	}
	if loaded.Reason != "too many errors" {
		t.Errorf("Reason = %q, want %q", loaded.Reason, "too many errors")
	}
	if loaded.OpenedAt == nil {
		t.Fatal("OpenedAt should not be nil")
	}
}

func TestLoadCircuitBreaker_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadCircuitBreaker(dir)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadProgress_Valid(t *testing.T) {
	dir := setupRalphDir(t)

	prog := Progress{
		SpecFile:     "spec.md",
		Iteration:    5,
		CompletedIDs: []string{"task-1", "task-2"},
		Status:       "in_progress",
		StartedAt:    time.Now().Add(-1 * time.Hour).Truncate(time.Second),
		UpdatedAt:    time.Now().Truncate(time.Second),
		Log: []IterationLog{
			{Iteration: 1, TaskID: "task-1", Result: "completed", Timestamp: time.Now().Truncate(time.Second)},
		},
	}
	writeJSON(t, filepath.Join(dir, ".ralph", "progress.json"), prog)

	loaded, err := LoadProgress(dir)
	if err != nil {
		t.Fatalf("LoadProgress: %v", err)
	}

	if loaded.Iteration != 5 {
		t.Errorf("Iteration = %d, want 5", loaded.Iteration)
	}
	if loaded.Status != "in_progress" {
		t.Errorf("Status = %q, want %q", loaded.Status, "in_progress")
	}
	if len(loaded.CompletedIDs) != 2 {
		t.Errorf("CompletedIDs len = %d, want 2", len(loaded.CompletedIDs))
	}
	if len(loaded.Log) != 1 {
		t.Errorf("Log len = %d, want 1", len(loaded.Log))
	}
}

func TestLoadProgress_MissingFile(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadProgress(dir)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadProgress_InvalidJSON(t *testing.T) {
	dir := setupRalphDir(t)
	if err := os.WriteFile(filepath.Join(dir, ".ralph", "progress.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadProgress(dir)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestRefreshRepo_AllFiles(t *testing.T) {
	dir := setupRalphDir(t)

	// Write status
	writeJSON(t, filepath.Join(dir, ".ralph", "status.json"), LoopStatus{
		LoopCount: 7,
		Status:    "running",
	})
	// Write circuit breaker
	writeJSON(t, filepath.Join(dir, ".ralph", ".circuit_breaker_state"), CircuitBreakerState{
		State: "CLOSED",
	})
	// Write progress
	writeJSON(t, filepath.Join(dir, ".ralph", "progress.json"), Progress{
		Iteration: 3,
		Status:    "in_progress",
	})
	// Write config
	if err := os.WriteFile(filepath.Join(dir, ".ralphrc"), []byte("MODEL=sonnet\n"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Repo{Path: dir}
	RefreshRepo(r)

	if r.Status == nil {
		t.Fatal("Status should not be nil after refresh")
	}
	if r.Status.LoopCount != 7 {
		t.Errorf("Status.LoopCount = %d, want 7", r.Status.LoopCount)
	}
	if r.Circuit == nil {
		t.Fatal("Circuit should not be nil after refresh")
	}
	if r.Circuit.State != "CLOSED" {
		t.Errorf("Circuit.State = %q, want %q", r.Circuit.State, "CLOSED")
	}
	if r.Progress == nil {
		t.Fatal("Progress should not be nil after refresh")
	}
	if r.Config == nil {
		t.Fatal("Config should not be nil after refresh")
	}
}

func TestRefreshRepo_CorruptStatus_ReturnsError(t *testing.T) {
	dir := setupRalphDir(t)

	// Write corrupt status.json
	if err := os.WriteFile(filepath.Join(dir, ".ralph", "status.json"), []byte("{bad json"), 0644); err != nil {
		t.Fatal(err)
	}
	// Write corrupt progress.json
	if err := os.WriteFile(filepath.Join(dir, ".ralph", "progress.json"), []byte("nope"), 0644); err != nil {
		t.Fatal(err)
	}

	r := &Repo{Path: dir}
	errs := RefreshRepo(r)

	if len(errs) == 0 {
		t.Fatal("expected errors for corrupt files, got none")
	}
	if r.Status != nil {
		t.Error("Status should be nil when status.json is corrupt")
	}
	if r.Progress != nil {
		t.Error("Progress should be nil when progress.json is corrupt")
	}
	// RefreshErrors should be stored on the repo
	if len(r.RefreshErrors) != len(errs) {
		t.Errorf("RefreshErrors len = %d, want %d", len(r.RefreshErrors), len(errs))
	}
}

func TestRefreshRepo_NoFiles(t *testing.T) {
	dir := t.TempDir()
	r := &Repo{Path: dir}
	errs := RefreshRepo(r)

	// Missing files are not errors — should not panic, all fields remain nil
	if len(errs) != 0 {
		t.Errorf("expected no errors for missing files, got %d: %v", len(errs), errs)
	}
	if r.Status != nil {
		t.Error("Status should be nil when no files exist")
	}
	if r.Circuit != nil {
		t.Error("Circuit should be nil when no files exist")
	}
	if r.Progress != nil {
		t.Error("Progress should be nil when no files exist")
	}
	if r.Config != nil {
		t.Error("Config should be nil when no files exist")
	}
}
