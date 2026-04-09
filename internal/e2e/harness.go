package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Harness wraps a session.Manager with mock hooks for e2e scenario testing.
type Harness struct {
	Manager  *session.Manager
	stateDir string
	t        *testing.T
	idSeq    atomic.Int64
}

// NewHarness creates a harness backed by a Manager with mock hooks.
func NewHarness(t *testing.T) *Harness {
	t.Helper()
	m := session.NewManager()
	m.SetStateDir(t.TempDir())

	return &Harness{
		Manager:  m,
		stateDir: t.TempDir(),
		t:        t,
	}
}

// RunScenario executes a single scenario through the full StepLoop lifecycle.
// It sets up mock launch/wait hooks that:
//  1. Return sessions with planner output (so plannerTasksFromSession can parse tasks)
//  2. Execute WorkerBehavior on the worktree (simulating what an LLM worker would write)
//  3. Populate SpentUSD and TurnCount from the scenario's mock values
//
// Returns the final loop status and any error from StepLoop.
func (h *Harness) RunScenario(ctx context.Context, s Scenario) (string, error) {
	repoPath := s.RepoSetup(h.t)

	h.Manager.SetHooksForTesting(
		// launch hook
		func(_ context.Context, opts session.LaunchOptions) (*session.Session, error) {
			id := fmt.Sprintf("mock-%d", h.idSeq.Add(1))
			now := time.Now()
			sess := &session.Session{
				ID:           id,
				Provider:     opts.Provider,
				RepoPath:     opts.RepoPath,
				Status:       session.StatusCompleted,
				Prompt:       opts.Prompt,
				Model:        opts.Model,
				SpentUSD:     s.MockCostUSD,
				TurnCount:    s.MockTurnCount,
				LaunchedAt:   now,
				LastActivity: now,
			}

			if strings.Contains(opts.SessionName, "loop-phase0-") {
				// Phase 0 analysis session
				sess.LastOutput = "Simulated first-principles analysis."
			} else if strings.Contains(opts.SessionName, "loop-plan-") {
				// Planner session
				sess.LastOutput = s.PlannerResponse
			} else if s.MockFailure != "" {
				// Simulate infrastructure failure (budget, timeout, CB, etc.)
				sess.Status = session.StatusErrored
				sess.Error = s.MockFailure
				sess.LastOutput = "error: " + s.MockFailure
			} else {
				// This is a worker session — execute WorkerBehavior on the worktree
				if s.WorkerBehavior != nil {
					if err := s.WorkerBehavior(opts.RepoPath); err != nil {
						sess.Status = session.StatusErrored
						sess.Error = err.Error()
					}
				}
				sess.LastOutput = "worker completed"
			}

			h.Manager.AddSessionForTesting(sess)
			return sess, nil
		},
		// wait hook — sessions are already completed
		func(_ context.Context, sess *session.Session) error {
			if sess.Status == session.StatusErrored {
				return fmt.Errorf("%s", sess.Error)
			}
			return nil
		},
	)

	// Configure subsystems on manager if scenario specifies it.
	if s.ManagerSetup != nil {
		s.ManagerSetup(h.Manager)
	}

	// Configure profile with scenario's verify commands
	profile := session.LoopProfile{
		PlannerProvider:      session.DefaultPrimaryProvider(),
		PlannerModel:         "mock",
		WorkerProvider:       session.DefaultPrimaryProvider(),
		WorkerModel:          "mock",
		VerifierProvider:     session.DefaultPrimaryProvider(),
		VerifierModel:        "mock",
		MaxConcurrentWorkers: 1,
		RetryLimit:           1,
		VerifyCommands:       s.VerifyCommands,
		WorktreePolicy:       "git",
	}
	if s.ProfilePatch != nil {
		s.ProfilePatch(&profile)
	}

	run, err := h.Manager.StartLoop(ctx, repoPath, profile)
	if err != nil {
		return "", fmt.Errorf("start loop: %w", err)
	}

	stepErr := h.Manager.StepLoop(ctx, run.ID)

	// Read final status
	updated, ok := h.Manager.GetLoop(run.ID)
	if !ok {
		return "", fmt.Errorf("loop disappeared after step")
	}
	updated.Lock()
	defer updated.Unlock()

	if len(updated.Iterations) == 0 {
		return "", fmt.Errorf("no iterations recorded")
	}
	lastIter := updated.Iterations[len(updated.Iterations)-1]
	return lastIter.Status, stepErr
}

// RunAll executes all scenarios and returns results.
func (h *Harness) RunAll(ctx context.Context, scenarios []Scenario) []ScenarioResult {
	results := make([]ScenarioResult, len(scenarios))
	for i, s := range scenarios {
		status, err := h.RunScenario(ctx, s)
		results[i] = ScenarioResult{
			Scenario: s,
			Status:   status,
			Error:    err,
		}
	}
	return results
}

// ScenarioResult captures the outcome of running one scenario.
type ScenarioResult struct {
	Scenario Scenario
	Status   string
	Error    error
}

// ObservationJSON returns the JSONL observation written during the scenario.
func (r *ScenarioResult) ObservationJSON() string {
	obs := session.LoopObservation{
		Timestamp:    time.Now(),
		RepoName:     "e2e-" + r.Scenario.Name,
		Status:       r.Status,
		TaskType:     r.Scenario.Category,
		TaskTitle:    r.Scenario.Name,
		TotalCostUSD: r.Scenario.MockCostUSD * 2, // planner + worker
		Mode:         "mock",
	}
	data, _ := json.Marshal(obs)
	return string(data)
}
