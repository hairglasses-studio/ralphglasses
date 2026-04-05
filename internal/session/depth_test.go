package session

import (
	"sync"
	"testing"
)

func TestEstimateDepth_Default(t *testing.T) {
	de := NewDepthEstimator(nil)
	depth := de.EstimateDepth(TaskInfo{})
	if depth < MinDepth || depth > MaxDepth {
		t.Fatalf("default depth %d out of range [%d, %d]", depth, MinDepth, MaxDepth)
	}
}

func TestEstimateDepth_SimpleTask(t *testing.T) {
	de := NewDepthEstimator(nil)
	depth := de.EstimateDepth(TaskInfo{
		Title:       "fix typo in README",
		FileCount:   1,
		LOCEstimate: 10,
		Tags:        []string{"docs"},
	})
	if depth > DefaultDepth {
		t.Fatalf("simple task got depth %d, expected <= %d", depth, DefaultDepth)
	}
	if depth < MinDepth {
		t.Fatalf("depth %d below minimum %d", depth, MinDepth)
	}
}

func TestEstimateDepth_ComplexTask(t *testing.T) {
	de := NewDepthEstimator(nil)
	depth := de.EstimateDepth(TaskInfo{
		Title:           "refactor session management across 20 files",
		FileCount:       20,
		LOCEstimate:     600,
		DependencyDepth: 5,
		Tags:            []string{"refactor"},
	})
	if depth <= DefaultDepth {
		t.Fatalf("complex task got depth %d, expected > %d", depth, DefaultDepth)
	}
	if depth > MaxDepth {
		t.Fatalf("depth %d above maximum %d", depth, MaxDepth)
	}
}

func TestEstimateDepth_ClampedMin(t *testing.T) {
	de := NewDepthEstimator(nil)
	// Very simple: single file, 5 LOC, docs tag = lots of negative adjustments.
	depth := de.EstimateDepth(TaskInfo{
		FileCount:   1,
		LOCEstimate: 5,
		Tags:        []string{"docs", "typo"},
	})
	if depth < MinDepth {
		t.Fatalf("depth %d below minimum %d", depth, MinDepth)
	}
}

func TestEstimateDepth_ClampedMax(t *testing.T) {
	de := NewDepthEstimator(nil)
	depth := de.EstimateDepth(TaskInfo{
		FileCount:       100,
		LOCEstimate:     5000,
		DependencyDepth: 20,
		Tags:            []string{"refactor", "migration"},
	})
	if depth > MaxDepth {
		t.Fatalf("depth %d above maximum %d", depth, MaxDepth)
	}
}

func TestEstimateDepth_WithReflexion(t *testing.T) {
	rs := &ReflexionStore{
		reflections: []Reflection{
			{TaskTitle: "add logging", IterationNum: 8},
			{TaskTitle: "add logging", IterationNum: 12},
		},
	}
	de := NewDepthEstimator(rs)
	depth := de.EstimateDepth(TaskInfo{Title: "add logging", FileCount: 5})
	// Historical average is 10. The estimate without history for 5 files is ~10.
	// Blended should be around 10.
	if depth < MinDepth || depth > MaxDepth {
		t.Fatalf("reflexion-aided depth %d out of range", depth)
	}
}

func TestShouldEarlyStop_BelowMinDepth(t *testing.T) {
	de := NewDepthEstimator(nil)
	// Even with zero deltas, should not stop below MinDepth.
	deltas := []float64{0, 0, 0}
	if de.ShouldEarlyStop(2, deltas) {
		t.Fatal("should not early stop below MinDepth")
	}
}

func TestShouldEarlyStop_InsufficientData(t *testing.T) {
	de := NewDepthEstimator(nil)
	deltas := []float64{0.001}
	if de.ShouldEarlyStop(5, deltas) {
		t.Fatal("should not early stop with insufficient data")
	}
}

func TestShouldEarlyStop_DiminishingReturns(t *testing.T) {
	de := NewDepthEstimator(nil)
	deltas := []float64{0.5, 0.3, 0.1, 0.005, 0.003, 0.001}
	if !de.ShouldEarlyStop(6, deltas) {
		t.Fatal("expected early stop with diminishing returns")
	}
}

func TestShouldEarlyStop_StillProgressing(t *testing.T) {
	de := NewDepthEstimator(nil)
	deltas := []float64{0.5, 0.3, 0.1, 0.005, 0.003, 0.05}
	if de.ShouldEarlyStop(6, deltas) {
		t.Fatal("should not early stop while still progressing")
	}
}

func TestShouldExtend_NotAtLimit(t *testing.T) {
	de := NewDepthEstimator(nil)
	if de.ShouldExtend(5, 10, 0.5, true) {
		t.Fatal("should not extend when not at limit")
	}
}

func TestShouldExtend_AtAbsoluteMax(t *testing.T) {
	de := NewDepthEstimator(nil)
	if de.ShouldExtend(MaxDepth, MaxDepth, 0.5, true) {
		t.Fatal("should not extend beyond absolute max")
	}
}

func TestShouldExtend_NoBudget(t *testing.T) {
	de := NewDepthEstimator(nil)
	if de.ShouldExtend(10, 10, 0.5, false) {
		t.Fatal("should not extend without budget")
	}
}

func TestShouldExtend_GoodProgress(t *testing.T) {
	de := NewDepthEstimator(nil)
	if !de.ShouldExtend(10, 10, 0.2, true) {
		t.Fatal("expected extension with good progress and budget")
	}
}

func TestShouldExtend_LowProgress(t *testing.T) {
	de := NewDepthEstimator(nil)
	if de.ShouldExtend(10, 10, 0.05, true) {
		t.Fatal("should not extend with low progress")
	}
}

func TestIterationProgressDelta_NoIterations(t *testing.T) {
	run := &LoopRun{Iterations: []LoopIteration{}}
	d := iterationProgressDelta(run, 0)
	if d != 0 {
		t.Fatalf("expected 0, got %f", d)
	}
}

func TestIterationProgressDelta_Failed(t *testing.T) {
	run := &LoopRun{
		Iterations: []LoopIteration{
			{Number: 1, Status: "failed"},
		},
	}
	d := iterationProgressDelta(run, 1)
	if d != 0 {
		t.Fatalf("expected 0 for failed iteration, got %f", d)
	}
}

func TestIterationProgressDelta_NoChanges(t *testing.T) {
	run := &LoopRun{
		Iterations: []LoopIteration{
			{Number: 1, Status: "idle", AcceptanceReason: "no_staged_files"},
		},
	}
	d := iterationProgressDelta(run, 1)
	if d != 0 {
		t.Fatalf("expected 0 for no staged files, got %f", d)
	}
}

func TestIterationProgressDelta_WithFiles(t *testing.T) {
	run := &LoopRun{
		Iterations: []LoopIteration{
			{
				Number:           1,
				Status:           "idle",
				AcceptanceReason: "auto_merged",
				StagedFilesCount: 5,
			},
		},
	}
	d := iterationProgressDelta(run, 1)
	expected := 0.5 // 5 / 10.0
	if d != expected {
		t.Fatalf("expected %f, got %f", expected, d)
	}
}

func TestIterationProgressDelta_AcceptancePaths(t *testing.T) {
	run := &LoopRun{
		Iterations: []LoopIteration{
			{
				Number:           1,
				Status:           "idle",
				AcceptanceReason: "auto_merged",
				StagedFilesCount: 0,
				Acceptance: &AcceptanceResult{
					SafePaths: []string{"a.go", "b.go", "c.go"},
				},
			},
		},
	}
	d := iterationProgressDelta(run, 1)
	expected := 0.3 // 3 / 10.0
	if d != expected {
		t.Fatalf("expected %f, got %f", expected, d)
	}
}

func TestIterationProgressDelta_CappedAtOne(t *testing.T) {
	run := &LoopRun{
		Iterations: []LoopIteration{
			{
				Number:           1,
				Status:           "idle",
				AcceptanceReason: "auto_merged",
				StagedFilesCount: 20,
			},
		},
	}
	d := iterationProgressDelta(run, 1)
	if d != 1.0 {
		t.Fatalf("expected 1.0 cap, got %f", d)
	}
}

func TestClampDepth(t *testing.T) {
	tests := []struct {
		input, want int
	}{
		{0, MinDepth},
		{1, MinDepth},
		{MinDepth, MinDepth},
		{10, 10},
		{MaxDepth, MaxDepth},
		{100, MaxDepth},
	}
	for _, tt := range tests {
		got := clampDepth(tt.input)
		if got != tt.want {
			t.Errorf("clampDepth(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestDepthEstimator_ConcurrentSafety(t *testing.T) {
	de := NewDepthEstimator(nil)
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			de.EstimateDepth(TaskInfo{FileCount: 5, LOCEstimate: 100})
			de.ShouldEarlyStop(5, []float64{0.1, 0.05, 0.001})
			de.ShouldExtend(10, 10, 0.2, true)
		})
	}
	wg.Wait()
}
