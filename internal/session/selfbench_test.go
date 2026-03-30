package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveBenchmark(t *testing.T) {
	dir := t.TempDir()
	b := SelfBenchmark{
		Timestamp:  time.Now(),
		Iteration:  1,
		BuildTime:  "1.5s",
		TestResult: "pass",
		BinarySize: 12345678,
		LintScore:  "clean",
		Coverage:   "86.0%",
	}

	if err := SaveBenchmark(dir, b); err != nil {
		t.Fatalf("SaveBenchmark: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, ".ralph", "benchmarks.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("benchmark file not created: %v", err)
	}

	// Load and verify
	results, err := LoadBenchmarks(dir)
	if err != nil {
		t.Fatalf("LoadBenchmarks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 benchmark, got %d", len(results))
	}
	if results[0].TestResult != "pass" {
		t.Errorf("expected pass, got %s", results[0].TestResult)
	}
	if results[0].BinarySize != 12345678 {
		t.Errorf("wrong binary size: %d", results[0].BinarySize)
	}
}

func TestLoadBenchmarks_EmptyDir(t *testing.T) {
	results, err := LoadBenchmarks(t.TempDir())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}
