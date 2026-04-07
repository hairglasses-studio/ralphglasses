// recovery.go — Fleet-distributed crash recovery orchestrator.
//
// Converts a CrashRecoveryPlan into fleet WorkItems and distributes them
// across available workers via the Coordinator queue. Workers with the
// matching repo execute the resume locally.
package fleet

import (
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// FleetRecoveryOrchestrator distributes crash recovery across fleet workers.
type FleetRecoveryOrchestrator struct {
	coordinator *Coordinator
	bus         *events.Bus

	mu         sync.Mutex
	dispatched map[string]string // claude session ID -> work item ID
}

// NewFleetRecoveryOrchestrator creates a fleet-aware recovery orchestrator.
func NewFleetRecoveryOrchestrator(coord *Coordinator, bus *events.Bus) *FleetRecoveryOrchestrator {
	return &FleetRecoveryOrchestrator{
		coordinator: coord,
		bus:         bus,
		dispatched:  make(map[string]string),
	}
}

// DistributeRecoveryPlan converts a recovery plan into fleet work items
// and submits them to the coordinator queue. Returns the number of items
// submitted and any error.
func (f *FleetRecoveryOrchestrator) DistributeRecoveryPlan(plan *session.CrashRecoveryPlan, budgetPerSession float64) (int, error) {
	if plan == nil || len(plan.SessionsToResume) == 0 {
		return 0, nil
	}

	submitted := 0
	var firstErr error

	for _, rs := range plan.SessionsToResume {
		provider := rs.Provider
		if provider == "" {
			provider = session.DefaultPrimaryProvider()
		}
		item := &WorkItem{
			Type:         WorkTypeSession,
			Status:       WorkPending,
			Priority:     100 - rs.Priority, // higher internal priority for lower recovery rank
			RepoName:     rs.RepoName,
			RepoPath:     rs.RepoPath,
			Prompt:       rs.ResumePrompt,
			Provider:     provider,
			MaxBudgetUSD: budgetPerSession,
			Constraints: WorkConstraints{
				RequireLocal: true, // repo must exist on assigned worker
			},
			SubmittedAt: time.Now(),
		}

		if err := f.coordinator.SubmitWork(item); err != nil {
			slog.Warn("fleet_recovery: failed to submit work item",
				"session", rs.SessionID,
				"repo", rs.RepoName,
				"error", err,
			)
			if firstErr == nil {
				firstErr = fmt.Errorf("submit %s (%s): %w", rs.RepoName, rs.SessionID, err)
			}
			continue
		}

		f.mu.Lock()
		f.dispatched[rs.SessionID] = item.ID
		f.mu.Unlock()
		submitted++

		slog.Info("fleet_recovery: submitted recovery work item",
			"session", rs.SessionID,
			"repo", rs.RepoName,
			"work_item", item.ID,
			"priority", item.Priority,
		)
	}

	if f.bus != nil {
		f.bus.Publish(events.Event{
			Type:      events.SessionRecovered,
			Timestamp: time.Now(),
			Data: map[string]any{
				"action":    "fleet_recovery_distributed",
				"submitted": submitted,
				"total":     len(plan.SessionsToResume),
				"severity":  plan.Severity,
			},
		})
	}

	return submitted, firstErr
}

// FindWorkerForRepo returns the ID of a worker that has the given repo,
// or empty string if none found.
func (f *FleetRecoveryOrchestrator) FindWorkerForRepo(repoPath string) string {
	if f.coordinator == nil {
		return ""
	}

	f.coordinator.mu.RLock()
	defer f.coordinator.mu.RUnlock()

	for _, w := range f.coordinator.workers {
		if w.Status != WorkerOnline {
			continue
		}
		if slices.Contains(w.Repos, repoPath) {
			return w.ID
		}
	}
	return ""
}

// DispatchedItems returns a copy of the dispatched session→work item mapping.
func (f *FleetRecoveryOrchestrator) DispatchedItems() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make(map[string]string, len(f.dispatched))
	maps.Copy(cp, f.dispatched)
	return cp
}
