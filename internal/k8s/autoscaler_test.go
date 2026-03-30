package k8s

import (
	"context"
	"fmt"
	"testing"
	"time"
)

// --- Mock implementations ---

type mockMetrics struct {
	metrics FleetMetrics
	err     error
}

func (m *mockMetrics) CurrentMetrics(_ context.Context) (FleetMetrics, error) {
	return m.metrics, m.err
}

type mockScaler struct {
	replicas   int
	scaleCalls []int // records every Scale(n) call
	err        error
}

func (m *mockScaler) CurrentReplicas(_ context.Context) (int, error) {
	return m.replicas, m.err
}

func (m *mockScaler) Scale(_ context.Context, replicas int) error {
	if m.err != nil {
		return m.err
	}
	m.scaleCalls = append(m.scaleCalls, replicas)
	m.replicas = replicas
	return nil
}

// --- Helpers ---

func simplePolicy() ScalePolicy {
	return ScalePolicy{
		MinReplicas:       1,
		MaxReplicas:       10,
		CooldownScaleUp:   30 * time.Second,
		CooldownScaleDown: 60 * time.Second,
		Targets: []MetricTarget{
			{
				Metric:             MetricQueueDepth,
				TargetValue:        5,
				ScaleUpThreshold:   10,
				ScaleDownThreshold: 2,
			},
		},
	}
}

func newTestAutoscaler(policy ScalePolicy, fm FleetMetrics, replicas int) (*Autoscaler, *mockMetrics, *mockScaler) {
	ms := &mockMetrics{metrics: fm}
	ps := &mockScaler{replicas: replicas}
	a := NewAutoscaler(policy, ms, ps)
	return a, ms, ps
}

// --- Tests ---

func TestScaleUpOnHighQueueDepth(t *testing.T) {
	a, _, ps := newTestAutoscaler(simplePolicy(), FleetMetrics{
		QueueDepth: 20, // well above threshold of 10
	}, 2)

	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Direction != ScaleUp {
		t.Errorf("expected ScaleUp, got %s (reason: %s)", evt.Direction, evt.Reason)
	}
	if evt.ToReplicas <= 2 {
		t.Errorf("expected replicas > 2, got %d", evt.ToReplicas)
	}
	if len(ps.scaleCalls) != 1 {
		t.Errorf("expected 1 scale call, got %d", len(ps.scaleCalls))
	}
}

func TestScaleDownOnIdle(t *testing.T) {
	a, _, ps := newTestAutoscaler(simplePolicy(), FleetMetrics{
		QueueDepth: 0, // well below scale-down threshold of 2
	}, 5)

	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Direction != ScaleDown {
		t.Errorf("expected ScaleDown, got %s (reason: %s)", evt.Direction, evt.Reason)
	}
	if evt.ToReplicas >= 5 {
		t.Errorf("expected replicas < 5, got %d", evt.ToReplicas)
	}
	if len(ps.scaleCalls) != 1 {
		t.Errorf("expected 1 scale call, got %d", len(ps.scaleCalls))
	}
}

func TestRespectCooldownScaleUp(t *testing.T) {
	a, ms, ps := newTestAutoscaler(simplePolicy(), FleetMetrics{
		QueueDepth: 20,
	}, 2)

	// First evaluation should scale up.
	_, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Immediately re-evaluate with same high metrics — should be blocked by cooldown.
	ms.metrics = FleetMetrics{QueueDepth: 25}
	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Direction != ScaleNone {
		t.Errorf("expected ScaleNone during cooldown, got %s", evt.Direction)
	}
	if len(ps.scaleCalls) != 1 {
		t.Errorf("expected only 1 scale call (cooldown blocked second), got %d", len(ps.scaleCalls))
	}
}

func TestRespectCooldownScaleDown(t *testing.T) {
	a, ms, ps := newTestAutoscaler(simplePolicy(), FleetMetrics{
		QueueDepth: 0,
	}, 5)

	// First evaluation should scale down.
	_, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Immediately re-evaluate — should be blocked by cooldown.
	ms.metrics = FleetMetrics{QueueDepth: 0}
	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Direction != ScaleNone {
		t.Errorf("expected ScaleNone during cooldown, got %s", evt.Direction)
	}
	if len(ps.scaleCalls) != 1 {
		t.Errorf("expected 1 scale call, got %d", len(ps.scaleCalls))
	}
}

func TestCooldownExpires(t *testing.T) {
	a, _, ps := newTestAutoscaler(simplePolicy(), FleetMetrics{
		QueueDepth: 20,
	}, 2)

	now := time.Now()
	a.nowFunc = func() time.Time { return now }

	// First scale-up.
	_, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ps.scaleCalls) != 1 {
		t.Fatalf("expected 1 scale call, got %d", len(ps.scaleCalls))
	}

	// Advance time past the cooldown.
	now = now.Add(31 * time.Second)

	// Should allow another scale-up.
	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Direction != ScaleUp {
		t.Errorf("expected ScaleUp after cooldown expired, got %s (reason: %s)", evt.Direction, evt.Reason)
	}
	if len(ps.scaleCalls) != 2 {
		t.Errorf("expected 2 scale calls, got %d", len(ps.scaleCalls))
	}
}

func TestMinReplicasFloor(t *testing.T) {
	policy := simplePolicy()
	policy.MinReplicas = 3

	a, _, ps := newTestAutoscaler(policy, FleetMetrics{
		QueueDepth: 0, // idle
	}, 3)

	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Already at min — should not scale down.
	if evt.Direction != ScaleNone {
		t.Errorf("expected ScaleNone at min replicas, got %s (reason: %s)", evt.Direction, evt.Reason)
	}
	if len(ps.scaleCalls) != 0 {
		t.Errorf("expected no scale calls, got %d", len(ps.scaleCalls))
	}
}

func TestMaxReplicasCeiling(t *testing.T) {
	policy := simplePolicy()
	policy.MaxReplicas = 4

	a, _, ps := newTestAutoscaler(policy, FleetMetrics{
		QueueDepth: 100, // very high
	}, 4)

	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Already at max — should not scale up further.
	if evt.Direction != ScaleNone {
		t.Errorf("expected ScaleNone at max replicas, got %s (reason: %s)", evt.Direction, evt.Reason)
	}
	if len(ps.scaleCalls) != 0 {
		t.Errorf("expected no scale calls at max, got %d", len(ps.scaleCalls))
	}
}

func TestScaleDownNeverBelowMin(t *testing.T) {
	policy := simplePolicy()
	policy.MinReplicas = 2

	a, _, ps := newTestAutoscaler(policy, FleetMetrics{
		QueueDepth: 0,
	}, 3)

	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Direction != ScaleDown {
		t.Fatalf("expected ScaleDown, got %s", evt.Direction)
	}
	if evt.ToReplicas < policy.MinReplicas {
		t.Errorf("scaled below min replicas: got %d, min is %d", evt.ToReplicas, policy.MinReplicas)
	}
	if len(ps.scaleCalls) == 0 {
		t.Fatal("expected at least one scale call")
	}
	if ps.scaleCalls[0] < policy.MinReplicas {
		t.Errorf("scale call %d is below min %d", ps.scaleCalls[0], policy.MinReplicas)
	}
}

func TestScaleUpNeverAboveMax(t *testing.T) {
	policy := simplePolicy()
	policy.MaxReplicas = 5

	a, _, _ := newTestAutoscaler(policy, FleetMetrics{
		QueueDepth: 200, // would want many replicas
	}, 3)

	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Direction != ScaleUp {
		t.Fatalf("expected ScaleUp, got %s", evt.Direction)
	}
	if evt.ToReplicas > policy.MaxReplicas {
		t.Errorf("scaled above max: got %d, max is %d", evt.ToReplicas, policy.MaxReplicas)
	}
}

func TestNoChangeWhenBalanced(t *testing.T) {
	a, _, ps := newTestAutoscaler(simplePolicy(), FleetMetrics{
		QueueDepth: 5, // between thresholds (2 and 10)
	}, 3)

	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Direction != ScaleNone {
		t.Errorf("expected ScaleNone for balanced fleet, got %s", evt.Direction)
	}
	if len(ps.scaleCalls) != 0 {
		t.Errorf("expected no scale calls, got %d", len(ps.scaleCalls))
	}
}

func TestMultipleMetricTargets(t *testing.T) {
	policy := ScalePolicy{
		MinReplicas:       1,
		MaxReplicas:       10,
		CooldownScaleUp:   1 * time.Millisecond,
		CooldownScaleDown: 1 * time.Millisecond,
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
		},
	}

	// Queue is fine, but cost rate is high -- should trigger scale-up.
	a, _, _ := newTestAutoscaler(policy, FleetMetrics{
		QueueDepth:  5,
		CostRateUSD: 1.50,
	}, 2)

	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Direction != ScaleUp {
		t.Errorf("expected ScaleUp due to cost rate, got %s (reason: %s)", evt.Direction, evt.Reason)
	}
}

func TestScaleDownRequiresAllMetricsBelowThreshold(t *testing.T) {
	policy := ScalePolicy{
		MinReplicas:       1,
		MaxReplicas:       10,
		CooldownScaleUp:   1 * time.Millisecond,
		CooldownScaleDown: 1 * time.Millisecond,
		Targets: []MetricTarget{
			{
				Metric:             MetricQueueDepth,
				TargetValue:        5,
				ScaleUpThreshold:   10,
				ScaleDownThreshold: 2,
			},
			{
				Metric:             MetricSessionCount,
				TargetValue:        3,
				ScaleUpThreshold:   5,
				ScaleDownThreshold: 1,
			},
		},
	}

	// Queue is low but session count is still above scale-down threshold.
	a, _, _ := newTestAutoscaler(policy, FleetMetrics{
		QueueDepth:   0,
		SessionCount: 3, // >= ScaleDownThreshold of 1
	}, 5)

	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Direction != ScaleNone {
		t.Errorf("expected ScaleNone (session count still active), got %s (reason: %s)", evt.Direction, evt.Reason)
	}
}

func TestMetricsSourceError(t *testing.T) {
	ms := &mockMetrics{err: fmt.Errorf("prometheus unreachable")}
	ps := &mockScaler{replicas: 3}
	a := NewAutoscaler(simplePolicy(), ms, ps)

	_, err := a.Evaluate(context.Background())
	if err == nil {
		t.Fatal("expected error from metrics source, got nil")
	}
}

func TestPodScalerError(t *testing.T) {
	ms := &mockMetrics{metrics: FleetMetrics{QueueDepth: 20}}
	ps := &mockScaler{replicas: 2, err: fmt.Errorf("kube api unavailable")}
	a := NewAutoscaler(simplePolicy(), ms, ps)

	_, err := a.Evaluate(context.Background())
	if err == nil {
		t.Fatal("expected error from pod scaler, got nil")
	}
}

func TestHistoryRecorded(t *testing.T) {
	a, _, _ := newTestAutoscaler(simplePolicy(), FleetMetrics{
		QueueDepth: 20,
	}, 2)

	_, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	h := a.History()
	if len(h) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(h))
	}
	if h[0].Direction != ScaleUp {
		t.Errorf("expected ScaleUp in history, got %s", h[0].Direction)
	}
	if h[0].FromReplicas != 2 {
		t.Errorf("expected FromReplicas=2, got %d", h[0].FromReplicas)
	}
}

func TestDefaultPolicyValidation(t *testing.T) {
	// Zero-value policy should get safe defaults.
	a := NewAutoscaler(ScalePolicy{}, &mockMetrics{}, &mockScaler{})
	p := a.Policy()
	if p.MinReplicas < 1 {
		t.Errorf("expected MinReplicas >= 1, got %d", p.MinReplicas)
	}
	if p.MaxReplicas < p.MinReplicas {
		t.Errorf("expected MaxReplicas >= MinReplicas, got max=%d min=%d", p.MaxReplicas, p.MinReplicas)
	}
	if p.CooldownScaleUp <= 0 {
		t.Error("expected positive CooldownScaleUp")
	}
	if p.CooldownScaleDown <= 0 {
		t.Error("expected positive CooldownScaleDown")
	}
}

func TestResetCooldown(t *testing.T) {
	a, _, ps := newTestAutoscaler(simplePolicy(), FleetMetrics{
		QueueDepth: 20,
	}, 2)

	// First scale-up.
	_, _ = a.Evaluate(context.Background())

	// Reset cooldown.
	a.ResetCooldown()

	// Should be able to scale again immediately.
	evt, err := a.Evaluate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if evt.Direction != ScaleUp {
		t.Errorf("expected ScaleUp after ResetCooldown, got %s", evt.Direction)
	}
	if len(ps.scaleCalls) != 2 {
		t.Errorf("expected 2 scale calls after reset, got %d", len(ps.scaleCalls))
	}
}

func TestFleetMetricsValue(t *testing.T) {
	fm := FleetMetrics{
		QueueDepth:   15,
		CostRateUSD:  0.75,
		SessionCount: 4,
	}

	tests := []struct {
		metric MetricType
		want   float64
	}{
		{MetricQueueDepth, 15},
		{MetricCostRate, 0.75},
		{MetricSessionCount, 4},
		{"unknown_metric", 0},
	}

	for _, tt := range tests {
		got := fm.Value(tt.metric)
		if got != tt.want {
			t.Errorf("Value(%s) = %v, want %v", tt.metric, got, tt.want)
		}
	}
}

func TestScaleDirectionString(t *testing.T) {
	tests := []struct {
		d    ScaleDirection
		want string
	}{
		{ScaleNone, "no_change"},
		{ScaleUp, "scale_up"},
		{ScaleDown, "scale_down"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("ScaleDirection(%d).String() = %q, want %q", tt.d, got, tt.want)
		}
	}
}
