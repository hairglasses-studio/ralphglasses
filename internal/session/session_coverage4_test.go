package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// Supervisor.recordGateFindings (0%)
// ---------------------------------------------------------------------------

func TestRecordGateFindings_HappyPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := NewManager()
	s := NewSupervisor(m, dir)

	findings := []CycleFinding{
		{ID: "f1", Description: "test finding", Category: "coverage", Severity: "medium", Source: "gate"},
		{ID: "f2", Description: "another finding", Category: "perf", Severity: "low", Source: "observation"},
	}
	s.recordGateFindings(dir, findings)

	// Verify the file was written.
	outPath := filepath.Join(dir, ".ralph", "gate_findings.json")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var loaded []CycleFinding
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("expected 2 findings, got %d", len(loaded))
	}
	if loaded[0].ID != "f1" {
		t.Errorf("first finding ID = %q, want f1", loaded[0].ID)
	}
}

func TestRecordGateFindings_EmptyFindings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := NewManager()
	s := NewSupervisor(m, dir)

	s.recordGateFindings(dir, []CycleFinding{})

	outPath := filepath.Join(dir, ".ralph", "gate_findings.json")
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var loaded []CycleFinding
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(loaded) != 0 {
		t.Errorf("expected 0 findings, got %d", len(loaded))
	}
}

func TestRecordGateFindings_CreatesDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repoPath := filepath.Join(dir, "nested", "repo")
	m := NewManager()
	s := NewSupervisor(m, dir)

	s.recordGateFindings(repoPath, []CycleFinding{
		{ID: "f1", Description: "test"},
	})

	outPath := filepath.Join(repoPath, ".ralph", "gate_findings.json")
	if _, err := os.Stat(outPath); os.IsNotExist(err) {
		t.Error("expected gate_findings.json to be created even with nested dirs")
	}
}

// ---------------------------------------------------------------------------
// Supervisor.RecursionGuard + SetSelfTestEnv (helpers used by runSelfTest)
// ---------------------------------------------------------------------------

func TestRecursionGuard_NotInSelfTest(t *testing.T) {
	t.Parallel()
	// In normal test, the guard should pass (unless RALPH_SELF_TEST is set).
	err := RecursionGuard()
	// If RALPH_SELF_TEST happens to be set in CI, this is expected to fail.
	if os.Getenv("RALPH_SELF_TEST") == "1" {
		if err == nil {
			t.Error("RecursionGuard should fail when RALPH_SELF_TEST=1")
		}
	} else {
		if err != nil {
			t.Errorf("RecursionGuard should pass in normal test: %v", err)
		}
	}
}

func TestSetSelfTestEnv_PreservesOriginal(t *testing.T) {
	t.Parallel()
	env := []string{"HOME=/home/test", "PATH=/usr/bin", "EXISTING=yes"}
	result := SetSelfTestEnv(env)

	found := false
	for _, v := range result {
		if v == "RALPH_SELF_TEST=1" {
			found = true
		}
	}
	if !found {
		t.Error("SetSelfTestEnv should add RALPH_SELF_TEST=1")
	}
	// All original env vars should be preserved.
	if len(result) != len(env)+1 {
		t.Errorf("expected %d env vars, got %d", len(env)+1, len(result))
	}
}

// ---------------------------------------------------------------------------
// Supervisor state persistence (saveState/resumeState partial 0%)
// ---------------------------------------------------------------------------

func TestSupervisor_PersistAndResume(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := NewManager()
	s := NewSupervisor(m, dir)

	// Set some internal state.
	s.mu.Lock()
	s.tickCount = 42
	s.cyclesLaunched = 5
	s.mu.Unlock()

	// Persist state.
	s.persistState()

	// Create a new supervisor and resume.
	s2 := NewSupervisor(m, dir)
	err := s2.ResumeFromState()
	if err != nil {
		t.Fatalf("ResumeFromState: %v", err)
	}

	s2.mu.Lock()
	defer s2.mu.Unlock()
	if s2.tickCount != 42 {
		t.Errorf("tickCount = %d, want 42", s2.tickCount)
	}
}

func TestSupervisor_ResumeFromState_NoFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := NewManager()
	s := NewSupervisor(m, dir)

	// ResumeFromState with no file should return an error.
	err := s.ResumeFromState()
	if err == nil {
		t.Error("ResumeFromState should fail when no state file exists")
	}
}
