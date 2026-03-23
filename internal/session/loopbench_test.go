package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAndLoadObservations(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "obs.jsonl")

	now := time.Now().UTC().Truncate(time.Millisecond)
	obs1 := LoopObservation{
		Timestamp:       now,
		LoopID:          "loop-1",
		RepoName:        "test-repo",
		IterationNumber: 1,
		TotalLatencyMs:  1500,
		PlannerCostUSD:  0.50,
		WorkerCostUSD:   1.00,
		TotalCostUSD:    1.50,
		PlannerProvider: "claude",
		WorkerProvider:  "codex",
		Status:          "idle",
		VerifyPassed:    true,
		WorkerCount:     2,
		FilesChanged:    3,
		LinesAdded:      42,
		LinesRemoved:    10,
		TaskType:        "refactor",
		TaskTitle:       "Refactor config parser",
		Mode:            "mock",
	}
	obs2 := LoopObservation{
		Timestamp:       now.Add(time.Hour),
		LoopID:          "loop-1",
		RepoName:        "test-repo",
		IterationNumber: 2,
		TotalLatencyMs:  3000,
		PlannerCostUSD:  0.75,
		WorkerCostUSD:   2.00,
		TotalCostUSD:    2.75,
		PlannerProvider: "claude",
		WorkerProvider:  "codex",
		Status:          "failed",
		VerifyPassed:    false,
		WorkerCount:     1,
		Error:           "verify failed: exit 1",
		TaskType:        "bug_fix",
		TaskTitle:       "Fix login bug",
		Mode:            "mock",
	}

	if err := WriteObservation(path, obs1); err != nil {
		t.Fatalf("WriteObservation(1): %v", err)
	}
	if err := WriteObservation(path, obs2); err != nil {
		t.Fatalf("WriteObservation(2): %v", err)
	}

	// Load all
	all, err := LoadObservations(path, time.Time{})
	if err != nil {
		t.Fatalf("LoadObservations(all): %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 observations, got %d", len(all))
	}
	if all[0].LoopID != "loop-1" || all[0].IterationNumber != 1 {
		t.Errorf("obs1 mismatch: %+v", all[0])
	}
	if all[1].Status != "failed" {
		t.Errorf("obs2 status = %q, want failed", all[1].Status)
	}

	// Load with time filter
	filtered, err := LoadObservations(path, now.Add(30*time.Minute))
	if err != nil {
		t.Fatalf("LoadObservations(filtered): %v", err)
	}
	if len(filtered) != 1 {
		t.Fatalf("expected 1 filtered observation, got %d", len(filtered))
	}
	if filtered[0].IterationNumber != 2 {
		t.Errorf("filtered obs iteration = %d, want 2", filtered[0].IterationNumber)
	}
}

func TestLoadObservations_MissingFile(t *testing.T) {
	t.Parallel()
	obs, err := LoadObservations("/nonexistent/path.jsonl", time.Time{})
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if obs != nil {
		t.Errorf("expected nil slice, got %v", obs)
	}
}

func TestObservationPath(t *testing.T) {
	t.Parallel()
	got := ObservationPath("/home/user/repo")
	want := filepath.Join("/home/user/repo", ".ralph", "logs", "loop_observations.jsonl")
	if got != want {
		t.Errorf("ObservationPath = %q, want %q", got, want)
	}
}

func TestGitDiffStats_NoGit(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files, added, removed := gitDiffStats(dir)
	if files != 0 || added != 0 || removed != 0 {
		t.Errorf("non-git dir: files=%d added=%d removed=%d", files, added, removed)
	}
}

func TestGitDiffStats_WithChanges(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	gitRun("init")
	gitRun("config", "user.email", "test@test.com")
	gitRun("config", "user.name", "Test")
	gitRun("config", "commit.gpgsign", "false")

	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun("add", ".")
	gitRun("commit", "-m", "initial")

	// Make changes
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line1\nmodified\nnew line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, added, removed := gitDiffStats(dir)
	if files != 1 {
		t.Errorf("files = %d, want 1", files)
	}
	if added < 1 || removed < 1 {
		t.Errorf("expected insertions and deletions, got added=%d removed=%d", added, removed)
	}
}
