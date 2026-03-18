package model

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendAndLoadBenchmarkEntries(t *testing.T) {
	dir := t.TempDir()
	now := time.Now().Truncate(time.Second)

	e1 := &BenchmarkEntry{
		Timestamp:    now,
		Loop:         1,
		TaskID:       "scan",
		InputTokens:  12000,
		OutputTokens: 3000,
		DurationSec:  45,
		Result:       "pass",
		CostUSD:      0.08,
		Model:        "sonnet",
	}
	e2 := &BenchmarkEntry{
		Timestamp:    now.Add(time.Minute),
		Loop:         2,
		TaskID:       "plan",
		InputTokens:  18000,
		OutputTokens: 4000,
		DurationSec:  62,
		Result:       "fail",
		CostUSD:      0.12,
		Model:        "sonnet",
		Spin:         true,
		SpinSignal:   "same_error_x3",
	}

	if err := AppendBenchmarkEntry(dir, e1); err != nil {
		t.Fatalf("Append e1: %v", err)
	}
	if err := AppendBenchmarkEntry(dir, e2); err != nil {
		t.Fatalf("Append e2: %v", err)
	}

	entries, err := LoadBenchmarkEntries(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].TaskID != "scan" {
		t.Errorf("entries[0].TaskID = %q, want %q", entries[0].TaskID, "scan")
	}
	if entries[1].Spin != true {
		t.Error("entries[1].Spin should be true")
	}
	if entries[1].SpinSignal != "same_error_x3" {
		t.Errorf("entries[1].SpinSignal = %q, want %q", entries[1].SpinSignal, "same_error_x3")
	}
}

func TestLoadBenchmarkEntriesEmpty(t *testing.T) {
	dir := t.TempDir()
	entries, err := LoadBenchmarkEntries(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestGenerateSummary(t *testing.T) {
	now := time.Now()
	entries := []BenchmarkEntry{
		{Timestamp: now, Loop: 1, TaskID: "a", InputTokens: 10000, OutputTokens: 2000, CostUSD: 0.10, Result: "pass", Model: "sonnet"},
		{Timestamp: now.Add(5 * time.Minute), Loop: 2, TaskID: "b", InputTokens: 15000, OutputTokens: 3000, CostUSD: 0.15, Result: "pass"},
		{Timestamp: now.Add(10 * time.Minute), Loop: 3, TaskID: "c", InputTokens: 20000, OutputTokens: 5000, CostUSD: 0.20, Result: "fail", Spin: true},
	}

	s := GenerateSummary("test-session", entries)

	if s.LoopCount != 3 {
		t.Errorf("LoopCount = %d, want 3", s.LoopCount)
	}
	if s.TotalTokens != 55000 {
		t.Errorf("TotalTokens = %d, want 55000", s.TotalTokens)
	}
	if s.TasksCompleted != 2 {
		t.Errorf("TasksCompleted = %d, want 2", s.TasksCompleted)
	}
	if s.TasksTotal != 3 {
		t.Errorf("TasksTotal = %d, want 3", s.TasksTotal)
	}
	if s.SpinEvents != 1 {
		t.Errorf("SpinEvents = %d, want 1", s.SpinEvents)
	}
	expectedCost := 0.45
	if s.CostEstimate < expectedCost-0.01 || s.CostEstimate > expectedCost+0.01 {
		t.Errorf("CostEstimate = %f, want ~%f", s.CostEstimate, expectedCost)
	}
}

func TestGenerateSummaryEmpty(t *testing.T) {
	s := GenerateSummary("empty", nil)
	if s.LoopCount != 0 {
		t.Errorf("LoopCount = %d, want 0", s.LoopCount)
	}
}

func TestWriteBenchmarkMarkdown(t *testing.T) {
	dir := t.TempDir()
	s := &BenchmarkSummary{
		SessionID:      "test",
		StartedAt:      time.Now(),
		LoopCount:      5,
		TotalTokens:    100000,
		InputTokens:    80000,
		OutputTokens:   20000,
		WallTime:       "0h 10m",
		TasksCompleted: 3,
		TasksTotal:     5,
		CostEstimate:   1.50,
		CostPerTask:    0.50,
		ExitReason:     "completion",
		Model:          "sonnet",
	}

	if err := WriteBenchmarkMarkdown(dir, s); err != nil {
		t.Fatalf("WriteBenchmarkMarkdown: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".ralph", "benchmarks.md"))
	if err != nil {
		t.Fatalf("read benchmarks.md: %v", err)
	}

	content := string(data)
	if !contains(content, "loop_count | 5") {
		t.Error("expected loop_count in markdown")
	}
	if !contains(content, "total_tokens | 100000") {
		t.Error("expected total_tokens in markdown")
	}
	if !contains(content, "3/5") {
		t.Error("expected tasks_completed fraction in markdown")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
