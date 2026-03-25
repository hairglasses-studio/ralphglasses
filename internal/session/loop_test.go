package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDefaultLoopProfile(t *testing.T) {
	profile := DefaultLoopProfile()
	if profile.PlannerProvider != ProviderCodex {
		t.Fatalf("planner provider = %q", profile.PlannerProvider)
	}
	if profile.PlannerModel != "o1-pro" {
		t.Fatalf("planner model = %q", profile.PlannerModel)
	}
	if profile.WorkerModel != "gpt-5.4-xhigh" {
		t.Fatalf("worker model = %q", profile.WorkerModel)
	}
	if len(profile.VerifyCommands) != 1 || profile.VerifyCommands[0] != "./scripts/dev/ci.sh" {
		t.Fatalf("verify commands = %#v", profile.VerifyCommands)
	}
}

func TestLoopStepSuccess(t *testing.T) {
	repoPath := setupLoopRepo(t)

	m := NewManager()
	m.SetStateDir(t.TempDir())
	m.SetHooksForTesting(
		func(_ context.Context, opts LaunchOptions) (*Session, error) {
			sess := &Session{
				ID:         sanitizeLoopName(opts.SessionName),
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   filepath.Base(opts.RepoPath),
				Prompt:     opts.Prompt,
				Model:      opts.Model,
				Status:     StatusCompleted,
				OutputCh:   make(chan string, 1),
				LaunchedAt: time.Now(),
			}
			if opts.Model == "o1-pro" {
				sess.LastOutput = `{"title":"Add README note","prompt":"Append a loop-generated marker comment to README.md."}`
				sess.OutputHistory = []string{sess.LastOutput}
			} else {
				sess.LastOutput = "worker complete"
				sess.OutputHistory = []string{"worker complete"}
			}
			return sess, nil
		},
		func(_ context.Context, sess *Session) error {
			sess.Lock()
			sess.Status = StatusCompleted
			now := time.Now()
			sess.EndedAt = &now
			sess.Unlock()
			return nil
		},
	)

	run, err := m.StartLoop(context.Background(), repoPath, LoopProfile{
		VerifyCommands: []string{"test -f README.md"},
	})
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	if err := m.StepLoop(context.Background(), run.ID); err != nil {
		t.Fatalf("StepLoop: %v", err)
	}

	run, ok := m.GetLoop(run.ID)
	if !ok {
		t.Fatal("loop not found after step")
	}

	run.Lock()
	defer run.Unlock()

	if run.Status != "idle" {
		t.Fatalf("run status = %q", run.Status)
	}
	if len(run.Iterations) != 1 {
		t.Fatalf("iterations = %d", len(run.Iterations))
	}
	iter := run.Iterations[0]
	if iter.Status != "idle" {
		t.Fatalf("iteration status = %q", iter.Status)
	}
	if iter.WorktreePath == "" {
		t.Fatal("expected worktree path")
	}
	if _, err := os.Stat(iter.WorktreePath); err != nil {
		t.Fatalf("stat worktree: %v", err)
	}
	if iter.Task.Title != "Add README note" {
		t.Fatalf("task title = %q", iter.Task.Title)
	}
	if len(iter.Verification) != 1 || iter.Verification[0].Status != "completed" {
		t.Fatalf("verification = %#v", iter.Verification)
	}
}

func TestLoopStepVerificationFailure(t *testing.T) {
	repoPath := setupLoopRepo(t)

	m := NewManager()
	m.SetStateDir(t.TempDir())
	m.SetHooksForTesting(
		func(_ context.Context, opts LaunchOptions) (*Session, error) {
			sess := &Session{
				ID:         sanitizeLoopName(opts.SessionName),
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   filepath.Base(opts.RepoPath),
				Prompt:     opts.Prompt,
				Model:      opts.Model,
				Status:     StatusCompleted,
				OutputCh:   make(chan string, 1),
				LaunchedAt: time.Now(),
			}
			if opts.Model == "o1-pro" {
				sess.LastOutput = `{"title":"Break verify","prompt":"Do some work that will fail verification."}`
			}
			return sess, nil
		},
		func(_ context.Context, sess *Session) error {
			sess.Lock()
			sess.Status = StatusCompleted
			now := time.Now()
			sess.EndedAt = &now
			sess.Unlock()
			return nil
		},
	)

	run, err := m.StartLoop(context.Background(), repoPath, LoopProfile{
		VerifyCommands: []string{"exit 7"},
	})
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	err = m.StepLoop(context.Background(), run.ID)
	if err == nil {
		t.Fatal("expected verification failure")
	}

	run, ok := m.GetLoop(run.ID)
	if !ok {
		t.Fatal("loop not found after failure")
	}

	run.Lock()
	defer run.Unlock()

	if run.Status != "failed" {
		t.Fatalf("run status = %q", run.Status)
	}
	if run.LastError == "" {
		t.Fatal("expected last error")
	}
	if len(run.Iterations) != 1 {
		t.Fatalf("iterations = %d", len(run.Iterations))
	}
	if run.Iterations[0].Status != "failed" {
		t.Fatalf("iteration status = %q", run.Iterations[0].Status)
	}
}

func TestStopLoopPreventsFurtherSteps(t *testing.T) {
	repoPath := setupLoopRepo(t)

	m := NewManager()
	m.SetStateDir(t.TempDir())
	run, err := m.StartLoop(context.Background(), repoPath, LoopProfile{})
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}
	if err := m.StopLoop(run.ID); err != nil {
		t.Fatalf("StopLoop: %v", err)
	}
	if err := m.StepLoop(context.Background(), run.ID); err == nil {
		t.Fatal("expected stopped loop to refuse stepping")
	}
}

func TestCompactionBetaSetAfterThreshold(t *testing.T) {
	repoPath := setupLoopRepo(t)

	var capturedBetas [][]string
	var mu sync.Mutex
	var callCount int

	m := NewManager()
	m.SetStateDir(t.TempDir())
	m.SetHooksForTesting(
		func(_ context.Context, opts LaunchOptions) (*Session, error) {
			mu.Lock()
			capturedBetas = append(capturedBetas, opts.Betas)
			callCount++
			n := callCount
			mu.Unlock()
			sess := &Session{
				ID:         sanitizeLoopName(opts.SessionName),
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   filepath.Base(opts.RepoPath),
				Prompt:     opts.Prompt,
				Model:      opts.Model,
				Status:     StatusCompleted,
				OutputCh:   make(chan string, 1),
				LaunchedAt: time.Now(),
			}
			if opts.Model == "o1-pro" {
				sess.LastOutput = fmt.Sprintf(`{"title":"Task %d","prompt":"Do work %d."}`, n, n)
				sess.OutputHistory = []string{sess.LastOutput}
			} else {
				sess.LastOutput = fmt.Sprintf("worker complete %d", n)
				sess.OutputHistory = []string{sess.LastOutput}
			}
			return sess, nil
		},
		func(_ context.Context, sess *Session) error {
			sess.Lock()
			sess.Status = StatusCompleted
			now := time.Now()
			sess.EndedAt = &now
			sess.Unlock()
			return nil
		},
	)

	// CompactionThreshold=2 so after 2 iterations, compaction kicks in on iteration 3.
	run, err := m.StartLoop(context.Background(), repoPath, LoopProfile{
		CompactionEnabled:   true,
		CompactionThreshold: 2,
		VerifyCommands:      []string{"true"},
		MaxIterations:       10,
	})
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	// Pre-populate 2 completed iterations with acceptance results to avoid
	// convergence detection (which fires when 2 idle iterations have no changes).
	run.Lock()
	for i := 0; i < 2; i++ {
		run.Iterations = append(run.Iterations, LoopIteration{
			Number: i + 1,
			Status: "idle",
			Task:   LoopTask{Title: fmt.Sprintf("Prior task %d", i+1), Prompt: "done"},
			Acceptance: &AcceptanceResult{
				SafePaths: []string{fmt.Sprintf("file%d.go", i)},
			},
			StartedAt: time.Now(),
		})
	}
	run.Unlock()

	// Step once more — this will be iteration 3 (Number > threshold=2).
	if err := m.StepLoop(context.Background(), run.ID); err != nil {
		t.Fatalf("StepLoop: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Each iteration launches planner + worker = 2 sessions + verifier(s).
	// We need to find worker sessions that have Betas set.
	// Iterations 1 and 2 (Number <= threshold=2): no compaction.
	// Iteration 3 (Number=3 > threshold=2): compaction beta should be set.
	var foundCompaction bool
	for _, betas := range capturedBetas {
		for _, b := range betas {
			if b == "compact-2026-01-12" {
				foundCompaction = true
			}
		}
	}
	if !foundCompaction {
		t.Fatalf("expected compaction beta to be set after threshold; captured betas: %v", capturedBetas)
	}

	// Verify that not ALL captured calls have compaction — the planner
	// session should not have it (only worker sessions do).
	var withoutCompaction bool
	for _, betas := range capturedBetas {
		hasCompact := false
		for _, b := range betas {
			if b == "compact-2026-01-12" {
				hasCompact = true
			}
		}
		if !hasCompact {
			withoutCompaction = true
		}
	}
	if !withoutCompaction {
		t.Fatal("expected at least one session (planner) without compaction beta")
	}
}

func TestNormalizeLoopProfileCompactionDefault(t *testing.T) {
	p := LoopProfile{CompactionEnabled: true}
	normalized, err := normalizeLoopProfile(p)
	if err != nil {
		t.Fatalf("normalizeLoopProfile: %v", err)
	}
	if normalized.CompactionThreshold != 10 {
		t.Fatalf("CompactionThreshold = %d, want 10", normalized.CompactionThreshold)
	}

	// Explicit threshold should be preserved.
	p2 := LoopProfile{CompactionEnabled: true, CompactionThreshold: 5}
	normalized2, err := normalizeLoopProfile(p2)
	if err != nil {
		t.Fatalf("normalizeLoopProfile: %v", err)
	}
	if normalized2.CompactionThreshold != 5 {
		t.Fatalf("CompactionThreshold = %d, want 5", normalized2.CompactionThreshold)
	}

	// Disabled compaction should not set threshold.
	p3 := LoopProfile{CompactionEnabled: false}
	normalized3, err := normalizeLoopProfile(p3)
	if err != nil {
		t.Fatalf("normalizeLoopProfile: %v", err)
	}
	if normalized3.CompactionThreshold != 0 {
		t.Fatalf("CompactionThreshold = %d, want 0 when disabled", normalized3.CompactionThreshold)
	}
}

func TestSelfImprovementProfileCompactionEnabled(t *testing.T) {
	p := SelfImprovementProfile()
	if !p.CompactionEnabled {
		t.Fatal("SelfImprovementProfile should have CompactionEnabled=true")
	}
}

func setupLoopRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# loop repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".ralphrc"), []byte("PROJECT_NAME=\"test\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "ROADMAP.md"), []byte("# Loop Roadmap\n\n## Phase 1\n\n### 1.1\n- [ ] 1.1.1 — Add README note\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runGitLoop(t, repoPath, "init")
	runGitLoop(t, repoPath, "config", "user.email", "test@example.com")
	runGitLoop(t, repoPath, "config", "user.name", "Test User")
	runGitLoop(t, repoPath, "config", "commit.gpgsign", "false")
	runGitLoop(t, repoPath, "add", ".")
	runGitLoop(t, repoPath, "commit", "-m", "initial")
	return repoPath
}

func runGitLoop(t *testing.T, repoPath string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestSanitizeTaskTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "Add README note", "Add README note"},
		{"whitespace", "  trim me  ", "trim me"},
		{"empty", "", ""},
		{"json object with title", `{"title":"Real Title","other":"data"}`, "Real Title"},
		{"json object with Title", `{"Title":"Cap Title","x":1}`, "Cap Title"},
		{"json object no title field", `{"foo":"bar"}`, `{"foo":"bar"}`},
		{"json array ignored", `[{"title":"x"}]`, `[{"title":"x"}]`},
		{"multiline takes first", "first line\nsecond line", "first line"},
		{"multiline carriage return", "first\r\nsecond", "first"},
		{"truncate long", strings.Repeat("X", 200), strings.Repeat("X", 120)},
		{"truncate exact boundary", strings.Repeat("Y", 120), strings.Repeat("Y", 120)},
		{"no truncate short", strings.Repeat("Z", 50), strings.Repeat("Z", 50)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeTaskTitle(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeTaskTitle(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSanitizeTaskTitle_WorkerOutput(t *testing.T) {
	workerOutputs := []string{
		"All tests pass. Here's what I did:",
		"I've completed the requested changes",
		"Successfully updated the test file",
		"Done. The changes have been applied.",
		"I added unit tests for the parser",
	}
	for _, input := range workerOutputs {
		t.Run(input, func(t *testing.T) {
			got := sanitizeTaskTitle(input)
			if got != "self-improvement iteration" {
				t.Errorf("sanitizeTaskTitle(%q) = %q, want %q", input, got, "self-improvement iteration")
			}
		})
	}
}

func TestSanitizeTaskTitle_ValidTitles(t *testing.T) {
	validTitles := []struct {
		name  string
		input string
		want  string
	}{
		{"imperative verb", "Add unit tests for RefreshRepo error propagation", "Add unit tests for RefreshRepo error propagation"},
		{"wire TUI", "Wire TUI to consume process.Manager ErrorChan", "Wire TUI to consume process.Manager ErrorChan"},
		{"refactor", "Refactor TUI key bindings for consistency", "Refactor TUI key bindings for consistency"},
		{"markdown JSON", "```json\n{\"title\": \"Wrapped title\"}\n```", "Wrapped title"},
	}
	for _, tc := range validTitles {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeTaskTitle(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeTaskTitle(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSanitizeTaskTitle_JSONExtraction(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"title key", `{"title": "Fix bug"}`, "Fix bug"},
		{"task key", `{"task": "Add tests"}`, "Add tests"},
		{"name key", `{"name": "Refactor module"}`, "Refactor module"},
		{"markdown fenced JSON", "```json\n{\"title\": \"Wrapped title\"}\n```", "Wrapped title"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeTaskTitle(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeTaskTitle(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestParsePlannerTask_SanitizesTitle(t *testing.T) {
	// JSON with a raw JSON string as title
	input := `{"title":"{\"nested\":\"json\",\"title\":\"Actual Title\"}","prompt":"do something"}`
	task, err := parsePlannerTask(input)
	if err != nil {
		t.Fatalf("parsePlannerTask: %v", err)
	}
	if task.Title != "Actual Title" {
		t.Errorf("title = %q, want %q", task.Title, "Actual Title")
	}
}

func TestParsePlannerTask_TruncatesLongTitle(t *testing.T) {
	longTitle := string(make([]byte, 200))
	for i := range longTitle {
		longTitle = longTitle[:i] + "A" + longTitle[i+1:]
	}
	// Build with a proper long title that will be over 120 chars.
	input := `{"title":"` + longTitle + `","prompt":"do work"}`
	task, err := parsePlannerTask(input)
	if err != nil {
		t.Fatalf("parsePlannerTask: %v", err)
	}
	if len(task.Title) > 120 {
		t.Errorf("title length = %d, want <= 120", len(task.Title))
	}
}

func TestBuildLoopPlannerPrompt_CompletedDedup(t *testing.T) {
	repoPath := setupLoopRepo(t)

	prevIterations := []LoopIteration{
		{Number: 1, Status: "idle", Task: LoopTask{Title: "Add README note"}},
		{Number: 2, Status: "failed", Task: LoopTask{Title: "Fix CI pipeline"}},
		{Number: 3, Status: "idle", Task: LoopTask{Title: "Add unit tests"}},
	}

	prompt, err := buildLoopPlannerPrompt(repoPath, prevIterations)
	if err != nil {
		t.Fatalf("buildLoopPlannerPrompt: %v", err)
	}

	// Should contain completed tasks section with successful iterations only
	if !contains(prompt, "Completed tasks (DO NOT repeat these):") {
		t.Error("missing completed tasks section")
	}
	if !contains(prompt, "Add README note") {
		t.Error("missing successful task 'Add README note'")
	}
	if !contains(prompt, "Add unit tests") {
		t.Error("missing successful task 'Add unit tests'")
	}
	// Failed task should NOT be in completed list
	// (it will appear in "Recent task types" though)

	// Should contain diversity steering
	if !contains(prompt, "Recent task types (prefer a different kind of task):") {
		t.Error("missing recent task types section")
	}

	// Should contain variety constraint
	if !contains(prompt, "Prefer variety in task types") {
		t.Error("missing variety constraint")
	}

	// Should contain git commits section
	if !contains(prompt, "Recent git commits:") {
		t.Error("missing recent git commits section")
	}
}

func TestBuildLoopPlannerPrompt_NoPrevIterations(t *testing.T) {
	repoPath := setupLoopRepo(t)

	prompt, err := buildLoopPlannerPrompt(repoPath, nil)
	if err != nil {
		t.Fatalf("buildLoopPlannerPrompt: %v", err)
	}

	// Should NOT have completed tasks or recent task types sections
	if contains(prompt, "Completed tasks") {
		t.Error("unexpected completed tasks section with no prev iterations")
	}
	if contains(prompt, "Recent task types") {
		t.Error("unexpected recent task types section with no prev iterations")
	}
	// Should still have git commits (independent of iterations)
	if !contains(prompt, "Recent git commits:") {
		t.Error("missing recent git commits section")
	}
}

func TestBuildLoopPlannerPrompt_JSONEnforcement(t *testing.T) {
	repoPath := setupLoopRepo(t)

	prompt, err := buildLoopPlannerPrompt(repoPath, nil)
	if err != nil {
		t.Fatalf("buildLoopPlannerPrompt: %v", err)
	}
	if !strings.Contains(prompt, "CRITICAL") {
		t.Error("prompt should contain CRITICAL enforcement")
	}
	if !strings.Contains(prompt, "ENTIRE response must be a single JSON") {
		t.Error("prompt should contain JSON enforcement language")
	}
	if !strings.Contains(prompt, "BAD (do NOT do this)") {
		t.Error("prompt should contain BAD example")
	}
	if !strings.Contains(prompt, "GOOD (do this)") {
		t.Error("prompt should contain GOOD example")
	}
	if !strings.Contains(prompt, "Output ONLY the JSON object") {
		t.Error("prompt should contain output-only constraint")
	}
}

func TestRecentGitLog(t *testing.T) {
	repoPath := setupLoopRepo(t)

	log, err := recentGitLog(repoPath, 5)
	if err != nil {
		t.Fatalf("recentGitLog: %v", err)
	}
	if log == "" {
		t.Error("expected non-empty git log")
	}
	if !contains(log, "initial") {
		t.Errorf("git log = %q, expected to contain 'initial'", log)
	}
}

func TestSelfImprovementProfile(t *testing.T) {
	p := SelfImprovementProfile()
	if p.PlannerProvider != ProviderClaude {
		t.Errorf("PlannerProvider = %q, want claude", p.PlannerProvider)
	}
	if p.MaxConcurrentWorkers != 1 {
		t.Error("MaxConcurrentWorkers should be 1 for serial self-modification")
	}
	if p.RetryLimit != 2 {
		t.Error("RetryLimit should be 2")
	}
	if !p.SelfImprovement {
		t.Error("SelfImprovement should be true")
	}
	if !p.EnableReflexion || !p.EnableEpisodicMemory || !p.EnableUncertainty || !p.EnableCurriculum {
		t.Error("all self-learning subsystems should be enabled")
	}
	if p.EnableCascade {
		t.Error("cascade should be disabled for self-modification")
	}
	if len(p.VerifyCommands) != 2 {
		t.Errorf("expected 2 verify commands, got %d", len(p.VerifyCommands))
	}
	if p.MaxIterations != 10 {
		t.Errorf("MaxIterations = %d, want 10", p.MaxIterations)
	}
	if p.PlannerModel != "claude-opus-4-6" {
		t.Errorf("PlannerModel = %q, want claude-opus-4-6", p.PlannerModel)
	}
	if p.WorkerModel != "claude-sonnet-4-6" {
		t.Errorf("WorkerModel = %q, want claude-sonnet-4-6", p.WorkerModel)
	}
	if p.MaxDurationSecs != 14400 {
		t.Errorf("MaxDurationSecs = %d, want 14400", p.MaxDurationSecs)
	}
	if !p.EnablePlannerEnhancement {
		t.Error("EnablePlannerEnhancement should be true for self-improvement (opus planner)")
	}
	if p.EnableWorkerEnhancement {
		t.Error("EnableWorkerEnhancement should be false for self-improvement")
	}
}

func TestSelfImprovementProfileDefaults(t *testing.T) {
	p := SelfImprovementProfile()
	if !p.EnablePlannerEnhancement {
		t.Error("SelfImprovementProfile should have EnablePlannerEnhancement=true")
	}
	if p.EnableWorkerEnhancement {
		t.Error("SelfImprovementProfile should have EnableWorkerEnhancement=false")
	}
	if !p.SelfImprovement {
		t.Error("SelfImprovementProfile should have SelfImprovement=true")
	}
}

func TestEnableEnhancementFlags(t *testing.T) {
	profile := LoopProfile{
		EnablePlannerEnhancement: true,
		EnableWorkerEnhancement:  false,
	}

	data, err := json.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)

	// Both fields serialized (no omitempty)
	if !strings.Contains(s, `"enable_planner_enhancement":true`) {
		t.Errorf("missing planner enhancement field: %s", s)
	}
	if !strings.Contains(s, `"enable_worker_enhancement":false`) {
		t.Errorf("missing worker enhancement field: %s", s)
	}

	// Round-trip
	var rt LoopProfile
	if err := json.Unmarshal(data, &rt); err != nil {
		t.Fatal(err)
	}
	if !rt.EnablePlannerEnhancement {
		t.Error("round-trip lost EnablePlannerEnhancement=true")
	}
	if rt.EnableWorkerEnhancement {
		t.Error("round-trip gained EnableWorkerEnhancement=true")
	}
}

func TestStepLoopMaxIterationsEnforced(t *testing.T) {
	repoPath := setupLoopRepo(t)

	m := NewManager()
	m.SetStateDir(t.TempDir())
	m.SetHooksForTesting(
		func(_ context.Context, opts LaunchOptions) (*Session, error) {
			return &Session{
				ID:         sanitizeLoopName(opts.SessionName),
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   "test",
				Prompt:     opts.Prompt,
				Model:      opts.Model,
				Status:     StatusCompleted,
				OutputCh:   make(chan string, 1),
				LaunchedAt: time.Now(),
			}, nil
		},
		func(_ context.Context, sess *Session) error {
			sess.Lock()
			sess.Status = StatusCompleted
			now := time.Now()
			sess.EndedAt = &now
			sess.Unlock()
			return nil
		},
	)

	run, err := m.StartLoop(context.Background(), repoPath, LoopProfile{
		MaxIterations:  1,
		VerifyCommands: []string{"true"},
	})
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	// Manually add a completed iteration so we're at the limit.
	run.Lock()
	run.Iterations = append(run.Iterations, LoopIteration{
		Number: 1,
		Status: "idle",
	})
	run.Unlock()

	err = m.StepLoop(context.Background(), run.ID)
	if err == nil {
		t.Fatal("expected max iterations error")
	}
	if !strings.Contains(err.Error(), "max iterations") {
		t.Fatalf("unexpected error: %v", err)
	}

	run, ok := m.GetLoop(run.ID)
	if !ok {
		t.Fatal("loop not found")
	}
	run.Lock()
	defer run.Unlock()
	if run.Status != "completed" {
		t.Fatalf("status = %q, want completed", run.Status)
	}
}

func TestStepLoopDeadlineEnforced(t *testing.T) {
	repoPath := setupLoopRepo(t)

	m := NewManager()
	m.SetStateDir(t.TempDir())
	m.SetHooksForTesting(
		func(_ context.Context, opts LaunchOptions) (*Session, error) {
			return &Session{
				ID:         sanitizeLoopName(opts.SessionName),
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   "test",
				Prompt:     opts.Prompt,
				Model:      opts.Model,
				Status:     StatusCompleted,
				OutputCh:   make(chan string, 1),
				LaunchedAt: time.Now(),
			}, nil
		},
		func(_ context.Context, sess *Session) error {
			sess.Lock()
			sess.Status = StatusCompleted
			now := time.Now()
			sess.EndedAt = &now
			sess.Unlock()
			return nil
		},
	)

	run, err := m.StartLoop(context.Background(), repoPath, LoopProfile{
		VerifyCommands: []string{"true"},
	})
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	// Set deadline in the past.
	past := time.Now().Add(-1 * time.Hour)
	run.Lock()
	run.Deadline = &past
	run.Unlock()

	err = m.StepLoop(context.Background(), run.ID)
	if err == nil {
		t.Fatal("expected duration limit error")
	}
	if !strings.Contains(err.Error(), "duration limit") {
		t.Fatalf("unexpected error: %v", err)
	}

	run, ok := m.GetLoop(run.ID)
	if !ok {
		t.Fatal("loop not found")
	}
	run.Lock()
	defer run.Unlock()
	if run.Status != "completed" {
		t.Fatalf("status = %q, want completed", run.Status)
	}
}

func TestStepLoopSelectTierSetsWorkerModel(t *testing.T) {
	repoPath := setupLoopRepo(t)
	stateDir := t.TempDir()

	cr := NewCascadeRouter(CascadeConfig{
		CheapProvider:       ProviderGemini,
		ExpensiveProvider:   ProviderClaude,
		ConfidenceThreshold: 0.7,
		MaxCheapBudgetUSD:   2.00,
		MaxCheapTurns:       15,
	}, nil, nil, stateDir)

	// Use default tiers: "test" task type maps to complexity 3 -> "coding" tier
	// (claude-sonnet, provider claude).

	var mu sync.Mutex
	var workerOpts []LaunchOptions

	m := NewManager()
	m.SetStateDir(stateDir)
	m.SetCascadeRouter(cr)
	m.SetHooksForTesting(
		func(_ context.Context, opts LaunchOptions) (*Session, error) {
			mu.Lock()
			workerOpts = append(workerOpts, opts)
			mu.Unlock()

			sess := &Session{
				ID:         sanitizeLoopName(opts.SessionName),
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   filepath.Base(opts.RepoPath),
				Prompt:     opts.Prompt,
				Model:      opts.Model,
				Status:     StatusCompleted,
				OutputCh:   make(chan string, 1),
				LaunchedAt: time.Now(),
			}
			// Planner returns a task with "test" in the title.
			if opts.Model == "o1-pro" {
				sess.LastOutput = `{"title":"Add test for parser","prompt":"Write unit tests for the parser module."}`
				sess.OutputHistory = []string{sess.LastOutput}
			} else {
				sess.LastOutput = "worker complete"
				sess.OutputHistory = []string{"worker complete"}
			}
			return sess, nil
		},
		func(_ context.Context, sess *Session) error {
			sess.Lock()
			sess.Status = StatusCompleted
			now := time.Now()
			sess.EndedAt = &now
			sess.Unlock()
			return nil
		},
	)

	run, err := m.StartLoop(context.Background(), repoPath, LoopProfile{
		EnableCascade:  true,
		VerifyCommands: []string{"true"},
	})
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	if err := m.StepLoop(context.Background(), run.ID); err != nil {
		t.Fatalf("StepLoop: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Expect at least 2 launches: planner + cheap worker (cascade path).
	// The cheap worker should have baseOpts.Model set by SelectTier before
	// CheapLaunchOpts overrides the provider.
	if len(workerOpts) < 2 {
		t.Fatalf("expected at least 2 launches, got %d", len(workerOpts))
	}

	// Find the cheap worker launch (session name ends with "-cheap").
	var found bool
	for _, opts := range workerOpts {
		if strings.HasSuffix(opts.SessionName, "-cheap") {
			found = true
			// SelectTier("test", 0) -> complexity 3 -> "coding" tier -> claude-sonnet.
			// CheapLaunchOpts overrides provider to gemini, but model comes from SelectTier.
			if opts.Model != "claude-sonnet" {
				t.Errorf("cheap worker model = %q, want %q (set by SelectTier)", opts.Model, "claude-sonnet")
			}
			// CheapLaunchOpts should have overridden provider to gemini.
			if opts.Provider != ProviderGemini {
				t.Errorf("cheap worker provider = %q, want %q (set by CheapLaunchOpts)", opts.Provider, ProviderGemini)
			}
			break
		}
	}
	if !found {
		t.Error("no cheap worker launch found (expected session name ending in '-cheap')")
	}
}

