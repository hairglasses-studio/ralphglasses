package fleet

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// buildCandidates constructs WorkerCandidate entries from all registered workers,
// incorporating health state and per-worker budget remaining.
func (c *Coordinator) buildCandidates() []WorkerCandidate {
	c.mu.RLock()
	defer c.mu.RUnlock()

	candidates := make([]WorkerCandidate, 0, len(c.workers))
	for _, w := range c.workers {
		// Skip paused and draining workers from routing candidates
		if w.Status == WorkerPaused || w.Status == WorkerDraining {
			continue
		}
		candidates = append(candidates, WorkerCandidate{
			ID:              w.ID,
			ActiveTasks:     w.ActiveSessions,
			HealthState:     c.health.GetState(w.ID),
			BudgetRemaining: c.budgetMgr.GetBudget(w.ID).Remaining(),
		})
	}
	return candidates
}

// assignWork finds the best pending work item for a worker and assigns it.
// Uses the router to validate the polling worker is a good candidate, then
// scores work items for best fit.
func (c *Coordinator) assignWork(workerID string, worker *WorkerInfo) *WorkItem {
	// Ensure all registered workers have health records (handles workers added
	// programmatically without going through the register handler).
	c.mu.RLock()
	for id, w := range c.workers {
		state := c.health.GetState(id)
		if (state == HealthUnhealthy || state == HealthUnknown) && w.Status != WorkerDisconnected {
			c.health.RecordHeartbeat(id)
		}
	}
	c.mu.RUnlock()

	// Use router to check if this worker should receive work
	candidates := c.buildCandidates()
	preferredWorker, err := c.router.SelectWorker(candidates)
	if err != nil {
		// No healthy workers available at all
		return nil
	}

	// Boost score if the router prefers this worker
	routerBoost := 0
	if preferredWorker == workerID {
		routerBoost = 15
	}

	// Skip if this worker is paused or draining
	c.mu.RLock()
	if w, ok := c.workers[workerID]; ok && (w.Status == WorkerPaused || w.Status == WorkerDraining) {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	// Skip if this worker's health is down
	if c.health.GetState(workerID) == HealthUnhealthy {
		return nil
	}

	c.mu.RLock()
	avail := c.budget.AvailableBudget()
	c.mu.RUnlock()

	repoSet := make(map[string]bool, len(worker.Repos))
	for _, r := range worker.Repos {
		repoSet[r] = true
	}
	providerSet := make(map[session.Provider]bool, len(worker.Providers))
	for _, p := range worker.Providers {
		providerSet[p] = true
	}

	item := c.queue.AssignBest(func(item *WorkItem) int {
		// Retry delay gate — skip items still in backoff
		if item.RetryAfter != nil && time.Now().Before(*item.RetryAfter) {
			return -1
		}

		// Budget gate
		if item.MaxBudgetUSD > 0 && item.MaxBudgetUSD > avail {
			return -1 // skip
		}

		// Per-worker budget gate
		if item.MaxBudgetUSD > 0 && item.MaxBudgetUSD > c.budgetMgr.GetBudget(workerID).Remaining() {
			return -1
		}

		score := item.Priority*100 + routerBoost

		// Provider match
		if item.Provider != "" && providerSet[item.Provider] {
			score += 10
		}
		if item.Constraints.RequireProvider != "" && !providerSet[item.Constraints.RequireProvider] {
			return -1
		}

		// Repo locality
		if repoSet[item.RepoName] {
			score += 5
		}
		if item.Constraints.RequireLocal && !repoSet[item.RepoName] {
			return -1
		}

		// Node preference
		if item.Constraints.NodePreference == workerID {
			score += 20
		}

		return score
	}, workerID)

	return item
}

// helper functions

func timePtr(t time.Time) *time.Time {
	return &t
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func mergeData(base, extra map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}

func generateID() string {
	// Reuse the uuid dependency already in go.mod
	return fmt.Sprintf("fl-%d", time.Now().UnixNano())
}
