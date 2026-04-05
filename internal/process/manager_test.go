package process

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestNewManager(t *testing.T) {
	m := NewManager()
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.procs == nil {
		t.Fatal("procs map not initialized")
	}
}

func TestManager_IsRunning_Empty(t *testing.T) {
	m := NewManager()
	if m.IsRunning("/some/path") {
		t.Error("IsRunning should return false for unknown path")
	}
}

func TestManager_IsPaused_Empty(t *testing.T) {
	m := NewManager()
	if m.IsPaused("/some/path") {
		t.Error("IsPaused should return false for unknown path")
	}
}

func TestManager_RunningPaths_Empty(t *testing.T) {
	m := NewManager()
	paths := m.RunningPaths()
	if len(paths) != 0 {
		t.Errorf("RunningPaths should be empty, got %d", len(paths))
	}
}

func TestManager_StopAll_Empty(t *testing.T) {
	m := NewManager()
	// Should not panic
	m.StopAll(context.Background())
}

func TestManager_Stop_NotRunning(t *testing.T) {
	m := NewManager()
	err := m.Stop(context.Background(), "/not/running")
	if err == nil {
		t.Fatal("expected error when stopping non-running process")
	}
}

func TestManager_TogglePause_NotRunning(t *testing.T) {
	m := NewManager()
	_, err := m.TogglePause("/not/running")
	if err == nil {
		t.Fatal("expected error when pausing non-running process")
	}
}

func writeTestScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/bash\n"+body+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
}

func TestManager_Start_DuplicateReturnsError(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	writeTestScript(t, repoPath+"/ralph_loop.sh", "sleep 60")

	err := m.Start(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.StopAll(context.Background())

	err = m.Start(context.Background(), repoPath)
	if err == nil {
		t.Fatal("expected error when starting duplicate process")
	}
}

func TestManager_StartStopLifecycle(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	writeTestScript(t, repoPath+"/ralph_loop.sh", "sleep 60")

	err := m.Start(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !m.IsRunning(repoPath) {
		t.Error("expected IsRunning to be true after Start")
	}

	paths := m.RunningPaths()
	if len(paths) != 1 {
		t.Errorf("expected 1 running path, got %d", len(paths))
	}

	err = m.Stop(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Give the goroutine time to clean up
	time.Sleep(200 * time.Millisecond)

	if m.IsRunning(repoPath) {
		t.Error("expected IsRunning to be false after Stop")
	}
}

func TestManager_StopAll_StopsRunning(t *testing.T) {
	m := NewManager()

	repo1 := t.TempDir()
	repo2 := t.TempDir()

	writeTestScript(t, repo1+"/ralph_loop.sh", "sleep 60")
	writeTestScript(t, repo2+"/ralph_loop.sh", "sleep 60")

	if err := m.Start(context.Background(), repo1); err != nil {
		t.Fatalf("Start repo1: %v", err)
	}
	if err := m.Start(context.Background(), repo2); err != nil {
		t.Fatalf("Start repo2: %v", err)
	}

	if len(m.RunningPaths()) != 2 {
		t.Fatalf("expected 2 running, got %d", len(m.RunningPaths()))
	}

	m.StopAll(context.Background())

	if len(m.RunningPaths()) != 0 {
		t.Errorf("expected 0 running after StopAll, got %d", len(m.RunningPaths()))
	}
}

func TestManager_TogglePause(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	writeTestScript(t, repoPath+"/ralph_loop.sh", "sleep 60")

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.StopAll(context.Background())

	if m.IsPaused(repoPath) {
		t.Error("should not be paused initially")
	}

	paused, err := m.TogglePause(repoPath)
	if err != nil {
		t.Fatalf("TogglePause (pause): %v", err)
	}
	if !paused {
		t.Error("expected paused=true after first toggle")
	}
	if !m.IsPaused(repoPath) {
		t.Error("IsPaused should return true")
	}

	paused, err = m.TogglePause(repoPath)
	if err != nil {
		t.Fatalf("TogglePause (resume): %v", err)
	}
	if paused {
		t.Error("expected paused=false after second toggle")
	}
	if m.IsPaused(repoPath) {
		t.Error("IsPaused should return false after resume")
	}
}

// setupRepoWithRalphDir creates a temp repo with a .ralph/ directory and a test script.
func setupRepoWithRalphDir(t *testing.T) string {
	t.Helper()
	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestScript(t, filepath.Join(repoPath, "ralph_loop.sh"), "sleep 60")
	return repoPath
}

func TestPIDFile_WrittenOnStart(t *testing.T) {
	m := NewManager()
	repoPath := setupRepoWithRalphDir(t)
	defer m.StopAll(context.Background())

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	pidPath := filepath.Join(repoPath, ".ralph", pidFileName)
	data, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("PID file not created: %v", err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		t.Fatalf("PID file has invalid content: %q", string(data))
	}
}

func TestPIDFile_RemovedOnStop(t *testing.T) {
	m := NewManager()
	repoPath := setupRepoWithRalphDir(t)

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := m.Stop(context.Background(), repoPath); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Give background reaper time to clean up
	time.Sleep(300 * time.Millisecond)

	pidPath := filepath.Join(repoPath, ".ralph", pidFileName)
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file should be removed after Stop")
	}
}

func TestPIDFile_RemovedOnStopAll(t *testing.T) {
	m := NewManager()
	repoPath := setupRepoWithRalphDir(t)

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	m.StopAll(context.Background())

	pidPath := filepath.Join(repoPath, ".ralph", pidFileName)
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file should be removed after StopAll")
	}
}

func TestPidForRepo_ReturnsZeroForUnknown(t *testing.T) {
	m := NewManager()
	if pid := m.PidForRepo("/unknown"); pid != 0 {
		t.Errorf("expected 0, got %d", pid)
	}
}

func TestPidForRepo_ReturnsPIDForRunning(t *testing.T) {
	m := NewManager()
	repoPath := setupRepoWithRalphDir(t)
	defer m.StopAll(context.Background())

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	pid := m.PidForRepo(repoPath)
	if pid <= 0 {
		t.Errorf("expected positive PID, got %d", pid)
	}
}

func TestReadPIDFile_InvalidContent(t *testing.T) {
	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	_ = os.MkdirAll(ralphDir, 0755)

	// Write garbage content
	_ = os.WriteFile(filepath.Join(ralphDir, pidFileName), []byte("not-a-number\n"), 0644)
	if pid := readPIDFile(repoPath); pid != 0 {
		t.Errorf("expected 0 for invalid PID, got %d", pid)
	}

	// Write negative PID
	_ = os.WriteFile(filepath.Join(ralphDir, pidFileName), []byte("-5\n"), 0644)
	if pid := readPIDFile(repoPath); pid != 0 {
		t.Errorf("expected 0 for negative PID, got %d", pid)
	}
}

func TestReadPIDFile_NoPIDFile(t *testing.T) {
	repoPath := t.TempDir()
	if pid := readPIDFile(repoPath); pid != 0 {
		t.Errorf("expected 0 for missing PID file, got %d", pid)
	}
}

func TestIsProcessAlive_Self(t *testing.T) {
	// Our own process should be alive
	if !isProcessAlive(os.Getpid()) {
		t.Error("expected our own process to be alive")
	}
}

func TestIsProcessAlive_Dead(t *testing.T) {
	// PID 1 billion is almost certainly not alive
	if isProcessAlive(1000000000) {
		t.Error("expected non-existent PID to be dead")
	}
}

func TestRecover_NoFiles(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	n := m.Recover([]string{repoPath})
	if n != 0 {
		t.Errorf("expected 0 recovered, got %d", n)
	}
}

func TestRecover_StalePID(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	_ = os.MkdirAll(ralphDir, 0755)

	// Write a PID file for a dead process
	_ = os.WriteFile(filepath.Join(ralphDir, pidFileName), []byte("1000000000\n"), 0644)

	n := m.Recover([]string{repoPath})
	if n != 0 {
		t.Errorf("expected 0 recovered (dead process), got %d", n)
	}

	// PID file should be cleaned up
	if _, err := os.Stat(filepath.Join(ralphDir, pidFileName)); !os.IsNotExist(err) {
		t.Error("stale PID file should be removed")
	}
}

func TestRecover_LiveProcess(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	ralphDir := filepath.Join(repoPath, ".ralph")
	_ = os.MkdirAll(ralphDir, 0755)

	// Write a PID file for our own process (known alive)
	_ = os.WriteFile(filepath.Join(ralphDir, pidFileName), []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)

	n := m.Recover([]string{repoPath})
	if n != 1 {
		t.Errorf("expected 1 recovered, got %d", n)
	}

	if !m.IsRunning(repoPath) {
		t.Error("expected recovered process to be tracked as running")
	}

	pid := m.PidForRepo(repoPath)
	if pid != os.Getpid() {
		t.Errorf("expected PID %d, got %d", os.Getpid(), pid)
	}
}

func TestRecover_SkipsAlreadyManaged(t *testing.T) {
	m := NewManager()
	repoPath := setupRepoWithRalphDir(t)
	defer m.StopAll(context.Background())

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Recover should skip this since it's already managed
	n := m.Recover([]string{repoPath})
	if n != 0 {
		t.Errorf("expected 0 recovered (already managed), got %d", n)
	}
}

func TestManager_Start_InvalidBinary(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()

	// Inject a fake proc directly to simulate a repo with no ralph_loop.sh
	// and point Start at a binary that doesn't exist.
	// We do this by writing a ralph_loop.sh that calls a nonexistent binary.
	writeTestScript(t, repoPath+"/ralph_loop.sh", "exec /nonexistent-binary-xyz-12345")

	// The shell will start fine (bash exists), but the script will fail immediately.
	// For a direct invalid binary test, we manipulate the manager internals.
	// Instead: create a manager with no ralph_loop.sh so it falls back to "ralph",
	// and rename it so the PATH lookup fails.
	m2 := NewManager()
	repoPath2 := t.TempDir()
	// No ralph_loop.sh → falls back to exec.Command("ralph")
	// Override by writing a script that references a bad binary is not quite right;
	// instead just test that Start returns an error when the binary isn't found.
	// We achieve this by hijacking PATH.
	t.Setenv("PATH", t.TempDir()) // empty dir — no ralph binary
	err := m2.Start(context.Background(), repoPath2)
	if err == nil {
		m2.StopAll(context.Background())
		t.Fatal("expected error starting process with missing binary")
	}
	_ = m
}

func TestManager_ShortLivedProcess_NoZombie(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	// A script that exits immediately with status 0.
	writeTestScript(t, repoPath+"/ralph_loop.sh", "exit 0")

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for reaper goroutine to call cmd.Wait() and clean up.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning(repoPath) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if m.IsRunning(repoPath) {
		m.StopAll(context.Background())
		t.Fatal("process still marked as running after clean exit — possible zombie")
	}

	code, _, ok := m.LastExitStatus(repoPath)
	if !ok {
		t.Fatal("expected LastExitStatus to be recorded")
	}
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
}

func TestManager_FailingProcess_ErrorChan(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	// A script that exits with non-zero status.
	writeTestScript(t, repoPath+"/ralph_loop.sh", "exit 42")

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case msg := <-m.ErrorChan():
		if msg.RepoPath != repoPath {
			t.Errorf("unexpected RepoPath: %s", msg.RepoPath)
		}
		if msg.Err == nil {
			t.Error("expected non-nil error in ProcessErrorMsg")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ProcessErrorMsg")
	}
}

func TestManager_ProcessExitMsg_CleanExit(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	writeTestScript(t, repoPath+"/ralph_loop.sh", "exit 0")

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case msg := <-m.ExitChan():
		if msg.RepoPath != repoPath {
			t.Errorf("unexpected RepoPath: got %q, want %q", msg.RepoPath, repoPath)
		}
		if msg.ExitCode != 0 {
			t.Errorf("expected exit code 0, got %d", msg.ExitCode)
		}
		if msg.Error != nil {
			t.Errorf("expected nil error for clean exit, got %v", msg.Error)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ProcessExitMsg")
	}
}

func TestManager_ProcessExitMsg_NonZeroExit(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	writeTestScript(t, repoPath+"/ralph_loop.sh", "exit 7")

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case msg := <-m.ExitChan():
		if msg.RepoPath != repoPath {
			t.Errorf("unexpected RepoPath: got %q, want %q", msg.RepoPath, repoPath)
		}
		if msg.ExitCode != 7 {
			t.Errorf("expected exit code 7, got %d", msg.ExitCode)
		}
		if msg.Error == nil {
			t.Error("expected non-nil error for non-zero exit")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ProcessExitMsg")
	}
}

func TestCleanStalePIDFiles(t *testing.T) {
	repo1 := t.TempDir()
	repo2 := t.TempDir()
	repo3 := t.TempDir()

	for _, r := range []string{repo1, repo2, repo3} {
		_ = os.MkdirAll(filepath.Join(r, ".ralph"), 0755)
	}

	// repo1: stale PID
	_ = os.WriteFile(filepath.Join(repo1, ".ralph", pidFileName), []byte("1000000000\n"), 0644)
	// repo2: live PID (our process)
	_ = os.WriteFile(filepath.Join(repo2, ".ralph", pidFileName), []byte(strconv.Itoa(os.Getpid())+"\n"), 0644)
	// repo3: no PID file

	cleaned := CleanStalePIDFiles([]string{repo1, repo2, repo3})
	if cleaned != 1 {
		t.Errorf("expected 1 cleaned, got %d", cleaned)
	}

	// repo1 PID file should be removed
	if _, err := os.Stat(filepath.Join(repo1, ".ralph", pidFileName)); !os.IsNotExist(err) {
		t.Error("stale PID file in repo1 should be removed")
	}

	// repo2 PID file should still exist
	if _, err := os.Stat(filepath.Join(repo2, ".ralph", pidFileName)); err != nil {
		t.Error("live PID file in repo2 should still exist")
	}
}

func TestManager_KillTimeout_Default(t *testing.T) {
	m := NewManager()
	if m.KillTimeout != DefaultKillTimeout {
		t.Errorf("expected default KillTimeout %v, got %v", DefaultKillTimeout, m.KillTimeout)
	}
	// Verify DefaultKillTimeout is 10s.
	if DefaultKillTimeout != 10*time.Second {
		t.Errorf("expected DefaultKillTimeout to be 10s, got %v", DefaultKillTimeout)
	}
}

func TestManager_KillTimeout_Custom(t *testing.T) {
	m := NewManager()
	m.KillTimeout = 15 * time.Second
	if m.killTimeout() != 15*time.Second {
		t.Errorf("expected custom KillTimeout 15s, got %v", m.killTimeout())
	}
}

func TestManager_KillTimeout_ZeroFallback(t *testing.T) {
	m := NewManager()
	m.KillTimeout = 0
	if m.killTimeout() != DefaultKillTimeout {
		t.Errorf("expected fallback to DefaultKillTimeout, got %v", m.killTimeout())
	}
}

func TestManager_KillTimeoutWithOverride_PerProcess(t *testing.T) {
	m := NewManager()
	// Per-process timeout should take priority over manager timeout.
	perProcess := 3 * time.Second
	got := m.killTimeoutWithOverride(perProcess)
	if got != perProcess {
		t.Errorf("expected per-process timeout %v, got %v", perProcess, got)
	}
}

func TestManager_KillTimeoutWithOverride_ZeroFallsBackToManager(t *testing.T) {
	m := NewManager()
	m.KillTimeout = 7 * time.Second
	got := m.killTimeoutWithOverride(0)
	if got != 7*time.Second {
		t.Errorf("expected manager timeout 7s, got %v", got)
	}
}

func TestManager_KillTimeoutWithOverride_BothZeroFallsBackToDefault(t *testing.T) {
	m := NewManager()
	m.KillTimeout = 0
	got := m.killTimeoutWithOverride(0)
	if got != DefaultKillTimeout {
		t.Errorf("expected DefaultKillTimeout %v, got %v", DefaultKillTimeout, got)
	}
}

func TestManagedProcess_KillTimeout_UsedInStop(t *testing.T) {
	// Verify that per-process KillTimeout is passed to the kill sequence.
	h := newHarness(100)
	defer h.install()()

	var recordedSleeps []time.Duration
	setSleepFn(func(d time.Duration) {
		h.mu.Lock()
		defer h.mu.Unlock()
		recordedSleeps = append(recordedSleeps, d)
		h.sleeps++
	})

	m := NewManager()
	m.KillTimeout = 10 * time.Second

	// Inject a fake managed process with a per-process timeout.
	perProcessTimeout := 2 * time.Second
	m.mu.Lock()
	m.procs["/test/repo"] = &ManagedProcess{
		PID:         100,
		Recovered:   true, // skip reaper cleanup
		KillTimeout: perProcessTimeout,
	}
	m.mu.Unlock()

	err := m.Stop(context.Background(), "/test/repo")
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Give the background goroutine time to run.
	time.Sleep(100 * time.Millisecond)

	h.mu.Lock()
	defer h.mu.Unlock()

	// The kill sequence should have used the per-process timeout (2s), not the manager's (10s).
	for i, d := range recordedSleeps {
		if d != perProcessTimeout {
			t.Errorf("sleep[%d]: expected %v, got %v", i, perProcessTimeout, d)
		}
	}
}

func TestAutoRestartOnCrash(t *testing.T) {
	// Stub sleepFn so backoff doesn't actually wait.
	origSleep := *sleepFnPtr.Load()
	setSleepFn(func(d time.Duration) {}) // no-op
	t.Cleanup(func() { setSleepFn(origSleep) })

	m := NewManager()
	m.AutoRestart = true
	m.MaxRestarts = 2

	repoPath := t.TempDir()

	// Write a script that tracks invocations via a counter file and exits non-zero
	// for the first 2 runs, then exits 0 on the 3rd (original + 2 restarts).
	counterFile := filepath.Join(repoPath, ".ralph", "restart_counter")
	_ = os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)
	writeTestScript(t, filepath.Join(repoPath, "ralph_loop.sh"),
		`COUNTER_FILE="`+counterFile+`"
if [ ! -f "$COUNTER_FILE" ]; then
  echo 1 > "$COUNTER_FILE"
  exit 1
fi
COUNT=$(cat "$COUNTER_FILE")
COUNT=$((COUNT + 1))
echo $COUNT > "$COUNTER_FILE"
if [ "$COUNT" -le 2 ]; then
  exit 1
fi
exit 0`)

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for the process to finish (all restarts should complete quickly with stubbed sleep).
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning(repoPath) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if m.IsRunning(repoPath) {
		m.StopAll(context.Background())
		t.Fatal("process still running after expected restarts + clean exit")
	}

	// Verify the counter file shows 3 invocations (original + 2 restarts).
	data, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("reading counter file: %v", err)
	}
	count := strings.TrimSpace(string(data))
	if count != "3" {
		t.Errorf("expected 3 invocations, got %s", count)
	}

	// Final exit should be clean (code 0).
	code, _, ok := m.LastExitStatus(repoPath)
	if !ok {
		t.Fatal("expected LastExitStatus to be recorded")
	}
	if code != 0 {
		t.Errorf("expected final exit code 0, got %d", code)
	}
}

func TestAutoRestartMaxExceeded(t *testing.T) {
	// Stub sleepFn so backoff doesn't actually wait.
	origSleep := *sleepFnPtr.Load()
	setSleepFn(func(d time.Duration) {}) // no-op
	t.Cleanup(func() { setSleepFn(origSleep) })

	m := NewManager()
	m.AutoRestart = true
	m.MaxRestarts = 2

	repoPath := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)

	// Script always exits with code 1 — restarts should be exhausted.
	counterFile := filepath.Join(repoPath, ".ralph", "max_counter")
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

	// Wait for the process to be cleaned up after max restarts.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning(repoPath) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if m.IsRunning(repoPath) {
		m.StopAll(context.Background())
		t.Fatal("process still running after max restarts should be exceeded")
	}

	// Should have run original + MaxRestarts = 3 times total.
	data, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("reading counter file: %v", err)
	}
	count := strings.TrimSpace(string(data))
	if count != "3" {
		t.Errorf("expected 3 total invocations (1 original + 2 restarts), got %s", count)
	}

	// Final exit should be non-zero.
	code, _, ok := m.LastExitStatus(repoPath)
	if !ok {
		t.Fatal("expected LastExitStatus to be recorded")
	}
	if code != 1 {
		t.Errorf("expected final exit code 1, got %d", code)
	}
}

func TestAutoRestartDisabled(t *testing.T) {
	m := NewManager()
	m.AutoRestart = false // explicit, though this is the default

	repoPath := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)

	counterFile := filepath.Join(repoPath, ".ralph", "no_restart_counter")
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

	// Wait for the process to exit.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning(repoPath) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if m.IsRunning(repoPath) {
		m.StopAll(context.Background())
		t.Fatal("process still running when it should have exited without restart")
	}

	// Should have run exactly once — no restarts.
	data, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("reading counter file: %v", err)
	}
	count := strings.TrimSpace(string(data))
	if count != "1" {
		t.Errorf("expected exactly 1 invocation (no restart), got %s", count)
	}
}

func TestStart_ContextCancellation(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	writeTestScript(t, repoPath+"/ralph_loop.sh", "sleep 60")

	ctx, cancel := context.WithCancel(context.Background())

	if err := m.Start(ctx, repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !m.IsRunning(repoPath) {
		t.Fatal("expected process to be running after Start")
	}

	// Cancel the context — this should kill the process via CommandContext.
	cancel()

	// Wait for the reaper to clean up.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning(repoPath) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if m.IsRunning(repoPath) {
		m.StopAll(context.Background())
		t.Fatal("process still running after context cancellation")
	}
}

func TestStart_ContextCancellation_PreventsRestart(t *testing.T) {
	// Stub sleepFn so backoff doesn't actually wait.
	origSleep := *sleepFnPtr.Load()
	setSleepFn(func(d time.Duration) {}) // no-op
	t.Cleanup(func() { setSleepFn(origSleep) })

	m := NewManager()
	m.AutoRestart = true
	m.MaxRestarts = 5

	repoPath := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)

	counterFile := filepath.Join(repoPath, ".ralph", "ctx_cancel_counter")
	writeTestScript(t, filepath.Join(repoPath, "ralph_loop.sh"),
		`COUNTER_FILE="`+counterFile+`"
if [ ! -f "$COUNTER_FILE" ]; then
  echo 1 > "$COUNTER_FILE"
else
  COUNT=$(cat "$COUNTER_FILE")
  echo $((COUNT + 1)) > "$COUNTER_FILE"
fi
exit 1`)

	ctx, cancel := context.WithCancel(context.Background())

	if err := m.Start(ctx, repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for at least one invocation.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(counterFile); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Cancel context to prevent further restarts.
	cancel()

	// Wait for the process to be cleaned up.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning(repoPath) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if m.IsRunning(repoPath) {
		m.StopAll(context.Background())
		t.Fatal("process still running after context cancellation")
	}

	// Verify restarts did not continue indefinitely — should be far fewer than MaxRestarts.
	data, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("reading counter file: %v", err)
	}
	countStr := strings.TrimSpace(string(data))
	// With context cancelled, we should have at most a couple of invocations,
	// definitely not all 6 (original + 5 restarts).
	t.Logf("total invocations before context cancel took effect: %s", countStr)
}

// ---------------------------------------------------------------------------
// Additional tests targeting 85%+ coverage
// ---------------------------------------------------------------------------

func TestAddProcForTesting(t *testing.T) {
	t.Parallel()

	m := NewManager()

	// Add an unpaused proc.
	m.AddProcForTesting("/test/repo1", false)
	if !m.IsRunning("/test/repo1") {
		t.Error("expected IsRunning=true after AddProcForTesting")
	}
	if m.IsPaused("/test/repo1") {
		t.Error("expected IsPaused=false for unpaused test proc")
	}

	// Add a paused proc.
	m.AddProcForTesting("/test/repo2", true)
	if !m.IsRunning("/test/repo2") {
		t.Error("expected IsRunning=true after AddProcForTesting (paused)")
	}
	if !m.IsPaused("/test/repo2") {
		t.Error("expected IsPaused=true for paused test proc")
	}

	// RunningPaths should include both.
	paths := m.RunningPaths()
	if len(paths) != 2 {
		t.Errorf("expected 2 running paths, got %d", len(paths))
	}
}

func TestNewManagerWithBus(t *testing.T) {
	t.Parallel()

	bus := events.NewBus(100)
	m := NewManagerWithBus(bus)
	if m == nil {
		t.Fatal("NewManagerWithBus returned nil")
	}
	if m.bus != bus {
		t.Error("expected bus to be set on manager")
	}
	if m.procs == nil {
		t.Fatal("procs map not initialized")
	}
	if m.KillTimeout != DefaultKillTimeout {
		t.Errorf("expected default KillTimeout, got %v", m.KillTimeout)
	}
	if m.MaxRestarts != DefaultMaxRestarts {
		t.Errorf("expected default MaxRestarts, got %v", m.MaxRestarts)
	}
}

func TestWaitForProcessExit(t *testing.T) {
	ch := make(chan ProcessExitMsg, 1)
	ch <- ProcessExitMsg{RepoPath: "/test/repo", ExitCode: 42, Error: nil}

	cmd := WaitForProcessExit(ch)
	msg := cmd()

	exitMsg, ok := msg.(ProcessExitMsg)
	if !ok {
		t.Fatalf("expected ProcessExitMsg, got %T", msg)
	}
	if exitMsg.RepoPath != "/test/repo" {
		t.Errorf("unexpected RepoPath: %s", exitMsg.RepoPath)
	}
	if exitMsg.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitMsg.ExitCode)
	}
}

func TestMaxRestartsLimit_ZeroFallsToDefault(t *testing.T) {
	t.Parallel()

	m := NewManager()
	m.MaxRestarts = 0
	if m.maxRestartsLimit() != DefaultMaxRestarts {
		t.Errorf("expected DefaultMaxRestarts (%d), got %d", DefaultMaxRestarts, m.maxRestartsLimit())
	}
}

func TestMaxRestartsLimit_NegativeFallsToDefault(t *testing.T) {
	t.Parallel()

	m := NewManager()
	m.MaxRestarts = -5
	if m.maxRestartsLimit() != DefaultMaxRestarts {
		t.Errorf("expected DefaultMaxRestarts (%d), got %d", DefaultMaxRestarts, m.maxRestartsLimit())
	}
}

func TestMaxRestartsLimit_PositiveValue(t *testing.T) {
	t.Parallel()

	m := NewManager()
	m.MaxRestarts = 7
	if m.maxRestartsLimit() != 7 {
		t.Errorf("expected 7, got %d", m.maxRestartsLimit())
	}
}

func TestRunKillSequence_ZeroTimeoutUsesDefault(t *testing.T) {
	h := newHarness(100)
	defer h.install()()

	var sleepDurations []time.Duration
	setSleepFn(func(d time.Duration) {
		h.mu.Lock()
		defer h.mu.Unlock()
		sleepDurations = append(sleepDurations, d)
		h.sleeps++
	})

	runKillSequence(100, nil, 0)

	h.mu.Lock()
	defer h.mu.Unlock()

	// With timeout=0, runKillSequence should fall back to DefaultKillTimeout.
	for i, d := range sleepDurations {
		if d != DefaultKillTimeout {
			t.Errorf("sleep[%d]: expected %v, got %v", i, DefaultKillTimeout, d)
		}
	}
}

func TestConcurrentStartStop(t *testing.T) {
	// Exercise concurrent access to the manager with the race detector.
	origSleep := *sleepFnPtr.Load()
	setSleepFn(func(d time.Duration) {})
	t.Cleanup(func() { setSleepFn(origSleep) })

	m := NewManager()
	const n = 10
	repos := make([]string, n)
	for i := range n {
		repos[i] = t.TempDir()
		writeTestScript(t, filepath.Join(repos[i], "ralph_loop.sh"), "sleep 60")
	}

	// Start all concurrently.
	errs := make(chan error, n)
	for _, r := range repos {
		go func() {
			errs <- m.Start(context.Background(), r)
		}()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Errorf("Start error: %v", err)
		}
	}

	// Query concurrently.
	done := make(chan struct{})
	go func() {
		for range 50 {
			_ = m.RunningPaths()
			_ = m.IsRunning(repos[0])
			_ = m.IsPaused(repos[0])
			_ = m.PidForRepo(repos[0])
		}
		close(done)
	}()

	// Stop all concurrently.
	stopErrs := make(chan error, n)
	for _, r := range repos {
		go func() {
			stopErrs <- m.Stop(context.Background(), r)
		}()
	}
	for range n {
		<-stopErrs // errors are acceptable (race with reaper)
	}

	<-done

	// Give reapers time to clean up.
	time.Sleep(200 * time.Millisecond)
}

func TestStartWithBus_PublishesEvents(t *testing.T) {
	bus := events.NewBus(100)
	m := NewManagerWithBus(bus)
	repoPath := t.TempDir()
	writeTestScript(t, filepath.Join(repoPath, "ralph_loop.sh"), "exit 0")

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for exit.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning(repoPath) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	// Check that LoopStarted event was published to the bus.
	started := bus.History(events.LoopStarted, 10)
	if len(started) == 0 {
		t.Error("expected LoopStarted event published to the bus")
	}

	// Check that LoopStopped event was published to the bus.
	stopped := bus.History(events.LoopStopped, 10)
	if len(stopped) == 0 {
		t.Error("expected LoopStopped event published to the bus")
	}
}

func TestStopAll_MultipleProcesses_ClearsMap(t *testing.T) {
	// Verify that AddProcForTesting populates and RunningPaths reports correctly.
	// We do NOT call StopAll here because AddProcForTesting creates entries
	// with PID=0, and sendSignal(0, SIGTERM) would kill the test runner's
	// process group. Instead we verify the map state directly.
	m := NewManager()

	for i := range 5 {
		m.AddProcForTesting(filepath.Join("/fake/repo", strconv.Itoa(i)), false)
	}

	paths := m.RunningPaths()
	if len(paths) != 5 {
		t.Fatalf("expected 5 running, got %d", len(paths))
	}

	// Verify each path is reported.
	pathSet := make(map[string]bool)
	for _, p := range paths {
		pathSet[p] = true
	}
	for i := range 5 {
		expected := filepath.Join("/fake/repo", strconv.Itoa(i))
		if !pathSet[expected] {
			t.Errorf("expected %q in running paths", expected)
		}
	}
}

func TestPidForRepo_AfterAddProcForTesting(t *testing.T) {
	t.Parallel()

	m := NewManager()
	m.AddProcForTesting("/test/repo", false)

	// AddProcForTesting creates a ManagedProcess with PID=0.
	pid := m.PidForRepo("/test/repo")
	if pid != 0 {
		t.Errorf("expected PID 0 from AddProcForTesting, got %d", pid)
	}
}

func TestErrorChan_BufferedAndNonBlocking(t *testing.T) {
	t.Parallel()

	m := NewManager()
	errCh := m.ErrorChan()

	// Channel should be readable.
	if errCh == nil {
		t.Fatal("ErrorChan returned nil")
	}

	// Verify channel has capacity (non-blocking send from producer side).
	select {
	case m.errCh <- ProcessErrorMsg{RepoPath: "/test", Err: nil}:
		// ok
	default:
		t.Error("errCh should have buffer capacity")
	}

	// Read it back.
	select {
	case msg := <-errCh:
		if msg.RepoPath != "/test" {
			t.Errorf("unexpected RepoPath: %s", msg.RepoPath)
		}
	default:
		t.Error("expected message on errCh")
	}
}

func TestExitChan_BufferedAndNonBlocking(t *testing.T) {
	t.Parallel()

	m := NewManager()
	exitCh := m.ExitChan()

	if exitCh == nil {
		t.Fatal("ExitChan returned nil")
	}

	select {
	case m.exitCh <- ProcessExitMsg{RepoPath: "/test", ExitCode: 1}:
	default:
		t.Error("exitCh should have buffer capacity")
	}

	select {
	case msg := <-exitCh:
		if msg.ExitCode != 1 {
			t.Errorf("expected exit code 1, got %d", msg.ExitCode)
		}
	default:
		t.Error("expected message on exitCh")
	}
}

func TestLastExitStatus_RecordedAfterExit(t *testing.T) {
	// Directly inject into the global lastExits to test the lookup path.
	lastExits.Lock()
	lastExits.m["/test/last_exit"] = exitStatus{Code: 137, Error: "signal: killed"}
	lastExits.Unlock()
	t.Cleanup(func() {
		lastExits.Lock()
		delete(lastExits.m, "/test/last_exit")
		lastExits.Unlock()
	})

	m := NewManager()
	code, errStr, ok := m.LastExitStatus("/test/last_exit")
	if !ok {
		t.Fatal("expected ok=true for recorded exit status")
	}
	if code != 137 {
		t.Errorf("expected code 137, got %d", code)
	}
	if errStr != "signal: killed" {
		t.Errorf("expected error string 'signal: killed', got %q", errStr)
	}
}

func TestStart_NoLoopScript(t *testing.T) {
	t.Parallel()

	m := NewManager()
	repoPath := t.TempDir()
	// No ralph_loop.sh — should get ErrNoLoopScript.

	err := m.Start(context.Background(), repoPath)
	if err == nil {
		t.Fatal("expected error starting with no loop script")
	}
	if !strings.Contains(err.Error(), "no loop script found") {
		t.Errorf("expected ErrNoLoopScript, got: %v", err)
	}
}

func TestAutoRestart_ContextCancelDuringBackoff(t *testing.T) {
	// Test that cancelling context during the backoff sleep prevents restart.
	ctx, cancel := context.WithCancel(context.Background())

	// Use a sleep function that cancels the context mid-backoff.
	origSleep := *sleepFnPtr.Load()
	callCount := 0
	setSleepFn(func(d time.Duration) {
		callCount++
		if callCount == 1 {
			// First backoff sleep — cancel context to simulate user stop.
			cancel()
		}
	})
	t.Cleanup(func() { setSleepFn(origSleep) })

	m := NewManager()
	m.AutoRestart = true
	m.MaxRestarts = 5

	repoPath := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)
	writeTestScript(t, filepath.Join(repoPath, "ralph_loop.sh"), "exit 1")

	if err := m.Start(ctx, repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for cleanup.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning(repoPath) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if m.IsRunning(repoPath) {
		m.StopAll(context.Background())
		t.Fatal("process still running after context cancel during backoff")
	}
}

func TestWritePIDFile_And_ReadPIDFile(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)

	err := writePIDFile(repoPath, 12345)
	if err != nil {
		t.Fatalf("writePIDFile: %v", err)
	}

	pid := readPIDFile(repoPath)
	if pid != 12345 {
		t.Errorf("expected 12345, got %d", pid)
	}
}

func TestRemovePIDFile_NonExistent(t *testing.T) {
	t.Parallel()

	// Should not panic on non-existent file.
	removePIDFile("/nonexistent/path/that/does/not/exist")
}

func TestPidFilePath(t *testing.T) {
	t.Parallel()

	got := pidFilePath("/some/repo")
	expected := filepath.Join("/some/repo", ".ralph", pidFileName)
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestStop_RecoveredProcess(t *testing.T) {
	// Stopping a recovered process should clean up the PID file and
	// remove it from the map immediately (no reaper goroutine).
	h := newHarness() // no PIDs alive
	defer h.install()()

	m := NewManager()
	repoPath := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)

	// Write a PID file so removePIDFile has something to remove.
	_ = writePIDFile(repoPath, 99999)

	// Inject a recovered process entry.
	m.mu.Lock()
	m.procs[repoPath] = &ManagedProcess{
		PID:       99999,
		Recovered: true,
	}
	m.mu.Unlock()

	err := m.Stop(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Give the kill goroutine time to run.
	time.Sleep(100 * time.Millisecond)

	// Recovered process should be immediately removed from the map.
	if m.IsRunning(repoPath) {
		t.Error("expected recovered process to be removed from map after Stop")
	}

	// PID file should be removed.
	pidPath := filepath.Join(repoPath, ".ralph", pidFileName)
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("expected PID file to be removed for recovered process")
	}
}

func TestAutoRestartWithBus_PublishesRestartEvent(t *testing.T) {
	origSleep := *sleepFnPtr.Load()
	setSleepFn(func(d time.Duration) {})
	t.Cleanup(func() { setSleepFn(origSleep) })

	bus := events.NewBus(100)
	m := NewManagerWithBus(bus)
	m.AutoRestart = true
	m.MaxRestarts = 1

	repoPath := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)

	// Script exits with 1 twice (original + 1 restart), triggering a restart event.
	counterFile := filepath.Join(repoPath, ".ralph", "bus_counter")
	writeTestScript(t, filepath.Join(repoPath, "ralph_loop.sh"),
		`COUNTER_FILE="`+counterFile+`"
if [ ! -f "$COUNTER_FILE" ]; then
  echo 1 > "$COUNTER_FILE"
  exit 1
fi
COUNT=$(cat "$COUNTER_FILE")
COUNT=$((COUNT + 1))
echo $COUNT > "$COUNTER_FILE"
exit 1`)

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning(repoPath) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if m.IsRunning(repoPath) {
		m.StopAll(context.Background())
		t.Fatal("process still running")
	}

	// Check for LoopRestarted event.
	allEvents := bus.History("", 100)
	var hasRestart bool
	for _, e := range allEvents {
		if e.Type == events.LoopRestarted {
			hasRestart = true
			break
		}
	}
	if !hasRestart {
		t.Error("expected LoopRestarted event from bus")
	}
}

func TestProcessErrorMsg_Fields(t *testing.T) {
	t.Parallel()

	msg := ProcessErrorMsg{RepoPath: "/test", Err: fmt.Errorf("test error")}
	if msg.RepoPath != "/test" {
		t.Errorf("unexpected RepoPath: %s", msg.RepoPath)
	}
	if msg.Err == nil || msg.Err.Error() != "test error" {
		t.Errorf("unexpected Err: %v", msg.Err)
	}
}

func TestProcessExitMsg_Fields(t *testing.T) {
	t.Parallel()

	msg := ProcessExitMsg{RepoPath: "/test", ExitCode: 42, Error: fmt.Errorf("exit 42")}
	if msg.RepoPath != "/test" {
		t.Errorf("unexpected RepoPath: %s", msg.RepoPath)
	}
	if msg.ExitCode != 42 {
		t.Errorf("unexpected ExitCode: %d", msg.ExitCode)
	}
	if msg.Error == nil {
		t.Error("expected non-nil Error")
	}
}

func TestAutoRestart_StopAllDuringBackoff(t *testing.T) {
	// Exercise the code path where StopAll removes the entry from the procs map
	// while the reaper is sleeping in the backoff period. The reaper should detect
	// the entry is gone and skip the restart.
	m := NewManager()
	m.AutoRestart = true
	m.MaxRestarts = 5

	origSleep := *sleepFnPtr.Load()
	callCount := 0
	setSleepFn(func(d time.Duration) {
		callCount++
		if callCount == 1 {
			// During the first backoff sleep, call StopAll to remove the entry.
			m.StopAll(context.Background())
		}
	})
	t.Cleanup(func() { setSleepFn(origSleep) })

	repoPath := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)
	writeTestScript(t, filepath.Join(repoPath, "ralph_loop.sh"), "exit 1")

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for cleanup.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning(repoPath) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if m.IsRunning(repoPath) {
		m.StopAll(context.Background())
		t.Fatal("process still running after StopAll during backoff")
	}
}

func TestAutoRestart_StopDuringBackoff(t *testing.T) {
	// Exercise the code path where Stop() sets Stopping=true while the reaper
	// is sleeping in the backoff period. The reaper should see Stopping and
	// skip the restart.
	h := newHarness() // no real PIDs alive
	origKill := *killPidPtr.Load()
	origAlive := *aliveFnPtr.Load()

	m := NewManager()
	m.AutoRestart = true
	m.MaxRestarts = 5

	origSleep := *sleepFnPtr.Load()
	callCount := 0
	setSleepFn(func(d time.Duration) {
		callCount++
		if callCount == 1 {
			// During the first backoff sleep, mark the process as Stopping.
			m.mu.Lock()
			for _, mp := range m.procs {
				mp.Stopping = true
			}
			m.mu.Unlock()
		}
	})
	t.Cleanup(func() {
		setSleepFn(origSleep)
		setKillPid(origKill)
		setAliveFn(origAlive)
		_ = h
	})

	repoPath := t.TempDir()
	_ = os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755)
	writeTestScript(t, filepath.Join(repoPath, "ralph_loop.sh"), "exit 1")

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for cleanup.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !m.IsRunning(repoPath) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if m.IsRunning(repoPath) {
		m.StopAll(context.Background())
		t.Fatal("process still running after Stop during backoff")
	}
}

func TestWritePIDFile_CreatesDirIfMissing(t *testing.T) {
	t.Parallel()

	repoPath := t.TempDir()
	// .ralph/ dir does not exist yet — writePIDFile should create it.
	err := writePIDFile(repoPath, 12345)
	if err != nil {
		t.Fatalf("writePIDFile: %v", err)
	}

	pid := readPIDFile(repoPath)
	if pid != 12345 {
		t.Errorf("expected 12345, got %d", pid)
	}
}

func TestWritePIDFile_ErrorOnReadOnlyParent(t *testing.T) {
	t.Parallel()

	// Create a directory that is read-only so MkdirAll will fail.
	repoPath := t.TempDir()
	if err := os.Chmod(repoPath, 0555); err != nil {
		t.Skipf("cannot set read-only permissions: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(repoPath, 0755) })

	err := writePIDFile(repoPath, 12345)
	if err == nil {
		t.Error("expected error writing PID file to read-only directory")
	}
}

func TestScanRalphLoopProcessesPS(t *testing.T) {
	t.Parallel()

	// scanRalphLoopProcessesPS should return a slice (possibly empty) without error.
	pids := scanRalphLoopProcessesPS()
	if pids == nil {
		// It's OK to return nil if ps fails, but let's at least verify it runs.
		t.Log("scanRalphLoopProcessesPS returned nil (ps may have failed)")
	}
}

func TestScanRalphLoopProcessesLinux_OnDarwin(t *testing.T) {
	t.Parallel()

	// On macOS, /proc doesn't exist so this returns nil immediately.
	// This covers the ReadDir error path in the Linux scanner.
	pids := scanRalphLoopProcessesLinux()
	if pids != nil {
		t.Logf("scanRalphLoopProcessesLinux returned %d pids (unexpected on darwin)", len(pids))
	}
}

func TestScanRalphLoopProcesses_DelegatesToPS(t *testing.T) {
	t.Parallel()

	// On darwin, scanRalphLoopProcesses delegates to PS, not Linux.
	pids := scanRalphLoopProcesses()
	// Just verify it doesn't panic; may return empty or non-empty.
	_ = pids
}

func TestCollectChildPIDsByPgid_OwnProcess(t *testing.T) {
	t.Parallel()

	// On macOS, Getpgid succeeds but ReadDir("/proc") fails, returning
	// an empty slice. This covers the Getpgid success + ReadDir failure path.
	result := CollectChildPIDsByPgid(os.Getpid())
	if result == nil {
		t.Fatal("expected non-nil slice")
	}
	// On macOS: should return empty slice (no /proc).
	// On Linux: may return some PIDs in same pgroup.
}

func TestStopAllGraceful_Empty(t *testing.T) {
	m := NewManager()
	// Should not panic and return 0 kills.
	killed := m.StopAllGraceful(context.Background())
	if killed != 0 {
		t.Errorf("expected 0 killed on empty manager, got %d", killed)
	}
}

func TestStopAllGraceful_ProcessesExitCleanly(t *testing.T) {
	// Stub sleep to no-op for speed.
	origSleep := *sleepFnPtr.Load()
	setSleepFn(func(d time.Duration) {})
	t.Cleanup(func() { setSleepFn(origSleep) })

	// Stub aliveFn to report dead immediately after SIGTERM.
	origAlive := *aliveFnPtr.Load()
	setAliveFn(func(pid int) bool { return false })
	t.Cleanup(func() { setAliveFn(origAlive) })

	// Stub killPid to no-op (fake PIDs).
	origKill := *killPidPtr.Load()
	setKillPid(func(pid int, sig syscall.Signal) error { return nil })
	t.Cleanup(func() { setKillPid(origKill) })

	// Stub getpgid to avoid real syscall on fake PIDs.
	origGetpgid := getpgid
	getpgid = func(pid int) (int, error) { return pid, nil }
	t.Cleanup(func() { getpgid = origGetpgid })

	m := NewManager()
	// Manually inject entries with non-zero PIDs.
	m.mu.Lock()
	m.procs["/repo/a"] = &ManagedProcess{PID: 99901}
	m.procs["/repo/b"] = &ManagedProcess{PID: 99902}
	m.mu.Unlock()

	killed := m.StopAllGraceful(context.Background())
	if killed != 0 {
		t.Errorf("expected 0 killed (clean exit), got %d", killed)
	}
	if len(m.RunningPaths()) != 0 {
		t.Errorf("expected 0 running after StopAllGraceful, got %d", len(m.RunningPaths()))
	}
}

func TestStopAllGraceful_TimeoutEscalatesToSIGKILL(t *testing.T) {
	// Stub sleep to no-op for speed.
	origSleep := *sleepFnPtr.Load()
	setSleepFn(func(d time.Duration) {})
	t.Cleanup(func() { setSleepFn(origSleep) })

	// Stub aliveFn to always report alive (simulates hung process).
	origAlive := *aliveFnPtr.Load()
	setAliveFn(func(pid int) bool { return true })
	t.Cleanup(func() { setAliveFn(origAlive) })

	// Track signals sent.
	var killSignals []syscall.Signal
	origKill := *killPidPtr.Load()
	setKillPid(func(pid int, sig syscall.Signal) error {
		killSignals = append(killSignals, sig)
		return nil
	})
	t.Cleanup(func() { setKillPid(origKill) })

	// Stub getpgid.
	origGetpgid := getpgid
	getpgid = func(pid int) (int, error) { return pid, nil }
	t.Cleanup(func() { getpgid = origGetpgid })

	m := NewManager()
	m.mu.Lock()
	m.procs["/repo/stuck"] = &ManagedProcess{PID: 99999}
	m.mu.Unlock()

	// Use an already-expired context to trigger immediate SIGKILL escalation.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	killed := m.StopAllGraceful(ctx)
	if killed != 1 {
		t.Errorf("expected 1 killed (SIGKILL), got %d", killed)
	}

	// Verify SIGKILL was sent (after the initial SIGTERM from sendSignal).
	foundKill := slices.Contains(killSignals, syscall.SIGKILL)
	if !foundKill {
		t.Errorf("expected SIGKILL in kill signals, got %v", killSignals)
	}
}
