package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStop_NonExistentRepo(t *testing.T) {
	t.Parallel()

	m := NewManager()
	err := m.Stop(context.Background(), "/nonexistent/repo/path")
	if err == nil {
		t.Fatal("expected error stopping non-existent repo, got nil")
	}
}

func TestStop_AlreadyStopped(t *testing.T) {
	t.Parallel()

	m := NewManager()
	// Stop on a repo that was never started.
	err := m.Stop(context.Background(), "/some/repo/that/was/never/started")
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
	m.StopAll(context.Background())
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

func TestStop_TwiceOnSameProcess(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestScript(t, filepath.Join(repoPath, "ralph_loop.sh"), "sleep 60")

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// First stop should succeed.
	err := m.Stop(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("first Stop: %v", err)
	}

	// Give the reaper goroutine time to clean up the process entry.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning(repoPath) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Second stop should return an error (not panic).
	err = m.Stop(context.Background(), repoPath)
	if err == nil {
		t.Fatal("expected error on second Stop, got nil")
	}
}

// TestStop_ReaperRace exercises the race between Stop() and reaperLoop.
// A short-lived process exits almost immediately, causing the reaper to
// fire. Meanwhile Stop() is called concurrently. With the stopping flag
// this must not panic, double-delete, or corrupt the procs map.
// Run with: go test -race -count=3 ./internal/process/...
func TestStop_ReaperRace(t *testing.T) {
	// Stub sleep so kill-sequence timeouts don't slow the test.
	origSleep := *sleepFnPtr.Load()
	setSleepFn(func(d time.Duration) {})
	t.Cleanup(func() { setSleepFn(origSleep) })

	for range 20 {
		t.Run("", func(t *testing.T) {
			m := NewManager()
			repoPath := t.TempDir()
			ralphDir := filepath.Join(repoPath, ".ralph")
			if err := os.MkdirAll(ralphDir, 0755); err != nil {
				t.Fatal(err)
			}
			// Script exits almost immediately — the reaper fires right away.
			writeTestScript(t, filepath.Join(repoPath, "ralph_loop.sh"), "exit 1")

			if err := m.Start(context.Background(), repoPath); err != nil {
				t.Fatalf("Start: %v", err)
			}

			// Call Stop concurrently with the reaper that is triggered by the
			// near-instant process exit. Either Stop wins (process is stopped)
			// or the reaper wins (process already cleaned up and Stop returns
			// ErrNotRunning). Both are acceptable — the important thing is no
			// panic, no data race, and no double-delete.
			_ = m.Stop(context.Background(), repoPath)

			// Wait for everything to settle.
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if !m.IsRunning(repoPath) {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}

			if m.IsRunning(repoPath) {
				m.StopAll(context.Background())
				t.Fatal("process still running after Stop + reaper race")
			}
		})
	}
}

// TestStop_ReaperRace_WithAutoRestart verifies the stopping flag prevents
// the reaper from auto-restarting a process that Stop() is shutting down.
func TestStop_ReaperRace_WithAutoRestart(t *testing.T) {
	origSleep := *sleepFnPtr.Load()
	setSleepFn(func(d time.Duration) {})
	t.Cleanup(func() { setSleepFn(origSleep) })

	for range 10 {
		t.Run("", func(t *testing.T) {
			m := NewManager()
			m.AutoRestart = true
			m.MaxRestarts = 5

			repoPath := t.TempDir()
			ralphDir := filepath.Join(repoPath, ".ralph")
			if err := os.MkdirAll(ralphDir, 0755); err != nil {
				t.Fatal(err)
			}
			counterFile := filepath.Join(ralphDir, "race_counter")
			writeTestScript(t, filepath.Join(repoPath, "ralph_loop.sh"),
				`COUNTER_FILE="`+counterFile+`"
if [ ! -f "$COUNTER_FILE" ]; then
  echo 1 > "$COUNTER_FILE"
else
  COUNT=$(cat "$COUNTER_FILE")
  echo $((COUNT + 1)) > "$COUNTER_FILE"
fi
exit 1`)

			if err := m.Start(context.Background(), repoPath); err != nil {
				t.Fatalf("Start: %v", err)
			}

			// Brief pause to let at least one iteration run.
			time.Sleep(50 * time.Millisecond)

			// Stop while the reaper may be mid-restart-cycle.
			_ = m.Stop(context.Background(), repoPath)

			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if !m.IsRunning(repoPath) {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}

			if m.IsRunning(repoPath) {
				m.StopAll(context.Background())
				t.Fatal("process still running after Stop with auto-restart race")
			}
		})
	}
}

func TestStop_NeverStarted_ReturnsError(t *testing.T) {
	t.Parallel()

	m := NewManager()
	err := m.Stop(context.Background(), "/completely/fake/repo/path")
	if err == nil {
		t.Fatal("expected error stopping process that was never started, got nil")
	}
}

func TestStopAll_ThenStop_DoesNotPanic(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestScript(t, filepath.Join(repoPath, "ralph_loop.sh"), "sleep 60")

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// StopAll clears the map immediately.
	m.StopAll(context.Background())

	// Stop after StopAll should return error, not panic.
	err := m.Stop(context.Background(), repoPath)
	if err == nil {
		t.Fatal("expected error on Stop after StopAll, got nil")
	}
}
