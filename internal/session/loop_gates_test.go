package session

import (
	"errors"
	"testing"
)

func TestInitBaselineFromFirstObservation_Empty(t *testing.T) {
	b := InitBaselineFromFirstObservation(nil)
	if b != nil {
		t.Errorf("expected nil for empty observations, got %+v", b)
	}
}

func TestInitBaselineFromFirstObservation_Single(t *testing.T) {
	obs := []LoopObservation{
		{
			PlannerLatencyMs: 100,
			WorkerLatencyMs:  200,
			TotalLatencyMs:   350,
			TotalCostUSD:     0.50,
			FilesChanged:     3,
			LinesAdded:       42,
		},
	}
	b := InitBaselineFromFirstObservation(obs)
	if b == nil {
		t.Fatal("expected non-nil baseline")
	}
	if b.AvgPlannerLatencyMs != 100 {
		t.Errorf("AvgPlannerLatencyMs = %d, want 100", b.AvgPlannerLatencyMs)
	}
	if b.AvgWorkerLatencyMs != 200 {
		t.Errorf("AvgWorkerLatencyMs = %d, want 200", b.AvgWorkerLatencyMs)
	}
	if b.AvgTotalLatencyMs != 350 {
		t.Errorf("AvgTotalLatencyMs = %d, want 350", b.AvgTotalLatencyMs)
	}
	if b.AvgTotalCostUSD != 0.50 {
		t.Errorf("AvgTotalCostUSD = %f, want 0.50", b.AvgTotalCostUSD)
	}
	if b.AvgFilesChanged != 3 {
		t.Errorf("AvgFilesChanged = %d, want 3", b.AvgFilesChanged)
	}
	if b.AvgLinesAdded != 42 {
		t.Errorf("AvgLinesAdded = %d, want 42", b.AvgLinesAdded)
	}
	if b.SampleCount != 1 {
		t.Errorf("SampleCount = %d, want 1", b.SampleCount)
	}
}

func TestBaselineFromObservations_Empty(t *testing.T) {
	b := BaselineFromObservations(nil)
	if b != nil {
		t.Errorf("expected nil for empty observations, got %+v", b)
	}
}

func TestBaselineFromObservations_Multiple(t *testing.T) {
	obs := []LoopObservation{
		{
			PlannerLatencyMs: 100,
			WorkerLatencyMs:  200,
			TotalLatencyMs:   300,
			TotalCostUSD:     1.00,
			FilesChanged:     4,
			LinesAdded:       40,
		},
		{
			PlannerLatencyMs: 200,
			WorkerLatencyMs:  400,
			TotalLatencyMs:   600,
			TotalCostUSD:     3.00,
			FilesChanged:     6,
			LinesAdded:       60,
		},
	}
	b := BaselineFromObservations(obs)
	if b == nil {
		t.Fatal("expected non-nil baseline")
	}
	if b.AvgPlannerLatencyMs != 150 {
		t.Errorf("AvgPlannerLatencyMs = %d, want 150", b.AvgPlannerLatencyMs)
	}
	if b.AvgWorkerLatencyMs != 300 {
		t.Errorf("AvgWorkerLatencyMs = %d, want 300", b.AvgWorkerLatencyMs)
	}
	if b.AvgTotalCostUSD != 2.00 {
		t.Errorf("AvgTotalCostUSD = %f, want 2.00", b.AvgTotalCostUSD)
	}
	if b.SampleCount != 2 {
		t.Errorf("SampleCount = %d, want 2", b.SampleCount)
	}
}

func TestInitBaselineFromFirstObservation_IgnoresRest(t *testing.T) {
	obs := []LoopObservation{
		{PlannerLatencyMs: 100, TotalCostUSD: 1.00},
		{PlannerLatencyMs: 900, TotalCostUSD: 9.00},
	}
	b := InitBaselineFromFirstObservation(obs)
	if b == nil {
		t.Fatal("expected non-nil baseline")
	}
	// Should only use the first observation.
	if b.AvgPlannerLatencyMs != 100 {
		t.Errorf("AvgPlannerLatencyMs = %d, want 100 (first only)", b.AvgPlannerLatencyMs)
	}
	if b.AvgTotalCostUSD != 1.00 {
		t.Errorf("AvgTotalCostUSD = %f, want 1.00 (first only)", b.AvgTotalCostUSD)
	}
}

// TestGateDeltasMeaningfulAfterBaselineInit verifies that deltas computed
// against a baseline initialized from first observation are non-zero and
// meaningful (QW-6: FINDING-226/238).
func TestGateDeltasMeaningfulAfterBaselineInit(t *testing.T) {
	firstObs := []LoopObservation{
		{
			PlannerLatencyMs: 100,
			WorkerLatencyMs:  200,
			TotalLatencyMs:   300,
			TotalCostUSD:     1.00,
			FilesChanged:     5,
			LinesAdded:       50,
		},
	}
	baseline := InitBaselineFromFirstObservation(firstObs)
	if baseline == nil {
		t.Fatal("expected non-nil baseline from first observation")
	}

	// A second observation that differs from the first should produce
	// meaningful (non-zero) deltas when compared.
	secondObs := LoopObservation{
		PlannerLatencyMs: 200,
		WorkerLatencyMs:  400,
		TotalLatencyMs:   600,
		TotalCostUSD:     2.00,
		FilesChanged:     10,
		LinesAdded:       100,
	}

	// Compute deltas — these should be non-zero because baseline was
	// initialized from real data, not zero-initialized.
	costDelta := secondObs.TotalCostUSD - baseline.AvgTotalCostUSD
	latencyDelta := secondObs.TotalLatencyMs - baseline.AvgTotalLatencyMs
	filesDelta := secondObs.FilesChanged - baseline.AvgFilesChanged

	if costDelta == 0 {
		t.Error("expected non-zero cost delta after baseline init from first observation")
	}
	if costDelta != 1.00 {
		t.Errorf("cost delta = %f, want 1.00", costDelta)
	}
	if latencyDelta == 0 {
		t.Error("expected non-zero latency delta after baseline init from first observation")
	}
	if latencyDelta != 300 {
		t.Errorf("latency delta = %d, want 300", latencyDelta)
	}
	if filesDelta == 0 {
		t.Error("expected non-zero files delta after baseline init from first observation")
	}
	if filesDelta != 5 {
		t.Errorf("files delta = %d, want 5", filesDelta)
	}
}

// TestBaselineNeverZeroInitialized verifies that a baseline created from real
// observations has non-zero values, preventing meaningless deltas (QW-6).
func TestBaselineNeverZeroInitialized(t *testing.T) {
	obs := []LoopObservation{
		{TotalLatencyMs: 500, TotalCostUSD: 0.50, FilesChanged: 3},
		{TotalLatencyMs: 700, TotalCostUSD: 0.70, FilesChanged: 5},
	}

	// InitBaselineFromFirstObservation should use the first obs, not zeros.
	b1 := InitBaselineFromFirstObservation(obs)
	if b1 == nil {
		t.Fatal("expected non-nil baseline")
	}
	if b1.AvgTotalLatencyMs == 0 || b1.AvgTotalCostUSD == 0 || b1.AvgFilesChanged == 0 {
		t.Errorf("baseline has zero values: latency=%d cost=%f files=%d — should use first observation",
			b1.AvgTotalLatencyMs, b1.AvgTotalCostUSD, b1.AvgFilesChanged)
	}

	// BaselineFromObservations should average, not zero-initialize.
	b2 := BaselineFromObservations(obs)
	if b2 == nil {
		t.Fatal("expected non-nil baseline")
	}
	if b2.AvgTotalLatencyMs == 0 || b2.AvgTotalCostUSD == 0 || b2.AvgFilesChanged == 0 {
		t.Errorf("baseline has zero values: latency=%d cost=%f files=%d — should average observations",
			b2.AvgTotalLatencyMs, b2.AvgTotalCostUSD, b2.AvgFilesChanged)
	}
}

// --- QW-6 fix: IsZero, CheckBaseline, ComputeDelta, EnsureBaseline ---

func TestLoopBaseline_IsZero(t *testing.T) {
	var nilBaseline *LoopBaseline
	if !nilBaseline.IsZero() {
		t.Error("nil baseline should be zero")
	}

	zeroInit := &LoopBaseline{} // SampleCount == 0
	if !zeroInit.IsZero() {
		t.Error("zero-initialized baseline should be zero")
	}

	real := &LoopBaseline{SampleCount: 1, AvgTotalCostUSD: 0.5}
	if real.IsZero() {
		t.Error("baseline with SampleCount=1 should not be zero")
	}
}

func TestCheckBaseline_Nil(t *testing.T) {
	check := CheckBaseline(nil)
	if check.Status != BaselineNotYet {
		t.Errorf("expected status %q, got %q", BaselineNotYet, check.Status)
	}
	if check.Baseline != nil {
		t.Error("expected nil baseline in check result")
	}
}

func TestCheckBaseline_ZeroInit(t *testing.T) {
	check := CheckBaseline(&LoopBaseline{})
	if check.Status != BaselineZeroInit {
		t.Errorf("expected status %q, got %q", BaselineZeroInit, check.Status)
	}
}

func TestCheckBaseline_Ready(t *testing.T) {
	bl := &LoopBaseline{SampleCount: 3, AvgTotalCostUSD: 1.5}
	check := CheckBaseline(bl)
	if check.Status != BaselineReady {
		t.Errorf("expected status %q, got %q", BaselineReady, check.Status)
	}
	if check.Baseline != bl {
		t.Error("expected baseline to be returned in check")
	}
}

func TestComputeDelta_NilBaseline(t *testing.T) {
	obs := LoopObservation{TotalCostUSD: 2.0, TotalLatencyMs: 500}
	delta := ComputeDelta(obs, nil)
	if delta.Valid {
		t.Error("delta should be invalid when baseline is nil")
	}
}

func TestComputeDelta_ZeroBaseline(t *testing.T) {
	obs := LoopObservation{TotalCostUSD: 2.0}
	delta := ComputeDelta(obs, &LoopBaseline{})
	if delta.Valid {
		t.Error("delta should be invalid when baseline is zero-initialized")
	}
}

func TestComputeDelta_MeaningfulAfterInit(t *testing.T) {
	// This is the core QW-6 regression test: after initializing baseline from
	// the first observation, a second observation must produce meaningful
	// (non-zero, valid) deltas.
	firstObs := []LoopObservation{{
		PlannerLatencyMs: 100,
		WorkerLatencyMs:  200,
		TotalLatencyMs:   300,
		TotalCostUSD:     1.00,
		FilesChanged:     5,
		LinesAdded:       50,
	}}
	baseline := InitBaselineFromFirstObservation(firstObs)

	secondObs := LoopObservation{
		TotalLatencyMs: 600,
		TotalCostUSD:   2.00,
		FilesChanged:   10,
		LinesAdded:     100,
	}
	delta := ComputeDelta(secondObs, baseline)
	if !delta.Valid {
		t.Fatal("delta should be valid after baseline init from real observation")
	}
	if delta.CostDelta != 1.00 {
		t.Errorf("CostDelta = %f, want 1.00", delta.CostDelta)
	}
	if delta.LatencyDelta != 300 {
		t.Errorf("LatencyDelta = %d, want 300", delta.LatencyDelta)
	}
	if delta.FilesDelta != 5 {
		t.Errorf("FilesDelta = %d, want 5", delta.FilesDelta)
	}
	if delta.LinesDelta != 50 {
		t.Errorf("LinesDelta = %d, want 50", delta.LinesDelta)
	}
}

func TestEnsureBaseline_ReturnsExisting(t *testing.T) {
	existing := &LoopBaseline{SampleCount: 5, AvgTotalCostUSD: 2.0}
	saveCalled := false
	bl, isNew, err := EnsureBaseline(existing, nil, func(_ *LoopBaseline) error {
		saveCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bl != existing {
		t.Error("should return existing baseline")
	}
	if isNew {
		t.Error("isNew should be false when existing baseline is valid (cycle 2+)")
	}
	if saveCalled {
		t.Error("save should not be called when existing baseline is valid")
	}
}

func TestEnsureBaseline_InitializesFromObservations(t *testing.T) {
	obs := []LoopObservation{{
		TotalLatencyMs: 500,
		TotalCostUSD:   0.75,
		FilesChanged:   3,
		LinesAdded:     30,
	}}
	var saved *LoopBaseline
	bl, isNew, err := EnsureBaseline(nil, obs, func(b *LoopBaseline) error {
		saved = b
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bl == nil {
		t.Fatal("expected non-nil baseline")
	}
	if bl.SampleCount != 1 {
		t.Errorf("SampleCount = %d, want 1", bl.SampleCount)
	}
	if !isNew {
		t.Error("isNew should be true when baseline is freshly initialized (cycle 1)")
	}
	if saved == nil {
		t.Error("save callback should have been called")
	}
}

func TestEnsureBaseline_PropagatesSaveError(t *testing.T) {
	obs := []LoopObservation{{TotalCostUSD: 1.0}}
	saveErr := errors.New("disk full")
	_, _, err := EnsureBaseline(nil, obs, func(_ *LoopBaseline) error {
		return saveErr
	})
	if err == nil {
		t.Fatal("expected error from save callback")
	}
	if !errors.Is(err, saveErr) {
		t.Errorf("expected wrapped save error, got %v", err)
	}
}

func TestEnsureBaseline_NoObservations(t *testing.T) {
	bl, isNew, err := EnsureBaseline(nil, nil, func(_ *LoopBaseline) error {
		t.Error("save should not be called with no observations")
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bl != nil {
		t.Error("expected nil baseline with no observations")
	}
	if isNew {
		t.Error("isNew should be false when no observations are available")
	}
}

// --- QW-6 cycle 1 / cycle 2 gate-firing behavior ---

// TestGateNotFiredOnCycle1 verifies that when EnsureBaseline creates a fresh
// baseline from the first real observation (cycle 1), isNew=true is returned,
// signaling callers to skip gate evaluation. ComputeDelta on the same observation
// against the just-initialized baseline would produce deltas of zero (trivially
// passing), so callers must check isNew before evaluating.
func TestGateNotFiredOnCycle1(t *testing.T) {
	cycle1Obs := []LoopObservation{{
		TotalLatencyMs: 300,
		TotalCostUSD:   1.00,
		FilesChanged:   5,
		LinesAdded:     50,
	}}

	bl, isNew, err := EnsureBaseline(nil, cycle1Obs, func(_ *LoopBaseline) error { return nil })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bl == nil {
		t.Fatal("expected non-nil baseline")
	}

	// isNew=true signals callers to skip gate evaluation on cycle 1.
	if !isNew {
		t.Error("isNew should be true on cycle 1 — baseline was just established from first observation")
	}

	// Callers should NOT call ComputeDelta when isNew=true. To illustrate why:
	// computing a delta of the same observation against its own baseline
	// produces a zero delta, which would trivially pass any gate check.
	delta := ComputeDelta(cycle1Obs[0], bl)
	if !delta.Valid {
		t.Fatal("delta unexpectedly invalid")
	}
	if delta.CostDelta != 0 || delta.LatencyDelta != 0 {
		t.Errorf("trivial self-delta should be 0, got cost=%f latency=%d — this is why isNew=true must suppress gate evaluation",
			delta.CostDelta, delta.LatencyDelta)
	}
}

// TestGateFiredOnCycle2_ScoreRegresses verifies that on cycle 2 (existing
// baseline, no longer isNew), ComputeDelta produces meaningful positive deltas
// when the new observation is worse than the baseline.
func TestGateFiredOnCycle2_ScoreRegresses(t *testing.T) {
	// Cycle 1 established this baseline.
	savedBaseline := &LoopBaseline{
		SampleCount:         1,
		AvgTotalCostUSD:     1.00,
		AvgTotalLatencyMs:   300,
		AvgFilesChanged:     5,
		AvgLinesAdded:       50,
	}

	// Cycle 2: EnsureBaseline should reuse the existing baseline, isNew=false.
	cycle2Obs := []LoopObservation{{
		TotalLatencyMs: 900, // 3× worse
		TotalCostUSD:   2.50, // 2.5× worse
		FilesChanged:   2,
		LinesAdded:     10,
	}}
	bl, isNew, err := EnsureBaseline(savedBaseline, cycle2Obs, func(_ *LoopBaseline) error {
		t.Error("save should not be called when reusing existing baseline")
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isNew {
		t.Error("isNew should be false on cycle 2 (existing baseline)")
	}

	// Gate evaluation proceeds: compute delta against saved baseline.
	delta := ComputeDelta(cycle2Obs[0], bl)
	if !delta.Valid {
		t.Fatal("delta should be valid on cycle 2 with real baseline")
	}
	if delta.CostDelta <= 0 {
		t.Errorf("CostDelta = %f, want > 0 (regression)", delta.CostDelta)
	}
	if delta.LatencyDelta <= 0 {
		t.Errorf("LatencyDelta = %d, want > 0 (regression)", delta.LatencyDelta)
	}
	if delta.CostDelta != 1.50 {
		t.Errorf("CostDelta = %f, want 1.50", delta.CostDelta)
	}
	if delta.LatencyDelta != 600 {
		t.Errorf("LatencyDelta = %d, want 600", delta.LatencyDelta)
	}
}

// TestGateFiredOnCycle2_MarginalImprovement verifies that on cycle 2, a small
// improvement produces a non-zero negative delta — not an auto-pass of zero.
// This is the core of QW-6: even marginal changes must yield real (non-trivial)
// deltas; only a zero-initialized baseline would produce a spurious auto-pass.
func TestGateFiredOnCycle2_MarginalImprovement(t *testing.T) {
	savedBaseline := &LoopBaseline{
		SampleCount:       1,
		AvgTotalCostUSD:   1.00,
		AvgTotalLatencyMs: 300,
		AvgFilesChanged:   5,
		AvgLinesAdded:     50,
	}

	// Cycle 2: marginally better — 5% cost reduction, same latency.
	cycle2Obs := []LoopObservation{{
		TotalLatencyMs: 300,
		TotalCostUSD:   0.95, // 5% cheaper
		FilesChanged:   5,
		LinesAdded:     50,
	}}
	bl, isNew, err := EnsureBaseline(savedBaseline, cycle2Obs, func(_ *LoopBaseline) error {
		t.Error("save should not be called when reusing existing baseline")
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isNew {
		t.Error("isNew should be false on cycle 2 (existing baseline)")
	}

	delta := ComputeDelta(cycle2Obs[0], bl)
	if !delta.Valid {
		t.Fatal("delta should be valid on cycle 2 with real baseline")
	}
	// Marginal improvement: delta is non-zero (not an auto-pass).
	if delta.CostDelta == 0 {
		t.Error("CostDelta should be non-zero for marginal improvement — zero delta means baseline was zero-initialized (QW-6 regression)")
	}
	const wantCostDelta = -0.05
	if delta.CostDelta < wantCostDelta-1e-9 || delta.CostDelta > wantCostDelta+1e-9 {
		t.Errorf("CostDelta = %f, want ~-0.05 (5%% improvement)", delta.CostDelta)
	}
	if delta.LatencyDelta != 0 {
		t.Errorf("LatencyDelta = %d, want 0 (unchanged)", delta.LatencyDelta)
	}
}

// TestGateNotFiredOnCycle2_ScoreStable verifies that on cycle 2, when the new
// observation matches the baseline, deltas are zero and no gate should fire.
func TestGateNotFiredOnCycle2_ScoreStable(t *testing.T) {
	// Cycle 1 established this baseline.
	savedBaseline := &LoopBaseline{
		SampleCount:       1,
		AvgTotalCostUSD:   1.00,
		AvgTotalLatencyMs: 300,
		AvgFilesChanged:   5,
		AvgLinesAdded:     50,
	}

	// Cycle 2: same metrics as baseline — stable.
	cycle2Obs := []LoopObservation{{
		TotalLatencyMs: 300,
		TotalCostUSD:   1.00,
		FilesChanged:   5,
		LinesAdded:     50,
	}}
	bl, isNew, err := EnsureBaseline(savedBaseline, cycle2Obs, func(_ *LoopBaseline) error {
		t.Error("save should not be called when reusing existing baseline")
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isNew {
		t.Error("isNew should be false on cycle 2 (existing baseline)")
	}

	delta := ComputeDelta(cycle2Obs[0], bl)
	if !delta.Valid {
		t.Fatal("delta should be valid on cycle 2 with real baseline")
	}
	if delta.CostDelta != 0 {
		t.Errorf("CostDelta = %f, want 0 (stable)", delta.CostDelta)
	}
	if delta.LatencyDelta != 0 {
		t.Errorf("LatencyDelta = %d, want 0 (stable)", delta.LatencyDelta)
	}
}
