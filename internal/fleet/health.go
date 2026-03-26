package fleet

import (
	"sync"
	"time"
)

// HealthState represents the health classification of a worker.
type HealthState string

const (
	HealthHealthy  HealthState = "healthy"
	HealthDegraded HealthState = "degraded"
	HealthDown     HealthState = "down"
)

// HealthConfig controls health tracking thresholds.
type HealthConfig struct {
	// DegradedAfter is the duration without a heartbeat before a worker is considered degraded.
	DegradedAfter time.Duration
	// DownAfter is the duration without a heartbeat before a worker is considered down.
	DownAfter time.Duration
}

// DefaultHealthConfig returns sensible defaults for health tracking.
func DefaultHealthConfig() HealthConfig {
	return HealthConfig{
		DegradedAfter: StaleThreshold,
		DownAfter:     DisconnectThreshold,
	}
}

// HealthTracker monitors worker health based on heartbeat recency.
type HealthTracker struct {
	mu     sync.RWMutex
	config HealthConfig
	// lastHeartbeat tracks the most recent heartbeat time per worker ID.
	lastHeartbeat map[string]time.Time
}

// NewHealthTracker creates a HealthTracker with the given config.
func NewHealthTracker(cfg HealthConfig) *HealthTracker {
	return &HealthTracker{
		config:        cfg,
		lastHeartbeat: make(map[string]time.Time),
	}
}

// RecordHeartbeat records a heartbeat for the given worker.
func (h *HealthTracker) RecordHeartbeat(workerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastHeartbeat[workerID] = time.Now()
}

// State returns the current health state for a worker.
func (h *HealthTracker) State(workerID string) HealthState {
	h.mu.RLock()
	defer h.mu.RUnlock()

	last, ok := h.lastHeartbeat[workerID]
	if !ok {
		return HealthDown
	}

	since := time.Since(last)
	switch {
	case since > h.config.DownAfter:
		return HealthDown
	case since > h.config.DegradedAfter:
		return HealthDegraded
	default:
		return HealthHealthy
	}
}

// Remove removes a worker from tracking.
func (h *HealthTracker) Remove(workerID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.lastHeartbeat, workerID)
}
