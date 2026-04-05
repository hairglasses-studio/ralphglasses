package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func writeLoopRunFile(t *testing.T, dir, id, status string, updatedAt time.Time) {
	t.Helper()
	// Use the lightweight prune view to avoid copying the mutex in LoopRun.
	run := loopRunPruneView{
		ID:        id,
		RepoName:  "test-repo",
		Status:    status,
		CreatedAt: updatedAt.Add(-time.Hour),
		UpdatedAt: updatedAt,
	}
	data, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestPruneLoopRuns_AgeFilter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	now := time.Now()
	writeLoopRunFile(t, dir, "old-1", "pending", now.Add(-96*time.Hour))
	writeLoopRunFile(t, dir, "old-2", "pending", now.Add(-80*time.Hour))
	writeLoopRunFile(t, dir, "recent", "pending", now.Add(-1*time.Hour))

	pruned, err := PruneLoopRuns(dir, 72*time.Hour, []string{"pending"}, false)
	if err != nil {
		t.Fatalf("PruneLoopRuns: %v", err)
	}
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2", pruned)
	}

	// recent file should still exist
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("remaining files = %d, want 1", len(entries))
	}
	if entries[0].Name() != "recent.json" {
		t.Errorf("remaining file = %s, want recent.json", entries[0].Name())
	}
}

func TestPruneLoopRuns_StatusFilter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	old := time.Now().Add(-96 * time.Hour)
	writeLoopRunFile(t, dir, "pending-1", "pending", old)
	writeLoopRunFile(t, dir, "failed-1", "failed", old)
	writeLoopRunFile(t, dir, "running-1", "running", old)
	writeLoopRunFile(t, dir, "completed-1", "completed", old)

	pruned, err := PruneLoopRuns(dir, 72*time.Hour, []string{"pending", "failed"}, false)
	if err != nil {
		t.Fatalf("PruneLoopRuns: %v", err)
	}
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2", pruned)
	}

	// running and completed should remain
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("remaining files = %d, want 2", len(entries))
	}
}

func TestPruneLoopRuns_DryRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	old := time.Now().Add(-96 * time.Hour)
	writeLoopRunFile(t, dir, "old-1", "pending", old)
	writeLoopRunFile(t, dir, "old-2", "failed", old)

	pruned, err := PruneLoopRuns(dir, 72*time.Hour, []string{"pending", "failed"}, true)
	if err != nil {
		t.Fatalf("PruneLoopRuns: %v", err)
	}
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2", pruned)
	}

	// Files should still exist in dry-run mode
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("dry-run should not delete files, got %d remaining", len(entries))
	}
}

func TestPruneLoopRuns_EmptyDirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	pruned, err := PruneLoopRuns(dir, 72*time.Hour, []string{"pending"}, false)
	if err != nil {
		t.Fatalf("PruneLoopRuns: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0", pruned)
	}
}

func TestPruneLoopRuns_NonexistentDirectory(t *testing.T) {
	t.Parallel()
	pruned, err := PruneLoopRuns("/nonexistent/path", 72*time.Hour, []string{"pending"}, false)
	if err != nil {
		t.Fatalf("PruneLoopRuns: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0", pruned)
	}
}

func TestPruneLoopRuns_EmptyPath(t *testing.T) {
	t.Parallel()
	pruned, err := PruneLoopRuns("", 72*time.Hour, []string{"pending"}, false)
	if err != nil {
		t.Fatalf("PruneLoopRuns: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0", pruned)
	}
}

func TestAutoPruneLoopRuns(t *testing.T) {
	t.Parallel()

	m := NewManager()
	dir := t.TempDir()
	m.SetStateDir(dir)

	loopDir := m.LoopStateDir()
	if err := os.MkdirAll(loopDir, 0755); err != nil {
		t.Fatal(err)
	}

	now := time.Now()

	// Create 5 stale loop run files (old, prunable statuses).
	writeLoopRunFile(t, loopDir, "stale-pending-1", "pending", now.Add(-10*24*time.Hour))
	writeLoopRunFile(t, loopDir, "stale-pending-2", "pending", now.Add(-9*24*time.Hour))
	writeLoopRunFile(t, loopDir, "stale-pending-3", "pending", now.Add(-8*24*time.Hour))
	writeLoopRunFile(t, loopDir, "stale-failed-1", "failed", now.Add(-15*24*time.Hour))
	writeLoopRunFile(t, loopDir, "stale-failed-2", "failed", now.Add(-20*24*time.Hour))

	// Create 2 fresh loop run files that should survive (running status, or recent).
	writeLoopRunFile(t, loopDir, "fresh-running", "running", now.Add(-10*24*time.Hour))
	writeLoopRunFile(t, loopDir, "fresh-pending", "pending", now.Add(-1*time.Hour))

	// Use the default 7-day retention (PruneRetention == 0 → 7 days).
	// All 5 stale files are >7 days old with pending/failed status.
	// fresh-running is old but "running" status (not prunable).
	// fresh-pending is <7 days old (not prunable by age).

	// Call autoPruneLoopRuns directly (Init runs it in a goroutine).
	m.autoPruneLoopRuns()

	if got := m.TotalPrunedThisSession(); got != 5 {
		t.Errorf("TotalPrunedThisSession() = %d, want 5", got)
	}

	entries, err := os.ReadDir(loopDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("remaining files = %d, want 2", len(entries))
		for _, e := range entries {
			t.Logf("  remaining: %s", e.Name())
		}
	}

	// Verify the correct files survived.
	remaining := make(map[string]bool)
	for _, e := range entries {
		remaining[e.Name()] = true
	}
	if !remaining["fresh-running.json"] {
		t.Error("fresh-running.json was incorrectly pruned")
	}
	if !remaining["fresh-pending.json"] {
		t.Error("fresh-pending.json was incorrectly pruned")
	}
}

func TestAutoPruneLoopRuns_CustomRetention(t *testing.T) {
	t.Parallel()

	m := NewManager()
	dir := t.TempDir()
	m.SetStateDir(dir)

	loopDir := m.LoopStateDir()
	if err := os.MkdirAll(loopDir, 0755); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	// Files are 3 days old — would survive default 7-day retention.
	writeLoopRunFile(t, loopDir, "pending-3d", "pending", now.Add(-3*24*time.Hour))
	writeLoopRunFile(t, loopDir, "failed-3d", "failed", now.Add(-3*24*time.Hour))

	// Set a short 1-day retention — both should be pruned.
	m.PruneRetention = 1 * 24 * time.Hour
	m.autoPruneLoopRuns()

	if got := m.TotalPrunedThisSession(); got != 2 {
		t.Errorf("TotalPrunedThisSession() = %d, want 2", got)
	}

	entries, _ := os.ReadDir(loopDir)
	if len(entries) != 0 {
		t.Errorf("remaining files = %d, want 0", len(entries))
	}
}

func TestAutoPruneLoopRuns_EmptyLoopDir(t *testing.T) {
	t.Parallel()

	m := NewManager()
	dir := t.TempDir()
	m.SetStateDir(dir)

	// Don't create the loops subdirectory — autoPrune should handle gracefully.
	m.autoPruneLoopRuns()

	if got := m.TotalPrunedThisSession(); got != 0 {
		t.Errorf("TotalPrunedThisSession() = %d, want 0", got)
	}
}

func TestPruneLoopRuns_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	old := time.Now().Add(-96 * time.Hour)
	for i := range 20 {
		id := "loop-" + string(rune('a'+i))
		writeLoopRunFile(t, dir, id, "pending", old)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 5)

	// Run concurrent dry-run prunes — these should all succeed without data races.
	for range 5 {
		wg.Go(func() {
			_, err := PruneLoopRuns(dir, 72*time.Hour, []string{"pending"}, true)
			if err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent PruneLoopRuns error: %v", err)
	}
}
