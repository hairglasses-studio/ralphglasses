package session

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestLoopVelocity(t *testing.T) {
	now := time.Now()
	observations := []LoopObservation{
		{Timestamp: now.Add(-30 * time.Minute), VerifyPassed: true, FilesChanged: 3},
		{Timestamp: now.Add(-45 * time.Minute), VerifyPassed: true, FilesChanged: 0}, // no files = not useful
		{Timestamp: now.Add(-50 * time.Minute), VerifyPassed: false, FilesChanged: 2}, // failed = not useful
		{Timestamp: now.Add(-55 * time.Minute), VerifyPassed: true, FilesChanged: 1},
		{Timestamp: now.Add(-25 * time.Hour), VerifyPassed: true, FilesChanged: 5}, // outside window
	}

	v := LoopVelocity(observations, 1.0)
	// 2 useful iterations in 1 hour = 2.0
	if v != 2.0 {
		t.Errorf("LoopVelocity = %f, want 2.0", v)
	}
}

func TestLoopVelocityZeroWindow(t *testing.T) {
	v := LoopVelocity(nil, 0)
	if v != 0 {
		t.Errorf("LoopVelocity(nil, 0) = %f, want 0", v)
	}
}

func TestAggregateObservations(t *testing.T) {
	now := time.Now()
	observations := []LoopObservation{
		// Current window (last 1 hour)
		{Timestamp: now.Add(-10 * time.Minute), Status: "idle", TotalCostUSD: 0.20, VerifyPassed: true, FilesChanged: 3, PlannerProvider: "claude", WorkerProvider: "gemini", PlannerCostUSD: 0.05, WorkerCostUSD: 0.15},
		{Timestamp: now.Add(-20 * time.Minute), Status: "idle", TotalCostUSD: 0.30, VerifyPassed: true, FilesChanged: 2, PlannerProvider: "claude", WorkerProvider: "claude", PlannerCostUSD: 0.10, WorkerCostUSD: 0.20},
		{Timestamp: now.Add(-30 * time.Minute), Status: "failed", TotalCostUSD: 0.10, VerifyPassed: false, FilesChanged: 0, PlannerProvider: "claude", WorkerProvider: "codex", PlannerCostUSD: 0.05, WorkerCostUSD: 0.05},
		// Previous window (1-2 hours ago)
		{Timestamp: now.Add(-90 * time.Minute), Status: "idle", TotalCostUSD: 0.50, VerifyPassed: true, FilesChanged: 5, PlannerProvider: "claude", WorkerProvider: "claude", PlannerCostUSD: 0.15, WorkerCostUSD: 0.35},
	}

	summary := AggregateObservations(observations, 1.0)

	if summary.TotalIterations != 3 {
		t.Errorf("TotalIterations = %d, want 3", summary.TotalIterations)
	}

	// 2 out of 3 completed
	wantRate := 2.0 / 3.0
	if diff := summary.CompletionRate - wantRate; diff > 0.01 || diff < -0.01 {
		t.Errorf("CompletionRate = %f, want ~%f", summary.CompletionRate, wantRate)
	}

	// Avg cost: (0.20 + 0.30 + 0.10) / 3 = 0.20
	if diff := summary.AvgCostPerIter - 0.20; diff > 0.01 || diff < -0.01 {
		t.Errorf("AvgCostPerIter = %f, want ~0.20", summary.AvgCostPerIter)
	}

	// Cost trend: current avg 0.20 vs previous avg 0.50 -> ratio 0.40 -> decreasing
	if summary.CostTrend != "decreasing" {
		t.Errorf("CostTrend = %q, want decreasing", summary.CostTrend)
	}

	// Cost by provider
	if summary.CostByProvider["claude"] == 0 {
		t.Error("expected non-zero claude cost")
	}
	if summary.CostByProvider["gemini"] == 0 {
		t.Error("expected non-zero gemini cost")
	}
}

func TestAggregateObservationsEmpty(t *testing.T) {
	summary := AggregateObservations(nil, 1.0)
	if summary.TotalIterations != 0 {
		t.Errorf("TotalIterations = %d, want 0", summary.TotalIterations)
	}
	if summary.CostByProvider == nil {
		t.Error("CostByProvider should not be nil")
	}
}

func TestBuildDiffSummary(t *testing.T) {
	tests := []struct {
		name  string
		paths []string
		want  string
	}{
		{"empty", nil, ""},
		{"one file", []string{"a.go"}, "1 files: a.go"},
		{"three files", []string{"a.go", "b.go", "c.go"}, "3 files: a.go, b.go, c.go"},
		{"five files", []string{"a.go", "b.go", "c.go", "d.go", "e.go"}, "5 files: a.go, b.go, c.go, +2 more"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildDiffSummary(tt.paths)
			if got != tt.want {
				t.Errorf("buildDiffSummary(%v) = %q, want %q", tt.paths, got, tt.want)
			}
		})
	}
}

func TestLoopObservationDiffPathsJSON(t *testing.T) {
	// Verify omitempty works — empty DiffPaths should not appear in JSON.
	obs := LoopObservation{LoopID: "test"}
	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "diff_paths") {
		t.Error("empty DiffPaths should be omitted from JSON")
	}
	if strings.Contains(string(data), "diff_summary") {
		t.Error("empty DiffSummary should be omitted from JSON")
	}

	// With data — should round-trip.
	obs.DiffPaths = []string{"foo.go", "bar.go"}
	obs.DiffSummary = "2 files: foo.go, bar.go"
	data, err = json.Marshal(obs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"diff_paths"`) {
		t.Error("populated DiffPaths should appear in JSON")
	}

	var decoded LoopObservation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.DiffPaths) != 2 {
		t.Errorf("got %d diff paths, want 2", len(decoded.DiffPaths))
	}
	if decoded.DiffSummary != obs.DiffSummary {
		t.Errorf("DiffSummary = %q, want %q", decoded.DiffSummary, obs.DiffSummary)
	}
}
