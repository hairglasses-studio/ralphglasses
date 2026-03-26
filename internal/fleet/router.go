package fleet

import (
	"errors"
	"math"
	"sort"
	"sync/atomic"
)

var ErrNoWorkers = errors.New("no eligible workers")

// HealthState represents a worker's health status for routing decisions.
// This is a local placeholder; Phase B will unify with health.go's HealthState.
type HealthState int

const (
	HealthHealthy  HealthState = iota
	HealthDegraded             // still routable, but not ideal
	HealthUnhealthy            // excluded from routing
)

// Router selects a worker for a given task.
type Router interface {
	SelectWorker(workers []WorkerCandidate) (string, error) // returns worker ID
}

// WorkerCandidate contains info needed for routing decisions.
type WorkerCandidate struct {
	ID              string
	Provider        string
	ActiveTasks     int
	BudgetRemaining float64
	HealthState     HealthState
	CostRate        float64 // estimated $/task for this provider
}

// RoundRobinRouter distributes work evenly across workers.
type RoundRobinRouter struct {
	counter atomic.Uint64
}

func (r *RoundRobinRouter) SelectWorker(workers []WorkerCandidate) (string, error) {
	eligible := filterHealthy(workers)
	if len(eligible) == 0 {
		return "", ErrNoWorkers
	}
	idx := r.counter.Add(1) - 1
	return eligible[idx%uint64(len(eligible))].ID, nil
}

// LeastLoadedRouter picks the worker with fewest active tasks.
type LeastLoadedRouter struct{}

func (r *LeastLoadedRouter) SelectWorker(workers []WorkerCandidate) (string, error) {
	eligible := filterHealthy(workers)
	if len(eligible) == 0 {
		return "", ErrNoWorkers
	}
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].ActiveTasks < eligible[j].ActiveTasks
	})
	return eligible[0].ID, nil
}

// CostOptimalRouter picks the worker with lowest cost rate that has budget.
type CostOptimalRouter struct{}

func (r *CostOptimalRouter) SelectWorker(workers []WorkerCandidate) (string, error) {
	eligible := filterHealthy(workers)
	if len(eligible) == 0 {
		return "", ErrNoWorkers
	}
	// Filter to workers with remaining budget
	var withBudget []WorkerCandidate
	for _, w := range eligible {
		if w.BudgetRemaining > 0 {
			withBudget = append(withBudget, w)
		}
	}
	if len(withBudget) == 0 {
		// Fall back to all eligible if none have budget tracking
		withBudget = eligible
	}
	sort.Slice(withBudget, func(i, j int) bool {
		return withBudget[i].CostRate < withBudget[j].CostRate
	})
	return withBudget[0].ID, nil
}

// ProviderAffinityRouter prefers workers running a specific provider.
type ProviderAffinityRouter struct {
	PreferredProvider string
	Fallback          Router
}

func (r *ProviderAffinityRouter) SelectWorker(workers []WorkerCandidate) (string, error) {
	eligible := filterHealthy(workers)
	if len(eligible) == 0 {
		return "", ErrNoWorkers
	}
	// Try preferred provider first
	var preferred []WorkerCandidate
	for _, w := range eligible {
		if w.Provider == r.PreferredProvider {
			preferred = append(preferred, w)
		}
	}
	if len(preferred) > 0 {
		if r.Fallback != nil {
			return r.Fallback.SelectWorker(preferred)
		}
		return preferred[0].ID, nil
	}
	// Fall back
	if r.Fallback != nil {
		return r.Fallback.SelectWorker(eligible)
	}
	return eligible[0].ID, nil
}

// CompositeRouter tries routers in order, using scores.
type CompositeRouter struct {
	Weights map[string]float64 // factor name -> weight
}

func (r *CompositeRouter) SelectWorker(workers []WorkerCandidate) (string, error) {
	eligible := filterHealthy(workers)
	if len(eligible) == 0 {
		return "", ErrNoWorkers
	}

	type scored struct {
		id    string
		score float64
	}
	var results []scored

	for _, w := range eligible {
		s := 0.0
		// Lower load = higher score
		if weight, ok := r.Weights["load"]; ok {
			s += weight * (1.0 / math.Max(float64(w.ActiveTasks+1), 1.0))
		}
		// More budget = higher score
		if weight, ok := r.Weights["budget"]; ok {
			s += weight * w.BudgetRemaining
		}
		// Lower cost = higher score
		if weight, ok := r.Weights["cost"]; ok && w.CostRate > 0 {
			s += weight * (1.0 / w.CostRate)
		}
		results = append(results, scored{id: w.ID, score: s})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})
	return results[0].id, nil
}

func filterHealthy(workers []WorkerCandidate) []WorkerCandidate {
	var result []WorkerCandidate
	for _, w := range workers {
		if w.HealthState == HealthHealthy || w.HealthState == HealthDegraded {
			result = append(result, w)
		}
	}
	return result
}
