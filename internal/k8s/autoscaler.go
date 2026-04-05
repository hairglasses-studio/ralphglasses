package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// MetricType identifies a fleet metric used for scaling decisions.
type MetricType string

const (
	MetricQueueDepth   MetricType = "queue_depth"
	MetricCostRate     MetricType = "cost_rate_usd_per_min"
	MetricSessionCount MetricType = "session_count"
)

// MetricTarget defines a threshold for a single metric that triggers scaling.
type MetricTarget struct {
	Metric             MetricType `json:"metric"`
	TargetValue        float64    `json:"target_value"`         // desired per-pod average
	ScaleUpThreshold   float64    `json:"scale_up_threshold"`   // scale up when value exceeds this
	ScaleDownThreshold float64    `json:"scale_down_threshold"` // scale down when value drops below this
}

// ScalePolicy configures the HPA-like autoscaler behavior.
type ScalePolicy struct {
	MinReplicas       int            `json:"min_replicas"`
	MaxReplicas       int            `json:"max_replicas"`
	CooldownScaleUp   time.Duration  `json:"cooldown_scale_up"`
	CooldownScaleDown time.Duration  `json:"cooldown_scale_down"`
	Targets           []MetricTarget `json:"targets"`
}

// DefaultScalePolicy returns a production-safe default policy.
func DefaultScalePolicy() ScalePolicy {
	return ScalePolicy{
		MinReplicas:       1,
		MaxReplicas:       10,
		CooldownScaleUp:   30 * time.Second,
		CooldownScaleDown: 120 * time.Second,
		Targets: []MetricTarget{
			{
				Metric:             MetricQueueDepth,
				TargetValue:        5,
				ScaleUpThreshold:   10,
				ScaleDownThreshold: 2,
			},
			{
				Metric:             MetricCostRate,
				TargetValue:        0.50,
				ScaleUpThreshold:   1.00,
				ScaleDownThreshold: 0.10,
			},
			{
				Metric:             MetricSessionCount,
				TargetValue:        3,
				ScaleUpThreshold:   5,
				ScaleDownThreshold: 1,
			},
		},
	}
}

// FleetMetrics is the snapshot of current fleet-level metrics.
type FleetMetrics struct {
	QueueDepth   float64 `json:"queue_depth"`
	CostRateUSD  float64 `json:"cost_rate_usd_per_min"`
	SessionCount float64 `json:"session_count"`
}

// Value returns the metric value for the given type.
func (m FleetMetrics) Value(mt MetricType) float64 {
	switch mt {
	case MetricQueueDepth:
		return m.QueueDepth
	case MetricCostRate:
		return m.CostRateUSD
	case MetricSessionCount:
		return m.SessionCount
	default:
		return 0
	}
}

// MetricsSource provides fleet metrics to the autoscaler.
// Implementations may read from Prometheus, the event bus, or in-memory state.
type MetricsSource interface {
	// CurrentMetrics returns the latest fleet metric snapshot.
	CurrentMetrics(ctx context.Context) (FleetMetrics, error)
}

// PodScaler controls the number of RalphSession pod replicas.
// Implementations may call the Kubernetes API, a local process manager, or a mock.
type PodScaler interface {
	// CurrentReplicas returns the number of running RalphSession pods.
	CurrentReplicas(ctx context.Context) (int, error)
	// Scale adjusts the replica count to the desired value.
	Scale(ctx context.Context, replicas int) error
}

// ScaleDirection indicates which way the autoscaler wants to move.
type ScaleDirection int

const (
	ScaleNone ScaleDirection = iota
	ScaleUp
	ScaleDown
)

// String returns a human-readable label.
func (d ScaleDirection) String() string {
	switch d {
	case ScaleUp:
		return "scale_up"
	case ScaleDown:
		return "scale_down"
	default:
		return "no_change"
	}
}

// ScaleEvent records a scaling decision for observability.
type ScaleEvent struct {
	Direction    ScaleDirection `json:"direction"`
	FromReplicas int            `json:"from_replicas"`
	ToReplicas   int            `json:"to_replicas"`
	Reason       string         `json:"reason"`
	Timestamp    time.Time      `json:"timestamp"`
	Metrics      FleetMetrics   `json:"metrics"`
}

// Autoscaler watches fleet metrics and scales RalphSession pods up or down,
// similar to a Kubernetes Horizontal Pod Autoscaler.
type Autoscaler struct {
	mu     sync.Mutex
	policy ScalePolicy

	metrics MetricsSource
	scaler  PodScaler

	lastScaleUp   time.Time
	lastScaleDown time.Time

	// History of scaling events for observability.
	history []ScaleEvent

	// nowFunc allows tests to control time.
	nowFunc func() time.Time
}

// NewAutoscaler creates an autoscaler with the given policy, metrics source,
// and pod scaler. The policy is validated and defaults are applied for zero values.
func NewAutoscaler(policy ScalePolicy, metrics MetricsSource, scaler PodScaler) *Autoscaler {
	if policy.MinReplicas <= 0 {
		policy.MinReplicas = 1
	}
	if policy.MaxReplicas <= 0 {
		policy.MaxReplicas = 10
	}
	if policy.MaxReplicas < policy.MinReplicas {
		policy.MaxReplicas = policy.MinReplicas
	}
	if policy.CooldownScaleUp <= 0 {
		policy.CooldownScaleUp = 30 * time.Second
	}
	if policy.CooldownScaleDown <= 0 {
		policy.CooldownScaleDown = 120 * time.Second
	}

	return &Autoscaler{
		policy:  policy,
		metrics: metrics,
		scaler:  scaler,
		nowFunc: time.Now,
	}
}

// Evaluate reads current metrics and replica count, computes a scaling
// decision, and applies it if not in cooldown. Returns the event describing
// what happened (or ScaleNone if no action was taken).
func (a *Autoscaler) Evaluate(ctx context.Context) (ScaleEvent, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := a.nowFunc()

	fm, err := a.metrics.CurrentMetrics(ctx)
	if err != nil {
		return ScaleEvent{}, fmt.Errorf("k8s autoscaler: fetch metrics: %w", err)
	}

	current, err := a.scaler.CurrentReplicas(ctx)
	if err != nil {
		return ScaleEvent{}, fmt.Errorf("k8s autoscaler: fetch replicas: %w", err)
	}

	direction, reason := a.decide(fm, current)
	desired := a.desiredReplicas(fm, current, direction)

	evt := ScaleEvent{
		Direction:    ScaleNone,
		FromReplicas: current,
		ToReplicas:   current,
		Reason:       reason,
		Timestamp:    now,
		Metrics:      fm,
	}

	if direction == ScaleNone || desired == current {
		evt.Reason = "fleet is balanced"
		return evt, nil
	}

	// Check cooldown.
	if direction == ScaleUp && !a.lastScaleUp.IsZero() && now.Sub(a.lastScaleUp) < a.policy.CooldownScaleUp {
		evt.Reason = "scale-up cooldown active"
		return evt, nil
	}
	if direction == ScaleDown && !a.lastScaleDown.IsZero() && now.Sub(a.lastScaleDown) < a.policy.CooldownScaleDown {
		evt.Reason = "scale-down cooldown active"
		return evt, nil
	}

	// Clamp to bounds.
	if desired < a.policy.MinReplicas {
		desired = a.policy.MinReplicas
	}
	if desired > a.policy.MaxReplicas {
		desired = a.policy.MaxReplicas
	}
	if desired == current {
		evt.Reason = "clamped to bounds; no change"
		return evt, nil
	}

	// Apply the scaling action.
	if err := a.scaler.Scale(ctx, desired); err != nil {
		return ScaleEvent{}, fmt.Errorf("k8s autoscaler: scale to %d: %w", desired, err)
	}

	evt.Direction = direction
	evt.ToReplicas = desired

	// Record cooldown.
	if direction == ScaleUp {
		a.lastScaleUp = now
	} else {
		a.lastScaleDown = now
	}

	a.history = append(a.history, evt)

	return evt, nil
}

// decide determines the scaling direction and reason based on metric targets.
// Scale-up wins if any metric exceeds its scale-up threshold.
// Scale-down requires all metrics to be below their scale-down thresholds.
func (a *Autoscaler) decide(fm FleetMetrics, current int) (ScaleDirection, string) {
	if current >= a.policy.MaxReplicas {
		// Cannot scale up further; check for scale-down only.
		allBelow := true
		for _, t := range a.policy.Targets {
			if fm.Value(t.Metric) >= t.ScaleDownThreshold {
				allBelow = false
				break
			}
		}
		if allBelow && current > a.policy.MinReplicas {
			return ScaleDown, "all metrics below scale-down thresholds"
		}
		return ScaleNone, "at max replicas"
	}

	// Check for scale-up: any metric above its threshold triggers scale-up.
	for _, t := range a.policy.Targets {
		if fm.Value(t.Metric) > t.ScaleUpThreshold {
			return ScaleUp, fmt.Sprintf("%s (%.2f) exceeds threshold (%.2f)", t.Metric, fm.Value(t.Metric), t.ScaleUpThreshold)
		}
	}

	// Check for scale-down: all metrics must be below scale-down thresholds.
	if current > a.policy.MinReplicas {
		allBelow := true
		for _, t := range a.policy.Targets {
			if fm.Value(t.Metric) >= t.ScaleDownThreshold {
				allBelow = false
				break
			}
		}
		if allBelow {
			return ScaleDown, "all metrics below scale-down thresholds"
		}
	}

	return ScaleNone, "fleet is balanced"
}

// desiredReplicas calculates the target replica count for the given direction.
// For scale-up, it picks the highest replica count needed across all metrics.
// For scale-down, it picks the lowest safe count.
func (a *Autoscaler) desiredReplicas(fm FleetMetrics, current int, dir ScaleDirection) int {
	if dir == ScaleNone {
		return current
	}

	desired := current
	for _, t := range a.policy.Targets {
		if t.TargetValue <= 0 {
			continue
		}
		// Compute how many replicas we would need for this metric to hit its target value.
		// replicas = ceil(metricValue / targetValue)
		metricReplicas := max(int(fm.Value(t.Metric)/t.TargetValue)+1, 1)

		if dir == ScaleUp && metricReplicas > desired {
			desired = metricReplicas
		}
		if dir == ScaleDown && metricReplicas < desired {
			desired = metricReplicas
		}
	}

	// For scale-down, never remove more than half the current pods at once.
	if dir == ScaleDown && desired < current/2 {
		desired = current / 2
	}

	// At minimum, move by 1 in the desired direction.
	if dir == ScaleUp && desired <= current {
		desired = current + 1
	}
	if dir == ScaleDown && desired >= current {
		desired = current - 1
	}

	return desired
}

// History returns a copy of recorded scaling events.
func (a *Autoscaler) History() []ScaleEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]ScaleEvent, len(a.history))
	copy(out, a.history)
	return out
}

// Policy returns a copy of the current scaling policy.
func (a *Autoscaler) Policy() ScalePolicy {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.policy
}

// ResetCooldown clears cooldown timers. Intended for testing.
func (a *Autoscaler) ResetCooldown() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastScaleUp = time.Time{}
	a.lastScaleDown = time.Time{}
}
