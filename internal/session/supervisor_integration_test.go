package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// TestSupervisor_FullCycleLifecycle exercises the complete chain:
// supervisor tick → health signal → decision proposal → cycle launch →
// RunCycle (stubbed) → completion tracking → event bus publication.
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
		s.mgr.mu.Lock()
		s.mgr.sessions[id] = sess
		s.mgr.mu.Unlock()
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
		s.mgr.mu.Lock()
		s.mgr.sessions["outcome-sess"] = sess
		s.mgr.mu.Unlock()
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
