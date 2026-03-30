package fleet

import (
	"testing"
	"time"
)

func TestAutoScaler_ScaleUpOnQueueDepth(t *testing.T) {
	cfg := DefaultAutoScalerConfig()
	cfg.CooldownDuration = 0 // disable cooldown for test
	as := NewAutoScaler(cfg)

	// 3 workers, queue depth = 10 → 10 > 2*3 = 6 → scale up
	snap := AutoScalerSnapshot{
		Workers: []WorkerSnapshot{
			{ID: "w1", Status: WorkerOnline, ActiveSessions: 2, MaxSessions: 4},
			{ID: "w2", Status: WorkerOnline, ActiveSessions: 3, MaxSessions: 4},
			{ID: "w3", Status: WorkerOnline, ActiveSessions: 1, MaxSessions: 4},
		},
		QueueDepth:  10,
		BudgetTotal: 500,
		BudgetSpent: 100,
	}

	d := as.Evaluate(snap)
	if d.Action != ScaleUp {
		t.Fatalf("expected ScaleUp, got %s (reason: %s)", d.Action, d.Reason)
	}
	if d.Delta <= 0 {
		t.Fatalf("expected positive delta, got %d", d.Delta)
	}
	if d.Target <= d.Current {
		t.Fatalf("target %d should exceed current %d", d.Target, d.Current)
	}
}

func TestAutoScaler_ScaleDownOnIdleWorkers(t *testing.T) {
	cfg := DefaultAutoScalerConfig()
	cfg.CooldownDuration = 0
	cfg.MinWorkers = 2
	as := NewAutoScaler(cfg)

	// 6 workers, 4 idle (66% > 50% threshold), queue empty → scale down
	snap := AutoScalerSnapshot{
		Workers: []WorkerSnapshot{
			{ID: "w1", Status: WorkerOnline, ActiveSessions: 2, MaxSessions: 4},
			{ID: "w2", Status: WorkerOnline, ActiveSessions: 1, MaxSessions: 4},
			{ID: "w3", Status: WorkerOnline, ActiveSessions: 0, MaxSessions: 4},
			{ID: "w4", Status: WorkerOnline, ActiveSessions: 0, MaxSessions: 4},
			{ID: "w5", Status: WorkerOnline, ActiveSessions: 0, MaxSessions: 4},
			{ID: "w6", Status: WorkerOnline, ActiveSessions: 0, MaxSessions: 4},
		},
		QueueDepth:  0,
		BudgetTotal: 500,
		BudgetSpent: 100,
	}

	d := as.Evaluate(snap)
	if d.Action != ScaleDown {
		t.Fatalf("expected ScaleDown, got %s (reason: %s)", d.Action, d.Reason)
	}
	if d.Delta >= 0 {
		t.Fatalf("expected negative delta for scale-down, got %d", d.Delta)
	}
	if d.Target < cfg.MinWorkers {
		t.Fatalf("target %d below MinWorkers %d", d.Target, cfg.MinWorkers)
	}
}

func TestAutoScaler_NoChangeWhenBalanced(t *testing.T) {
	cfg := DefaultAutoScalerConfig()
	cfg.CooldownDuration = 0
	as := NewAutoScaler(cfg)

	// 3 workers, queue = 4 → 4 <= 2*3 = 6, only 1 idle (33% < 50%) → no change
	snap := AutoScalerSnapshot{
		Workers: []WorkerSnapshot{
			{ID: "w1", Status: WorkerOnline, ActiveSessions: 2, MaxSessions: 4},
			{ID: "w2", Status: WorkerOnline, ActiveSessions: 1, MaxSessions: 4},
			{ID: "w3", Status: WorkerOnline, ActiveSessions: 0, MaxSessions: 4},
		},
		QueueDepth:  4,
		BudgetTotal: 500,
		BudgetSpent: 100,
	}

	d := as.Evaluate(snap)
	if d.Action != ScaleNone {
		t.Fatalf("expected ScaleNone, got %s (reason: %s)", d.Action, d.Reason)
	}
}

func TestAutoScaler_BudgetGatePreventsScaleUp(t *testing.T) {
	cfg := DefaultAutoScalerConfig()
	cfg.CooldownDuration = 0
	cfg.BudgetFloorFraction = 0.10
	as := NewAutoScaler(cfg)

	// Queue is deep enough to trigger scale-up, but budget is 95% spent.
	snap := AutoScalerSnapshot{
		Workers: []WorkerSnapshot{
			{ID: "w1", Status: WorkerOnline, ActiveSessions: 4, MaxSessions: 4},
		},
		QueueDepth:  10,
		BudgetTotal: 100,
		BudgetSpent: 95,
	}

	d := as.Evaluate(snap)
	if d.Action == ScaleUp {
		t.Fatalf("expected budget gate to suppress scale-up, got %s (reason: %s)", d.Action, d.Reason)
	}
	if d.Reason != "scale-up suppressed: budget below floor" {
		t.Fatalf("unexpected reason: %s", d.Reason)
	}
}

func TestAutoScaler_RespectsMaxWorkers(t *testing.T) {
	cfg := DefaultAutoScalerConfig()
	cfg.CooldownDuration = 0
	cfg.MaxWorkers = 5
	as := NewAutoScaler(cfg)

	// 4 workers, huge queue → should cap at 5.
	snap := AutoScalerSnapshot{
		Workers: []WorkerSnapshot{
			{ID: "w1", Status: WorkerOnline, ActiveSessions: 4, MaxSessions: 4},
			{ID: "w2", Status: WorkerOnline, ActiveSessions: 4, MaxSessions: 4},
			{ID: "w3", Status: WorkerOnline, ActiveSessions: 4, MaxSessions: 4},
			{ID: "w4", Status: WorkerOnline, ActiveSessions: 4, MaxSessions: 4},
		},
		QueueDepth:  100,
		BudgetTotal: 500,
		BudgetSpent: 0,
	}

	d := as.Evaluate(snap)
	if d.Action != ScaleUp {
		t.Fatalf("expected ScaleUp, got %s (reason: %s)", d.Action, d.Reason)
	}
	if d.Target > cfg.MaxWorkers {
		t.Fatalf("target %d exceeds MaxWorkers %d", d.Target, cfg.MaxWorkers)
	}
}

func TestAutoScaler_RespectsMinWorkers(t *testing.T) {
	cfg := DefaultAutoScalerConfig()
	cfg.CooldownDuration = 0
	cfg.MinWorkers = 3
	as := NewAutoScaler(cfg)

	// 4 workers, 3 idle, queue empty → should not go below min=3
	snap := AutoScalerSnapshot{
		Workers: []WorkerSnapshot{
			{ID: "w1", Status: WorkerOnline, ActiveSessions: 1, MaxSessions: 4},
			{ID: "w2", Status: WorkerOnline, ActiveSessions: 0, MaxSessions: 4},
			{ID: "w3", Status: WorkerOnline, ActiveSessions: 0, MaxSessions: 4},
			{ID: "w4", Status: WorkerOnline, ActiveSessions: 0, MaxSessions: 4},
		},
		QueueDepth:  0,
		BudgetTotal: 500,
		BudgetSpent: 100,
	}

	d := as.Evaluate(snap)
	if d.Action == ScaleDown && d.Target < cfg.MinWorkers {
		t.Fatalf("target %d below MinWorkers %d", d.Target, cfg.MinWorkers)
	}
}

func TestAutoScaler_CooldownPreventsFLapping(t *testing.T) {
	cfg := DefaultAutoScalerConfig()
	cfg.CooldownDuration = 10 * time.Minute // long cooldown
	as := NewAutoScaler(cfg)

	// First eval should scale up.
	snap := AutoScalerSnapshot{
		Workers: []WorkerSnapshot{
			{ID: "w1", Status: WorkerOnline, ActiveSessions: 4, MaxSessions: 4},
		},
		QueueDepth:  10,
		BudgetTotal: 500,
		BudgetSpent: 0,
	}

	d1 := as.Evaluate(snap)
	if d1.Action != ScaleUp {
		t.Fatalf("first eval: expected ScaleUp, got %s", d1.Action)
	}

	// Second eval immediately after should be suppressed by cooldown.
	d2 := as.Evaluate(snap)
	if d2.Action != ScaleNone {
		t.Fatalf("second eval: expected ScaleNone (cooldown), got %s (reason: %s)", d2.Action, d2.Reason)
	}
	if d2.Reason != "cooldown active" {
		t.Fatalf("expected cooldown reason, got %q", d2.Reason)
	}
}

func TestAutoScaler_ScaleUpWithNoWorkers(t *testing.T) {
	cfg := DefaultAutoScalerConfig()
	cfg.CooldownDuration = 0
	cfg.MinWorkers = 2
	as := NewAutoScaler(cfg)

	// No workers, pending work → should recommend MinWorkers.
	snap := AutoScalerSnapshot{
		Workers:     nil,
		QueueDepth:  5,
		BudgetTotal: 500,
		BudgetSpent: 0,
	}

	d := as.Evaluate(snap)
	if d.Action != ScaleUp {
		t.Fatalf("expected ScaleUp, got %s (reason: %s)", d.Action, d.Reason)
	}
	if d.Target != cfg.MinWorkers {
		t.Fatalf("expected target=%d (MinWorkers), got %d", cfg.MinWorkers, d.Target)
	}
}

func TestAutoScaler_HealthScores(t *testing.T) {
	as := NewAutoScaler(DefaultAutoScalerConfig())

	// Record some outcomes.
	as.RecordTaskOutcome("w1", true, 1.2, false)
	as.RecordTaskOutcome("w1", true, 0.8, false)
	as.RecordTaskOutcome("w1", false, 5.0, true)
	as.RecordTaskOutcome("w2", true, 0.5, false)

	scores := as.HealthScores()
	if len(scores) != 2 {
		t.Fatalf("expected 2 worker scores, got %d", len(scores))
	}

	// Find w1's score.
	var w1Score WorkerHealthScore
	for _, s := range scores {
		if s.WorkerID == "w1" {
			w1Score = s
			break
		}
	}

	// w1: 2 successes, 1 failure → rate = 2/3 ≈ 0.667
	expectedRate := 2.0 / 3.0
	if w1Score.SuccessRate < expectedRate-0.01 || w1Score.SuccessRate > expectedRate+0.01 {
		t.Fatalf("expected success rate ~%.3f, got %.3f", expectedRate, w1Score.SuccessRate)
	}
	// w1: 1 stale out of 3 tasks → 0.333
	expectedStale := 1.0 / 3.0
	if w1Score.StaleRatio < expectedStale-0.01 || w1Score.StaleRatio > expectedStale+0.01 {
		t.Fatalf("expected stale ratio ~%.3f, got %.3f", expectedStale, w1Score.StaleRatio)
	}
	// p99 latency should be close to 5.0 (the highest value).
	if w1Score.LatencyP99 < 4.0 {
		t.Fatalf("expected p99 latency near 5.0, got %.2f", w1Score.LatencyP99)
	}
}

func TestAutoScaler_ScaleActionString(t *testing.T) {
	cases := []struct {
		action ScaleAction
		want   string
	}{
		{ScaleNone, "no_change"},
		{ScaleUp, "scale_up"},
		{ScaleDown, "scale_down"},
	}
	for _, tc := range cases {
		if got := tc.action.String(); got != tc.want {
			t.Errorf("ScaleAction(%d).String() = %q, want %q", tc.action, got, tc.want)
		}
	}
}

func TestAutoScaler_DisconnectedWorkersNotCounted(t *testing.T) {
	cfg := DefaultAutoScalerConfig()
	cfg.CooldownDuration = 0
	as := NewAutoScaler(cfg)

	// 1 online worker, 2 disconnected — only 1 active. Queue = 5 > 2*1 → scale up.
	snap := AutoScalerSnapshot{
		Workers: []WorkerSnapshot{
			{ID: "w1", Status: WorkerOnline, ActiveSessions: 1, MaxSessions: 4},
			{ID: "w2", Status: WorkerDisconnected, ActiveSessions: 0, MaxSessions: 4},
			{ID: "w3", Status: WorkerDisconnected, ActiveSessions: 0, MaxSessions: 4},
		},
		QueueDepth:  5,
		BudgetTotal: 500,
		BudgetSpent: 0,
	}

	d := as.Evaluate(snap)
	if d.Current != 1 {
		t.Fatalf("expected current=1 (only online workers), got %d", d.Current)
	}
	if d.Action != ScaleUp {
		t.Fatalf("expected ScaleUp, got %s", d.Action)
	}
}

func TestDefaultAutoScalerConfig(t *testing.T) {
	cfg := DefaultAutoScalerConfig()
	if cfg.MinWorkers != 2 {
		t.Errorf("MinWorkers = %d, want 2", cfg.MinWorkers)
	}
	if cfg.MaxWorkers != 32 {
		t.Errorf("MaxWorkers = %d, want 32", cfg.MaxWorkers)
	}
	if cfg.QueueDepthMultiplier != 2.0 {
		t.Errorf("QueueDepthMultiplier = %f, want 2.0", cfg.QueueDepthMultiplier)
	}
	if cfg.IdleWorkerThreshold != 0.5 {
		t.Errorf("IdleWorkerThreshold = %f, want 0.5", cfg.IdleWorkerThreshold)
	}
	if cfg.BudgetFloorFraction != 0.10 {
		t.Errorf("BudgetFloorFraction = %f, want 0.10", cfg.BudgetFloorFraction)
	}
	if cfg.CooldownDuration != 60*time.Second {
		t.Errorf("CooldownDuration = %v, want 60s", cfg.CooldownDuration)
	}
}
