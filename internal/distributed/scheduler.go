package distributed

import (
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"sync"
	"time"
)

// WorkerCapability describes what a worker node can do.
type WorkerCapability struct {
	NodeID      string    `json:"node_id"`
	Providers   []string  `json:"providers"`     // e.g. ["claude", "gemini"]
	MaxSessions int       `json:"max_sessions"`  // concurrent session capacity
	Active      int       `json:"active"`        // currently running sessions
	HealthScore float64   `json:"health_score"`  // 0-1, higher = healthier
	CostRateUSD float64   `json:"cost_rate_usd"` // estimated $/hour
	LastSeen    time.Time `json:"last_seen"`
}

// Scheduler assigns tasks to workers based on capabilities, load, and budget.
// Informed by PILOT (ArXiv 2508.21141) — budget-aware task scheduling.
type Scheduler struct {
	mu      sync.RWMutex
	workers map[string]*WorkerCapability // node ID -> capability
	queue   *DistributedQueue
}

// NewScheduler creates a scheduler backed by the given queue.
func NewScheduler(queue *DistributedQueue) *Scheduler {
	return &Scheduler{
		workers: make(map[string]*WorkerCapability),
		queue:   queue,
	}
}

// RegisterWorker adds or updates a worker's capabilities.
func (s *Scheduler) RegisterWorker(w WorkerCapability) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.LastSeen = time.Now()
	s.workers[w.NodeID] = &w
}

// RemoveWorker unregisters a worker.
func (s *Scheduler) RemoveWorker(nodeID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.workers, nodeID)
}

// Workers returns all registered workers.
func (s *Scheduler) Workers() []WorkerCapability {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]WorkerCapability, 0, len(s.workers))
	for _, w := range s.workers {
		result = append(result, *w)
	}
	return result
}

// AvailableWorkers returns workers with remaining capacity for the given provider.
func (s *Scheduler) AvailableWorkers(provider string) []WorkerCapability {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var available []WorkerCapability
	for _, w := range s.workers {
		if w.Active >= w.MaxSessions {
			continue // at capacity
		}
		if time.Since(w.LastSeen) > 2*time.Minute {
			continue // stale
		}
		if provider != "" {
			found := slices.Contains(w.Providers, provider)
			if !found {
				continue
			}
		}
		available = append(available, *w)
	}

	return available
}

// AssignNext picks the best worker for the highest-priority pending task.
// Returns the task and worker ID, or ("", nil) if no assignment is possible.
func (s *Scheduler) AssignNext() (*DistributedTask, string) {
	s.mu.RLock()
	pending := s.queue.PendingTasks()
	s.mu.RUnlock()

	if len(pending) == 0 {
		return nil, ""
	}

	// Try each pending task in priority order
	for _, task := range pending {
		workers := s.AvailableWorkers(task.Provider)
		if len(workers) == 0 {
			continue
		}

		// Select best worker: highest health score, lowest load, lowest cost
		best := selectBestWorker(workers, task)
		if best == "" {
			continue
		}

		// Claim the task
		claimed := s.queue.Claim(best)
		if claimed == nil {
			continue // race condition, try next
		}

		slog.Info("scheduler: assigned task",
			"task", claimed.ID, "worker", best, "type", claimed.Type)
		return claimed, best
	}

	return nil, ""
}

// selectBestWorker picks the optimal worker for a task using a composite score.
func selectBestWorker(workers []WorkerCapability, task *DistributedTask) string {
	if len(workers) == 0 {
		return ""
	}

	type scored struct {
		nodeID string
		score  float64
	}

	var candidates []scored
	for _, w := range workers {
		// Composite score: 40% health, 30% available capacity, 30% cost efficiency
		capacityRatio := 1.0 - float64(w.Active)/float64(max(w.MaxSessions, 1))
		costScore := 1.0
		if w.CostRateUSD > 0 {
			costScore = 1.0 / (1.0 + w.CostRateUSD) // cheaper = higher score
		}

		// Budget pressure: if task has tight budget, weight cost more
		costWeight := 0.3
		if task.BudgetUSD > 0 && task.BudgetUSD < 5.0 {
			costWeight = 0.5
		}

		healthWeight := 0.4
		capacityWeight := 1.0 - healthWeight - costWeight

		score := healthWeight*w.HealthScore + capacityWeight*capacityRatio + costWeight*costScore
		candidates = append(candidates, scored{w.NodeID, score})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	return candidates[0].nodeID
}

// PruneStaleWorkers removes workers not seen within the given duration.
func (s *Scheduler) PruneStaleWorkers(maxAge time.Duration) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	pruned := 0
	for id, w := range s.workers {
		if w.LastSeen.Before(cutoff) {
			delete(s.workers, id)
			pruned++
		}
	}
	return pruned
}

// FleetCapacity returns aggregate fleet capacity stats.
func (s *Scheduler) FleetCapacity() FleetCapacityStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stats FleetCapacityStats
	for _, w := range s.workers {
		stats.TotalWorkers++
		stats.TotalCapacity += w.MaxSessions
		stats.TotalActive += w.Active
		if w.Active < w.MaxSessions {
			stats.AvailableSlots += w.MaxSessions - w.Active
		}
	}

	if stats.TotalCapacity > 0 {
		stats.UtilizationPct = float64(stats.TotalActive) / float64(stats.TotalCapacity) * 100
	}
	return stats
}

// FleetCapacityStats is a snapshot of fleet worker capacity.
type FleetCapacityStats struct {
	TotalWorkers   int     `json:"total_workers"`
	TotalCapacity  int     `json:"total_capacity"`
	TotalActive    int     `json:"total_active"`
	AvailableSlots int     `json:"available_slots"`
	UtilizationPct float64 `json:"utilization_pct"`
}

// RecommendScale suggests whether to scale up or down based on queue depth.
func (s *Scheduler) RecommendScale() (action string, reason string) {
	stats := s.queue.Stats()
	capacity := s.FleetCapacity()

	if stats.Pending > capacity.AvailableSlots*2 && capacity.AvailableSlots < 3 {
		return "scale_up", fmt.Sprintf("%d pending tasks, only %d slots available", stats.Pending, capacity.AvailableSlots)
	}

	if capacity.UtilizationPct < 20 && capacity.TotalWorkers > 1 {
		return "scale_down", fmt.Sprintf("%.0f%% utilization across %d workers", capacity.UtilizationPct, capacity.TotalWorkers)
	}

	return "none", "fleet capacity adequate"
}
