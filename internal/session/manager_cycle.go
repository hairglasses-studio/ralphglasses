package session

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"
)

// CycleSafety holds the safety configuration for cycle operations.
// If nil, DefaultCycleSafety is used.
var CycleSafety *CycleSafetyConfig

func cycleSafetyConfig() CycleSafetyConfig {
	if CycleSafety != nil {
		return *CycleSafety
	}
	return DefaultCycleSafety
}

// CreateCycle creates a new CycleRun in the proposed phase and persists it.
func (m *Manager) CreateCycle(repoPath, name, objective string, criteria []string) (*CycleRun, error) {
	// Safety: enforce concurrent cycle limit.
	existing, _ := ListCycles(repoPath)
	if err := ValidateCycleStart(repoPath, existing, cycleSafetyConfig()); err != nil {
		return nil, err
	}

	cycle := NewCycleRun(name, repoPath, objective, criteria)
	if err := SaveCycle(repoPath, cycle); err != nil {
		return nil, fmt.Errorf("persist new cycle: %w", err)
	}
	slog.Info("cycle created", "cycle_id", cycle.ID, "name", name, "repo", repoPath)
	return cycle, nil
}

// GetCycle loads a cycle by ID from disk.
func (m *Manager) GetCycle(repoPath, cycleID string) (*CycleRun, error) {
	return LoadCycle(repoPath, cycleID)
}

// GetActiveCycle returns the first non-terminal cycle for a repo, or nil.
func (m *Manager) GetActiveCycle(repoPath string) (*CycleRun, error) {
	return ActiveCycle(repoPath)
}

// ListCycles returns all cycles for a repo, sorted by UpdatedAt desc.
func (m *Manager) ListCycles(repoPath string) ([]*CycleRun, error) {
	return ListCycles(repoPath)
}

// AdvanceCycle transitions a cycle to the next phase and persists it.
func (m *Manager) AdvanceCycle(cycle *CycleRun, opts ...AdvanceOption) error {
	// Safety: validate before advancing.
	if err := ValidateCycleAdvance(cycle, cycleSafetyConfig(), opts...); err != nil {
		return err
	}

	next, ok := validTransitions[cycle.Phase]
	if !ok {
		return fmt.Errorf("no valid transition from %s", cycle.Phase)
	}
	if err := cycle.Advance(next); err != nil {
		return err
	}
	if err := SaveCycle(cycle.RepoPath, cycle); err != nil {
		return fmt.Errorf("persist cycle after advance to %s: %w", next, err)
	}
	slog.Info("cycle advanced", "cycle_id", cycle.ID, "phase", next)
	return nil
}

// FailCycle marks a cycle as failed with an error message and persists it.
func (m *Manager) FailCycle(cycle *CycleRun, errMsg string) error {
	cycle.Fail(errMsg)
	if err := SaveCycle(cycle.RepoPath, cycle); err != nil {
		return fmt.Errorf("persist failed cycle: %w", err)
	}
	slog.Warn("cycle failed", "cycle_id", cycle.ID, "error", errMsg)
	return nil
}

// PlanCycleTasks generates tasks from observations and roadmap, populates the
// cycle's Tasks slice, and persists. The cycle must be in the baselining phase
// (about to advance to executing).
func (m *Manager) PlanCycleTasks(cycle *CycleRun, observations []LoopObservation, maxTasks int) error {
	if cycle.Phase != CycleBaselining {
		return fmt.Errorf("PlanCycleTasks requires baselining phase, got %s", cycle.Phase)
	}

	// Generate tasks from observations.
	obsTasks := ObservationsToTasks(observations)

	// Generate tasks from ROADMAP.md if it exists.
	roadmapPath := filepath.Join(cycle.RepoPath, "ROADMAP.md")
	roadmapTasks, _ := RoadmapToTasks(roadmapPath, maxTasks) // ignore error if no roadmap

	// Merge: observation tasks first (higher priority), then roadmap tasks.
	all := append(obsTasks, roadmapTasks...)

	// Deduplicate across both sources.
	all = deduplicateTasks(all, 0.7)

	// Enhance task prompts with the local deterministic pipeline.
	all = EnhanceCycleTasks(all)

	// Cap to maxTasks.
	if maxTasks > 0 && len(all) > maxTasks {
		all = all[:maxTasks]
	}

	cycle.Tasks = all
	cycle.UpdatedAt = timeNow()

	if err := SaveCycle(cycle.RepoPath, cycle); err != nil {
		return fmt.Errorf("persist cycle tasks: %w", err)
	}
	slog.Info("cycle tasks planned", "cycle_id", cycle.ID, "task_count", len(all))
	return nil
}

// LaunchCycleTask starts a loop for the given task index and records the loop ID.
// The cycle must be in the executing phase.
func (m *Manager) LaunchCycleTask(ctx context.Context, cycle *CycleRun, taskIdx int, opts LaunchOptions) (string, error) {
	if cycle.Phase != CycleExecuting {
		return "", fmt.Errorf("LaunchCycleTask requires executing phase, got %s", cycle.Phase)
	}
	if taskIdx < 0 || taskIdx >= len(cycle.Tasks) {
		return "", fmt.Errorf("task index %d out of range (0-%d)", taskIdx, len(cycle.Tasks)-1)
	}

	task := &cycle.Tasks[taskIdx]
	if task.Status != "pending" {
		return "", fmt.Errorf("task %q is %s, not pending", task.Title, task.Status)
	}

	// Override the prompt with the cycle task prompt.
	opts.Prompt = task.Prompt
	if opts.RepoPath == "" {
		opts.RepoPath = cycle.RepoPath
	}

	sess, err := m.Launch(ctx, opts)
	if err != nil {
		task.Status = "failed"
		_ = SaveCycle(cycle.RepoPath, cycle)
		return "", fmt.Errorf("launch loop for task %q: %w", task.Title, err)
	}

	task.Status = "executing"
	task.LoopID = sess.ID
	cycle.LoopIDs = append(cycle.LoopIDs, sess.ID)
	cycle.UpdatedAt = timeNow()

	if err := SaveCycle(cycle.RepoPath, cycle); err != nil {
		return sess.ID, fmt.Errorf("persist after launch: %w", err)
	}
	slog.Info("cycle task launched", "cycle_id", cycle.ID, "task", task.Title, "session_id", sess.ID)
	return sess.ID, nil
}

// CollectCycleFindings gathers findings from loop observations associated with
// the cycle's loop IDs. The cycle must be in the observing phase.
func (m *Manager) CollectCycleFindings(cycle *CycleRun, observations []LoopObservation) error {
	if cycle.Phase != CycleObserving {
		return fmt.Errorf("CollectCycleFindings requires observing phase, got %s", cycle.Phase)
	}

	loopSet := make(map[string]bool, len(cycle.LoopIDs))
	for _, id := range cycle.LoopIDs {
		loopSet[id] = true
	}

	findingNum := len(cycle.Findings)
	for _, obs := range observations {
		if !loopSet[obs.LoopID] {
			continue
		}

		var category, severity string
		switch obs.Status {
		case "failed":
			category, severity = "failure", "high"
		case "regressed":
			category, severity = "regression", "critical"
		case "noop":
			category, severity = "no-op", "low"
		case "stalled":
			category, severity = "stall", "medium"
		default:
			continue // skip pass/completed observations
		}

		findingNum++
		cycle.AddFinding(CycleFinding{
			ID:          fmt.Sprintf("CF-%d", findingNum),
			Description: fmt.Sprintf("[%s] %s: %s", obs.LoopID, obs.TaskTitle, obs.Error),
			Category:    category,
			Severity:    severity,
			Source:      "observation",
		})
	}

	if err := SaveCycle(cycle.RepoPath, cycle); err != nil {
		return fmt.Errorf("persist findings: %w", err)
	}
	slog.Info("cycle findings collected", "cycle_id", cycle.ID, "finding_count", len(cycle.Findings))
	return nil
}

// SetCycleSynthesis sets the synthesis on a cycle in the synthesizing phase and persists.
func (m *Manager) SetCycleSynthesis(cycle *CycleRun, synthesis CycleSynthesis) error {
	if cycle.Phase != CycleSynthesizing {
		return fmt.Errorf("SetCycleSynthesis requires synthesizing phase, got %s", cycle.Phase)
	}
	cycle.SetSynthesis(synthesis)
	if err := SaveCycle(cycle.RepoPath, cycle); err != nil {
		return fmt.Errorf("persist synthesis: %w", err)
	}
	slog.Info("cycle synthesis set", "cycle_id", cycle.ID)
	return nil
}

// cyclePollInterval is the polling interval for RunCycle's wait loop (overridable in tests).
var cyclePollInterval = 2 * time.Second

// RunCycle drives a full R&D cycle through all phases synchronously.
// It creates a cycle, gathers observations, plans tasks, launches them,
// waits for completion, collects findings, synthesizes, and completes.
func (m *Manager) RunCycle(ctx context.Context, repoPath, name, objective string, criteria []string, maxTasks int) (*CycleRun, error) {
	// 1. Create the cycle.
	cycle, err := m.CreateCycle(repoPath, name, objective, criteria)
	if err != nil {
		return nil, fmt.Errorf("create cycle: %w", err)
	}
	slog.Info("RunCycle: created", "cycle_id", cycle.ID)

	fail := func(msg string) (*CycleRun, error) {
		_ = m.FailCycle(cycle, msg)
		// Write a failure observation so the health monitor can detect and
		// self-correct. Without this, failed cycles are invisible to the
		// supervisor's signal evaluation.
		obs := LoopObservation{
			Timestamp: timeNow(),
			LoopID:    "cycle:" + cycle.ID,
			RepoName:  filepath.Base(repoPath),
			Status:    "cycle_failed",
			Error:     msg,
		}
		if err := WriteObservation(ObservationPath(repoPath), obs); err != nil {
			slog.Warn("RunCycle: failed to write failure observation", "error", err)
		}
		return cycle, fmt.Errorf("%s", msg)
	}

	// 2. Advance proposed → baselining.
	if err := m.AdvanceCycle(cycle); err != nil {
		return fail(fmt.Sprintf("advance to baselining: %v", err))
	}

	// 3. Gather observations.
	obsPath := ObservationPath(repoPath)
	observations, err := LoadObservations(obsPath, time.Time{})
	if err != nil {
		return fail(fmt.Sprintf("load observations: %v", err))
	}

	// 4. Plan tasks from observations.
	if err := m.PlanCycleTasks(cycle, observations, maxTasks); err != nil {
		return fail(fmt.Sprintf("plan tasks: %v", err))
	}

	// 4b. Establish baseline from observations so the require_baseline safety
	// gate passes. Without this, cycles with RequireBaseline=true would always
	// fail when advancing from baselining to executing (QW-6).
	if cycle.BaselineID == "" && len(observations) > 0 {
		baseline := BaselineFromObservations(observations)
		if baseline != nil && !baseline.IsZero() {
			cycle.BaselineID = fmt.Sprintf("auto-%s", cycle.ID[:8])
			cycle.UpdatedAt = timeNow()
			if err := SaveCycle(cycle.RepoPath, cycle); err != nil {
				return fail(fmt.Sprintf("persist baseline ID: %v", err))
			}
		}
	}

	// All advances within RunCycle are part of a single logical operation,
	// so skip phase cooldown to avoid blocking rapid sequential transitions.
	batch := WithBatchAdvance()

	// If no tasks were planned, synthesize immediately.
	if len(cycle.Tasks) == 0 {
		if err := m.AdvanceCycle(cycle, batch); err != nil { // baselining → executing
			return fail(fmt.Sprintf("advance to executing (empty): %v", err))
		}
		if err := m.AdvanceCycle(cycle, batch); err != nil { // executing → observing
			return fail(fmt.Sprintf("advance to observing (empty): %v", err))
		}
		if err := m.AdvanceCycle(cycle, batch); err != nil { // observing → synthesizing
			return fail(fmt.Sprintf("advance to synthesizing (empty): %v", err))
		}
		synthesis := CycleSynthesis{
			Summary:   "No tasks planned — cycle completed with no work.",
			Remaining: []string{objective},
		}
		if err := m.SetCycleSynthesis(cycle, synthesis); err != nil {
			return fail(fmt.Sprintf("set synthesis (empty): %v", err))
		}
		if err := m.AdvanceCycle(cycle, batch); err != nil { // synthesizing → complete
			return fail(fmt.Sprintf("advance to complete (empty): %v", err))
		}
		return cycle, nil
	}

	// 5. Advance baselining → executing.
	if err := m.AdvanceCycle(cycle, batch); err != nil {
		return fail(fmt.Sprintf("advance to executing: %v", err))
	}

	// 6. Launch each pending task.
	var loopIDs []string
	for i := range cycle.Tasks {
		if cycle.Tasks[i].Status != "pending" {
			continue
		}
		opts := LaunchOptions{
			Provider:     DefaultPrimaryProvider(),
			RepoPath:     repoPath,
			SessionName:  fmt.Sprintf("%s-task-%d", name, i),
			MaxTurns:     20,
			MaxBudgetUSD: 1.0,
		}
		loopID, err := m.LaunchCycleTask(ctx, cycle, i, opts)
		if err != nil {
			slog.Warn("RunCycle: task launch failed", "task_idx", i, "error", err)
			continue // non-fatal: other tasks may still succeed
		}
		loopIDs = append(loopIDs, loopID)
	}

	// 7. Wait for all launched loops to finish (poll with 2s interval).
	if err := m.waitForLoops(ctx, loopIDs); err != nil {
		return fail(fmt.Sprintf("wait for loops: %v", err))
	}

	// 8. Advance executing → observing.
	if err := m.AdvanceCycle(cycle, batch); err != nil {
		return fail(fmt.Sprintf("advance to observing: %v", err))
	}

	// 9. Re-gather observations and collect findings.
	observations2, err := LoadObservations(obsPath, time.Time{})
	if err != nil {
		return fail(fmt.Sprintf("reload observations: %v", err))
	}
	if err := m.CollectCycleFindings(cycle, observations2); err != nil {
		return fail(fmt.Sprintf("collect findings: %v", err))
	}

	// 10. Advance observing → synthesizing.
	if err := m.AdvanceCycle(cycle, batch); err != nil {
		return fail(fmt.Sprintf("advance to synthesizing: %v", err))
	}

	// 11. Build synthesis from findings.
	synthesis := buildSynthesisFromFindings(cycle)
	if err := m.SetCycleSynthesis(cycle, synthesis); err != nil {
		return fail(fmt.Sprintf("set synthesis: %v", err))
	}

	// 12. Advance synthesizing → complete.
	if err := m.AdvanceCycle(cycle, batch); err != nil {
		return fail(fmt.Sprintf("advance to complete: %v", err))
	}

	// 13. Return the cycle.
	slog.Info("RunCycle: complete", "cycle_id", cycle.ID, "tasks", len(cycle.Tasks), "findings", len(cycle.Findings))
	return cycle, nil
}

// waitForLoops polls session status for the given loop IDs until all are
// non-running or the context is cancelled.
func (m *Manager) waitForLoops(ctx context.Context, loopIDs []string) error {
	if len(loopIDs) == 0 {
		return nil
	}

	pending := make(map[string]bool, len(loopIDs))
	for _, id := range loopIDs {
		pending[id] = true
	}

	for len(pending) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		for id := range pending {
			sess, ok := m.Get(id)
			if !ok {
				// Session not in memory — check if it was a loop ID instead.
				loop, lok := m.GetLoop(id)
				if !lok {
					delete(pending, id) // unknown — consider done
					continue
				}
				if loop.Status != "running" && loop.Status != "pending" {
					delete(pending, id)
				}
				continue
			}
			if sess.Status.IsTerminal() {
				delete(pending, id)
			}
		}

		if len(pending) > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(cyclePollInterval):
			}
		}
	}
	return nil
}

// buildSynthesisFromFindings constructs a basic CycleSynthesis from a cycle's
// tasks and findings.
func buildSynthesisFromFindings(cycle *CycleRun) CycleSynthesis {
	var accomplished, remaining []string
	var patterns []string

	for _, t := range cycle.Tasks {
		switch t.Status {
		case "done":
			accomplished = append(accomplished, t.Title)
		case "failed":
			remaining = append(remaining, t.Title)
		case "pending", "executing":
			remaining = append(remaining, t.Title)
		}
	}

	for _, f := range cycle.Findings {
		patterns = append(patterns, fmt.Sprintf("[%s] %s", f.Category, f.Description))
	}

	summary := fmt.Sprintf("Cycle %q completed: %d accomplished, %d remaining, %d findings",
		cycle.Name, len(accomplished), len(remaining), len(cycle.Findings))

	nextObj := ""
	if len(remaining) > 0 {
		nextObj = fmt.Sprintf("Address remaining: %s", remaining[0])
	}

	return CycleSynthesis{
		Summary:       summary,
		Accomplished:  accomplished,
		Remaining:     remaining,
		NextObjective: nextObj,
		Patterns:      patterns,
	}
}

// timeNow is a package-level function for testability.
var timeNow = func() time.Time { return time.Now() }
