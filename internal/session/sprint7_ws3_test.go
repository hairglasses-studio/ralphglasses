package session

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 1. aggregateLoopSpend — sum SpentUSD across planner and worker sessions
// ---------------------------------------------------------------------------

func TestAggregateLoopSpend_Empty(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	run := &LoopRun{
		ID:         "loop-spend-empty",
		Iterations: []LoopIteration{},
	}
	got := m.aggregateLoopSpend(run)
	if got != 0 {
		t.Errorf("expected 0 spend for empty iterations, got %f", got)
	}
}

func TestAggregateLoopSpend_WithSessions(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	// Create mock sessions with known spend values
	planner := &Session{ID: "planner-1", SpentUSD: 1.50}
	worker1 := &Session{ID: "worker-1", SpentUSD: 3.00}
	worker2 := &Session{ID: "worker-2", SpentUSD: 2.25}

	m.AddSessionForTesting(planner)
	m.AddSessionForTesting(worker1)
	m.AddSessionForTesting(worker2)

	run := &LoopRun{
		ID: "loop-spend-1",
		Iterations: []LoopIteration{
			{
				PlannerSessionID: "planner-1",
				WorkerSessionIDs: []string{"worker-1"},
			},
			{
				WorkerSessionIDs: []string{"worker-2"},
			},
		},
	}

	got := m.aggregateLoopSpend(run)
	want := 6.75
	if got != want {
		t.Errorf("aggregateLoopSpend = %f, want %f", got, want)
	}
}

func TestAggregateLoopSpend_MissingSessions(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	// Only register one session; the others are missing
	worker := &Session{ID: "worker-exists", SpentUSD: 4.00}
	m.AddSessionForTesting(worker)

	run := &LoopRun{
		ID: "loop-spend-missing",
		Iterations: []LoopIteration{
			{
				PlannerSessionID: "planner-gone",
				WorkerSessionIDs: []string{"worker-exists", "worker-gone"},
			},
		},
	}

	got := m.aggregateLoopSpend(run)
	if got != 4.00 {
		t.Errorf("aggregateLoopSpend = %f, want 4.00 (missing sessions skipped)", got)
	}
}

// ---------------------------------------------------------------------------
// 2. handleSelfImprovementAcceptance — acceptance logic with no changes
// ---------------------------------------------------------------------------

func TestHandleSelfImprovementAcceptance_NoChanges(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	run := &LoopRun{
		ID:      "loop-accept-1",
		Profile: LoopProfile{SelfImprovement: true},
		Iterations: []LoopIteration{
			{Number: 0, Status: "running"},
		},
	}

	// Pass empty worktrees — no changes expected
	result, err := m.handleSelfImprovementAcceptance(context.Background(), run, 0, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.AutoMerged {
		t.Error("should not auto-merge when there are no changes")
	}
	if result.PRCreated {
		t.Error("should not create PR when there are no changes")
	}
}

// ---------------------------------------------------------------------------
// 3. handleSelfImprovementAcceptanceTraced — traced variant returns trace
// ---------------------------------------------------------------------------

func TestHandleSelfImprovementAcceptanceTraced_NoWorktrees(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	run := &LoopRun{
		ID:      "loop-traced-1",
		Profile: LoopProfile{SelfImprovement: true},
		Iterations: []LoopIteration{
			{Number: 0, Status: "running"},
		},
	}

	atr, err := m.handleSelfImprovementAcceptanceTraced(context.Background(), run, 0, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atr.Result == nil {
		t.Fatal("expected non-nil result")
	}
	if atr.Trace.Reason != "worker_no_changes" {
		t.Errorf("trace reason = %q, want %q", atr.Trace.Reason, "worker_no_changes")
	}
}

func TestHandleSelfImprovementAcceptanceTraced_EmptyWorktreeStrings(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	run := &LoopRun{
		ID:      "loop-traced-2",
		Profile: LoopProfile{SelfImprovement: true},
	}

	// Empty string worktrees should be skipped
	atr, err := m.handleSelfImprovementAcceptanceTraced(context.Background(), run, 0, []string{"", ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if atr.Trace.Reason != "worker_no_changes" {
		t.Errorf("trace reason = %q, want %q", atr.Trace.Reason, "worker_no_changes")
	}
}

// ---------------------------------------------------------------------------
// 4. enhanceForProvider — provider-specific prompt enhancement mapping
// ---------------------------------------------------------------------------

func TestMapProvider_Sprint7Coverage(t *testing.T) {
	tests := []struct {
		input Provider
		want  string
	}{
		{ProviderClaude, "claude"},
		{ProviderGemini, "gemini"},
		{ProviderCodex, "openai"},
		{"", "openai"},
		{"unknown", "claude"},
	}
	for _, tc := range tests {
		got := mapProvider(tc.input)
		if string(got) != tc.want {
			t.Errorf("mapProvider(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// enhanceForProvider requires a non-nil Enhancer for the LLM path.
// Without one, we test that the function still works (local-only path).
func TestEnhanceForProvider_NoEnhancer(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())
	// Enhancer is nil — should use local-only pipeline
	result := m.enhanceForProvider(context.Background(), "Fix the bug in auth.go", ProviderClaude)
	if result.prompt == "" {
		t.Error("expected non-empty enhanced prompt")
	}
	// Source should be "local" when no LLM enhancer is available
	if result.source != "local" {
		t.Errorf("source = %q, want %q", result.source, "local")
	}
}

// ---------------------------------------------------------------------------
// 5. BudgetOptimizedSelfImprovementProfile — profile defaults
// ---------------------------------------------------------------------------

func TestBudgetOptimizedSelfImprovementProfile_Defaults(t *testing.T) {
	p := BudgetOptimizedSelfImprovementProfile(0)
	// Zero budget should default to 100
	if p.HardBudgetCapUSD != 95.0 {
		t.Errorf("HardBudgetCapUSD = %f, want 95.0 (95%% of $100)", p.HardBudgetCapUSD)
	}
	if p.NoopPlateauLimit != 3 {
		t.Errorf("NoopPlateauLimit = %d, want 3", p.NoopPlateauLimit)
	}
	if p.PlannerProvider != ProviderCodex {
		t.Errorf("PlannerProvider = %q, want codex", p.PlannerProvider)
	}
	if !p.SelfImprovement {
		t.Error("SelfImprovement should be true")
	}
	if !p.AutoMergeAll {
		t.Error("AutoMergeAll should be true")
	}
	if p.MaxIterations != 150 {
		t.Errorf("MaxIterations = %d, want 150 (100 * 1.5)", p.MaxIterations)
	}
}

func TestBudgetOptimizedSelfImprovementProfile_CustomBudget(t *testing.T) {
	p := BudgetOptimizedSelfImprovementProfile(200)
	if p.HardBudgetCapUSD != 190.0 {
		t.Errorf("HardBudgetCapUSD = %f, want 190.0 (95%% of $200)", p.HardBudgetCapUSD)
	}
	if p.PlannerBudgetUSD != 3.0 {
		t.Errorf("PlannerBudgetUSD = %f, want 3.0 (1.5%% of $200)", p.PlannerBudgetUSD)
	}
	if p.WorkerBudgetUSD != 6.0 {
		t.Errorf("WorkerBudgetUSD = %f, want 6.0 (3%% of $200)", p.WorkerBudgetUSD)
	}
	if p.VerifierBudgetUSD != 1.0 {
		t.Errorf("VerifierBudgetUSD = %f, want 1.0 (0.5%% of $200)", p.VerifierBudgetUSD)
	}
	if p.MaxIterations != 300 {
		t.Errorf("MaxIterations = %d, want 300 (200 * 1.5)", p.MaxIterations)
	}
	if p.StallTimeout != 10*time.Minute {
		t.Errorf("StallTimeout = %v, want 10m", p.StallTimeout)
	}
}

func TestBudgetOptimizedSelfImprovementProfile_CodexDefaults(t *testing.T) {
	p := BudgetOptimizedSelfImprovementProfile(50)
	if p.PlannerModel != ProviderDefaults(ProviderCodex) {
		t.Errorf("PlannerModel = %q, want %q", p.PlannerModel, ProviderDefaults(ProviderCodex))
	}
	if p.WorkerModel != ProviderDefaults(ProviderCodex) {
		t.Errorf("WorkerModel = %q, want %q", p.WorkerModel, ProviderDefaults(ProviderCodex))
	}
	if p.VerifierModel != ProviderDefaults(ProviderCodex) {
		t.Errorf("VerifierModel = %q, want %q", p.VerifierModel, ProviderDefaults(ProviderCodex))
	}
	if p.CompactionThreshold != 5 {
		t.Errorf("CompactionThreshold = %d, want 5", p.CompactionThreshold)
	}
}

// ---------------------------------------------------------------------------
// 6. Init — manager initialization (sweeps orphans, auto-prune)
// ---------------------------------------------------------------------------

func TestManagerInit_NoStateDir(t *testing.T) {
	m := NewManager()
	dir := t.TempDir()
	m.SetStateDir(filepath.Join(dir, "sessions"))

	// Init should not panic even with no sessions
	m.Init()

	// Wait briefly for background goroutine (auto-prune)
	time.Sleep(100 * time.Millisecond)

	// Verify the pruned counter is 0 (nothing to prune)
	if got := m.TotalPrunedThisSession(); got != 0 {
		t.Errorf("TotalPrunedThisSession = %d, want 0", got)
	}
}

func TestManagerInit_PrunesStaleLoops(t *testing.T) {
	m := NewManager()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "sessions")
	m.SetStateDir(stateDir)

	// Set a very short retention so our stale loop gets pruned
	m.PruneRetention = 1 * time.Millisecond

	// Create loop state dir and a stale "failed" loop file
	loopDir := filepath.Join(stateDir, "loops")
	if err := os.MkdirAll(loopDir, 0755); err != nil {
		t.Fatal(err)
	}
	staleJSON := `{"id":"stale-1","status":"failed","created_at":"2020-01-01T00:00:00Z","updated_at":"2020-01-01T00:00:00Z"}`
	if err := os.WriteFile(filepath.Join(loopDir, "stale-1.json"), []byte(staleJSON), 0644); err != nil {
		t.Fatal(err)
	}

	m.Init()

	// Wait for the background goroutine
	time.Sleep(200 * time.Millisecond)

	if got := m.TotalPrunedThisSession(); got != 1 {
		t.Errorf("TotalPrunedThisSession = %d, want 1", got)
	}
}

// ---------------------------------------------------------------------------
// 7. Resume — session resume constructs correct LaunchOptions
// ---------------------------------------------------------------------------

func TestResume_DefaultProvider(t *testing.T) {
	// Resume builds LaunchOptions and delegates to Launch. Since Launch
	// validates binaries on PATH, we test the option construction logic
	// directly by verifying Resume resolves an empty provider to the
	// repo-wide primary provider.

	// Test that empty provider defaults to codex
	m := NewManager()
	m.SetStateDir(t.TempDir())

	// Resume will fail because the provider binary isn't real in this test
	// environment, but we can verify
	// that the method at least defaults provider correctly by observing that
	// it does NOT return "unknown provider" error.
	_, err := m.Resume(context.Background(), t.TempDir(), "", "old-session-id", "continue task")
	if err == nil {
		// If it somehow succeeds (unlikely), that's fine too
		return
	}
	// The error should be about the binary, not "unknown provider"
	errStr := err.Error()
	if errStr == "" {
		t.Error("expected non-empty error")
	}
	// Should not complain about unknown provider — codex is valid
	if contains(errStr, "unknown provider") {
		t.Errorf("Resume with empty provider produced unknown provider error: %v", err)
	}
}

func TestResume_WithProvider(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	// Resume with explicit provider should pass that provider through.
	// We verify by checking the error message references the correct binary.
	repoDir := t.TempDir()
	os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)
	_, err := m.Resume(context.Background(), repoDir, ProviderGemini, "gem-session", "do stuff")
	if err == nil {
		return // unexpected success is fine
	}
	errStr := err.Error()
	// Should reference gemini binary, confirming provider was set
	if !contains(errStr, "gemini") {
		t.Errorf("expected error to mention gemini binary, got: %v", err)
	}
}

// contains is already declared in agents_test.go — reuse it.

// ---------------------------------------------------------------------------
// 8. AddTeamForTesting — team addition and retrieval
// ---------------------------------------------------------------------------

func TestAddTeamForTesting(t *testing.T) {
	m := NewManager()

	team := &TeamStatus{
		Name:     "test-team",
		RepoPath: "/tmp/repo",
		LeadID:   "lead-1",
		Status:   StatusRunning,
	}

	m.AddTeamForTesting(team)

	got, ok := m.GetTeam("test-team")
	if !ok {
		t.Fatal("expected team to be found")
	}
	if got.Name != "test-team" {
		t.Errorf("team name = %q, want test-team", got.Name)
	}
	if got.LeadID != "lead-1" {
		t.Errorf("lead ID = %q, want lead-1", got.LeadID)
	}
}

func TestAddTeamForTesting_Overwrite(t *testing.T) {
	m := NewManager()

	team1 := &TeamStatus{Name: "alpha", LeadID: "v1"}
	team2 := &TeamStatus{Name: "alpha", LeadID: "v2"}

	m.AddTeamForTesting(team1)
	m.AddTeamForTesting(team2)

	got, _ := m.GetTeam("alpha")
	if got.LeadID != "v2" {
		t.Errorf("expected overwritten team, lead = %q, want v2", got.LeadID)
	}
}

func TestAddTeamForTesting_ConcurrentSafe(t *testing.T) {
	m := NewManager()
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.AddTeamForTesting(&TeamStatus{Name: "team", LeadID: "lead"})
		}(i)
	}
	wg.Wait()
	_, ok := m.GetTeam("team")
	if !ok {
		t.Error("expected team after concurrent adds")
	}
}

// ---------------------------------------------------------------------------
// 9. gitDiffPaths — test with a real temp git repo
// ---------------------------------------------------------------------------

func TestGitDiffPaths_CleanRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.email", "test@test.com")
	mustGit(t, dir, "config", "user.name", "Test")

	// Create initial commit
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", "init")

	paths, err := gitDiffPaths(dir)
	if err != nil {
		t.Fatalf("gitDiffPaths error: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 changed paths in clean repo, got %d: %v", len(paths), paths)
	}
}

func TestGitDiffPaths_WithChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found on PATH")
	}

	dir := t.TempDir()
	mustGit(t, dir, "init")
	mustGit(t, dir, "config", "user.email", "test@test.com")
	mustGit(t, dir, "config", "user.name", "Test")

	// Create initial commit
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", ".")
	mustGit(t, dir, "commit", "-m", "init")

	// Modify file
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n// changed"), 0644); err != nil {
		t.Fatal(err)
	}

	paths, err := gitDiffPaths(dir)
	if err != nil {
		t.Fatalf("gitDiffPaths error: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 changed path, got %d: %v", len(paths), paths)
	}
	if paths[0] != "main.go" {
		t.Errorf("changed path = %q, want main.go", paths[0])
	}
}

// ---------------------------------------------------------------------------
// 10. buildCmdForProvider — command construction per provider
// ---------------------------------------------------------------------------

func TestBuildClaudeCmd(t *testing.T) {
	opts := LaunchOptions{
		Provider:     ProviderClaude,
		RepoPath:     "/tmp/repo",
		Prompt:       "test prompt",
		Model:        "sonnet",
		MaxBudgetUSD: 5.0,
		MaxTurns:     10,
	}
	cmd := buildClaudeCmd(context.Background(), opts)

	if cmd.Dir != "/tmp/repo" {
		t.Errorf("cmd.Dir = %q, want /tmp/repo", cmd.Dir)
	}
	args := cmd.Args
	if args[0] != "claude" {
		t.Errorf("binary = %q, want claude", args[0])
	}
	assertContainsArg(t, args, "-p")
	assertContainsArg(t, args, "--verbose")
	assertContainsArg(t, args, "--output-format")
	assertContainsArg(t, args, "--model")
	assertContainsArg(t, args, "--max-budget-usd")
	assertContainsArg(t, args, "--max-turns")
}

func TestBuildGeminiCmd_AllFlags(t *testing.T) {
	opts := LaunchOptions{
		Provider: ProviderGemini,
		RepoPath: "/tmp/repo",
		Prompt:   "test prompt",
		Model:    "gemini-2.5-pro",
	}
	cmd := buildGeminiCmd(context.Background(), opts)

	if cmd.Args[0] != "gemini" {
		t.Errorf("binary = %q, want gemini", cmd.Args[0])
	}
	assertContainsArg(t, cmd.Args, "--output-format")
	assertContainsArg(t, cmd.Args, "--approval-mode")
	assertContainsArg(t, cmd.Args, "-p")
}

func TestBuildCodexCmd_AllFlags(t *testing.T) {
	opts := LaunchOptions{
		Provider: ProviderCodex,
		RepoPath: "/tmp/repo",
		Prompt:   "test prompt",
		Model:    "gpt-5.4",
	}
	cmd := buildCodexCmd(context.Background(), opts)

	if cmd.Args[0] != "codex" {
		t.Errorf("binary = %q, want codex", cmd.Args[0])
	}
	assertContainsArg(t, cmd.Args, "exec")
	assertContainsArg(t, cmd.Args, "--json")
	assertContainsArg(t, cmd.Args, "--full-auto")
}

func TestBuildClaudeCmd_ResumeFlag(t *testing.T) {
	opts := LaunchOptions{
		Provider: ProviderClaude,
		RepoPath: "/tmp/repo",
		Prompt:   "continue",
		Resume:   "session-abc",
	}
	cmd := buildClaudeCmd(context.Background(), opts)
	assertContainsArg(t, cmd.Args, "--resume")
	assertContainsArg(t, cmd.Args, "session-abc")
}

func TestBuildClaudeCmd_ContinueFlag(t *testing.T) {
	opts := LaunchOptions{
		Provider: ProviderClaude,
		RepoPath: "/tmp/repo",
		Prompt:   "go on",
		Continue: true,
	}
	cmd := buildClaudeCmd(context.Background(), opts)
	assertContainsArg(t, cmd.Args, "--continue")
}

func TestBuildClaudeCmd_WorktreeFlag(t *testing.T) {
	opts := LaunchOptions{
		Provider: ProviderClaude,
		RepoPath: "/tmp/repo",
		Prompt:   "test",
		Worktree: "true",
	}
	cmd := buildClaudeCmd(context.Background(), opts)
	assertContainsArg(t, cmd.Args, "-w")
}

func TestBuildClaudeCmd_BareAndEffort(t *testing.T) {
	opts := LaunchOptions{
		Provider: ProviderClaude,
		RepoPath: "/tmp/repo",
		Prompt:   "test",
		Bare:     true,
		Effort:   "high",
	}
	cmd := buildClaudeCmd(context.Background(), opts)
	assertContainsArg(t, cmd.Args, "--bare")
	assertContainsArg(t, cmd.Args, "--effort")
	assertContainsArg(t, cmd.Args, "high")
}

// ---------------------------------------------------------------------------
// 11. startupProbe — test with mock session
// ---------------------------------------------------------------------------

func TestStartupProbe_AlreadyHasOutput(t *testing.T) {
	s := &Session{
		ID:               "probe-test-1",
		TotalOutputCount: 5,
		doneCh:           make(chan struct{}),
	}

	err := startupProbe(s, 1*time.Second)
	if err != nil {
		t.Errorf("expected nil error for session with output, got: %v", err)
	}
}

func TestStartupProbe_SurvivesWindow(t *testing.T) {
	s := &Session{
		ID:     "probe-test-2",
		doneCh: make(chan struct{}),
	}

	start := time.Now()
	err := startupProbe(s, 200*time.Millisecond)
	elapsed := time.Since(start)

	if err != nil {
		t.Errorf("expected nil error when surviving probe window, got: %v", err)
	}
	if elapsed < 150*time.Millisecond {
		t.Errorf("probe returned too quickly: %v (expected ~200ms)", elapsed)
	}
}

func TestStartupProbe_ProcessExitsDuringStartup(t *testing.T) {
	doneCh := make(chan struct{})
	s := &Session{
		ID:     "probe-test-3",
		Status: StatusRunning,
		doneCh: doneCh,
	}

	// Simulate process exit after 50ms
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.mu.Lock()
		s.Status = StatusErrored
		s.Error = "binary not found"
		s.mu.Unlock()
		close(doneCh)
	}()

	err := startupProbe(s, 2*time.Second)
	if err == nil {
		t.Fatal("expected error when process exits during startup")
	}
	if got := err.Error(); got == "" {
		t.Error("expected non-empty error message")
	}
}

func TestStartupProbe_CompletedNormally(t *testing.T) {
	doneCh := make(chan struct{})
	s := &Session{
		ID:     "probe-test-4",
		Status: StatusRunning,
		doneCh: doneCh,
	}

	// Simulate process completing successfully
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.mu.Lock()
		s.Status = StatusCompleted
		s.mu.Unlock()
		close(doneCh)
	}()

	err := startupProbe(s, 2*time.Second)
	if err != nil {
		t.Errorf("expected nil error when process completes normally, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 12. ReflexionStore.RecentForTask — rule extraction by keyword overlap
// ---------------------------------------------------------------------------

func TestReflexionStore_RecentForTask_Empty(t *testing.T) {
	rs := NewReflexionStore("")
	results := rs.RecentForTask("fix auth middleware", 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty store, got %d", len(results))
	}
}

func TestReflexionStore_RecentForTask_MatchByKeyword(t *testing.T) {
	rs := NewReflexionStore("")

	rs.Store(Reflection{
		Timestamp:   time.Now(),
		LoopID:      "loop-1",
		TaskTitle:   "implement auth middleware",
		FailureMode: "verify_failed",
		RootCause:   "missing import",
		Correction:  "add missing import",
	})
	rs.Store(Reflection{
		Timestamp:   time.Now(),
		LoopID:      "loop-1",
		TaskTitle:   "fix database connection pooling",
		FailureMode: "worker_error",
		RootCause:   "timeout",
		Correction:  "increase timeout",
	})

	// Query for auth-related tasks — should match the first
	results := rs.RecentForTask("auth handler", 5)
	if len(results) != 1 {
		t.Fatalf("expected 1 result for 'auth handler', got %d", len(results))
	}
	if results[0].TaskTitle != "implement auth middleware" {
		t.Errorf("matched wrong task: %q", results[0].TaskTitle)
	}
}

func TestReflexionStore_RecentForTask_EmptyQuery(t *testing.T) {
	rs := NewReflexionStore("")

	rs.Store(Reflection{TaskTitle: "task-a", LoopID: "l1"})
	rs.Store(Reflection{TaskTitle: "task-b", LoopID: "l2"})
	rs.Store(Reflection{TaskTitle: "task-c", LoopID: "l3"})

	// Empty query should return all (up to limit), most recent first
	results := rs.RecentForTask("", 2)
	if len(results) != 2 {
		t.Fatalf("expected 2 results for empty query, got %d", len(results))
	}
	// Most recent first
	if results[0].TaskTitle != "task-c" {
		t.Errorf("first result = %q, want task-c (newest)", results[0].TaskTitle)
	}
}

func TestReflexionStore_RecentForTask_LimitRespected(t *testing.T) {
	rs := NewReflexionStore("")
	for range 10 {
		rs.Store(Reflection{TaskTitle: "test task", LoopID: "l1"})
	}
	results := rs.RecentForTask("test task", 3)
	if len(results) != 3 {
		t.Errorf("expected 3 results with limit=3, got %d", len(results))
	}
}

func TestReflexionStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	rs1 := NewReflexionStore(dir)
	rs1.Store(Reflection{
		LoopID:      "loop-persist",
		TaskTitle:   "persist test",
		FailureMode: "test",
		RootCause:   "test cause",
		Correction:  "test fix",
	})

	// Load from disk
	rs2 := NewReflexionStore(dir)
	results := rs2.RecentForTask("", 10)
	if len(results) != 1 {
		t.Fatalf("expected 1 persisted reflection, got %d", len(results))
	}
	if results[0].LoopID != "loop-persist" {
		t.Errorf("loaded loop_id = %q, want loop-persist", results[0].LoopID)
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME="+dir,
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func assertContainsArg(t *testing.T, args []string, want string) {
	t.Helper()
	if slices.Contains(args, want) {
		return
	}
	t.Errorf("args %v missing %q", args, want)
}
