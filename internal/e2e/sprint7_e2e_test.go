package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- RefreshBaseline ---

func TestRefreshBaseline_NoObservations(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	_ = os.MkdirAll(ralphDir, 0o755)

	// No observation file exists.
	bl, err := RefreshBaseline(dir, 24)
	if err == nil {
		// If LoadObservations returns empty on missing file, we get a baseline.
		if bl != nil && len(bl.Entries) != 0 {
			t.Errorf("expected empty entries, got %d", len(bl.Entries))
		}
	}
	// Error is acceptable if observation file doesn't exist.
}

func TestRefreshBaseline_WithObservations(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	_ = os.MkdirAll(ralphDir, 0o755)

	// Write observation JSONL.
	obsPath := session.ObservationPath(dir)
	_ = os.MkdirAll(filepath.Dir(obsPath), 0o755)

	obs := []session.LoopObservation{
		{
			Timestamp:       time.Now(),
			TaskTitle:       "test-task",
			PlannerProvider: "claude",
			TotalCostUSD:    0.50,
			TotalLatencyMs:  1000,
			Status:          "idle",
			VerifyPassed:    true,
		},
		{
			Timestamp:       time.Now().Add(-1 * time.Hour),
			TaskTitle:       "test-task",
			PlannerProvider: "claude",
			TotalCostUSD:    0.75,
			TotalLatencyMs:  1500,
			Status:          "idle",
			VerifyPassed:    true,
		},
	}

	f, err := os.Create(obsPath)
	if err != nil {
		t.Fatalf("create obs file: %v", err)
	}
	enc := json.NewEncoder(f)
	for _, o := range obs {
		_ = enc.Encode(o)
	}
	f.Close()

	bl, err := RefreshBaseline(dir, 48)
	if err != nil {
		t.Fatalf("RefreshBaseline: %v", err)
	}
	if bl == nil {
		t.Fatal("expected non-nil baseline")
	}
	if len(bl.Entries) == 0 {
		t.Error("expected at least one baseline entry")
	}
	if bl.Aggregate == nil {
		t.Error("expected aggregate stats")
	}
	if bl.Rates == nil {
		t.Error("expected rates")
	}
}

// --- RunE2EGate ---

func TestRunE2EGate_NoTestBinary(t *testing.T) {
	t.Parallel()
	// RunE2EGate runs `go test` against the repo. We test with a non-Go dir
	// so it fails fast rather than running the full suite.
	dir := t.TempDir()

	_, err := RunE2EGate(dir)
	if err == nil {
		t.Fatal("expected error running E2E gate on non-Go directory")
	}
}

// --- EvaluateGates (exercised by RunE2EGate internally) ---

func TestEvaluateGates_PassingObservations(t *testing.T) {
	t.Parallel()

	obs := []session.LoopObservation{
		{TotalCostUSD: 0.10, TotalLatencyMs: 500, Status: "idle", VerifyPassed: true},
		{TotalCostUSD: 0.12, TotalLatencyMs: 600, Status: "idle", VerifyPassed: true},
		{TotalCostUSD: 0.11, TotalLatencyMs: 550, Status: "idle", VerifyPassed: true},
		{TotalCostUSD: 0.09, TotalLatencyMs: 480, Status: "idle", VerifyPassed: true},
		{TotalCostUSD: 0.13, TotalLatencyMs: 620, Status: "idle", VerifyPassed: true},
	}

	baseline := BuildBaseline(obs, 0)
	thresholds := MockGateThresholds()

	report := EvaluateGates(obs, baseline, thresholds)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if report.Overall == VerdictFail {
		t.Errorf("expected pass or warn, got fail")
	}
	if report.SampleCount != 5 {
		t.Errorf("sample count: got %d, want 5", report.SampleCount)
	}
}

func TestEvaluateGates_InsufficientSamples(t *testing.T) {
	t.Parallel()

	obs := []session.LoopObservation{
		{TotalCostUSD: 0.10, TotalLatencyMs: 500},
	}

	thresholds := DefaultGateThresholds() // MinSamples = 5
	report := EvaluateGates(obs, nil, thresholds)

	if report.Overall != VerdictSkip {
		t.Errorf("expected skip for insufficient samples, got %s", report.Overall)
	}
}

func TestEvaluateGates_HighErrorRate(t *testing.T) {
	t.Parallel()

	obs := make([]session.LoopObservation, 10)
	for i := range obs {
		obs[i] = session.LoopObservation{
			TotalCostUSD:   0.10,
			TotalLatencyMs: 500,
			Error:          "something failed",
			Status:         "failed",
		}
	}

	baseline := BuildBaseline(obs, 0)
	thresholds := MockGateThresholds()

	report := EvaluateGates(obs, baseline, thresholds)
	if report.Overall != VerdictFail {
		t.Errorf("expected fail for high error rate, got %s", report.Overall)
	}
}

// --- runIteration (SelfTestRunner) ---

func TestSelfTestRunner_DryRun(t *testing.T) {
	t.Parallel()

	config := SelfTestConfig{
		RepoPath:      t.TempDir(),
		MaxIterations: 3,
		BudgetUSD:     5.0,
		UseSnapshot:   false,
		DryRun:        true,
		BinaryPath:    "/bin/echo", // just needs to exist
	}

	runner := &SelfTestRunner{
		Config:     config,
		BinaryPath: config.BinaryPath,
		BinaryHash: "abc123",
		PreparedAt: time.Now(),
	}

	result, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Iterations != 0 {
		t.Errorf("expected 0 iterations for dry run, got %d", result.Iterations)
	}
	if len(result.Observations) == 0 {
		t.Error("expected at least one observation (dry_run status)")
	}
}

func TestSelfTestRunner_RunIteration_WithEcho(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	runner := &SelfTestRunner{
		Config: SelfTestConfig{
			RepoPath:      dir,
			MaxIterations: 1,
			BudgetUSD:     5.0,
		},
		BinaryPath: "/bin/echo",
		BinaryHash: "testhash",
		PreparedAt: time.Now(),
	}

	obs, cost, err := runner.runIteration(context.Background(), 0)
	// /bin/echo won't accept "selftest" subcommand meaningfully but should not crash.
	_ = err
	_ = cost

	if obs == nil {
		t.Fatal("expected non-nil observation map")
	}
	if obs["binary_hash"] != "testhash" {
		t.Errorf("binary_hash: got %v, want testhash", obs["binary_hash"])
	}
	if _, ok := obs["duration_ms"]; !ok {
		t.Error("expected duration_ms in observation")
	}
}

func TestSelfTestRunner_BudgetLimit(t *testing.T) {
	t.Parallel()

	runner := &SelfTestRunner{
		Config: SelfTestConfig{
			RepoPath:      t.TempDir(),
			MaxIterations: 100,
			BudgetUSD:     0, // zero budget means no iterations
		},
		BinaryPath: "/bin/echo",
		BinaryHash: "hash",
		PreparedAt: time.Now(),
	}

	result, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With zero budget, the loop should break immediately.
	if result.Iterations > 0 {
		t.Errorf("expected 0 iterations with zero budget, got %d", result.Iterations)
	}
}

func TestSelfTestRunner_ContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	runner := &SelfTestRunner{
		Config: SelfTestConfig{
			RepoPath:      t.TempDir(),
			MaxIterations: 10,
			BudgetUSD:     100,
		},
		BinaryPath: "/bin/echo",
		BinaryHash: "hash",
		PreparedAt: time.Now(),
	}

	result, err := runner.Run(ctx)
	if err == nil {
		t.Fatal("expected context error")
	}
	if result.Iterations != 0 {
		t.Errorf("expected 0 iterations after cancel, got %d", result.Iterations)
	}
}

// --- CompareResults ---

func TestCompareResults_CostRegression(t *testing.T) {
	t.Parallel()

	prev := SelfTestResult{
		Iterations:   3,
		TotalCostUSD: 1.0,
		Duration:     10 * time.Second,
		BinaryHash:   "aaa",
	}
	curr := SelfTestResult{
		Iterations:   3,
		TotalCostUSD: 2.0, // >50% increase
		Duration:     10 * time.Second,
		BinaryHash:   "bbb",
	}

	regressions := CompareResults(curr, prev)
	found := false
	for _, r := range regressions {
		if r.Field == "total_cost_usd" {
			found = true
		}
	}
	if !found {
		t.Error("expected cost regression to be detected")
	}
}

// --- Baseline save/load round-trip ---

func TestBaselineSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "baseline.json")

	bl := &LoopBaseline{
		GeneratedAt: time.Now(),
		WindowHours: 24,
		Entries: map[string]*BaselineStats{
			"task:claude": {CostP50: 0.10, CostP95: 0.20, SampleCount: 5},
		},
		Aggregate: &BaselineStats{CostP50: 0.10, CostP95: 0.20, SampleCount: 5},
	}

	if err := SaveBaseline(path, bl); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.WindowHours != 24 {
		t.Errorf("window hours: got %f, want 24", loaded.WindowHours)
	}
	if len(loaded.Entries) != 1 {
		t.Errorf("entries: got %d, want 1", len(loaded.Entries))
	}
}
