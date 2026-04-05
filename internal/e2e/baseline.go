package e2e

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// BaselineKey identifies a unique baseline context.
type BaselineKey struct {
	Scenario string `json:"scenario"`
	Provider string `json:"provider"`
}

// BaselineStats holds P50/P95 values for a single metric group.
type BaselineStats struct {
	CostP50     float64 `json:"cost_p50"`
	CostP95     float64 `json:"cost_p95"`
	LatencyP50  float64 `json:"latency_p50_ms"`
	LatencyP95  float64 `json:"latency_p95_ms"`
	SampleCount int     `json:"sample_count"`
}

// LoopBaseline stores per-(scenario,provider) performance baselines.
type LoopBaseline struct {
	GeneratedAt time.Time                 `json:"generated_at"`
	WindowHours float64                   `json:"window_hours"`
	Entries     map[string]*BaselineStats `json:"entries"` // key: "scenario:provider"
	Aggregate   *BaselineStats            `json:"aggregate,omitempty"`
	Rates       *BaselineRates            `json:"rates,omitempty"`
}

// BaselineRates captures aggregate success/failure rates.
type BaselineRates struct {
	CompletionRate float64 `json:"completion_rate"`
	VerifyPassRate float64 `json:"verify_pass_rate"`
	ErrorRate      float64 `json:"error_rate"`
}

func baselineKey(scenario, provider string) string {
	if provider == "" {
		provider = "unknown"
	}
	return scenario + ":" + provider
}

// BuildBaseline computes baseline stats from observations within a time window.
func BuildBaseline(observations []session.LoopObservation, windowHours float64) *LoopBaseline {
	cutoff := time.Now().Add(-time.Duration(windowHours) * time.Hour)

	// Filter to window
	var filtered []session.LoopObservation
	for _, obs := range observations {
		if obs.Timestamp.After(cutoff) || windowHours <= 0 {
			filtered = append(filtered, obs)
		}
	}

	bl := &LoopBaseline{
		GeneratedAt: time.Now(),
		WindowHours: windowHours,
		Entries:     make(map[string]*BaselineStats),
	}

	if len(filtered) == 0 {
		return bl
	}

	// Group by (scenario, provider)
	type group struct {
		costs     []float64
		latencies []float64
	}
	groups := make(map[string]*group)
	for _, obs := range filtered {
		key := baselineKey(obs.TaskTitle, obs.PlannerProvider)
		g, ok := groups[key]
		if !ok {
			g = &group{}
			groups[key] = g
		}
		g.costs = append(g.costs, obs.TotalCostUSD)
		g.latencies = append(g.latencies, float64(obs.TotalLatencyMs))
	}

	for key, g := range groups {
		sort.Float64s(g.costs)
		sort.Float64s(g.latencies)
		bl.Entries[key] = &BaselineStats{
			CostP50:     percentileF(g.costs, 50),
			CostP95:     percentileF(g.costs, 95),
			LatencyP50:  percentileF(g.latencies, 50),
			LatencyP95:  percentileF(g.latencies, 95),
			SampleCount: len(g.costs),
		}
	}

	// Aggregate stats
	var allCosts, allLatencies []float64
	for _, g := range groups {
		allCosts = append(allCosts, g.costs...)
		allLatencies = append(allLatencies, g.latencies...)
	}
	sort.Float64s(allCosts)
	sort.Float64s(allLatencies)
	bl.Aggregate = &BaselineStats{
		CostP50:     percentileF(allCosts, 50),
		CostP95:     percentileF(allCosts, 95),
		LatencyP50:  percentileF(allLatencies, 50),
		LatencyP95:  percentileF(allLatencies, 95),
		SampleCount: len(allCosts),
	}

	// Rates — use the same lenient formula as gates.go: count as verified
	// when either VerifyPassed is true OR the observation didn't fail
	// (status != "failed" and no error). This prevents early incomplete
	// observations from permanently dragging down aggregate metrics.
	var completed, verifyPassed, errored int
	for _, obs := range filtered {
		if obs.Status == "idle" {
			completed++
		}
		if obs.VerifyPassed || (obs.Status != "failed" && obs.Error == "") {
			verifyPassed++
		}
		if obs.Error != "" {
			errored++
		}
	}
	total := len(filtered)
	bl.Rates = &BaselineRates{
		CompletionRate: float64(completed) / float64(total),
		VerifyPassRate: float64(verifyPassed) / float64(total),
		ErrorRate:      float64(errored) / float64(total),
	}

	return bl
}

// RefreshBaseline loads observations and rebuilds the baseline.
func RefreshBaseline(repoPath string, windowHours float64) (*LoopBaseline, error) {
	obsPath := session.ObservationPath(repoPath)
	observations, err := session.LoadObservations(obsPath, time.Time{})
	if err != nil {
		return nil, fmt.Errorf("load observations: %w", err)
	}
	return BuildBaseline(observations, windowHours), nil
}

// BaselinePath returns the canonical path for a repo's loop baseline.
func BaselinePath(repoPath string) string {
	return filepath.Join(repoPath, ".ralph", "loop_baseline.json")
}

// SaveBaseline writes a baseline to disk.
func SaveBaseline(path string, bl *LoopBaseline) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create baseline dir: %w", err)
	}
	data, err := json.MarshalIndent(bl, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal baseline: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadBaseline reads a baseline from disk.
func LoadBaseline(path string) (*LoopBaseline, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bl LoopBaseline
	if err := json.Unmarshal(data, &bl); err != nil {
		return nil, fmt.Errorf("unmarshal baseline: %w", err)
	}
	return &bl, nil
}

// percentileF returns the p-th percentile from a sorted float64 slice.
func percentileF(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := max(int(math.Ceil(float64(p)/100*float64(len(sorted))))-1, 0)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
