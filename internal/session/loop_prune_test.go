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

func TestPruneLoopRuns_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	old := time.Now().Add(-96 * time.Hour)
	for i := 0; i < 20; i++ {
		id := "loop-" + string(rune('a'+i))
		writeLoopRunFile(t, dir, id, "pending", old)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 5)

	// Run concurrent dry-run prunes — these should all succeed without data races.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := PruneLoopRuns(dir, 72*time.Hour, []string{"pending"}, true)
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent PruneLoopRuns error: %v", err)
	}
}
