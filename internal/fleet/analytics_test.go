package fleet

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestFleetAnalytics_RecordAndSnapshot(t *testing.T) {
	fa := NewFleetAnalytics(1000, time.Hour)

	for i := 0; i < 100; i++ {
		provider := "claude"
		if i%3 == 0 {
			provider = "gemini"
		}
		workerID := fmt.Sprintf("worker-%d", i%5)
		fa.RecordCompletion(workerID, provider, time.Duration(100+i)*time.Millisecond, 0.01)
	}

	snap := fa.Snapshot(time.Hour)

	if snap.TotalCompletions != 100 {
		t.Errorf("expected 100 completions, got %d", snap.TotalCompletions)
	}
	if snap.TotalFailures != 0 {
		t.Errorf("expected 0 failures, got %d", snap.TotalFailures)
	}
	if snap.FailureRate != 0 {
		t.Errorf("expected 0 failure rate, got %f", snap.FailureRate)
	}

	// Total cost: 100 * 0.01 = 1.00
	if snap.TotalCostUSD < 0.99 || snap.TotalCostUSD > 1.01 {
		t.Errorf("expected total cost ~1.00, got %f", snap.TotalCostUSD)
	}

	// Should have both providers.
	if _, ok := snap.CostPerProvider["claude"]; !ok {
		t.Error("expected claude in CostPerProvider")
	}
	if _, ok := snap.CostPerProvider["gemini"]; !ok {
		t.Error("expected gemini in CostPerProvider")
	}

	// Latency P50 should be around 150ms (midpoint of 100-199).
	if snap.LatencyP50Ms < 100 || snap.LatencyP50Ms > 200 {
		t.Errorf("expected P50 latency between 100-200ms, got %f", snap.LatencyP50Ms)
	}

	// P95 should be higher than P50.
	if snap.LatencyP95Ms <= snap.LatencyP50Ms {
		t.Errorf("expected P95 (%f) > P50 (%f)", snap.LatencyP95Ms, snap.LatencyP50Ms)
	}

	// 5 distinct workers.
	if len(snap.WorkerUtilization) != 5 {
		t.Errorf("expected 5 workers, got %d", len(snap.WorkerUtilization))
	}
}

func TestFleetAnalytics_FailureRate(t *testing.T) {
	fa := NewFleetAnalytics(1000, time.Hour)

	// 80 successes, 20 failures => 20% failure rate.
	for i := 0; i < 80; i++ {
		fa.RecordCompletion("w1", "claude", 100*time.Millisecond, 0.01)
	}
	for i := 0; i < 20; i++ {
		fa.RecordFailure("w1", "timeout")
	}

	snap := fa.Snapshot(time.Hour)

	if snap.TotalCompletions != 80 {
		t.Errorf("expected 80 completions, got %d", snap.TotalCompletions)
	}
	if snap.TotalFailures != 20 {
		t.Errorf("expected 20 failures, got %d", snap.TotalFailures)
	}

	expectedRate := 0.20
	if snap.FailureRate < expectedRate-0.01 || snap.FailureRate > expectedRate+0.01 {
		t.Errorf("expected failure rate ~0.20, got %f", snap.FailureRate)
	}
}

func TestFleetAnalytics_CostForecast(t *testing.T) {
	fa := NewFleetAnalytics(10000, time.Hour)

	now := time.Now()

	// Record 60 completions over the last 30 minutes at $0.10 each = $6.00 total.
	// Rate = $6.00 / 30min = $0.20/min = $12.00/hr.
	for i := 0; i < 60; i++ {
		ts := now.Add(-30*time.Minute + time.Duration(i)*30*time.Second)
		fa.recordCompletionAt(ts, "w1", "claude", 100*time.Millisecond, 0.10)
	}

	forecast := fa.CostForecast(time.Hour)

	// Expected: ~$12/hr. Allow some rounding tolerance.
	if forecast < 10.0 || forecast > 14.0 {
		t.Errorf("expected forecast ~12.0/hr, got %f", forecast)
	}
}

func TestFleetAnalytics_WindowFiltering(t *testing.T) {
	fa := NewFleetAnalytics(10000, 2*time.Hour)

	now := time.Now()

	// Old samples: 2 hours ago.
	for i := 0; i < 50; i++ {
		ts := now.Add(-2 * time.Hour)
		fa.recordCompletionAt(ts, "w-old", "gemini", 500*time.Millisecond, 1.0)
	}
	for i := 0; i < 10; i++ {
		fa.recordFailureAt(now.Add(-2*time.Hour), "w-old", "old error")
	}

	// Recent samples: within the last 5 minutes.
	for i := 0; i < 20; i++ {
		ts := now.Add(-time.Duration(i) * time.Second)
		fa.recordCompletionAt(ts, "w-new", "claude", 100*time.Millisecond, 0.05)
	}
	for i := 0; i < 5; i++ {
		fa.recordFailureAt(now.Add(-time.Duration(i)*time.Second), "w-new", "new error")
	}

	// 10-minute window should only see recent samples.
	snap := fa.Snapshot(10 * time.Minute)

	if snap.TotalCompletions != 20 {
		t.Errorf("expected 20 completions in 10min window, got %d", snap.TotalCompletions)
	}
	if snap.TotalFailures != 5 {
		t.Errorf("expected 5 failures in 10min window, got %d", snap.TotalFailures)
	}

	// Only the new worker should appear.
	if _, ok := snap.WorkerUtilization["w-old"]; ok {
		t.Error("old worker should not appear in 10min window")
	}
	if count, ok := snap.WorkerUtilization["w-new"]; !ok || count != 20 {
		t.Errorf("expected w-new with 20 completions, got %d", count)
	}

	// 3-hour window should see everything.
	snapAll := fa.Snapshot(3 * time.Hour)
	if snapAll.TotalCompletions != 70 {
		t.Errorf("expected 70 completions in 3hr window, got %d", snapAll.TotalCompletions)
	}
	if snapAll.TotalFailures != 15 {
		t.Errorf("expected 15 failures in 3hr window, got %d", snapAll.TotalFailures)
	}
}

func TestFleetAnalytics_Concurrent(t *testing.T) {
	fa := NewFleetAnalytics(10000, time.Hour)

	var wg sync.WaitGroup

	// Writers: record completions concurrently.
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				fa.RecordCompletion(fmt.Sprintf("w-%d", id), "claude", 50*time.Millisecond, 0.001)
			}
		}(g)
	}

	// Writers: record failures concurrently.
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				fa.RecordFailure(fmt.Sprintf("w-%d", id), "err")
			}
		}(g)
	}

	// Readers: take snapshots concurrently.
	for g := 0; g < 5; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				_ = fa.Snapshot(time.Hour)
			}
		}()
	}

	// Reader: cost forecast concurrently.
	for g := 0; g < 3; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				_ = fa.CostForecast(time.Hour)
			}
		}()
	}

	wg.Wait()

	snap := fa.Snapshot(time.Hour)
	// 10 goroutines * 100 completions = 1000.
	if snap.TotalCompletions != 1000 {
		t.Errorf("expected 1000 completions, got %d", snap.TotalCompletions)
	}
	// 5 goroutines * 50 failures = 250.
	if snap.TotalFailures != 250 {
		t.Errorf("expected 250 failures, got %d", snap.TotalFailures)
	}
}
