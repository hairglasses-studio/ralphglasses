package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestToolCallRecorder_InMemory(t *testing.T) {
	rec := NewToolCallRecorder("", nil, 10)

	rec.Record(ToolCallEntry{
		ToolName:  "test_tool",
		Timestamp: time.Now(),
		LatencyMs: 42,
		Success:   true,
		InputSize: 100,
	})

	entries := rec.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].ToolName != "test_tool" {
		t.Errorf("tool = %q, want test_tool", entries[0].ToolName)
	}
}

func TestToolCallRecorder_NilSafe(t *testing.T) {
	var rec *ToolCallRecorder
	rec.Record(ToolCallEntry{ToolName: "x"}) // should not panic
	rec.Close()                               // should not panic
	entries := rec.Entries()
	if entries != nil {
		t.Errorf("nil recorder should return nil entries")
	}
}

func TestToolCallRecorder_FlushToFile(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "bench.jsonl")
	rec := NewToolCallRecorder(fp, nil, 2) // flush after 2

	now := time.Now()
	rec.Record(ToolCallEntry{ToolName: "a", Timestamp: now, LatencyMs: 10, Success: true})
	rec.Record(ToolCallEntry{ToolName: "b", Timestamp: now, LatencyMs: 20, Success: false, ErrorMsg: "boom"})

	// After 2 records, should have flushed.
	entries, err := rec.LoadEntries(now.Add(-time.Hour))
	if err != nil {
		t.Fatalf("LoadEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries from file, got %d", len(entries))
	}
	if entries[1].ErrorMsg != "boom" {
		t.Errorf("error msg = %q, want boom", entries[1].ErrorMsg)
	}
}

func TestToolCallRecorder_Close(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "bench.jsonl")
	rec := NewToolCallRecorder(fp, nil, 100) // won't auto-flush

	rec.Record(ToolCallEntry{ToolName: "x", Timestamp: time.Now(), Success: true})
	rec.Close()

	data, err := os.ReadFile(fp)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected data after Close")
	}
}

func TestToolCallRecorder_LoadEntriesSince(t *testing.T) {
	dir := t.TempDir()
	fp := filepath.Join(dir, "bench.jsonl")
	rec := NewToolCallRecorder(fp, nil, 1)

	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now()

	rec.Record(ToolCallEntry{ToolName: "old", Timestamp: old, Success: true})
	rec.Record(ToolCallEntry{ToolName: "new", Timestamp: recent, Success: true})

	entries, err := rec.LoadEntries(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("LoadEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry since cutoff, got %d", len(entries))
	}
	if entries[0].ToolName != "new" {
		t.Errorf("expected 'new', got %q", entries[0].ToolName)
	}
}

func TestSummarize(t *testing.T) {
	entries := []ToolCallEntry{
		{ToolName: "a", LatencyMs: 10, Success: true},
		{ToolName: "a", LatencyMs: 20, Success: true},
		{ToolName: "a", LatencyMs: 100, Success: false, ErrorMsg: "fail"},
		{ToolName: "b", LatencyMs: 5, Success: true},
	}

	summaries := Summarize(entries)
	if len(summaries) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(summaries))
	}

	a := summaries["a"]
	if a.CallCount != 3 {
		t.Errorf("a.CallCount = %d, want 3", a.CallCount)
	}
	if a.ErrorCount != 1 {
		t.Errorf("a.ErrorCount = %d, want 1", a.ErrorCount)
	}
	if a.MaxLatencyMs != 100 {
		t.Errorf("a.MaxLatencyMs = %d, want 100", a.MaxLatencyMs)
	}
	// Success rate: 2/3 ≈ 66.67%
	if a.SuccessRate < 66 || a.SuccessRate > 67 {
		t.Errorf("a.SuccessRate = %.2f, want ~66.67", a.SuccessRate)
	}

	b := summaries["b"]
	if b.CallCount != 1 || b.ErrorCount != 0 {
		t.Errorf("b stats unexpected: calls=%d errors=%d", b.CallCount, b.ErrorCount)
	}
	if b.SuccessRate != 100 {
		t.Errorf("b.SuccessRate = %.2f, want 100", b.SuccessRate)
	}
}

func TestCompareRuns(t *testing.T) {
	baseline := map[string]*ToolBenchmarkSummary{
		"a": {ToolName: "a", P95LatencyMs: 100, SuccessRate: 99},
		"b": {ToolName: "b", P95LatencyMs: 50, SuccessRate: 100},
	}
	current := map[string]*ToolBenchmarkSummary{
		"a": {ToolName: "a", P95LatencyMs: 160, SuccessRate: 99}, // +60% → warning
		"b": {ToolName: "b", P95LatencyMs: 50, SuccessRate: 88},  // -12% → regression
		"c": {ToolName: "c", P95LatencyMs: 200, SuccessRate: 95}, // new, no baseline
	}

	regs := CompareRuns(baseline, current)
	if len(regs) != 2 {
		t.Fatalf("expected 2 regressions, got %d", len(regs))
	}

	// Regressions first, then warnings.
	found := map[string]string{}
	for _, r := range regs {
		found[r.ToolName+"_"+r.Metric] = r.Severity
	}

	if found["a_p95_latency"] != "warning" {
		t.Errorf("a p95 latency should be warning, got %q", found["a_p95_latency"])
	}
	if found["b_success_rate"] != "regression" {
		t.Errorf("b success_rate should be regression, got %q", found["b_success_rate"])
	}
}

func TestCompareRuns_NoBaseline(t *testing.T) {
	current := map[string]*ToolBenchmarkSummary{
		"new_tool": {ToolName: "new_tool", P95LatencyMs: 100, SuccessRate: 90},
	}
	regs := CompareRuns(nil, current)
	if len(regs) != 0 {
		t.Errorf("expected 0 regressions for nil baseline, got %d", len(regs))
	}
}

func TestPercentile(t *testing.T) {
	sorted := []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	p50 := percentile(sorted, 50)
	if p50 != 6 {
		t.Errorf("p50 = %d, want 6", p50)
	}
	p95 := percentile(sorted, 95)
	if p95 != 10 {
		t.Errorf("p95 = %d, want 10", p95)
	}
	p0 := percentile(nil, 50)
	if p0 != 0 {
		t.Errorf("p50 of empty = %d, want 0", p0)
	}
}
