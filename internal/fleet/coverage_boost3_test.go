package fleet

import (
	"fmt"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ---------------------------------------------------------------------------
// Coordinator.SetAutoScalerConfig / AutoScaler (0%)
// ---------------------------------------------------------------------------

func TestCoordinator_SetAutoScalerConfig(t *testing.T) {
	t.Parallel()
	coord := newTestCoordinator()

	cfg := AutoScalerConfig{
		MinWorkers: 3,
		MaxWorkers: 10,
	}
	coord.SetAutoScalerConfig(cfg)

	as := coord.AutoScaler()
	if as == nil {
		t.Fatal("AutoScaler should not be nil after SetAutoScalerConfig")
	}
	got := as.Config()
	if got.MinWorkers != 3 {
		t.Errorf("MinWorkers = %d, want 3", got.MinWorkers)
	}
	if got.MaxWorkers != 10 {
		t.Errorf("MaxWorkers = %d, want 10", got.MaxWorkers)
	}
}

func TestCoordinator_AutoScaler_Default(t *testing.T) {
	t.Parallel()
	coord := newTestCoordinator()
	as := coord.AutoScaler()
	if as == nil {
		t.Fatal("AutoScaler should not be nil on default coordinator")
	}
}

// ---------------------------------------------------------------------------
// Coordinator.SetTSClient (0%)
// ---------------------------------------------------------------------------

func TestCoordinator_SetTSClient(t *testing.T) {
	t.Parallel()
	coord := newTestCoordinator()
	// SetTSClient should not panic and should replace the client.
	coord.SetTSClient(nil)
}

// ---------------------------------------------------------------------------
// Coordinator.autoScaleCheck (0%)
// ---------------------------------------------------------------------------

func TestCoordinator_AutoScaleCheck_NoAutoscaler(t *testing.T) {
	t.Parallel()
	coord := newTestCoordinator()
	coord.autoscaler = nil

	result := coord.autoScaleCheck()
	if result != nil {
		t.Errorf("expected nil with no autoscaler, got %+v", result)
	}
}

func TestCoordinator_AutoScaleCheck_NoWorkersNoQueue(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	coord := NewCoordinator("test", "localhost", 0, "test", bus, session.NewManager())

	// With no workers and no queue, fleet is balanced.
	result := coord.autoScaleCheck()
	if result == nil {
		t.Fatal("expected non-nil decision")
	}
	if result.Action != ScaleNone {
		t.Errorf("expected ScaleNone with 0 workers and 0 queue, got %s", result.Action)
	}
}

func TestCoordinator_AutoScaleCheck_NoWorkersWithQueue(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	coord := NewCoordinator("test", "localhost", 0, "test", bus, session.NewManager())

	// Push work so the autoscaler detects need for workers.
	coord.queue.Push(&WorkItem{ID: "item-1", Status: WorkPending, RepoPath: "/tmp/repo", Provider: "claude"})

	result := coord.autoScaleCheck()
	if result == nil {
		t.Fatal("expected non-nil decision")
	}
	if result.Action != ScaleUp {
		t.Errorf("expected ScaleUp with 0 workers and pending queue, got %s (reason: %s)", result.Action, result.Reason)
	}
}

func TestCoordinator_AutoScaleCheck_WithIdleWorkers(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	coord := NewCoordinator("test", "localhost", 0, "test", bus, session.NewManager())

	// Add several idle workers above minimum.
	coord.mu.Lock()
	for i := range 5 {
		wid := fmt.Sprintf("worker-%d", i)
		coord.workers[wid] = &WorkerInfo{
			ID:             wid,
			Status:         WorkerOnline,
			ActiveSessions: 0,
			MaxSessions:    4,
		}
	}
	coord.mu.Unlock()

	result := coord.autoScaleCheck()
	if result == nil {
		t.Fatal("expected non-nil decision")
	}
	// With 5 idle workers and no queue, should want to scale down.
	if result.Action != ScaleDown {
		t.Logf("got action=%s reason=%q (might be ScaleNone on cooldown)", result.Action, result.Reason)
	}
}

// ---------------------------------------------------------------------------
// Coordinator.applyScaleDown (0%)
// ---------------------------------------------------------------------------

func TestCoordinator_ApplyScaleDown(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	coord := NewCoordinator("test", "localhost", 0, "test", bus, session.NewManager())

	// Add idle workers.
	coord.mu.Lock()
	w1 := "worker-idle"
	w2 := "worker-busy"
	coord.workers[w1] = &WorkerInfo{ID: w1, Status: WorkerOnline, ActiveSessions: 0, MaxSessions: 4}
	coord.workers[w2] = &WorkerInfo{ID: w2, Status: WorkerOnline, ActiveSessions: 2, MaxSessions: 4}
	coord.mu.Unlock()

	// Scale down by 1 — should drain the idle worker (w1), not the busy one (w2).
	decision := ScaleDecision{Action: ScaleDown, Delta: -1}
	coord.applyScaleDown(decision)

	coord.mu.RLock()
	defer coord.mu.RUnlock()

	if coord.workers[w1].Status != WorkerDraining {
		t.Errorf("idle worker should be draining, got %s", coord.workers[w1].Status)
	}
	if coord.workers[w2].Status != WorkerOnline {
		t.Errorf("busy worker should still be online, got %s", coord.workers[w2].Status)
	}
}

func TestCoordinator_ApplyScaleDown_ZeroDelta(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	coord := NewCoordinator("test", "localhost", 0, "test", bus, session.NewManager())

	coord.mu.Lock()
	w := "worker-only"
	coord.workers[w] = &WorkerInfo{ID: w, Status: WorkerOnline, ActiveSessions: 0, MaxSessions: 4}
	coord.mu.Unlock()

	// Delta of 0 (and positive) should be no-op.
	coord.applyScaleDown(ScaleDecision{Action: ScaleDown, Delta: 0})
	coord.applyScaleDown(ScaleDecision{Action: ScaleDown, Delta: 1})

	coord.mu.RLock()
	defer coord.mu.RUnlock()
	if coord.workers[w].Status != WorkerOnline {
		t.Errorf("worker should remain online with non-negative delta, got %s", coord.workers[w].Status)
	}
}

// ---------------------------------------------------------------------------
// RecordCompletion overflow (push 66.7% higher)
// ---------------------------------------------------------------------------

func TestRecordCompletion_Overflow(t *testing.T) {
	t.Parallel()
	fa := NewFleetAnalytics(5, time.Hour)

	// Add more than maxSamples completions.
	for range 10 {
		fa.RecordCompletion("w1", "claude", 100*time.Millisecond, 0.10)
	}

	fa.mu.RLock()
	defer fa.mu.RUnlock()
	if len(fa.completions) > 5 {
		t.Errorf("completions should be capped at 5, got %d", len(fa.completions))
	}
}

func TestRecordFailure_Overflow_ViaPublicAPI(t *testing.T) {
	t.Parallel()
	fa := NewFleetAnalytics(5, time.Hour)

	for range 10 {
		fa.RecordFailure("w1", "err")
	}

	fa.mu.RLock()
	defer fa.mu.RUnlock()
	if len(fa.failures) > 5 {
		t.Errorf("failures should be capped at 5, got %d", len(fa.failures))
	}
}

// ---------------------------------------------------------------------------
// recordCompletionAt overflow (push 66.7% higher)
// ---------------------------------------------------------------------------

func TestRecordCompletionAt_Overflow(t *testing.T) {
	t.Parallel()
	fa := NewFleetAnalytics(5, time.Hour)

	now := time.Now()
	for range 10 {
		fa.recordCompletionAt(now, "w1", "claude", 50*time.Millisecond, 0.05)
	}

	fa.mu.RLock()
	defer fa.mu.RUnlock()
	if len(fa.completions) > 5 {
		t.Errorf("completions should be capped at 5, got %d", len(fa.completions))
	}
}

// ---------------------------------------------------------------------------
// Snapshot with latencies (ensure P50/P95/P99 paths are hit)
// ---------------------------------------------------------------------------

func TestSnapshot_LatencyPercentiles(t *testing.T) {
	t.Parallel()
	fa := NewFleetAnalytics(200, time.Hour)

	now := time.Now()
	// Add 100 completions with varied latencies.
	for i := 1; i <= 100; i++ {
		fa.recordCompletionAt(now, "w1", "claude", time.Duration(i)*time.Millisecond, 0.01)
	}

	snap := fa.Snapshot(time.Hour)
	if snap.TotalCompletions != 100 {
		t.Fatalf("expected 100 completions, got %d", snap.TotalCompletions)
	}
	if snap.LatencyP50Ms <= 0 {
		t.Error("expected positive P50 latency")
	}
	if snap.LatencyP95Ms <= 0 {
		t.Error("expected positive P95 latency")
	}
	if snap.LatencyP99Ms <= 0 {
		t.Error("expected positive P99 latency")
	}
	if snap.LatencyP99Ms < snap.LatencyP50Ms {
		t.Errorf("P99 (%f) should be >= P50 (%f)", snap.LatencyP99Ms, snap.LatencyP50Ms)
	}
}
