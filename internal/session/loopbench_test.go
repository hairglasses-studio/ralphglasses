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

func TestResolveMainRepoPath_NormalRepo(t *testing.T) {
	tmp := t.TempDir()
	// Init a normal git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmp
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	resolved, err := ResolveMainRepoPath(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should resolve to the same directory (EvalSymlinks for macOS /var -> /private/var)
	absTmp, _ := filepath.EvalSymlinks(tmp)
	absResolved, _ := filepath.EvalSymlinks(resolved)
	if absTmp != absResolved {
		t.Errorf("expected %s, got %s", absTmp, absResolved)
	}
}

func TestResolveMainRepoPath_Worktree(t *testing.T) {
	// Create main repo with initial commit
	main := t.TempDir()
	for _, args := range [][]string{
		{"init"},
		{"checkout", "-b", "main"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = main
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	// Need at least one commit for worktree
	os.WriteFile(filepath.Join(main, "file.txt"), []byte("x"), 0o644)
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = main
	cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "init")
	cmd.Dir = main
	cmd.Run()

	// Create worktree
	wt := filepath.Join(t.TempDir(), "wt")
	cmd = exec.Command("git", "worktree", "add", wt, "-b", "work")
	cmd.Dir = main
	if err := cmd.Run(); err != nil {
		t.Fatalf("worktree add: %v", err)
	}

	resolved, err := ResolveMainRepoPath(wt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	absMain, _ := filepath.EvalSymlinks(main)
	absResolved, _ := filepath.EvalSymlinks(resolved)
	if absMain != absResolved {
		t.Errorf("expected main repo %s, got %s", absMain, absResolved)
	}
}

func TestResolveMainRepoPath_NonGitDir(t *testing.T) {
	tmp := t.TempDir()
	resolved, err := ResolveMainRepoPath(tmp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolved != tmp {
		t.Errorf("expected %s, got %s", tmp, resolved)
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

func TestSummarizeObservationsEmpty(t *testing.T) {
	t.Parallel()
	s := SummarizeObservations(nil)
	if s.TotalIterations != 0 {
		t.Errorf("TotalIterations = %d, want 0", s.TotalIterations)
	}
	if s.CompletedCount != 0 {
		t.Errorf("CompletedCount = %d, want 0", s.CompletedCount)
	}
	if s.FailedCount != 0 {
		t.Errorf("FailedCount = %d, want 0", s.FailedCount)
	}
	if s.AvgDurationSec != 0 {
		t.Errorf("AvgDurationSec = %f, want 0", s.AvgDurationSec)
	}
	if s.AcceptanceCounts == nil {
		t.Error("AcceptanceCounts should not be nil")
	}
	if s.ModelUsage == nil {
		t.Error("ModelUsage should not be nil")
	}
}

func TestSummarizeObservationsMixed(t *testing.T) {
	t.Parallel()
	obs := []LoopObservation{
		{
			Status:           "idle",
			TotalLatencyMs:   10000,
			FilesChanged:     3,
			LinesAdded:       50,
			LinesRemoved:     10,
			PlannerModelUsed: "claude-opus-4-6",
			WorkerModelUsed:  "claude-sonnet-4-6",
			AcceptancePath:   "auto_merge",
			StallCount:       0,
		},
		{
			Status:           "failed",
			TotalLatencyMs:   5000,
			FilesChanged:     0,
			LinesAdded:       0,
			LinesRemoved:     0,
			PlannerModelUsed: "claude-opus-4-6",
			WorkerModelUsed:  "gemini-2.5-pro",
			AcceptancePath:   "rejected",
			StallCount:       2,
		},
		{
			Status:         "idle",
			TotalLatencyMs: 15000,
			GitDiffStat: &DiffStat{
				FilesChanged: 5,
				Insertions:   100,
				Deletions:    20,
			},
			PlannerModelUsed: "o1-pro",
			WorkerModelUsed:  "claude-sonnet-4-6",
			AcceptancePath:   "pr",
			StallCount:       1,
		},
	}

	s := SummarizeObservations(obs)

	if s.TotalIterations != 3 {
		t.Errorf("TotalIterations = %d, want 3", s.TotalIterations)
	}
	if s.CompletedCount != 2 {
		t.Errorf("CompletedCount = %d, want 2", s.CompletedCount)
	}
	if s.FailedCount != 1 {
		t.Errorf("FailedCount = %d, want 1", s.FailedCount)
	}
	if s.TotalStalls != 3 {
		t.Errorf("TotalStalls = %d, want 3", s.TotalStalls)
	}

	// AvgDurationSec: (10000+5000+15000)/3/1000 = 10.0
	if diff := s.AvgDurationSec - 10.0; diff > 0.01 || diff < -0.01 {
		t.Errorf("AvgDurationSec = %f, want 10.0", s.AvgDurationSec)
	}

	// obs[0]: flat fields 3+50+10, obs[1]: flat 0+0+0, obs[2]: DiffStat 5+100+20
	if s.TotalFilesChanged != 8 {
		t.Errorf("TotalFilesChanged = %d, want 8", s.TotalFilesChanged)
	}
	if s.TotalInsertions != 150 {
		t.Errorf("TotalInsertions = %d, want 150", s.TotalInsertions)
	}
	if s.TotalDeletions != 30 {
		t.Errorf("TotalDeletions = %d, want 30", s.TotalDeletions)
	}

	// Acceptance counts
	if s.AcceptanceCounts["auto_merge"] != 1 {
		t.Errorf("AcceptanceCounts[auto_merge] = %d, want 1", s.AcceptanceCounts["auto_merge"])
	}
	if s.AcceptanceCounts["pr"] != 1 {
		t.Errorf("AcceptanceCounts[pr] = %d, want 1", s.AcceptanceCounts["pr"])
	}
	if s.AcceptanceCounts["rejected"] != 1 {
		t.Errorf("AcceptanceCounts[rejected] = %d, want 1", s.AcceptanceCounts["rejected"])
	}

	// Model usage: claude-opus-4-6 x2 (planner), o1-pro x1, claude-sonnet-4-6 x2, gemini-2.5-pro x1
	if s.ModelUsage["claude-opus-4-6"] != 2 {
		t.Errorf("ModelUsage[claude-opus-4-6] = %d, want 2", s.ModelUsage["claude-opus-4-6"])
	}
	if s.ModelUsage["claude-sonnet-4-6"] != 2 {
		t.Errorf("ModelUsage[claude-sonnet-4-6] = %d, want 2", s.ModelUsage["claude-sonnet-4-6"])
	}
	if s.ModelUsage["o1-pro"] != 1 {
		t.Errorf("ModelUsage[o1-pro] = %d, want 1", s.ModelUsage["o1-pro"])
	}
	if s.ModelUsage["gemini-2.5-pro"] != 1 {
		t.Errorf("ModelUsage[gemini-2.5-pro] = %d, want 1", s.ModelUsage["gemini-2.5-pro"])
	}
}

func TestStallCountPopulatedInObservation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "stall_obs.jsonl")

	obs := LoopObservation{
		Timestamp:       time.Now().UTC().Truncate(time.Millisecond),
		LoopID:          "loop-stall",
		RepoName:        "test-repo",
		IterationNumber: 1,
		Status:          "idle",
		StallCount:      5,
	}

	if err := WriteObservation(path, obs); err != nil {
		t.Fatalf("WriteObservation: %v", err)
	}

	loaded, err := LoadObservations(path, time.Time{})
	if err != nil {
		t.Fatalf("LoadObservations: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(loaded))
	}
	if loaded[0].StallCount != 5 {
		t.Errorf("StallCount = %d, want 5", loaded[0].StallCount)
	}

	// Verify SummarizeObservations aggregates stalls correctly.
	obs2 := LoopObservation{
		LoopID:     "loop-stall",
		Status:     "failed",
		StallCount: 3,
	}
	summary := SummarizeObservations([]LoopObservation{obs, obs2})
	if summary.TotalStalls != 8 {
		t.Errorf("TotalStalls = %d, want 8", summary.TotalStalls)
	}

	// Zero stall count should be omitted from JSON (omitempty).
	zeroObs := LoopObservation{LoopID: "no-stall", StallCount: 0}
	data, err := json.Marshal(zeroObs)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "stall_count") {
		t.Error("zero StallCount should be omitted from JSON")
	}
}

func TestNewFieldsJSONRoundTrip(t *testing.T) {
	t.Parallel()

	obs := LoopObservation{
		LoopID:           "loop-rt",
		PlannerModelUsed: "claude-opus-4-6",
		WorkerModelUsed:  "claude-sonnet-4-6",
		AcceptancePath:   "auto_merge",
		StallCount:       3,
		GitDiffStat: &DiffStat{
			FilesChanged: 4,
			Insertions:   77,
			Deletions:    12,
		},
	}

	data, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded LoopObservation
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.PlannerModelUsed != "claude-opus-4-6" {
		t.Errorf("PlannerModelUsed = %q, want claude-opus-4-6", decoded.PlannerModelUsed)
	}
	if decoded.WorkerModelUsed != "claude-sonnet-4-6" {
		t.Errorf("WorkerModelUsed = %q, want claude-sonnet-4-6", decoded.WorkerModelUsed)
	}
	if decoded.AcceptancePath != "auto_merge" {
		t.Errorf("AcceptancePath = %q, want auto_merge", decoded.AcceptancePath)
	}
	if decoded.StallCount != 3 {
		t.Errorf("StallCount = %d, want 3", decoded.StallCount)
	}
	if decoded.GitDiffStat == nil {
		t.Fatal("GitDiffStat is nil after round-trip")
	}
	if decoded.GitDiffStat.FilesChanged != 4 {
		t.Errorf("GitDiffStat.FilesChanged = %d, want 4", decoded.GitDiffStat.FilesChanged)
	}
	if decoded.GitDiffStat.Insertions != 77 {
		t.Errorf("GitDiffStat.Insertions = %d, want 77", decoded.GitDiffStat.Insertions)
	}
	if decoded.GitDiffStat.Deletions != 12 {
		t.Errorf("GitDiffStat.Deletions = %d, want 12", decoded.GitDiffStat.Deletions)
	}

	// Omitempty: zero-valued new fields should not appear in JSON.
	obsEmpty := LoopObservation{LoopID: "empty"}
	emptyData, err := json.Marshal(obsEmpty)
	if err != nil {
		t.Fatalf("Marshal empty: %v", err)
	}
	emptyJSON := string(emptyData)
	for _, field := range []string{"git_diff_stat", "planner_model_used", "worker_model_used", "acceptance_path", "stall_count"} {
		if strings.Contains(emptyJSON, field) {
			t.Errorf("zero-valued %q should be omitted from JSON", field)
		}
	}
}

func TestPercentile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		sorted []float64
		p      float64
		want   float64
		tol    float64
	}{
		{"empty", nil, 50, 0, 0},
		{"single", []float64{5.0}, 50, 5.0, 0},
		{"single_p99", []float64{5.0}, 99, 5.0, 0},
		{"two_p50", []float64{1.0, 3.0}, 50, 2.0, 0.001},
		{"ten_p50", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 50, 5.5, 0.001},
		{"ten_p95", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 95, 9.55, 0.001},
		{"ten_p99", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 99, 9.91, 0.001},
		{"ten_p0", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 0, 1.0, 0},
		{"ten_p100", []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 100, 10.0, 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := percentile(tc.sorted, tc.p)
			diff := got - tc.want
			if diff < 0 {
				diff = -diff
			}
			if diff > tc.tol {
				t.Errorf("percentile(%v, %f) = %f, want %f (tol %f)", tc.sorted, tc.p, got, tc.want, tc.tol)
			}
		})
	}
}

func TestSummarizeObservationsPercentiles(t *testing.T) {
	t.Parallel()

	obs := make([]LoopObservation, 10)
	for i := range obs {
		obs[i] = LoopObservation{
			Status:         "idle",
			TotalLatencyMs: int64((i + 1) * 1000),
			TotalCostUSD:   float64(i+1) * 0.01,
		}
	}

	s := SummarizeObservations(obs)

	// p50 of 1..10 = 5.5
	if s.LatencyP50 < 5.4 || s.LatencyP50 > 5.6 {
		t.Errorf("LatencyP50 = %f, want ~5.5", s.LatencyP50)
	}
	if s.LatencyP95 < 9.5 || s.LatencyP95 > 9.6 {
		t.Errorf("LatencyP95 = %f, want ~9.55", s.LatencyP95)
	}
	if s.CostP50 < 0.054 || s.CostP50 > 0.056 {
		t.Errorf("CostP50 = %f, want ~0.055", s.CostP50)
	}
}

func TestWriteObservation_CreatesIntermediateDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "dir", "obs.jsonl")

	err := WriteObservation(path, LoopObservation{LoopID: "dir-test"})
	if err != nil {
		t.Fatalf("WriteObservation should create intermediate dirs: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("file not created at nested path: %v", err)
	}
}

func TestLoadObservations_MalformedLines(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "malformed.jsonl")

	f, _ := os.Create(path)
	_, _ = f.WriteString(`{"loop_id":"good1","ts":"2026-01-01T00:00:00Z"}` + "\n")
	_, _ = f.WriteString("this is not valid json\n")
	_, _ = f.WriteString(`{"loop_id":"good2","ts":"2026-02-01T00:00:00Z"}` + "\n")
	_, _ = f.WriteString("\n") // empty line
	f.Close()

	loaded, err := LoadObservations(path, time.Time{})
	if err != nil {
		t.Fatalf("LoadObservations: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("expected 2 valid observations (skipping malformed), got %d", len(loaded))
	}
}

func TestAggregateObservations_ZeroWindowReturnsEmpty(t *testing.T) {
	t.Parallel()
	obs := []LoopObservation{{LoopID: "a", TotalCostUSD: 1.0}}
	summary := AggregateObservations(obs, 0)
	if summary.TotalIterations != 0 {
		t.Errorf("expected 0 iterations for zero window, got %d", summary.TotalIterations)
	}
}

func TestAggregateObservations_EfficiencyScore(t *testing.T) {
	t.Parallel()
	now := time.Now()
	obs := []LoopObservation{
		{Timestamp: now.Add(-30 * time.Minute), Status: "idle", TotalCostUSD: 0.25},
		{Timestamp: now.Add(-20 * time.Minute), Status: "idle", TotalCostUSD: 0.25},
		{Timestamp: now.Add(-10 * time.Minute), Status: "failed", TotalCostUSD: 0.50},
	}

	summary := AggregateObservations(obs, 1)
	// 2 completed / $1.00 total = 2.0
	if diff := summary.EfficiencyScore - 2.0; diff > 0.01 || diff < -0.01 {
		t.Errorf("EfficiencyScore = %f, want 2.0", summary.EfficiencyScore)
	}
}

func TestAggregateObservations_ZeroCostEfficiency(t *testing.T) {
	t.Parallel()
	now := time.Now()
	obs := []LoopObservation{
		{Timestamp: now.Add(-30 * time.Minute), Status: "idle", TotalCostUSD: 0},
	}

	summary := AggregateObservations(obs, 1)
	if summary.EfficiencyScore != 0 {
		t.Errorf("EfficiencyScore = %f, want 0 for zero cost", summary.EfficiencyScore)
	}
}

func TestAggregateObservations_StableCostTrend(t *testing.T) {
	t.Parallel()
	now := time.Now()
	obs := []LoopObservation{
		// Previous window (1-2 hours ago)
		{Timestamp: now.Add(-90 * time.Minute), Status: "idle", TotalCostUSD: 1.00},
		// Current window (last hour)
		{Timestamp: now.Add(-30 * time.Minute), Status: "idle", TotalCostUSD: 1.05},
	}

	summary := AggregateObservations(obs, 1)
	if summary.CostTrend != "stable" {
		t.Errorf("CostTrend = %q, want stable (ratio ~1.05)", summary.CostTrend)
	}
}

func TestAggregateObservations_IncreasingCostTrend(t *testing.T) {
	t.Parallel()
	now := time.Now()
	obs := []LoopObservation{
		{Timestamp: now.Add(-90 * time.Minute), Status: "idle", TotalCostUSD: 0.10},
		{Timestamp: now.Add(-30 * time.Minute), Status: "idle", TotalCostUSD: 2.00},
	}

	summary := AggregateObservations(obs, 1)
	if summary.CostTrend != "increasing" {
		t.Errorf("CostTrend = %q, want increasing", summary.CostTrend)
	}
}

func TestAggregateObservations_VelocityIncluded(t *testing.T) {
	t.Parallel()
	now := time.Now()
	obs := []LoopObservation{
		{Timestamp: now.Add(-30 * time.Minute), Status: "idle", TotalCostUSD: 0.10, VerifyPassed: true, FilesChanged: 3},
		{Timestamp: now.Add(-15 * time.Minute), Status: "idle", TotalCostUSD: 0.10, VerifyPassed: true, FilesChanged: 1},
	}

	summary := AggregateObservations(obs, 1)
	// 2 useful in 1 hour = 2.0
	if summary.Velocity != 2.0 {
		t.Errorf("Velocity = %f, want 2.0", summary.Velocity)
	}
}

func TestAggregateObservations_NoPreviousWindow(t *testing.T) {
	t.Parallel()
	now := time.Now()
	obs := []LoopObservation{
		{Timestamp: now.Add(-30 * time.Minute), Status: "idle", TotalCostUSD: 1.00},
	}

	summary := AggregateObservations(obs, 1)
	if summary.CostTrend != "stable" {
		t.Errorf("CostTrend = %q, want stable with no previous window", summary.CostTrend)
	}
}

func TestAggregateObservations_AllOutsideWindow(t *testing.T) {
	t.Parallel()
	now := time.Now()
	obs := []LoopObservation{
		{Timestamp: now.Add(-5 * time.Hour), Status: "idle", TotalCostUSD: 1.00},
	}

	summary := AggregateObservations(obs, 1)
	if summary.TotalIterations != 0 {
		t.Errorf("expected 0 iterations when all outside window, got %d", summary.TotalIterations)
	}
}

func TestLoopVelocity_NegativeWindow(t *testing.T) {
	t.Parallel()
	if v := LoopVelocity(nil, -1); v != 0 {
		t.Errorf("expected 0 for negative window, got %f", v)
	}
}

func TestSummarizeObservations_SingleObs(t *testing.T) {
	t.Parallel()
	obs := []LoopObservation{
		{Status: "idle", TotalLatencyMs: 3000, TotalCostUSD: 0.25},
	}

	s := SummarizeObservations(obs)
	if s.LatencyP50 != 3.0 {
		t.Errorf("LatencyP50 = %f, want 3.0 for single obs", s.LatencyP50)
	}
	if s.LatencyP95 != 3.0 {
		t.Errorf("LatencyP95 = %f, want 3.0 for single obs", s.LatencyP95)
	}
	if s.LatencyP99 != 3.0 {
		t.Errorf("LatencyP99 = %f, want 3.0 for single obs", s.LatencyP99)
	}
	if s.CostP50 != 0.25 {
		t.Errorf("CostP50 = %f, want 0.25 for single obs", s.CostP50)
	}
}
