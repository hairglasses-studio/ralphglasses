package session

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"
)

// CreateCycle creates a new CycleRun in the proposed phase and persists it.
func (m *Manager) CreateCycle(repoPath, name, objective string, criteria []string) (*CycleRun, error) {
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
func (m *Manager) AdvanceCycle(cycle *CycleRun) error {
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

// timeNow is a package-level function for testability.
var timeNow = func() time.Time { return time.Now() }
