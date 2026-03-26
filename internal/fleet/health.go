package fleet

import (
	"sync"
	"time"
)

// HealthState represents a worker's health status.
type HealthState string

const (
	HealthHealthy   HealthState = "healthy"
	HealthDegraded  HealthState = "degraded"
	HealthUnhealthy HealthState = "unhealthy"
	HealthUnknown   HealthState = "unknown"
)

// HealthTransition records a state change.
type HealthTransition struct {
	From      HealthState
	To        HealthState
	Reason    string
	Timestamp time.Time
}

// WorkerHealth tracks a single worker's health.
type WorkerHealth struct {
	State             HealthState
	LastHeartbeat     time.Time
	ConsecutiveMisses int
	ErrorRate         float64 // errors / total in sliding window
	AvgLatencyMs      float64
	History           []HealthTransition
}

// HealthConfig defines thresholds for state transitions.
type HealthConfig struct {
	HeartbeatInterval    time.Duration // expected interval between heartbeats
	DegradedAfterMisses  int           // misses before degraded (default 2)
	UnhealthyAfterMisses int           // misses before unhealthy (default 5)
	ErrorRateThreshold   float64       // error rate for degraded (default 0.1)
	LatencyThresholdMs   float64       // p99 latency for degraded (default 5000)
	MaxHistory           int           // max transitions to keep (default 50)
}

// DefaultHealthConfig returns sensible defaults.
func DefaultHealthConfig() HealthConfig {
	return HealthConfig{
		HeartbeatInterval:    30 * time.Second,
		DegradedAfterMisses:  2,
		UnhealthyAfterMisses: 5,
		ErrorRateThreshold:   0.1,
		LatencyThresholdMs:   5000,
		MaxHistory:           50,
	}
}

// HealthTracker manages health state for all workers.
type HealthTracker struct {
	mu      sync.RWMutex
	workers map[string]*WorkerHealth // worker ID -> health
	config  HealthConfig
}

// NewHealthTracker creates a tracker with the given config.
func NewHealthTracker(cfg HealthConfig) *HealthTracker {
	return &HealthTracker{
		workers: make(map[string]*WorkerHealth),
		config:  cfg,
	}
}

// RecordHeartbeat updates the worker's last heartbeat time and re-evaluates state.
func (ht *HealthTracker) RecordHeartbeat(workerID string) {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	w, ok := ht.workers[workerID]
	if !ok {
		w = &WorkerHealth{State: HealthUnknown}
		ht.workers[workerID] = w
	}
	w.LastHeartbeat = time.Now()
	w.ConsecutiveMisses = 0
	ht.transition(w, HealthHealthy, "heartbeat received")
}

// RecordMiss increments miss count and may degrade health.
func (ht *HealthTracker) RecordMiss(workerID string) {
	ht.mu.Lock()
	defer ht.mu.Unlock()

	w, ok := ht.workers[workerID]
	if !ok {
		return
	}
	w.ConsecutiveMisses++

	if w.ConsecutiveMisses >= ht.config.UnhealthyAfterMisses {
		ht.transition(w, HealthUnhealthy, "too many missed heartbeats")
	} else if w.ConsecutiveMisses >= ht.config.DegradedAfterMisses {
		ht.transition(w, HealthDegraded, "missed heartbeats")
	}
}

// GetState returns the current health state of a worker.
func (ht *HealthTracker) GetState(workerID string) HealthState {
	ht.mu.RLock()
	defer ht.mu.RUnlock()
	if w, ok := ht.workers[workerID]; ok {
		return w.State
	}
	return HealthUnknown
}

// GetHealth returns full health info for a worker.
func (ht *HealthTracker) GetHealth(workerID string) (WorkerHealth, bool) {
	ht.mu.RLock()
	defer ht.mu.RUnlock()
	if w, ok := ht.workers[workerID]; ok {
		return *w, true
	}
	return WorkerHealth{}, false
}

// HealthyWorkers returns IDs of all healthy workers.
func (ht *HealthTracker) HealthyWorkers() []string {
	ht.mu.RLock()
	defer ht.mu.RUnlock()
	var ids []string
	for id, w := range ht.workers {
		if w.State == HealthHealthy {
			ids = append(ids, id)
		}
	}
	return ids
}

func (ht *HealthTracker) transition(w *WorkerHealth, to HealthState, reason string) {
	if w.State == to {
		return
	}
	t := HealthTransition{
		From:      w.State,
		To:        to,
		Reason:    reason,
		Timestamp: time.Now(),
	}
	w.History = append(w.History, t)
	if len(w.History) > ht.config.MaxHistory {
		w.History = w.History[len(w.History)-ht.config.MaxHistory:]
	}
	w.State = to
}
