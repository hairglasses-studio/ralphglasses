package e2e

import (
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// GateVerdict represents the outcome of a regression gate check.
type GateVerdict string

const (
	VerdictPass GateVerdict = "pass"
	VerdictWarn GateVerdict = "warn"
	VerdictFail GateVerdict = "fail"
	VerdictSkip GateVerdict = "skip"
)

// GateThresholds defines warn/fail boundaries for regression gates.
// Cost and latency thresholds are relative multipliers over P95 baseline.
// Rate thresholds are absolute floors/ceilings.
type GateThresholds struct {
	CostPerIterWarn    float64 // multiplier over P95 baseline (e.g. 1.3 = +30%)
	CostPerIterFail    float64
	LatencyWarn        float64 // multiplier over P95 baseline
	LatencyFail        float64
	CompletionRateWarn float64 // absolute floor
	CompletionRateFail float64
	VerifyPassRateWarn float64
	VerifyPassRateFail float64
	ErrorRateWarn      float64 // absolute ceiling
	ErrorRateFail      float64
	MinSamples         int // minimum observations for non-skip verdict
	MaxObservations    int // rolling window: only use last N observations (0 = all)
}

// DefaultGateThresholds returns production-grade gate thresholds.
func DefaultGateThresholds() GateThresholds {
	return GateThresholds{
		CostPerIterWarn:    1.3,
		CostPerIterFail:    1.8,
		LatencyWarn:        1.5,
		LatencyFail:        2.5,
		CompletionRateWarn: 0.85,
		CompletionRateFail: 0.50,
		VerifyPassRateWarn: 0.80,
		VerifyPassRateFail: 0.50,
		ErrorRateWarn:      0.15,
		ErrorRateFail:      0.45,
		MinSamples:         5,
		MaxObservations:    10, // rolling window: only recent observations count
	}
}

// MockGateThresholds returns thresholds for deterministic mock scenarios.
func MockGateThresholds() GateThresholds {
	t := DefaultGateThresholds()
	t.MinSamples = 1
	return t
}

// GateResult is one metric evaluation within a gate report.
type GateResult struct {
	Metric      string      `json:"metric"`
	Verdict     GateVerdict `json:"verdict"`
	BaselineVal float64     `json:"baseline"`
	CurrentVal  float64     `json:"current"`
	DeltaPct    float64     `json:"delta_pct"`
}

// GateReport is the overall gate evaluation output.
type GateReport struct {
	Timestamp   time.Time    `json:"ts"`
	SampleCount int          `json:"sample_count"`
	Overall     GateVerdict  `json:"overall"`
	Results     []GateResult `json:"results"`
}

// EvaluateGates compares current observations against a baseline.
func EvaluateGates(observations []session.LoopObservation, baseline *LoopBaseline, thresholds GateThresholds) *GateReport {
	report := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: len(observations),
		Overall:     VerdictPass,
	}

	if len(observations) < thresholds.MinSamples {
		report.Overall = VerdictSkip
		report.Results = append(report.Results, GateResult{
			Metric:  "sample_count",
			Verdict: VerdictSkip,
		})
		return report
	}

	// Compute current metrics
	var totalCost, totalLatency float64
	var completed, verifyPassed, errored int
	for _, obs := range observations {
		totalCost += obs.TotalCostUSD
		totalLatency += float64(obs.TotalLatencyMs)
		if obs.Status != "failed" && obs.Error == "" {
			completed++
		}
		if obs.VerifyPassed || (obs.Status != "failed" && obs.Error == "") {
			verifyPassed++
		}
		if obs.Error != "" {
			errored++
		}
	}

	n := float64(len(observations))
	avgCost := totalCost / n
	avgLatency := totalLatency / n
	completionRate := float64(completed) / n
	verifyPassRate := float64(verifyPassed) / n
	errorRate := float64(errored) / n

	// Cost gate (relative to baseline P95)
	if baseline != nil && baseline.Aggregate != nil && baseline.Aggregate.CostP95 > 0 {
		ratio := avgCost / baseline.Aggregate.CostP95
		report.Results = append(report.Results, relativeGate(
			"cost_per_iteration", baseline.Aggregate.CostP95, avgCost, ratio,
			thresholds.CostPerIterWarn, thresholds.CostPerIterFail,
		))
	}

	// Latency gate (relative to baseline P95)
	if baseline != nil && baseline.Aggregate != nil && baseline.Aggregate.LatencyP95 > 0 {
		ratio := avgLatency / baseline.Aggregate.LatencyP95
		report.Results = append(report.Results, relativeGate(
			"total_latency", baseline.Aggregate.LatencyP95, avgLatency, ratio,
			thresholds.LatencyWarn, thresholds.LatencyFail,
		))
	}

	// Completion rate gate (absolute floor)
	report.Results = append(report.Results, absoluteFloorGate(
		"completion_rate", completionRate,
		thresholds.CompletionRateWarn, thresholds.CompletionRateFail,
	))

	// Verify pass rate gate (absolute floor)
	report.Results = append(report.Results, absoluteFloorGate(
		"verify_pass_rate", verifyPassRate,
		thresholds.VerifyPassRateWarn, thresholds.VerifyPassRateFail,
	))

	// Error rate gate (absolute ceiling)
	report.Results = append(report.Results, absoluteCeilingGate(
		"error_rate", errorRate,
		thresholds.ErrorRateWarn, thresholds.ErrorRateFail,
	))

	// Overall = worst verdict
	for _, r := range report.Results {
		if r.Verdict == VerdictFail {
			report.Overall = VerdictFail
			break
		}
		if r.Verdict == VerdictWarn && report.Overall != VerdictFail {
			report.Overall = VerdictWarn
		}
	}

	return report
}

// relativeGate evaluates a metric as a ratio against a baseline value.
// Returns VerdictSkip if the baseline is zero to avoid division-by-zero or
// infinite ratio edge cases.
func relativeGate(metric string, baseline, current, ratio, warnThresh, failThresh float64) GateResult {
	if baseline == 0 {
		return GateResult{
			Metric:      metric,
			Verdict:     VerdictSkip,
			BaselineVal: baseline,
			CurrentVal:  current,
			DeltaPct:    0,
		}
	}
	deltaPct := (ratio - 1) * 100
	verdict := VerdictPass
	if ratio >= failThresh {
		verdict = VerdictFail
	} else if ratio >= warnThresh {
		verdict = VerdictWarn
	}
	return GateResult{
		Metric:      metric,
		Verdict:     verdict,
		BaselineVal: baseline,
		CurrentVal:  current,
		DeltaPct:    deltaPct,
	}
}

// absoluteFloorGate evaluates a rate metric against absolute floors.
func absoluteFloorGate(metric string, current, warnFloor, failFloor float64) GateResult {
	verdict := VerdictPass
	if current < failFloor {
		verdict = VerdictFail
	} else if current < warnFloor {
		verdict = VerdictWarn
	}
	return GateResult{
		Metric:     metric,
		Verdict:    verdict,
		CurrentVal: current,
		DeltaPct:   0,
	}
}

// EvaluateFromObservations loads observations from the repo's observation file,
// builds a baseline, and evaluates gates against it. This is the shared logic
// used by both mock E2E gate checks and live self-test harness.
//
// If a saved baseline exists at .ralph/loop_baseline.json, it is loaded as
// the comparison target. Otherwise, a fresh baseline is built from the
// observations and persisted for future runs.
//
// hours controls the time window for filtering observations (0 = use all).
func EvaluateFromObservations(repoRoot string, thresholds GateThresholds, hours int) (*GateReport, error) {
	// Resolve worktree to main repo so observations are always found.
	if resolved, err := session.ResolveMainRepoPath(repoRoot); err == nil && resolved != "" {
		repoRoot = resolved
	}
	obsPath := session.ObservationPath(repoRoot)
	since := time.Time{}
	if hours > 0 {
		since = time.Now().Add(-time.Duration(hours) * time.Hour)
	}
	observations, err := session.LoadObservations(obsPath, since)
	if err != nil {
		return nil, fmt.Errorf("load observations: %w", err)
	}

	// Rolling window: keep only the most recent N observations so that
	// early development failures don't permanently drag down rates.
	if thresholds.MaxObservations > 0 && len(observations) > thresholds.MaxObservations {
		observations = observations[len(observations)-thresholds.MaxObservations:]
	}

	if len(observations) == 0 {
		return &GateReport{
			Timestamp: time.Now(),
			Overall:   VerdictSkip,
			Results:   []GateResult{{Metric: "observations", Verdict: VerdictSkip}},
		}, nil
	}

	// Try to load a persisted baseline for comparison
	blPath := filepath.Join(repoRoot, ".ralph", "loop_baseline.json")
	baseline, loadErr := LoadBaseline(blPath)
	if loadErr != nil {
		// No saved baseline — build one from current observations, persist it,
		// and return skip since we have no prior reference point.
		baseline = BuildBaseline(observations, float64(hours))
		if saveErr := SaveBaseline(blPath, baseline); saveErr != nil {
			return nil, fmt.Errorf("save initial baseline: %w", saveErr)
		}
		return &GateReport{
			Timestamp:   time.Now(),
			SampleCount: len(observations),
			Overall:     VerdictSkip,
			Results:     []GateResult{{Metric: "baseline", Verdict: VerdictSkip}},
		}, nil
	}

	// Evaluate gates against the saved baseline
	report := EvaluateGates(observations, baseline, thresholds)

	// Rebuild and persist updated baseline for next run
	freshBaseline := BuildBaseline(observations, float64(hours))
	if saveErr := SaveBaseline(blPath, freshBaseline); saveErr != nil {
		// Non-fatal — log but don't fail the gate
		slog.Warn("failed to save baseline", "err", saveErr)
	}

	return report, nil
}

// RunE2EGate executes the E2E test suite and evaluates regression gates.
// It runs tests, loads observations, builds a baseline, and returns a gate report.
// This is the entry point for autonomous test gating after config changes.
func RunE2EGate(repoRoot string) (*GateReport, error) {
	// Run E2E tests
	cmd := exec.Command("go", "test", "-run", "TestE2EAllScenarios", "./internal/e2e/")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("E2E tests failed: %w\n%s", err, output)
	}

	return EvaluateFromObservations(repoRoot, DefaultGateThresholds(), 0)
}

// absoluteCeilingGate evaluates a rate metric against absolute ceilings.
func absoluteCeilingGate(metric string, current, warnCeiling, failCeiling float64) GateResult {
	verdict := VerdictPass
	if current >= failCeiling {
		verdict = VerdictFail
	} else if current >= warnCeiling {
		verdict = VerdictWarn
	}
	return GateResult{
		Metric:     metric,
		Verdict:    verdict,
		CurrentVal: current,
		DeltaPct:   0,
	}
}
