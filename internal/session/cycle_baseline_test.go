package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunCycleBaseline(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/baseline\n\ngo 1.26.1\n"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	baseline, err := RunCycleBaseline(dir)
	if err != nil {
		t.Fatalf("RunCycleBaseline returned error: %v", err)
	}
	if baseline.RepoPath != dir {
		t.Fatalf("RepoPath = %q, want %q", baseline.RepoPath, dir)
	}
	if baseline.TestCount != 0 {
		t.Fatalf("TestCount = %d, want 0 for repo without tests", baseline.TestCount)
	}
	if baseline.CoveragePC != 0 {
		t.Fatalf("CoveragePC = %v, want 0 for repo without tests", baseline.CoveragePC)
	}
	if baseline.BuildTimeSec <= 0 {
		t.Fatalf("BuildTimeSec = %v, want > 0", baseline.BuildTimeSec)
	}
}

func TestWriteCycleBaselineToFile(t *testing.T) {
	baseline := &CycleBaseline{}
	path := "test_baseline.json"
	defer os.Remove(path)
	_ = WriteCycleBaselineToFile(baseline, path)
}
