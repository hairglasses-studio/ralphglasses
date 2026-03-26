package fleet

import (
	"errors"
	"math/rand"
)

// ErrNoWorkers is returned when no worker candidates are available for routing.
var ErrNoWorkers = errors.New("no worker candidates available")

// WorkerCandidate represents a worker eligible for task assignment.
type WorkerCandidate struct {
	ID              string
	ActiveSessions  int
	MaxSessions     int
	HealthState     HealthState
	BudgetRemaining float64
}

// Router selects a worker from a set of candidates.
type Router interface {
	SelectWorker(candidates []WorkerCandidate) (string, error)
}

// LeastLoadedRouter picks the worker with the lowest load ratio (active/max sessions).
type LeastLoadedRouter struct{}

// SelectWorker picks the least-loaded healthy worker.
func (r *LeastLoadedRouter) SelectWorker(candidates []WorkerCandidate) (string, error) {
	if len(candidates) == 0 {
		return "", ErrNoWorkers
	}

	var bestID string
	bestLoad := 2.0 // higher than any possible load

	for _, c := range candidates {
		if c.HealthState == HealthDown {
			continue
		}
		if c.MaxSessions == 0 {
			continue
		}
		load := float64(c.ActiveSessions) / float64(c.MaxSessions)
		if load < bestLoad {
			bestLoad = load
			bestID = c.ID
		}
	}

	if bestID == "" {
		return "", ErrNoWorkers
	}
	return bestID, nil
}

// RoundRobinRouter cycles through workers in order.
type RoundRobinRouter struct {
	counter int
}

// SelectWorker picks the next worker in round-robin order.
func (r *RoundRobinRouter) SelectWorker(candidates []WorkerCandidate) (string, error) {
	healthy := make([]WorkerCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.HealthState != HealthDown && c.ActiveSessions < c.MaxSessions {
			healthy = append(healthy, c)
		}
	}
	if len(healthy) == 0 {
		return "", ErrNoWorkers
	}

	idx := r.counter % len(healthy)
	r.counter++
	return healthy[idx].ID, nil
}

// RandomRouter picks a random eligible worker.
type RandomRouter struct{}

// SelectWorker picks a random healthy worker with capacity.
func (r *RandomRouter) SelectWorker(candidates []WorkerCandidate) (string, error) {
	healthy := make([]WorkerCandidate, 0, len(candidates))
	for _, c := range candidates {
		if c.HealthState != HealthDown && c.ActiveSessions < c.MaxSessions {
			healthy = append(healthy, c)
		}
	}
	if len(healthy) == 0 {
		return "", ErrNoWorkers
	}

	return healthy[rand.Intn(len(healthy))].ID, nil
}
