package fleet

import (
	"testing"
	"time"
)

func TestAutoScaler_ResetCooldown(t *testing.T) {
	t.Parallel()
	cfg := DefaultAutoScalerConfig()
	cfg.CooldownDuration = 10 * time.Minute
	as := NewAutoScaler(cfg)

	// Trigger a scaling event to set the cooldown.
	snap := AutoScalerSnapshot{
		Workers: []WorkerSnapshot{
			{ID: "w1", Status: WorkerOnline, ActiveSessions: 4, MaxSessions: 4},
		},
		QueueDepth:  20,
		BudgetTotal: 500,
		BudgetSpent: 0,
	}
	d1 := as.Evaluate(snap)
	if d1.Action != ScaleUp {
		t.Fatalf("first eval: expected ScaleUp, got %s", d1.Action)
	}

	// Verify cooldown blocks second eval.
	d2 := as.Evaluate(snap)
	if d2.Action != ScaleNone {
		t.Fatalf("second eval (cooldown): expected ScaleNone, got %s", d2.Action)
	}

	// Reset cooldown and verify scaling works again.
	as.ResetCooldown()
	d3 := as.Evaluate(snap)
	if d3.Action != ScaleUp {
		t.Fatalf("after ResetCooldown: expected ScaleUp, got %s (reason: %s)", d3.Action, d3.Reason)
	}
}

func TestAutoScaler_Config(t *testing.T) {
	t.Parallel()
	cfg := DefaultAutoScalerConfig()
	cfg.MinWorkers = 5
	cfg.MaxWorkers = 20
	as := NewAutoScaler(cfg)

	got := as.Config()
	if got.MinWorkers != 5 {
		t.Errorf("Config().MinWorkers = %d, want 5", got.MinWorkers)
	}
	if got.MaxWorkers != 20 {
		t.Errorf("Config().MaxWorkers = %d, want 20", got.MaxWorkers)
	}
}

func TestNewAutoScaler_DefaultsForZeroConfig(t *testing.T) {
	t.Parallel()
	// All zero values should be replaced with defaults.
	as := NewAutoScaler(AutoScalerConfig{})
	cfg := as.Config()

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

func TestNewAutoScaler_MaxLessThanMin(t *testing.T) {
	t.Parallel()
	// MaxWorkers < MinWorkers should be corrected to MinWorkers.
	as := NewAutoScaler(AutoScalerConfig{
		MinWorkers: 10,
		MaxWorkers: 5,
	})
	cfg := as.Config()
	if cfg.MaxWorkers < cfg.MinWorkers {
		t.Errorf("MaxWorkers %d < MinWorkers %d", cfg.MaxWorkers, cfg.MinWorkers)
	}
}

func TestLatencyP99_SingleValue(t *testing.T) {
	t.Parallel()
	got := latencyP99([]float64{3.14})
	if got != 3.14 {
		t.Errorf("latencyP99([3.14]) = %f, want 3.14", got)
	}
}

func TestLatencyP99_Empty(t *testing.T) {
	t.Parallel()
	got := latencyP99(nil)
	if got != 0 {
		t.Errorf("latencyP99(nil) = %f, want 0", got)
	}
}

func TestLatencyP99_MultipleValues(t *testing.T) {
	t.Parallel()
	vals := []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0}
	got := latencyP99(vals)
	// P99 of [1..10] should be 10 (the max or very close to it).
	if got < 9.5 {
		t.Errorf("latencyP99 of 1-10 = %f, want >= 9.5", got)
	}
}

func TestAutoScaler_RecordTaskOutcome_NewWorker(t *testing.T) {
	t.Parallel()
	as := NewAutoScaler(DefaultAutoScalerConfig())

	// Recording for a new worker should create metrics.
	as.RecordTaskOutcome("new-worker", true, 1.5, false)

	scores := as.HealthScores()
	found := false
	for _, s := range scores {
		if s.WorkerID == "new-worker" {
			found = true
			if s.SuccessRate != 1.0 {
				t.Errorf("success rate = %f, want 1.0", s.SuccessRate)
			}
		}
	}
	if !found {
		t.Error("expected new-worker in health scores")
	}
}
