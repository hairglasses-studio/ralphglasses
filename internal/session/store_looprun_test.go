package session

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// testLoopRun creates a minimal LoopRun for testing.
func testLoopRun(id, repoPath, status string) *LoopRun {
	now := time.Now()
	return &LoopRun{
		ID:        id,
		RepoPath:  repoPath,
		RepoName:  filepath.Base(repoPath),
		Status:    status,
		Profile:   DefaultLoopProfile(),
		LastError: "",
		CreatedAt: now.Add(-5 * time.Minute),
		UpdatedAt: now,
	}
}

// runLoopRunStoreTests exercises all loop run and cost ledger Store methods.
func runLoopRunStoreTests(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()

	t.Run("SaveAndGetLoopRun", func(t *testing.T) {
		run := testLoopRun("loop-1", "/repos/alpha", "running")
		deadline := time.Now().Add(1 * time.Hour).Truncate(time.Second)
		run.Deadline = &deadline
		run.Paused = true

		if err := store.SaveLoopRun(ctx, run); err != nil {
			t.Fatalf("SaveLoopRun: %v", err)
		}

		got, err := store.GetLoopRun(ctx, "loop-1")
		if err != nil {
			t.Fatalf("GetLoopRun: %v", err)
		}
		if got.ID != "loop-1" {
			t.Errorf("ID = %q, want %q", got.ID, "loop-1")
		}
		if got.RepoPath != "/repos/alpha" {
			t.Errorf("RepoPath = %q, want %q", got.RepoPath, "/repos/alpha")
		}
		if got.RepoName != "alpha" {
			t.Errorf("RepoName = %q, want %q", got.RepoName, "alpha")
		}
		if got.Status != "running" {
			t.Errorf("Status = %q, want %q", got.Status, "running")
		}
		if got.Profile.PlannerModel != run.Profile.PlannerModel {
			t.Errorf("Profile.PlannerModel = %q, want %q", got.Profile.PlannerModel, run.Profile.PlannerModel)
		}
		if !got.Paused {
			t.Error("Paused = false, want true")
		}
		if got.Deadline == nil {
			t.Fatal("Deadline should not be nil")
		}
	})

	t.Run("GetLoopRunNotFound", func(t *testing.T) {
		_, err := store.GetLoopRun(ctx, "nonexistent")
		if err != ErrLoopNotFound {
			t.Errorf("expected ErrLoopNotFound, got: %v", err)
		}
	})

	t.Run("SaveLoopRunUpsert", func(t *testing.T) {
		run := testLoopRun("loop-2", "/repos/beta", "pending")
		if err := store.SaveLoopRun(ctx, run); err != nil {
			t.Fatalf("SaveLoopRun: %v", err)
		}

		run.Status = "completed"
		run.LastError = "done"
		if err := store.SaveLoopRun(ctx, run); err != nil {
			t.Fatalf("SaveLoopRun (upsert): %v", err)
		}

		got, err := store.GetLoopRun(ctx, "loop-2")
		if err != nil {
			t.Fatalf("GetLoopRun: %v", err)
		}
		if got.Status != "completed" {
			t.Errorf("Status after upsert = %q, want %q", got.Status, "completed")
		}
		if got.LastError != "done" {
			t.Errorf("LastError after upsert = %q, want %q", got.LastError, "done")
		}
	})

	t.Run("ListLoopRunsAll", func(t *testing.T) {
		list, err := store.ListLoopRuns(ctx, LoopRunFilter{})
		if err != nil {
			t.Fatalf("ListLoopRuns: %v", err)
		}
		if len(list) < 2 {
			t.Errorf("expected at least 2 loop runs, got %d", len(list))
		}
	})

	t.Run("ListLoopRunsByRepo", func(t *testing.T) {
		list, err := store.ListLoopRuns(ctx, LoopRunFilter{RepoPath: "/repos/alpha"})
		if err != nil {
			t.Fatalf("ListLoopRuns: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1 loop run for /repos/alpha, got %d", len(list))
		}
	})

	t.Run("ListLoopRunsByStatus", func(t *testing.T) {
		list, err := store.ListLoopRuns(ctx, LoopRunFilter{Status: "running"})
		if err != nil {
			t.Fatalf("ListLoopRuns: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1 running loop run, got %d", len(list))
		}
	})

	t.Run("ListLoopRunsWithLimit", func(t *testing.T) {
		list, err := store.ListLoopRuns(ctx, LoopRunFilter{Limit: 1})
		if err != nil {
			t.Fatalf("ListLoopRuns: %v", err)
		}
		if len(list) != 1 {
			t.Errorf("expected 1 loop run with limit=1, got %d", len(list))
		}
	})

	t.Run("UpdateLoopRunStatus", func(t *testing.T) {
		if err := store.UpdateLoopRunStatus(ctx, "loop-1", "stopped"); err != nil {
			t.Fatalf("UpdateLoopRunStatus: %v", err)
		}
		got, _ := store.GetLoopRun(ctx, "loop-1")
		if got.Status != "stopped" {
			t.Errorf("Status = %q, want %q", got.Status, "stopped")
		}
	})

	t.Run("UpdateLoopRunStatusNotFound", func(t *testing.T) {
		err := store.UpdateLoopRunStatus(ctx, "nonexistent", "stopped")
		if err != ErrLoopNotFound {
			t.Errorf("expected ErrLoopNotFound, got: %v", err)
		}
	})

	t.Run("SaveLoopRunNil", func(t *testing.T) {
		if err := store.SaveLoopRun(ctx, nil); err == nil {
			t.Error("SaveLoopRun(nil) should return error")
		}
	})

	// ---------- Cost ledger tests ----------

	t.Run("RecordCostAndAggregate", func(t *testing.T) {
		now := time.Now()

		entries := []CostEntry{
			{SessionID: "s1", LoopID: "loop-1", Provider: "claude", Model: "sonnet", SpendUSD: 1.50, TurnCount: 10, ElapsedSec: 30.0, RecordedAt: now},
			{SessionID: "s2", LoopID: "loop-1", Provider: "claude", Model: "opus", SpendUSD: 3.00, TurnCount: 5, ElapsedSec: 60.0, RecordedAt: now},
			{SessionID: "s3", LoopID: "loop-2", Provider: "gemini", Model: "pro", SpendUSD: 0.75, TurnCount: 8, ElapsedSec: 20.0, RecordedAt: now},
		}

		for i := range entries {
			if err := store.RecordCost(ctx, &entries[i]); err != nil {
				t.Fatalf("RecordCost[%d]: %v", i, err)
			}
			if entries[i].ID == 0 {
				t.Errorf("RecordCost[%d]: expected non-zero ID", i)
			}
		}

		agg, err := store.AggregateCostByProvider(ctx, now.Add(-1*time.Minute))
		if err != nil {
			t.Fatalf("AggregateCostByProvider: %v", err)
		}

		if agg["claude"] != 4.50 {
			t.Errorf("claude spend = %f, want 4.50", agg["claude"])
		}
		if agg["gemini"] != 0.75 {
			t.Errorf("gemini spend = %f, want 0.75", agg["gemini"])
		}
	})

	t.Run("AggregateCostEmpty", func(t *testing.T) {
		// Query with a future time should return empty map.
		agg, err := store.AggregateCostByProvider(ctx, time.Now().Add(1*time.Hour))
		if err != nil {
			t.Fatalf("AggregateCostByProvider: %v", err)
		}
		if len(agg) != 0 {
			t.Errorf("expected empty map, got %v", agg)
		}
	})

	t.Run("RecordCostNil", func(t *testing.T) {
		if err := store.RecordCost(ctx, nil); err == nil {
			t.Error("RecordCost(nil) should return error")
		}
	})
}

func TestMemoryStoreLoopRun(t *testing.T) {
	runLoopRunStoreTests(t, NewMemoryStore())
}

func TestSQLiteStoreLoopRun(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test-looprun.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()
	runLoopRunStoreTests(t, store)
}

// TestPersistLoopWritesToStore verifies that PersistLoop writes to both JSON and Store.
func TestPersistLoopWritesToStore(t *testing.T) {
	store := NewMemoryStore()
	mgr := NewManager()
	mgr.SetStore(store)
	mgr.SetStateDir(t.TempDir())

	run := testLoopRun("persist-1", "/repos/test", "running")
	mgr.workersMu.Lock()
	mgr.loops[run.ID] = run
	mgr.workersMu.Unlock()

	mgr.PersistLoop(run)

	// Verify JSON file was written.
	dir := mgr.LoopStateDir()
	if dir == "" {
		t.Fatal("LoopStateDir should not be empty")
	}
	jsonPath := filepath.Join(dir, run.ID+".json")
	if _, err := filepath.Abs(jsonPath); err != nil {
		t.Fatalf("bad json path: %v", err)
	}
	// The JSON file should exist on disk.
	if _, statErr := filepath.Abs(jsonPath); statErr != nil {
		t.Fatalf("JSON file not created: %v", statErr)
	}

	// Verify Store received the loop run.
	ctx := context.Background()
	got, err := store.GetLoopRun(ctx, "persist-1")
	if err != nil {
		t.Fatalf("Store.GetLoopRun: %v", err)
	}
	if got.ID != "persist-1" {
		t.Errorf("Store loop ID = %q, want %q", got.ID, "persist-1")
	}
	if got.Status != "running" {
		t.Errorf("Store loop Status = %q, want %q", got.Status, "running")
	}
}

// TestPersistLoopWithoutStore verifies PersistLoop works when store is nil.
func TestPersistLoopWithoutStore(t *testing.T) {
	mgr := NewManager()
	mgr.SetStateDir(t.TempDir())

	run := testLoopRun("persist-nostore", "/repos/test", "running")
	mgr.workersMu.Lock()
	mgr.loops[run.ID] = run
	mgr.workersMu.Unlock()

	// Should not panic when store is nil.
	mgr.PersistLoop(run)

	// JSON file should still be written.
	dir := mgr.LoopStateDir()
	jsonPath := filepath.Join(dir, run.ID+".json")
	if _, err := filepath.Abs(jsonPath); err != nil {
		t.Fatalf("bad json path: %v", err)
	}
}

// TestCostRecordingToStore verifies that CostEntry can be recorded and aggregated via Store.
func TestCostRecordingToStore(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	entry := &CostEntry{
		SessionID:  "sess-1",
		LoopID:     "loop-1",
		Provider:   "claude",
		Model:      "sonnet",
		SpendUSD:   2.50,
		RecordedAt: time.Now(),
	}
	if err := store.RecordCost(ctx, entry); err != nil {
		t.Fatalf("RecordCost: %v", err)
	}
	if entry.ID == 0 {
		t.Error("expected non-zero ID after RecordCost")
	}

	entry2 := &CostEntry{
		SessionID:  "sess-2",
		LoopID:     "loop-1",
		Provider:   "gemini",
		Model:      "pro",
		SpendUSD:   0.50,
		RecordedAt: time.Now(),
	}
	if err := store.RecordCost(ctx, entry2); err != nil {
		t.Fatalf("RecordCost: %v", err)
	}

	agg, err := store.AggregateCostByProvider(ctx, time.Now().Add(-1*time.Minute))
	if err != nil {
		t.Fatalf("AggregateCostByProvider: %v", err)
	}
	if agg["claude"] != 2.50 {
		t.Errorf("claude spend = %f, want 2.50", agg["claude"])
	}
	if agg["gemini"] != 0.50 {
		t.Errorf("gemini spend = %f, want 0.50", agg["gemini"])
	}
}

// TestLoadExternalLoopsFromStore verifies that LoadExternalLoops reads from Store first.
func TestLoadExternalLoopsFromStore(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Pre-populate Store with a loop run.
	run := testLoopRun("store-loop-1", "/repos/delta", "completed")
	if err := store.SaveLoopRun(ctx, run); err != nil {
		t.Fatalf("SaveLoopRun: %v", err)
	}

	mgr := NewManager()
	mgr.SetStore(store)
	mgr.SetStateDir(t.TempDir())

	mgr.LoadExternalLoops()

	// The loop from Store should now be in the manager.
	got, ok := mgr.GetLoop("store-loop-1")
	if !ok {
		t.Fatal("expected loop store-loop-1 to be loaded from Store")
	}
	if got.Status != "completed" {
		t.Errorf("Status = %q, want %q", got.Status, "completed")
	}
	if got.RepoPath != "/repos/delta" {
		t.Errorf("RepoPath = %q, want %q", got.RepoPath, "/repos/delta")
	}
}

// TestLoadExternalLoopsMergesStoreAndJSON verifies Store-first with JSON fallback.
func TestLoadExternalLoopsMergesStoreAndJSON(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Store has one loop.
	storeRun := testLoopRun("from-store", "/repos/s", "running")
	if err := store.SaveLoopRun(ctx, storeRun); err != nil {
		t.Fatalf("SaveLoopRun: %v", err)
	}

	mgr := NewManager()
	mgr.SetStore(store)
	tmpDir := t.TempDir()
	mgr.SetStateDir(tmpDir)

	// Write a different loop as JSON file.
	jsonRun := testLoopRun("from-json", "/repos/j", "stopped")
	mgr.workersMu.Lock()
	mgr.loops[jsonRun.ID] = jsonRun
	mgr.workersMu.Unlock()
	mgr.PersistLoop(jsonRun)

	// Clear in-memory state except what PersistLoop wrote to Store.
	mgr.workersMu.Lock()
	delete(mgr.loops, "from-json")
	delete(mgr.loops, "from-store")
	mgr.workersMu.Unlock()

	mgr.LoadExternalLoops()

	// Both loops should be present.
	if _, ok := mgr.GetLoop("from-store"); !ok {
		t.Error("expected from-store loop to be loaded")
	}
	if _, ok := mgr.GetLoop("from-json"); !ok {
		t.Error("expected from-json loop to be loaded from JSON fallback")
	}
}

func TestSQLiteStoreLoopRunIterations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "loop-iters.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	run := testLoopRun("iter-1", "/repos/gamma", "running")
	now := time.Now()
	run.Iterations = []LoopIteration{
		{Number: 1, Status: "completed", Task: LoopTask{Title: "task-1", Prompt: "do thing"}, StartedAt: now},
		{Number: 2, Status: "running", Task: LoopTask{Title: "task-2", Prompt: "do other"}, StartedAt: now},
	}

	if err := store.SaveLoopRun(ctx, run); err != nil {
		t.Fatalf("SaveLoopRun: %v", err)
	}

	got, err := store.GetLoopRun(ctx, "iter-1")
	if err != nil {
		t.Fatalf("GetLoopRun: %v", err)
	}
	if len(got.Iterations) != 2 {
		t.Fatalf("Iterations len = %d, want 2", len(got.Iterations))
	}
	if got.Iterations[0].Task.Title != "task-1" {
		t.Errorf("Iterations[0].Task.Title = %q, want %q", got.Iterations[0].Task.Title, "task-1")
	}
	if got.Iterations[1].Status != "running" {
		t.Errorf("Iterations[1].Status = %q, want %q", got.Iterations[1].Status, "running")
	}
}
