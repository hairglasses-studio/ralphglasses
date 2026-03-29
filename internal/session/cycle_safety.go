package session

import (
	"fmt"
	"time"
)

// CycleSafetyConfig defines safety limits for R&D cycles at autonomy levels 2-3.
type CycleSafetyConfig struct {
	MaxConcurrentCycles int           // per repo, default 2
	MaxTasksPerCycle    int           // default 10
	PhaseCooldown       time.Duration // minimum time between phase transitions, default 30s
	RequireBaseline     bool          // must have baseline before executing, default true
	MaxCycleAge         time.Duration // auto-fail cycles older than this, default 24h
}

// DefaultCycleSafety provides conservative defaults for cycle safety.
var DefaultCycleSafety = CycleSafetyConfig{
	MaxConcurrentCycles: 2,
	MaxTasksPerCycle:    10,
	PhaseCooldown:       30 * time.Second,
	RequireBaseline:     true,
	MaxCycleAge:         24 * time.Hour,
}

// DisabledCycleSafety is a permissive config that disables all safety checks.
// Intended for tests and migration scenarios.
var DisabledCycleSafety = CycleSafetyConfig{
	MaxConcurrentCycles: 0, // 0 = no limit
	MaxTasksPerCycle:    0,
	PhaseCooldown:       0,
	RequireBaseline:     false,
	MaxCycleAge:         0,
}

// CycleSafetyError is returned when a cycle safety check fails.
type CycleSafetyError struct {
	Check   string // which check failed
	Message string // human-readable explanation
}

func (e *CycleSafetyError) Error() string {
	return fmt.Sprintf("cycle safety: %s: %s", e.Check, e.Message)
}

// ValidateCycleAdvance checks whether a cycle can advance to the next phase.
// It enforces age limits, phase cooldown, baseline requirements, and task caps.
func ValidateCycleAdvance(cycle *CycleRun, config CycleSafetyConfig) error {
	now := timeNow()

	// Check: cycle age — fail cycles that have been running too long.
	if config.MaxCycleAge > 0 && now.Sub(cycle.CreatedAt) > config.MaxCycleAge {
		return &CycleSafetyError{
			Check:   "max_cycle_age",
			Message: fmt.Sprintf("cycle %s is %s old, exceeding max age %s", cycle.ID, now.Sub(cycle.CreatedAt).Round(time.Second), config.MaxCycleAge),
		}
	}

	// Check: phase cooldown — prevent rapid phase transitions.
	// Skip cooldown for the initial proposed→baselining transition (same logical op as creation).
	if config.PhaseCooldown > 0 && cycle.Phase != CycleProposed && now.Sub(cycle.UpdatedAt) < config.PhaseCooldown {
		remaining := config.PhaseCooldown - now.Sub(cycle.UpdatedAt)
		return &CycleSafetyError{
			Check:   "phase_cooldown",
			Message: fmt.Sprintf("phase cooldown not elapsed: %s remaining (min %s between transitions)", remaining.Round(time.Second), config.PhaseCooldown),
		}
	}

	// Check: require baseline before executing.
	if config.RequireBaseline && cycle.Phase == CycleBaselining && cycle.BaselineID == "" {
		return &CycleSafetyError{
			Check:   "require_baseline",
			Message: "cycle must have a baseline before advancing to executing phase",
		}
	}

	// Check: max tasks not exceeded.
	if config.MaxTasksPerCycle > 0 && len(cycle.Tasks) > config.MaxTasksPerCycle {
		return &CycleSafetyError{
			Check:   "max_tasks",
			Message: fmt.Sprintf("cycle has %d tasks, exceeding limit of %d", len(cycle.Tasks), config.MaxTasksPerCycle),
		}
	}

	return nil
}

// ValidateCycleStart checks whether a new cycle can be started for a repo.
// It enforces the concurrent cycle limit by counting non-terminal existing cycles.
func ValidateCycleStart(repoPath string, existingCycles []*CycleRun, config CycleSafetyConfig) error {
	if config.MaxConcurrentCycles <= 0 {
		return nil // no limit
	}

	active := 0
	for _, c := range existingCycles {
		if c.RepoPath == repoPath && c.Phase != CycleComplete && c.Phase != CycleFailed {
			active++
		}
	}

	if active >= config.MaxConcurrentCycles {
		return &CycleSafetyError{
			Check:   "max_concurrent_cycles",
			Message: fmt.Sprintf("repo %s already has %d active cycles (limit: %d)", repoPath, active, config.MaxConcurrentCycles),
		}
	}

	return nil
}
