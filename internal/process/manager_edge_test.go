package process

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStop_NonExistentRepo(t *testing.T) {
	t.Parallel()

	m := NewManager()
	err := m.Stop("/nonexistent/repo/path")
	if err == nil {
		t.Fatal("expected error stopping non-existent repo, got nil")
	}
}

func TestStop_AlreadyStopped(t *testing.T) {
	t.Parallel()

	m := NewManager()
	// Stop on a repo that was never started.
	err := m.Stop("/some/repo/that/was/never/started")
	if err == nil {
		t.Fatal("expected error on double-stop, got nil")
	}
}

func TestIsRunning_AfterNeverStarted(t *testing.T) {
	t.Parallel()

	m := NewManager()
	if m.IsRunning("/never/started") {
		t.Error("expected IsRunning=false for never-started repo")
	}
}

func TestRecover_NoPIDFiles(t *testing.T) {
	t.Parallel()

	m := NewManager()
	dir := t.TempDir()

	// Create a repo dir with .ralph/ but no PID file.
	repoPath := filepath.Join(dir, "myrepo")
	if err := os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}

	recovered := m.Recover([]string{repoPath})
	if recovered != 0 {
		t.Errorf("expected 0 recovered, got %d", recovered)
	}
}

func TestRecover_StalePIDFile(t *testing.T) {
	t.Parallel()

	m := NewManager()
	dir := t.TempDir()

	repoPath := filepath.Join(dir, "stalerepo")
	if err := os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}

	// Write a PID file with a PID that almost certainly doesn't exist.
	// Use a very high PID number that is unlikely to be a real process.
	stalePID := 2147483647 // max PID on most systems
	if err := os.WriteFile(
		filepath.Join(repoPath, ".ralph", pidFileName),
		[]byte("2147483647\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	recovered := m.Recover([]string{repoPath})
	if recovered != 0 {
		t.Errorf("expected 0 recovered for stale PID %d, got %d", stalePID, recovered)
	}

	// PID file should be cleaned up.
	if _, err := os.Stat(filepath.Join(repoPath, ".ralph", pidFileName)); !os.IsNotExist(err) {
		t.Error("expected stale PID file to be removed")
	}
}

func TestRecover_EmptyPIDFile(t *testing.T) {
	t.Parallel()

	m := NewManager()
	dir := t.TempDir()

	repoPath := filepath.Join(dir, "emptyrepo")
	if err := os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}

	// Write an empty PID file.
	if err := os.WriteFile(
		filepath.Join(repoPath, ".ralph", pidFileName),
		[]byte(""),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	recovered := m.Recover([]string{repoPath})
	if recovered != 0 {
		t.Errorf("expected 0 recovered for empty PID file, got %d", recovered)
	}
}

func TestRecover_InvalidPIDFile(t *testing.T) {
	t.Parallel()

	m := NewManager()
	dir := t.TempDir()

	repoPath := filepath.Join(dir, "badpid")
	if err := os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}

	// Write non-numeric content.
	if err := os.WriteFile(
		filepath.Join(repoPath, ".ralph", pidFileName),
		[]byte("notanumber\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	recovered := m.Recover([]string{repoPath})
	if recovered != 0 {
		t.Errorf("expected 0 recovered for invalid PID file, got %d", recovered)
	}
}

func TestPidForRepo_Unmanaged(t *testing.T) {
	t.Parallel()

	m := NewManager()
	pid := m.PidForRepo("/unmanaged/repo")
	if pid != 0 {
		t.Errorf("expected PID=0 for unmanaged repo, got %d", pid)
	}
}

func TestIsPaused_Unmanaged(t *testing.T) {
	t.Parallel()

	m := NewManager()
	if m.IsPaused("/unmanaged/repo") {
		t.Error("expected IsPaused=false for unmanaged repo")
	}
}

func TestTogglePause_Unmanaged(t *testing.T) {
	t.Parallel()

	m := NewManager()
	_, err := m.TogglePause("/unmanaged/repo")
	if err == nil {
		t.Fatal("expected error toggling pause on unmanaged repo")
	}
}

func TestRunningPaths_Empty(t *testing.T) {
	t.Parallel()

	m := NewManager()
	paths := m.RunningPaths()
	if len(paths) != 0 {
		t.Errorf("expected 0 running paths, got %d", len(paths))
	}
}

func TestStopAll_Empty(t *testing.T) {
	t.Parallel()

	m := NewManager()
	// StopAll on empty manager should not panic.
	m.StopAll()
}

func TestLastExitStatus_Unknown(t *testing.T) {
	t.Parallel()

	m := NewManager()
	code, errStr, ok := m.LastExitStatus("/unknown/repo")
	if ok {
		t.Error("expected ok=false for unknown repo")
	}
	if code != 0 {
		t.Errorf("expected code=0, got %d", code)
	}
	if errStr != "" {
		t.Errorf("expected empty error string, got %q", errStr)
	}
}

func TestCleanStalePIDFiles_NoPIDFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	repoPath := filepath.Join(dir, "cleanrepo")
	if err := os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}

	cleaned := CleanStalePIDFiles([]string{repoPath})
	if cleaned != 0 {
		t.Errorf("expected 0 cleaned, got %d", cleaned)
	}
}

func TestCleanStalePIDFiles_DeadProcess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	repoPath := filepath.Join(dir, "deadrepo")
	if err := os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}

	// Write PID file for a process that doesn't exist.
	if err := os.WriteFile(
		filepath.Join(repoPath, ".ralph", pidFileName),
		[]byte("2147483647\n"),
		0644,
	); err != nil {
		t.Fatal(err)
	}

	cleaned := CleanStalePIDFiles([]string{repoPath})
	if cleaned != 1 {
		t.Errorf("expected 1 cleaned, got %d", cleaned)
	}

	// PID file should be removed.
	if _, err := os.Stat(filepath.Join(repoPath, ".ralph", pidFileName)); !os.IsNotExist(err) {
		t.Error("expected PID file to be removed after cleaning")
	}
}
