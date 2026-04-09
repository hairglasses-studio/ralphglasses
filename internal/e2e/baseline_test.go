package e2e

import (
	"math"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestBuildBaseline_Empty(t *testing.T) {
	bl := BuildBaseline(nil, 24)
	if bl == nil {
		t.Fatal("BuildBaseline returned nil")
	}
	if len(bl.Entries) != 0 {
		t.Errorf("Entries should be empty, got %d", len(bl.Entries))
	}
	if bl.Aggregate != nil {
		t.Error("Aggregate should be nil for empty input")
	}
	if bl.Rates != nil {
		t.Error("Rates should be nil for empty input")
	}
	if bl.WindowHours != 24 {
		t.Errorf("WindowHours = %f, want 24", bl.WindowHours)
	}
}

func TestBuildBaseline_WindowFiltering(t *testing.T) {
	now := time.Now()
	obs := []session.LoopObservation{
		{Timestamp: now.Add(-1 * time.Hour), TaskTitle: "recent", TotalCostUSD: 0.10, Status: "idle"},
		{Timestamp: now.Add(-48 * time.Hour), TaskTitle: "old", TotalCostUSD: 0.50, Status: "idle"},
	}

	bl := BuildBaseline(obs, 24)
	if bl.Aggregate == nil {
		t.Fatal("Aggregate should not be nil")
	}
	// Only the recent observation should be included
	if bl.Aggregate.SampleCount != 1 {
		t.Errorf("SampleCount = %d, want 1 (old observation filtered out)", bl.Aggregate.SampleCount)
	}
}

func TestBuildBaseline_ZeroWindowIncludesAll(t *testing.T) {
	now := time.Now()
	obs := []session.LoopObservation{
		{Timestamp: now.Add(-1000 * time.Hour), TaskTitle: "ancient", TotalCostUSD: 0.01},
		{Timestamp: now, TaskTitle: "now", TotalCostUSD: 0.02},
	}

	bl := BuildBaseline(obs, 0)
	if bl.Aggregate == nil {
		t.Fatal("Aggregate should not be nil")
	}
	if bl.Aggregate.SampleCount != 2 {
		t.Errorf("SampleCount = %d, want 2 (zero window includes all)", bl.Aggregate.SampleCount)
	}
}

func TestBuildBaseline_ExcludesStandaloneObservations(t *testing.T) {
	now := time.Now()
	obs := []session.LoopObservation{
		{Timestamp: now, Mode: "standalone", TaskTitle: "session smoke test", PlannerProvider: "codex", TotalCostUSD: 1.0, Status: "failed", Error: "launch failed"},
		{Timestamp: now, Mode: "mock", TaskTitle: "e2e scenario", PlannerProvider: "claude", TotalCostUSD: 0.2, Status: "idle"},
		{Timestamp: now, Mode: "live", TaskTitle: "loop task", PlannerProvider: "codex", TotalCostUSD: 0.4, Status: "idle"},
	}

	bl := BuildBaseline(obs, 0)
	if bl.Aggregate == nil {
		t.Fatal("Aggregate should not be nil")
	}
	if bl.Aggregate.SampleCount != 2 {
		t.Fatalf("SampleCount = %d, want 2 (standalone excluded)", bl.Aggregate.SampleCount)
	}
	if _, ok := bl.Entries["session smoke test:codex"]; ok {
		t.Fatal("standalone observation should not contribute a baseline entry")
	}
	if _, ok := bl.Entries["e2e scenario:claude"]; !ok {
		t.Fatal("mock observation should contribute a baseline entry")
	}
	if _, ok := bl.Entries["loop task:codex"]; !ok {
		t.Fatal("live observation should contribute a baseline entry")
	}
}

func TestBuildBaseline_PercentileCalculations(t *testing.T) {
	now := time.Now()
	var obs []session.LoopObservation
	// Create 100 observations with costs 1..100 and latencies 100..10000
	for i := 1; i <= 100; i++ {
		obs = append(obs, session.LoopObservation{
			Timestamp:       now,
			TaskTitle:       "scenario",
			PlannerProvider: "claude",
			TotalCostUSD:    float64(i),
			TotalLatencyMs:  int64(i * 100),
			Status:          "idle",
		})
	}

	bl := BuildBaseline(obs, 0)
	if bl.Aggregate == nil {
		t.Fatal("Aggregate should not be nil")
	}

	// P50 of 1..100 sorted: index = ceil(50/100*100) - 1 = 49 -> value 50
	if bl.Aggregate.CostP50 != 50 {
		t.Errorf("CostP50 = %f, want 50", bl.Aggregate.CostP50)
	}
	// P95: index = ceil(95/100*100) - 1 = 94 -> value 95
	if bl.Aggregate.CostP95 != 95 {
		t.Errorf("CostP95 = %f, want 95", bl.Aggregate.CostP95)
	}

	if bl.Aggregate.LatencyP50 != 5000 {
		t.Errorf("LatencyP50 = %f, want 5000", bl.Aggregate.LatencyP50)
	}
	if bl.Aggregate.LatencyP95 != 9500 {
		t.Errorf("LatencyP95 = %f, want 9500", bl.Aggregate.LatencyP95)
	}
}

func TestBuildBaseline_PercentileSingleElement(t *testing.T) {
	now := time.Now()
	obs := []session.LoopObservation{
		{Timestamp: now, TaskTitle: "solo", TotalCostUSD: 42, TotalLatencyMs: 999, Status: "idle"},
	}
	bl := BuildBaseline(obs, 0)
	if bl.Aggregate.CostP50 != 42 {
		t.Errorf("CostP50 = %f, want 42", bl.Aggregate.CostP50)
	}
	if bl.Aggregate.CostP95 != 42 {
		t.Errorf("CostP95 = %f, want 42", bl.Aggregate.CostP95)
	}
}

func TestBuildBaseline_Rates(t *testing.T) {
	now := time.Now()
	obs := []session.LoopObservation{
		{Timestamp: now, TaskTitle: "a", Status: "idle", VerifyPassed: true},
		{Timestamp: now, TaskTitle: "b", Status: "failed", Error: "timeout"},
		{Timestamp: now, TaskTitle: "c", Status: "idle", VerifyPassed: false, Error: ""},
		{Timestamp: now, TaskTitle: "d", Status: "running", Error: ""},
	}

	bl := BuildBaseline(obs, 0)
	if bl.Rates == nil {
		t.Fatal("Rates should not be nil")
	}

	// Completion: 2 idle out of 4 = 0.5
	if bl.Rates.CompletionRate != 0.5 {
		t.Errorf("CompletionRate = %f, want 0.5", bl.Rates.CompletionRate)
	}

	// Verify: a (VerifyPassed=true), c (status!=failed, no error), d (status!=failed, no error) = 3/4
	if bl.Rates.VerifyPassRate != 0.75 {
		t.Errorf("VerifyPassRate = %f, want 0.75", bl.Rates.VerifyPassRate)
	}

	// Error: b has error = 1/4
	if bl.Rates.ErrorRate != 0.25 {
		t.Errorf("ErrorRate = %f, want 0.25", bl.Rates.ErrorRate)
	}
}

func TestBuildBaseline_GroupsByScenarioAndProvider(t *testing.T) {
	now := time.Now()
	obs := []session.LoopObservation{
		{Timestamp: now, TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.10},
		{Timestamp: now, TaskTitle: "task-a", PlannerProvider: "gemini", TotalCostUSD: 0.05},
		{Timestamp: now, TaskTitle: "task-b", PlannerProvider: "claude", TotalCostUSD: 0.20},
	}

	bl := BuildBaseline(obs, 0)
	if len(bl.Entries) != 3 {
		t.Errorf("Entries count = %d, want 3", len(bl.Entries))
	}

	if _, ok := bl.Entries["task-a:claude"]; !ok {
		t.Error("missing entry for task-a:claude")
	}
	if _, ok := bl.Entries["task-a:gemini"]; !ok {
		t.Error("missing entry for task-a:gemini")
	}
	if _, ok := bl.Entries["task-b:claude"]; !ok {
		t.Error("missing entry for task-b:claude")
	}
}

func TestBuildBaseline_EmptyProviderDefaultsToUnknown(t *testing.T) {
	now := time.Now()
	obs := []session.LoopObservation{
		{Timestamp: now, TaskTitle: "task", PlannerProvider: "", TotalCostUSD: 0.10},
	}

	bl := BuildBaseline(obs, 0)
	if _, ok := bl.Entries["task:unknown"]; !ok {
		t.Error("expected empty provider to default to 'unknown'")
	}
}

func TestPercentileF_Empty(t *testing.T) {
	result := percentileF(nil, 50)
	if result != 0 {
		t.Errorf("percentileF(nil, 50) = %f, want 0", result)
	}
}

func TestPercentileF_Boundary(t *testing.T) {
	data := []float64{10, 20, 30, 40, 50}

	p0 := percentileF(data, 0)
	if p0 != 10 {
		t.Errorf("percentileF(data, 0) = %f, want 10", p0)
	}

	p100 := percentileF(data, 100)
	if p100 != 50 {
		t.Errorf("percentileF(data, 100) = %f, want 50", p100)
	}
}

func TestBaselineKey(t *testing.T) {
	if got := baselineKey("scenario", "claude"); got != "scenario:claude" {
		t.Errorf("baselineKey = %s, want scenario:claude", got)
	}
	if got := baselineKey("scenario", ""); got != "scenario:unknown" {
		t.Errorf("baselineKey = %s, want scenario:unknown", got)
	}
}

func TestBaselinePath(t *testing.T) {
	path := BaselinePath("/tmp/repo")
	if path != "/tmp/repo/.ralph/loop_baseline.json" {
		t.Errorf("BaselinePath = %s, want /tmp/repo/.ralph/loop_baseline.json", path)
	}
}

func TestBuildBaseline_GeneratedAtIsRecent(t *testing.T) {
	bl := BuildBaseline(nil, 24)
	if time.Since(bl.GeneratedAt) > 5*time.Second {
		t.Errorf("GeneratedAt is too old: %v", bl.GeneratedAt)
	}
}

// Suppress unused import warning.
var _ = math.Ceil
