package session

import (
	"testing"
	"time"
)

func TestLoopStallDetector_NonStalledIteration(t *testing.T) {
	d := NewLoopStallDetector(10*time.Minute, nil)
	d.now = func() time.Time { return time.Now() }

	run := &LoopRun{ID: "test-run"}
	iter := &LoopIteration{
		Number:    1,
		Status:    "executing",
		StartedAt: time.Now(), // just started
	}

	if d.CheckIteration(run, iter) {
		t.Error("expected non-stalled iteration to return false")
	}
}

func TestLoopStallDetector_StalledIteration(t *testing.T) {
	d := NewLoopStallDetector(10*time.Minute, nil)
	past := time.Now().Add(-15 * time.Minute) // started 15 min ago
	d.now = func() time.Time { return time.Now() }

	run := &LoopRun{ID: "test-run"}
	iter := &LoopIteration{
		Number:    1,
		Status:    "planning",
		StartedAt: past,
	}

	if !d.CheckIteration(run, iter) {
		t.Error("expected stalled iteration to return true")
	}
}

func TestLoopStallDetector_CompletedIterationNotStalled(t *testing.T) {
	d := NewLoopStallDetector(10*time.Minute, nil)
	past := time.Now().Add(-30 * time.Minute)

	run := &LoopRun{ID: "test-run"}
	for _, status := range []string{"completed", "failed", "verified", "merged"} {
		iter := &LoopIteration{
			Number:    1,
			Status:    status,
			StartedAt: past,
		}
		if d.CheckIteration(run, iter) {
			t.Errorf("expected completed/failed iteration (status=%q) to not be detected as stalled", status)
		}
	}
}

func TestLoopStallDetector_CheckRunMixed(t *testing.T) {
	now := time.Now()
	d := NewLoopStallDetector(10*time.Minute, nil)
	d.now = func() time.Time { return now }

	run := &LoopRun{
		ID: "test-run",
		Iterations: []LoopIteration{
			{Number: 1, Status: "completed", StartedAt: now.Add(-20 * time.Minute)},  // done, not stalled
			{Number: 2, Status: "executing", StartedAt: now.Add(-15 * time.Minute)},  // active, stalled
			{Number: 3, Status: "planning", StartedAt: now.Add(-2 * time.Minute)},    // active, not stalled
			{Number: 4, Status: "verifying", StartedAt: now.Add(-12 * time.Minute)},  // active, stalled
			{Number: 5, Status: "failed", StartedAt: now.Add(-30 * time.Minute)},     // done, not stalled
		},
	}

	stalled := d.CheckRun(run)
	if len(stalled) != 2 {
		t.Fatalf("expected 2 stalled iterations, got %d: %v", len(stalled), stalled)
	}
	if stalled[0] != 1 || stalled[1] != 3 {
		t.Errorf("expected stalled indices [1, 3], got %v", stalled)
	}
}

func TestLoopStallDetector_ZeroTimeoutDisables(t *testing.T) {
	d := NewLoopStallDetector(0, nil)

	run := &LoopRun{ID: "test-run"}
	iter := &LoopIteration{
		Number:    1,
		Status:    "executing",
		StartedAt: time.Now().Add(-1 * time.Hour),
	}

	if d.CheckIteration(run, iter) {
		t.Error("expected zero timeout to disable detection")
	}

	run.Iterations = []LoopIteration{*iter}
	stalled := d.CheckRun(run)
	if len(stalled) != 0 {
		t.Errorf("expected zero timeout CheckRun to return nil, got %v", stalled)
	}
}

func TestLoopStallDetector_OnStallCallback(t *testing.T) {
	var called int
	d := NewLoopStallDetector(5*time.Minute, func(run *LoopRun, iter *LoopIteration) {
		called++
	})
	now := time.Now()
	d.now = func() time.Time { return now }

	run := &LoopRun{
		ID: "test-run",
		Iterations: []LoopIteration{
			{Number: 1, Status: "executing", StartedAt: now.Add(-10 * time.Minute)},
			{Number: 2, Status: "executing", StartedAt: now.Add(-10 * time.Minute)},
		},
	}

	d.CheckRun(run)
	if called != 2 {
		t.Errorf("expected onStall called 2 times, got %d", called)
	}
}

func TestLoopStallDetector_DefaultCheckEvery(t *testing.T) {
	d := NewLoopStallDetector(9*time.Minute, nil)
	if d.checkEvery != 3*time.Minute {
		t.Errorf("expected checkEvery=3m, got %v", d.checkEvery)
	}

	// For short timeouts, checkEvery should be clamped to 30s minimum.
	d2 := NewLoopStallDetector(30*time.Second, nil)
	if d2.checkEvery != 30*time.Second {
		t.Errorf("expected checkEvery=30s (min), got %v", d2.checkEvery)
	}
}

func TestLoopStallDetector_AllActiveStatuses(t *testing.T) {
	now := time.Now()
	d := NewLoopStallDetector(5*time.Minute, nil)
	d.now = func() time.Time { return now }
	run := &LoopRun{ID: "test-run"}

	for _, status := range []string{"running", "planning", "executing", "verifying"} {
		iter := &LoopIteration{
			Number:    1,
			Status:    status,
			StartedAt: now.Add(-10 * time.Minute),
		}
		if !d.CheckIteration(run, iter) {
			t.Errorf("expected status %q to be detected as stalled", status)
		}
	}
}
