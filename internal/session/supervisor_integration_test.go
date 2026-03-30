package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// TestSupervisorIntegration verifies the full autonomous pipeline:
// sprint planner, budget envelope, config validation, and state persistence/resume.
func TestSupervisorIntegration(t *testing.T) {
	// --- 1. Set up temp dir with fake ROADMAP.md ---
	repoDir := t.TempDir()
	roadmapPath := filepath.Join(repoDir, "ROADMAP.md")
	roadmapContent := `## Test Section
- [ ] Item 1 — small fix ` + "`P0`" + ` ` + "`S`" + `
- [ ] Item 2 — medium feature ` + "`P1`" + ` ` + "`M`" + `
- [ ] Item 3 — another fix ` + "`P0`" + ` ` + "`S`" + `
- [x] Item 4 — already done ` + "`P1`" + ` ` + "`S`" + `
- [ ] Item 5 — low priority ` + "`P2`" + ` ` + "`S`" + `
`
	if err := os.WriteFile(roadmapPath, []byte(roadmapContent), 0644); err != nil {
		t.Fatalf("write ROADMAP.md: %v", err)
	}

	// --- 2. Create Manager ---
	mgr := NewManager()
	mgr.SetStateDir(filepath.Join(repoDir, "sessions"))

	// --- 3. Create Supervisor with all components wired ---
	s := NewSupervisor(mgr, repoDir)
	s.TickInterval = 10 * time.Millisecond

	s.SetMonitor(NewHealthMonitor(DefaultHealthThresholds()))
	s.SetChainer(NewCycleChainer())
	s.SetSprintPlanner(NewSprintPlanner(roadmapPath))
	s.SetStallHandler(NewSupervisorStallHandler())

	gates := DefaultSupervisorGates()
	gates.RequireBuild = false
	gates.RequireTest = false
	gates.RequireVet = false
	s.SetGates(gates)

	s.SetBudget(NewBudgetEnvelope(10.0))
	bus := events.NewBus(100)
	s.SetBus(bus)

	// --- 4. Test sprint planner independently ---
	t.Run("SprintPlanner", func(t *testing.T) {
		planner := NewSprintPlanner(roadmapPath)
		cycle := planner.PlanNextSprint(repoDir)
		if cycle == nil {
			t.Fatal("PlanNextSprint returned nil, expected a CycleRun")
		}
		if len(cycle.Tasks) == 0 {
			t.Fatal("expected at least one task")
		}
		if len(cycle.Tasks) > 5 {
			t.Errorf("expected <= 5 tasks, got %d", len(cycle.Tasks))
		}

		// Verify priority ordering: P0 (1.0) before P1 (0.8) before P2 (0.5).
		for i := 1; i < len(cycle.Tasks); i++ {
			if cycle.Tasks[i].Priority > cycle.Tasks[i-1].Priority {
				t.Errorf("tasks not sorted by priority: task[%d].Priority=%.1f > task[%d].Priority=%.1f",
					i, cycle.Tasks[i].Priority, i-1, cycle.Tasks[i-1].Priority)
			}
		}

		// Verify the checked item (Item 4) is excluded.
		for _, task := range cycle.Tasks {
			if strings.Contains(task.Title, "already done") || strings.Contains(task.Title, "Item 4") {
				t.Errorf("checked item should be excluded: %s", task.Title)
			}
		}

		t.Logf("Sprint planned: %d tasks, objective: %s", len(cycle.Tasks), cycle.Objective)
		for i, task := range cycle.Tasks {
			t.Logf("  task[%d]: %s (priority=%.1f, size=%s)", i, task.Title, task.Priority, task.Size)
		}
	})

	// --- 5. Test budget envelope ---
	t.Run("BudgetEnvelope", func(t *testing.T) {
		budget := NewBudgetEnvelope(10.0)

		if !budget.CanSpend(1.0) {
			t.Error("CanSpend(1.0) should be true initially")
		}

		budget.RecordSpend(9.5)

		if budget.CanSpend(1.0) {
			t.Error("CanSpend(1.0) should be false after spending 9.5 of 10.0")
		}

		remaining := budget.Remaining()
		if remaining != 0.5 {
			t.Errorf("Remaining() = %.2f, want 0.5", remaining)
		}

		if budget.Spent() != 9.5 {
			t.Errorf("Spent() = %.2f, want 9.5", budget.Spent())
		}

		// Verify PerCycleCap defaults to TotalBudget/10.
		if cap := budget.PerCycleCap(); cap != 1.0 {
			t.Errorf("PerCycleCap() = %.2f, want 1.0", cap)
		}
	})

	// --- 6. Test config validation ---
	t.Run("ConfigValidation", func(t *testing.T) {
		// Create a valid repo structure.
		validRepo := t.TempDir()
		gitDir := filepath.Join(validRepo, ".git")
		if err := os.Mkdir(gitDir, 0755); err != nil {
			t.Fatalf("mkdir .git: %v", err)
		}
		ralphDir := filepath.Join(validRepo, ".ralph")
		if err := os.Mkdir(ralphDir, 0755); err != nil {
			t.Fatalf("mkdir .ralph: %v", err)
		}
		validRoadmap := filepath.Join(validRepo, "ROADMAP.md")
		if err := os.WriteFile(validRoadmap, []byte("- [ ] unchecked item\n"), 0644); err != nil {
			t.Fatalf("write ROADMAP.md: %v", err)
		}

		vr := ValidateConfig(validRepo)
		if !vr.OK() {
			t.Errorf("ValidateConfig should be OK, got errors: %v", vr.Errors)
		}

		// Delete .git and verify validation fails.
		if err := os.RemoveAll(gitDir); err != nil {
			t.Fatalf("remove .git: %v", err)
		}
		vr2 := ValidateConfig(validRepo)
		if vr2.OK() {
			t.Error("ValidateConfig should fail without .git directory")
		}

		foundGitError := false
		for _, e := range vr2.Errors {
			if strings.Contains(e, ".git") {
				foundGitError = true
				break
			}
		}
		if !foundGitError {
			t.Errorf("expected .git-related error, got: %v", vr2.Errors)
		}
	})

	// --- 7. Test supervisor state persistence and resume ---
	t.Run("StatePersistenceAndResume", func(t *testing.T) {
		persistDir := t.TempDir()
		ralphDir := filepath.Join(persistDir, ".ralph")
		if err := os.MkdirAll(ralphDir, 0755); err != nil {
			t.Fatalf("mkdir .ralph: %v", err)
		}

		// Create a supervisor and set some state.
		mgr1 := NewManager()
		s1 := NewSupervisor(mgr1, persistDir)
		s1.mu.Lock()
		s1.tickCount = 42
		s1.lastCycleLaunch = time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)
		s1.running = true
		s1.mu.Unlock()

		// Persist the state.
		s1.persistState()

		// Verify the state file was created.
		statePath := filepath.Join(ralphDir, "supervisor_state.json")
		data, err := os.ReadFile(statePath)
		if err != nil {
			t.Fatalf("read supervisor_state.json: %v", err)
		}

		var state SupervisorState
		if err := json.Unmarshal(data, &state); err != nil {
			t.Fatalf("unmarshal state: %v", err)
		}
		if state.TickCount != 42 {
			t.Errorf("persisted TickCount = %d, want 42", state.TickCount)
		}

		// Create a new supervisor and resume from persisted state.
		mgr2 := NewManager()
		s2 := NewSupervisor(mgr2, persistDir)
		if err := s2.ResumeFromState(); err != nil {
			t.Fatalf("ResumeFromState: %v", err)
		}

		s2.mu.Lock()
		restoredTick := s2.tickCount
		restoredLaunch := s2.lastCycleLaunch
		s2.mu.Unlock()

		if restoredTick != 42 {
			t.Errorf("restored tickCount = %d, want 42", restoredTick)
		}
		if !restoredLaunch.Equal(time.Date(2026, 3, 29, 10, 0, 0, 0, time.UTC)) {
			t.Errorf("restored lastCycleLaunch = %v, want 2026-03-29 10:00:00 UTC", restoredLaunch)
		}
	})
}

// TestSupervisorComponentsWired verifies all setters work without panicking
// and the supervisor can Start/Stop cleanly with all components attached.
func TestSupervisorComponentsWired(t *testing.T) {
	dir := t.TempDir()
	mgr := NewManager()
	mgr.SetStateDir(filepath.Join(dir, "sessions"))

	s := NewSupervisor(mgr, dir)
	s.TickInterval = 10 * time.Millisecond

	// Wire all components — none of these should panic.
	s.SetMonitor(NewHealthMonitor(DefaultHealthThresholds()))
	s.SetChainer(NewCycleChainer())
	s.SetSprintPlanner(NewSprintPlanner(filepath.Join(dir, "ROADMAP.md")))
	s.SetStallHandler(NewSupervisorStallHandler())

	gates := DefaultSupervisorGates()
	gates.RequireBuild = false
	gates.RequireTest = false
	gates.RequireVet = false
	s.SetGates(gates)

	s.SetBudget(NewBudgetEnvelope(10.0))

	bus := events.NewBus(100)
	s.SetBus(bus)

	dl := NewDecisionLog("", LevelAutoOptimize)
	s.SetDecisionLog(dl)

	// Verify the supervisor starts and stops cleanly.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !s.Running() {
		t.Fatal("expected running after Start")
	}

	// Let it tick a couple of times.
	time.Sleep(50 * time.Millisecond)

	s.Stop()
	if s.Running() {
		t.Fatal("expected not running after Stop")
	}

	// Verify at least one tick happened.
	if s.TickCount() < 1 {
		t.Errorf("tickCount = %d, want >= 1", s.TickCount())
	}

	// Verify status snapshot works after stop.
	st := s.Status()
	if st.RepoPath != dir {
		t.Errorf("Status().RepoPath = %q, want %q", st.RepoPath, dir)
	}
}

// TestSupervisor_FullCycleLifecycle exercises the complete chain:
// supervisor tick -> health signal -> decision proposal -> cycle launch ->
// RunCycle (stubbed) -> completion tracking -> event bus publication.
func TestSupervisor_FullCycleLifecycle(t *testing.T) {
	disableCycleSafety(t)
	origPoll := cyclePollInterval
	cyclePollInterval = 10 * time.Millisecond
	defer func() { cyclePollInterval = origPoll }()

	// --- Setup ---
	s, dir := newTestSupervisor(t)
	s.CooldownBetween = 0 // no cooldown between cycles
	s.MaxCycles = 1        // terminate after one cycle

	// Decision log at autonomy level 2 (auto-optimize).
	dl := NewDecisionLog("", LevelAutoOptimize)
	s.SetDecisionLog(dl)

	// Event bus for verifying published events.
	bus := events.NewBus(100)
	s.SetBus(bus)
	ch := bus.Subscribe("integration-test")

	// Stub the manager's Launch so RunCycle completes without real processes.
	var launchCount atomic.Int32
	s.mgr.launchSession = func(_ context.Context, opts LaunchOptions) (*Session, error) {
		n := launchCount.Add(1)
		id := fmt.Sprintf("integ-sess-%d", n)
		sess := &Session{
			ID:       id,
			Provider: ProviderClaude,
			RepoPath: opts.RepoPath,
			RepoName: "test",
			Status:   StatusCompleted,
			Prompt:   opts.Prompt,
		}
		s.mgr.sessionsMu.Lock()
		s.mgr.sessions[id] = sess
		s.mgr.sessionsMu.Unlock()
		return sess, nil
	}

	// Health monitor that emits a DecisionLaunch signal on first call,
	// then nothing (so the supervisor terminates via MaxCycles).
	var evalCount atomic.Int32
	s.SetMonitor(&HealthMonitor{
		EvaluateFunc: func(_ string) []HealthSignal {
			if evalCount.Add(1) == 1 {
				return []HealthSignal{{
					Category:        DecisionLaunch,
					Metric:          "idle_time",
					Value:           300,
					Threshold:       60,
					Rationale:       "repo idle, launch R&D cycle",
					SuggestedAction: "launch",
				}}
			}
			return nil
		},
	})

	// --- Act ---
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for the supervisor to process at least one cycle launch.
	// The cycle runs in a goroutine, so give it time to complete.
	deadline := time.After(8 * time.Second)
	for {
		select {
		case <-deadline:
			s.Stop()
			t.Fatal("timed out waiting for cycle completion")
		case <-time.After(50 * time.Millisecond):
		}

		s.mu.Lock()
		launched := s.cyclesLaunched
		s.mu.Unlock()
		if launched >= 1 {
			break
		}
	}

	// Give the async RunCycle goroutine time to record outcome.
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	// --- Assert ---

	// 1. Supervisor launched exactly 1 cycle.
	s.mu.Lock()
	launched := s.cyclesLaunched
	s.mu.Unlock()
	if launched != 1 {
		t.Errorf("cyclesLaunched = %d, want 1", launched)
	}

	// 2. Decision log has the launch decision recorded.
	decisions := dl.Recent(10)
	if len(decisions) == 0 {
		t.Fatal("expected at least one decision in log")
	}
	found := false
	for _, d := range decisions {
		if d.Category == DecisionLaunch && d.Executed {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected an executed DecisionLaunch decision")
	}

	// 3. Event bus received LoopStarted from the cycle launch.
	var gotLoopStarted, gotAutoOptimized bool
	drainTimeout := time.After(500 * time.Millisecond)
drain:
	for {
		select {
		case evt := <-ch:
			switch evt.Type {
			case events.LoopStarted:
				gotLoopStarted = true
			case events.AutoOptimized:
				gotAutoOptimized = true
			}
		case <-drainTimeout:
			break drain
		}
	}
	if !gotLoopStarted {
		t.Error("expected LoopStarted event on bus")
	}
	if !gotAutoOptimized {
		t.Error("expected AutoOptimized event on bus")
	}

	// 4. Verify the cycle exists on disk and reached a terminal state.
	cycles, err := ListCycles(dir)
	if err != nil {
		t.Fatalf("ListCycles: %v", err)
	}
	if len(cycles) == 0 {
		t.Fatal("expected at least one persisted cycle")
	}
	// Cycle should be complete (no tasks from empty repo = fast path to synthesis).
	lastCycle := cycles[len(cycles)-1]
	if lastCycle.Phase != CycleComplete && lastCycle.Phase != CycleFailed {
		t.Errorf("cycle phase = %s, want complete or failed", lastCycle.Phase)
	}

	// 5. Supervisor state was persisted.
	st := s.Status()
	if st.TickCount < 1 {
		t.Errorf("tickCount = %d, want >= 1", st.TickCount)
	}
}

// TestSupervisor_BudgetTermination verifies the supervisor stops when
// the cost budget is exhausted.
func TestSupervisor_BudgetTermination(t *testing.T) {
	s, dir := newTestSupervisor(t)
	s.MaxTotalCostUSD = 0.01 // very low budget

	// Set startedAt to before the observation so the filter includes it.
	s.mu.Lock()
	s.startedAt = time.Now().Add(-1 * time.Hour)
	s.mu.Unlock()

	// Write a cost observation to the path the supervisor actually reads.
	obsPath := filepath.Join(dir, ".ralph", "cost_observations.json")
	obs := LoopObservation{
		Timestamp:    time.Now(),
		LoopID:       "budget-test",
		TotalCostUSD: 0.05, // exceeds $0.01 cap
		Status:       "pass",
	}
	if err := WriteObservation(obsPath, obs); err != nil {
		t.Fatalf("write observation: %v", err)
	}

	reason := s.shouldTerminate()
	if reason == "" {
		t.Fatal("expected budget termination")
	}
	if !contains(reason, "budget") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

// TestSupervisor_DecisionOutcomeRecorded verifies that after a cycle
// completes, the decision log has an outcome attached.
func TestSupervisor_DecisionOutcomeRecorded(t *testing.T) {
	disableCycleSafety(t)
	origPoll := cyclePollInterval
	cyclePollInterval = 10 * time.Millisecond
	defer func() { cyclePollInterval = origPoll }()

	s, _ := newTestSupervisor(t)
	s.CooldownBetween = 0

	dl := NewDecisionLog("", LevelAutoOptimize)
	s.SetDecisionLog(dl)

	// Stub Launch for fast completion.
	s.mgr.launchSession = func(_ context.Context, opts LaunchOptions) (*Session, error) {
		sess := &Session{
			ID:       "outcome-sess",
			Provider: ProviderClaude,
			RepoPath: opts.RepoPath,
			RepoName: "test",
			Status:   StatusCompleted,
			Prompt:   opts.Prompt,
		}
		s.mgr.sessionsMu.Lock()
		s.mgr.sessions["outcome-sess"] = sess
		s.mgr.sessionsMu.Unlock()
		return sess, nil
	}

	// Launch a cycle via the supervisor's launchCycle method directly.
	signal := HealthSignal{
		Category: DecisionLaunch, Metric: "idle_time",
		Value: 300, Threshold: 60, Rationale: "test",
	}
	s.executeDecision(context.Background(), signal)

	// Wait for the async goroutine to complete and record outcome.
	time.Sleep(500 * time.Millisecond)

	decisions := dl.Recent(10)
	var decisionWithOutcome *AutonomousDecision
	for i := range decisions {
		if decisions[i].Category == DecisionLaunch && decisions[i].Outcome != nil {
			decisionWithOutcome = &decisions[i]
			break
		}
	}
	if decisionWithOutcome == nil {
		t.Fatal("expected decision with recorded outcome")
	}
	if !decisionWithOutcome.Outcome.Success {
		t.Errorf("outcome success = false, details: %s", decisionWithOutcome.Outcome.Details)
	}
}

// TestSupervisor_ReflexionLoop verifies that runConsolidation generates
// improvement notes from consolidated patterns and auto-applies eligible ones.
func TestSupervisor_ReflexionLoop(t *testing.T) {
	s, dir := newTestSupervisor(t)
	ralphDir := filepath.Join(dir, ".ralph")
	os.MkdirAll(ralphDir, 0o755)

	// Write journal entries with recurring patterns so consolidation produces rules.
	journalPath := filepath.Join(ralphDir, "improvement_journal.jsonl")
	for i := 0; i < 5; i++ {
		entry := JournalEntry{
			Timestamp: time.Now().Add(-time.Duration(i) * time.Hour),
			Worked:    []string{"use gemini for refactoring tasks"},
			Failed:    []string{"budget exceeded on large prompts"},
		}
		data, _ := json.Marshal(entry)
		data = append(data, '\n')
		f, _ := os.OpenFile(journalPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		f.Write(data)
		f.Close()
	}

	// Set up optimizer so notes get generated.
	dl := NewDecisionLog("", LevelAutoOptimize)
	ao := NewAutoOptimizer(nil, dl, nil, nil)
	s.SetOptimizer(ao)
	s.SetDecisionLog(dl)

	// Run consolidation directly.
	s.runConsolidation()

	// Verify patterns were consolidated.
	patternsPath := filepath.Join(ralphDir, "improvement_patterns.json")
	if _, err := os.Stat(patternsPath); err != nil {
		t.Fatalf("expected improvement_patterns.json, got: %v", err)
	}

	// Verify improvement notes were generated.
	notesPath := filepath.Join(ralphDir, "improvement_notes.jsonl")
	notesData, err := os.ReadFile(notesPath)
	if err != nil {
		t.Fatalf("expected improvement_notes.jsonl, got: %v", err)
	}
	if len(notesData) == 0 {
		t.Fatal("expected non-empty improvement notes")
	}

	// Verify at least one note exists.
	notes, err := ReadPendingNotes(dir)
	if err != nil {
		t.Fatalf("ReadPendingNotes: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("expected at least one improvement note")
	}
	t.Logf("generated %d improvement notes", len(notes))
	for _, n := range notes {
		t.Logf("  [%s] %s (status=%s, auto=%v)", n.Category, n.Title, n.Status, n.AutoApply)
	}
}
