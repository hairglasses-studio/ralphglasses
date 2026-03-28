package session

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// StartLoop registers a new loop run for a repo.
func (m *Manager) StartLoop(_ context.Context, repoPath string, profile LoopProfile) (*LoopRun, error) {
	if strings.TrimSpace(repoPath) == "" {
		return nil, ErrRepoPathRequired
	}
	if _, err := os.Stat(repoPath); err != nil {
		return nil, fmt.Errorf("stat repo: %w", err)
	}

	if err := ValidateLoopProfile(profile); err != nil {
		return nil, fmt.Errorf("invalid loop profile: %w", err)
	}

	profile, err := normalizeLoopProfile(profile)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	run := &LoopRun{
		ID:         uuid.NewString(),
		RepoPath:   repoPath,
		RepoName:   filepath.Base(repoPath),
		Status:     "pending",
		Profile:    profile,
		Iterations: []LoopIteration{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if profile.MaxDurationSecs > 0 {
		d := time.Now().Add(time.Duration(profile.MaxDurationSecs) * time.Second)
		run.Deadline = &d
	}

	// Opportunistic cleanup of stale loop worktrees (best-effort).
	if _, err := CleanupStaleWorktrees(repoPath, 24*time.Hour); err != nil {
		slog.Warn("failed to cleanup stale worktrees", "repo", repoPath, "error", err)
	}

	m.mu.Lock()
	m.loops[run.ID] = run
	m.mu.Unlock()

	m.PersistLoop(run)
	return run, nil
}

// RunLoop drives a loop to completion by calling StepLoop repeatedly until
// max iterations, duration limit, retry limit, or stop signal is reached.
// It runs synchronously — callers should launch it in a goroutine if needed.
//
// When autonomy level >= 1 and auto-recover is enabled in .ralphrc, failed
// iterations are retried with exponential backoff (30s, 60s, 120s...) up to
// MaxRecoveries times before giving up.
func (m *Manager) RunLoop(ctx context.Context, id string) error {
	run, ok := m.GetLoop(id)
	if !ok {
		return fmt.Errorf("loop %s: %w", id, ErrLoopNotFound)
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	run.mu.Lock()
	run.cancel = cancel
	run.done = done
	repoPath := run.RepoPath
	run.mu.Unlock()

	defer func() {
		cancel()
		close(done)
	}()

	// Bootstrap autonomy config from .ralphrc (best-effort; defaults to level 0).
	autonomyCfg := m.bootstrapLoopAutonomy(repoPath)
	recoveryCount := 0
	consecutiveNoops := 0

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		run.mu.Lock()
		paused := run.Paused
		run.mu.Unlock()
		if paused {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
				continue
			}
		}
		// WS5: Check aggregate budget before each step iteration.
		if exceeded, reason := m.checkLoopBudget(run); exceeded {
			run.mu.Lock()
			run.Status = "completed"
			run.LastError = "budget exceeded: " + reason
			run.UpdatedAt = time.Now()
			run.mu.Unlock()
			m.PersistLoop(run)
			slog.Warn("loop budget exceeded, stopping", "loop", id, "reason", reason)
			return nil
		}

		// Hard budget cap: absolute ceiling to preserve a buffer (e.g. $95 of $100).
		if cap := run.Profile.HardBudgetCapUSD; cap > 0 {
			if spent := m.aggregateLoopSpend(run); spent >= cap {
				run.mu.Lock()
				run.Status = "completed"
				run.LastError = fmt.Sprintf("hard budget cap reached: spent $%.2f of $%.2f cap", spent, cap)
				run.UpdatedAt = time.Now()
				run.mu.Unlock()
				m.PersistLoop(run)
				slog.Warn("loop hard budget cap reached", "loop", id, "spent", spent, "cap", cap)
				return nil
			}
		}

		// MaxWorkerTurns: absolute cap on total iterations (default 20).
		maxTurns := run.Profile.MaxWorkerTurns
		if maxTurns <= 0 {
			maxTurns = 20
		}
		run.mu.Lock()
		totalIters := len(run.Iterations)
		run.mu.Unlock()
		if totalIters >= maxTurns {
			run.mu.Lock()
			run.Status = "stopped"
			run.LastError = fmt.Sprintf("max worker turns reached: %d", maxTurns)
			run.UpdatedAt = time.Now()
			run.mu.Unlock()
			m.PersistLoop(run)
			slog.Warn("loop max worker turns reached", "loop", id, "turns", totalIters, "max", maxTurns)
			return fmt.Errorf("loop %s: max worker turns (%d) exceeded", id, maxTurns)
		}

		err := m.StepLoop(ctx, id)
		if err != nil {
			run.mu.Lock()
			status := run.Status
			run.mu.Unlock()
			if status == "completed" || status == "stopped" || status == "converged" {
				return nil
			}

			// Auto-recovery: if enabled and under limit, wait with backoff and retry.
			if autonomyCfg.ShouldRecover(recoveryCount) {
				backoff := RecoveryBackoff(recoveryCount)
				recoveryCount++
				slog.Info("loop auto-recovery: scheduling retry",
					"loop", id, "attempt", recoveryCount,
					"max", autonomyCfg.MaxRecoveries, "backoff", backoff,
					"error", err.Error())
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
					continue
				}
			}

			if autonomyCfg.Level >= LevelAutoRecover && autonomyCfg.AutoRecover && recoveryCount >= autonomyCfg.MaxRecoveries {
				slog.Warn("loop auto-recovery: max recoveries exceeded, manual intervention required",
					"loop", id, "recoveries", recoveryCount, "max", autonomyCfg.MaxRecoveries)
			}

			return err
		}
		// Reset recovery count on successful step.
		recoveryCount = 0

		// No-op plateau detection: stop if N consecutive iterations produced no changes.
		if limit := run.Profile.NoopPlateauLimit; limit > 0 {
			run.mu.Lock()
			if n := len(run.Iterations); n > 0 {
				reason := run.Iterations[n-1].AcceptanceReason
				if reason == "no_staged_files" || reason == "worker_no_changes" {
					consecutiveNoops++
				} else {
					consecutiveNoops = 0
				}
			}
			run.mu.Unlock()
			if consecutiveNoops >= limit {
				run.mu.Lock()
				run.Status = "converged"
				run.LastError = fmt.Sprintf("no-op plateau: %d consecutive iterations with no changes", consecutiveNoops)
				run.UpdatedAt = time.Now()
				run.mu.Unlock()
				m.PersistLoop(run)
				slog.Info("loop converged (no-op plateau)", "loop", id, "consecutive_noops", consecutiveNoops)
				return nil
			}
		}
	}
}

// bootstrapLoopAutonomy reads .ralphrc from the repo path and returns
// an AutonomyConfig. Falls back to defaults (level 0, no auto-recover) on error.
func (m *Manager) bootstrapLoopAutonomy(repoPath string) *AutonomyConfig {
	rcPath := filepath.Join(repoPath, ".ralphrc")
	cfg := make(map[string]string)
	data, err := os.ReadFile(rcPath)
	if err != nil {
		return BootstrapAutonomy(cfg)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		cfg[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), "\"")
	}
	return BootstrapAutonomy(cfg)
}

// checkLoopBudget computes aggregate spend across all sessions in a loop run
// and checks whether the total budget (planner + worker) has been exceeded.
// Returns (false, "") when no budget is configured or spend is within limits.
func (m *Manager) checkLoopBudget(run *LoopRun) (bool, string) {
	if m.budgetEnforcer == nil {
		return false, ""
	}

	run.mu.Lock()
	profile := run.Profile
	iterations := make([]LoopIteration, len(run.Iterations))
	copy(iterations, run.Iterations)
	run.mu.Unlock()

	totalBudget := profile.PlannerBudgetUSD + profile.WorkerBudgetUSD
	if totalBudget <= 0 {
		return false, ""
	}

	var totalSpent float64
	for _, iter := range iterations {
		if iter.PlannerSessionID != "" {
			if ps, ok := m.Get(iter.PlannerSessionID); ok {
				ps.Lock()
				totalSpent += ps.SpentUSD
				ps.Unlock()
			}
		}
		for _, wid := range iter.WorkerSessionIDs {
			if ws, ok := m.Get(wid); ok {
				ws.Lock()
				totalSpent += ws.SpentUSD
				ws.Unlock()
			}
		}
	}

	threshold := totalBudget * m.budgetEnforcer.Headroom
	if totalSpent >= threshold {
		return true, fmt.Sprintf("spent $%.2f of $%.2f budget (%.0f%% headroom)",
			totalSpent, totalBudget, m.budgetEnforcer.Headroom*100)
	}
	return false, ""
}

// aggregateLoopSpend sums SpentUSD across all planner and worker sessions in a loop run.
func (m *Manager) aggregateLoopSpend(run *LoopRun) float64 {
	run.mu.Lock()
	iterations := make([]LoopIteration, len(run.Iterations))
	copy(iterations, run.Iterations)
	run.mu.Unlock()

	var total float64
	for _, iter := range iterations {
		if iter.PlannerSessionID != "" {
			if ps, ok := m.Get(iter.PlannerSessionID); ok {
				ps.Lock()
				total += ps.SpentUSD
				ps.Unlock()
			}
		}
		for _, wid := range iter.WorkerSessionIDs {
			if ws, ok := m.Get(wid); ok {
				ws.Lock()
				total += ws.SpentUSD
				ws.Unlock()
			}
		}
	}
	return total
}

// GetLoop returns a loop run by ID.
func (m *Manager) GetLoop(id string) (*LoopRun, bool) {
	m.LoadExternalLoops()

	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.loops[id]
	return run, ok
}

// ListLoops returns all known loop runs.
func (m *Manager) ListLoops() []*LoopRun {
	m.LoadExternalLoops()

	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]*LoopRun, 0, len(m.loops))
	for _, run := range m.loops {
		out = append(out, run)
	}
	return out
}

// StopLoop marks a loop run as stopped.
func (m *Manager) StopLoop(id string) error {
	run, ok := m.GetLoop(id)
	if !ok {
		return fmt.Errorf("loop %s: %w", id, ErrLoopNotFound)
	}

	run.mu.Lock()
	run.Status = "stopped"
	run.UpdatedAt = time.Now()
	repoPath := run.RepoPath
	cancelFn := run.cancel
	doneCh := run.done
	run.mu.Unlock()

	// Cancel the RunLoop context and wait for it to exit.
	if cancelFn != nil {
		cancelFn()
	}
	if doneCh != nil {
		<-doneCh
	}

	m.PersistLoop(run)
	if err := CleanupLoopWorktrees(repoPath, id); err != nil {
		slog.Warn("failed to cleanup loop worktrees", "loop", id, "repo", repoPath, "error", err)
	}
	return nil
}

// PauseLoop pauses auto-advance for a running loop.
func (m *Manager) PauseLoop(id string) error {
	run, ok := m.GetLoop(id)
	if !ok {
		return fmt.Errorf("loop %s: %w", id, ErrLoopNotFound)
	}
	run.mu.Lock()
	run.Paused = true
	run.UpdatedAt = time.Now()
	run.mu.Unlock()
	m.PersistLoop(run)
	return nil
}

// ResumeLoop resumes auto-advance for a paused loop.
func (m *Manager) ResumeLoop(id string) error {
	run, ok := m.GetLoop(id)
	if !ok {
		return fmt.Errorf("loop %s: %w", id, ErrLoopNotFound)
	}
	run.mu.Lock()
	run.Paused = false
	run.UpdatedAt = time.Now()
	run.mu.Unlock()
	m.PersistLoop(run)
	return nil
}

func (m *Manager) failLoopIteration(run *LoopRun, index int, err error) error {
	run.updateLoopAfterVerification(index, run.iterationVerification(index), "failed", err.Error())
	m.PersistLoop(run)
	if err := writeLoopJournal(run, run.iterationsSnapshot()[index]); err != nil {
		slog.Warn("failed to write loop journal", "loop", run.ID, "error", err)
	}
	return err
}

func (m *Manager) updateLoopIteration(run *LoopRun, index int, status string, mutate func(*LoopIteration, *LoopRun)) {
	run.mu.Lock()
	defer run.mu.Unlock()

	if index < 0 || index >= len(run.Iterations) {
		return
	}
	if status != "" {
		run.Iterations[index].Status = status
	}
	if mutate != nil {
		mutate(&run.Iterations[index], run)
	}
	run.Status = "running"
	run.UpdatedAt = time.Now()
}
