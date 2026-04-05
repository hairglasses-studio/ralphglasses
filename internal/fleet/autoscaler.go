package fleet

import (
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

// ScaleAction represents the direction of a scaling decision.
type ScaleAction int

const (
	ScaleNone ScaleAction = iota
	ScaleUp
	ScaleDown
)

// String returns a human-readable label for the action.
func (a ScaleAction) String() string {
	switch a {
	case ScaleUp:
		return "scale_up"
	case ScaleDown:
		return "scale_down"
	default:
		return "no_change"
	}
}

// ScaleDecision is the output of an autoscaler evaluation.
type ScaleDecision struct {
	Action  ScaleAction `json:"action"`
	Delta   int         `json:"delta"` // positive = add workers, negative = remove
	Reason  string      `json:"reason"`
	Current int         `json:"current"` // current active worker count
	Target  int         `json:"target"`  // desired worker count after scaling
}

// AutoScalerConfig holds the tunable thresholds for scaling decisions.
type AutoScalerConfig struct {
	MinWorkers int // floor — never scale below this (default 2)
	MaxWorkers int // ceiling — never scale above this (default 32)

	// Scale-up: trigger when pending queue depth exceeds this multiple of active worker count.
	QueueDepthMultiplier float64 // default 2.0

	// Scale-down: trigger when the fraction of idle workers exceeds this threshold.
	IdleWorkerThreshold float64 // default 0.5

	// Budget gate: block scale-up when remaining budget fraction is below this.
	BudgetFloorFraction float64 // default 0.10

	// Cooldown prevents flapping by enforcing a minimum interval between scale actions.
	CooldownDuration time.Duration // default 60s
}

// DefaultAutoScalerConfig returns production-safe defaults.
func DefaultAutoScalerConfig() AutoScalerConfig {
	return AutoScalerConfig{
		MinWorkers:           2,
		MaxWorkers:           32,
		QueueDepthMultiplier: 2.0,
		IdleWorkerThreshold:  0.5,
		BudgetFloorFraction:  0.10,
		CooldownDuration:     60 * time.Second,
	}
}

// WorkerHealthScore captures per-worker performance metrics used by the autoscaler.
type WorkerHealthScore struct {
	WorkerID    string
	SuccessRate float64 // 0.0–1.0, fraction of completed tasks that succeeded
	LatencyP99  float64 // seconds, 99th percentile task latency
	StaleRatio  float64 // 0.0–1.0, fraction of assigned tasks that timed out
	Idle        bool    // true when the worker has zero active tasks
}

// AutoScaler evaluates fleet metrics and recommends worker count changes.
type AutoScaler struct {
	mu     sync.Mutex
	config AutoScalerConfig

	// Cooldown state
	lastScaleTime   time.Time
	lastScaleAction ScaleAction

	// Per-worker metrics sliding window (written by RecordTaskOutcome)
	metrics map[string]*workerMetrics
}

// workerMetrics is an internal sliding window of recent task outcomes.
type workerMetrics struct {
	successes   int
	failures    int
	latencies   []float64 // seconds, most recent at end
	staleTasks  int
	totalTasks  int
	lastUpdated time.Time
}

// NewAutoScaler creates an autoscaler with the given config.
func NewAutoScaler(cfg AutoScalerConfig) *AutoScaler {
	if cfg.MinWorkers <= 0 {
		cfg.MinWorkers = 2
	}
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 32
	}
	if cfg.MaxWorkers < cfg.MinWorkers {
		cfg.MaxWorkers = cfg.MinWorkers
	}
	if cfg.QueueDepthMultiplier <= 0 {
		cfg.QueueDepthMultiplier = 2.0
	}
	if cfg.IdleWorkerThreshold <= 0 {
		cfg.IdleWorkerThreshold = 0.5
	}
	if cfg.BudgetFloorFraction <= 0 {
		cfg.BudgetFloorFraction = 0.10
	}
	if cfg.CooldownDuration <= 0 {
		cfg.CooldownDuration = 60 * time.Second
	}
	return &AutoScaler{
		config:  cfg,
		metrics: make(map[string]*workerMetrics),
	}
}

// AutoScalerSnapshot is the fleet state snapshot passed into Evaluate.
type AutoScalerSnapshot struct {
	Workers        []WorkerSnapshot
	QueueDepth     int     // pending items
	BudgetTotal    float64 // total budget limit
	BudgetSpent    float64 // total spent so far
	BudgetReserved float64 // reserved for in-flight work
}

// WorkerSnapshot is a point-in-time view of a single worker for scaling decisions.
type WorkerSnapshot struct {
	ID             string
	Status         WorkerStatus
	ActiveSessions int
	MaxSessions    int
}

// Evaluate inspects the fleet snapshot and returns a scaling recommendation.
// It is safe to call from any goroutine.
func (as *AutoScaler) Evaluate(snap AutoScalerSnapshot) ScaleDecision {
	as.mu.Lock()
	defer as.mu.Unlock()

	// Count workers that are eligible (online or degraded-equivalent).
	var active, idle int
	for _, w := range snap.Workers {
		if w.Status == WorkerOnline || w.Status == WorkerDraining {
			active++
			if w.ActiveSessions == 0 {
				idle++
			}
		}
	}

	decision := ScaleDecision{
		Action:  ScaleNone,
		Current: active,
		Target:  active,
	}

	// Enforce cooldown.
	if !as.lastScaleTime.IsZero() && time.Since(as.lastScaleTime) < as.config.CooldownDuration {
		decision.Reason = "cooldown active"
		return decision
	}

	// --- Scale-up check ---
	// Queue depth exceeds multiplier * active workers → need more capacity.
	if active > 0 && snap.QueueDepth > int(as.config.QueueDepthMultiplier*float64(active)) {
		// Budget gate: only scale up if remaining budget > floor fraction.
		budgetRemaining := snap.BudgetTotal - snap.BudgetSpent - snap.BudgetReserved
		if budgetRemaining < 0 {
			budgetRemaining = 0
		}
		budgetFraction := 1.0
		if snap.BudgetTotal > 0 {
			budgetFraction = budgetRemaining / snap.BudgetTotal
		}

		if budgetFraction < as.config.BudgetFloorFraction {
			decision.Reason = "scale-up suppressed: budget below floor"
			return decision
		}

		// Calculate how many workers to add: aim for queue/multiplier ratio.
		desired := max(snap.QueueDepth/int(as.config.QueueDepthMultiplier), active+1)
		if desired > as.config.MaxWorkers {
			desired = as.config.MaxWorkers
		}
		delta := desired - active
		if delta <= 0 {
			decision.Reason = "already at max workers"
			return decision
		}

		decision.Action = ScaleUp
		decision.Delta = delta
		decision.Target = desired
		decision.Reason = "queue depth exceeds capacity"
		as.lastScaleTime = time.Now()
		as.lastScaleAction = ScaleUp
		return decision
	}

	// Scale-up when there are zero active workers but pending work exists.
	if active == 0 && snap.QueueDepth > 0 {
		decision.Action = ScaleUp
		decision.Delta = as.config.MinWorkers
		decision.Target = as.config.MinWorkers
		decision.Reason = "no active workers with pending work"
		as.lastScaleTime = time.Now()
		as.lastScaleAction = ScaleUp
		return decision
	}

	// --- Scale-down check ---
	// Idle fraction exceeds threshold → shed excess workers.
	if active > as.config.MinWorkers && active > 0 {
		idleFraction := float64(idle) / float64(active)
		if idleFraction > as.config.IdleWorkerThreshold && snap.QueueDepth == 0 {
			// Scale down by the number of excess idle workers, but respect MinWorkers.
			desired := max(active-idle, as.config.MinWorkers)
			delta := active - desired
			if delta <= 0 {
				decision.Reason = "at minimum workers"
				return decision
			}

			decision.Action = ScaleDown
			decision.Delta = -delta
			decision.Target = desired
			decision.Reason = "idle workers exceed threshold"
			as.lastScaleTime = time.Now()
			as.lastScaleAction = ScaleDown
			return decision
		}
	}

	decision.Reason = "fleet is balanced"
	return decision
}

// HealthScores computes per-worker health scores from recorded metrics.
func (as *AutoScaler) HealthScores() []WorkerHealthScore {
	as.mu.Lock()
	defer as.mu.Unlock()

	scores := make([]WorkerHealthScore, 0, len(as.metrics))
	for id, m := range as.metrics {
		score := WorkerHealthScore{WorkerID: id}
		total := m.successes + m.failures
		if total > 0 {
			score.SuccessRate = float64(m.successes) / float64(total)
		}
		if m.totalTasks > 0 {
			score.StaleRatio = float64(m.staleTasks) / float64(m.totalTasks)
		}
		if len(m.latencies) > 0 {
			score.LatencyP99 = latencyP99(m.latencies)
		}
		scores = append(scores, score)
	}
	return scores
}

// RecordTaskOutcome logs a task completion for health scoring.
func (as *AutoScaler) RecordTaskOutcome(workerID string, success bool, latencySeconds float64, stale bool) {
	as.mu.Lock()
	defer as.mu.Unlock()

	m, ok := as.metrics[workerID]
	if !ok {
		m = &workerMetrics{}
		as.metrics[workerID] = m
	}
	m.totalTasks++
	m.lastUpdated = time.Now()
	if success {
		m.successes++
	} else {
		m.failures++
	}
	if stale {
		m.staleTasks++
	}

	// Keep a bounded sliding window of latencies (last 100).
	const maxLatencies = 100
	m.latencies = append(m.latencies, latencySeconds)
	if len(m.latencies) > maxLatencies {
		m.latencies = m.latencies[len(m.latencies)-maxLatencies:]
	}
}

// ResetCooldown clears the cooldown timer (useful for testing).
func (as *AutoScaler) ResetCooldown() {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.lastScaleTime = time.Time{}
}

// Config returns a copy of the current config.
func (as *AutoScaler) Config() AutoScalerConfig {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.config
}

// latencyP99 returns the 99th percentile latency from an unsorted slice.
// Uses the package-level percentile() from analytics.go on a sorted copy.
func latencyP99(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	// Simple insertion sort — window is bounded at 100 elements.
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}
	return percentile(sorted, 99)
}

// coordinatorAutoScaleCheck is called from the maintenance loop to evaluate
// scaling and log the decision. The coordinator can act on the returned
// decision (e.g., by draining idle workers or requesting new registrations).
func (c *Coordinator) autoScaleCheck() *ScaleDecision {
	if c.autoscaler == nil {
		return nil
	}

	c.mu.RLock()
	workers := make([]WorkerSnapshot, 0, len(c.workers))
	for _, w := range c.workers {
		workers = append(workers, WorkerSnapshot{
			ID:             w.ID,
			Status:         w.Status,
			ActiveSessions: w.ActiveSessions,
			MaxSessions:    w.MaxSessions,
		})
	}
	budget := c.budget
	c.mu.RUnlock()

	snap := AutoScalerSnapshot{
		Workers:        workers,
		QueueDepth:     c.queue.Counts()[WorkPending],
		BudgetTotal:    budget.LimitUSD,
		BudgetSpent:    budget.SpentUSD,
		BudgetReserved: budget.ReservedUSD,
	}

	decision := c.autoscaler.Evaluate(snap)

	if decision.Action != ScaleNone {
		util.Debug.Debugf("autoscaler: %s delta=%d current=%d target=%d reason=%q",
			decision.Action, decision.Delta, decision.Current, decision.Target, decision.Reason)

		// Act on scale-down by draining excess idle workers.
		if decision.Action == ScaleDown {
			c.applyScaleDown(decision)
		}
		// Scale-up is advisory — the coordinator publishes an event so external
		// orchestration (or a future provisioner) can add capacity.
		if decision.Action == ScaleUp && c.bus != nil {
			c.bus.Publish(events.Event{
				Type: "fleet.autoscale",
				Data: map[string]any{
					"action":  decision.Action.String(),
					"delta":   decision.Delta,
					"current": decision.Current,
					"target":  decision.Target,
					"reason":  decision.Reason,
				},
			})
		}
	}

	return &decision
}

// applyScaleDown drains the N least-loaded idle workers.
func (c *Coordinator) applyScaleDown(d ScaleDecision) {
	toDrain := -d.Delta
	if toDrain <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	drained := 0
	for _, w := range c.workers {
		if drained >= toDrain {
			break
		}
		if w.Status == WorkerOnline && w.ActiveSessions == 0 {
			w.Status = WorkerDraining
			drained++
			util.Debug.Debugf("autoscaler: draining idle worker %s", w.ID)
		}
	}
}
