package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
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
	m.StopAll()
}

func TestManager_Stop_NotRunning(t *testing.T) {
	m := NewManager()
	err := m.Stop("/not/running")
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

	err := m.Start(repoPath)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.StopAll()

	err = m.Start(repoPath)
	if err == nil {
		t.Fatal("expected error when starting duplicate process")
	}
}

func TestManager_StartStopLifecycle(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	writeTestScript(t, repoPath+"/ralph_loop.sh", "sleep 60")

	err := m.Start(repoPath)
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

	err = m.Stop(repoPath)
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

	if err := m.Start(repo1); err != nil {
		t.Fatalf("Start repo1: %v", err)
	}
	if err := m.Start(repo2); err != nil {
		t.Fatalf("Start repo2: %v", err)
	}

	if len(m.RunningPaths()) != 2 {
		t.Fatalf("expected 2 running, got %d", len(m.RunningPaths()))
	}

	m.StopAll()

	if len(m.RunningPaths()) != 0 {
		t.Errorf("expected 0 running after StopAll, got %d", len(m.RunningPaths()))
	}
}

func TestManager_TogglePause(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	writeTestScript(t, repoPath+"/ralph_loop.sh", "sleep 60")

	if err := m.Start(repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.StopAll()

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
	defer m.StopAll()

	if err := m.Start(repoPath); err != nil {
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

	if err := m.Start(repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := m.Stop(repoPath); err != nil {
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

	if err := m.Start(repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	m.StopAll()

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
	defer m.StopAll()

	if err := m.Start(repoPath); err != nil {
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
	defer m.StopAll()

	if err := m.Start(repoPath); err != nil {
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
	err := m2.Start(repoPath2)
	if err == nil {
		m2.StopAll()
		t.Fatal("expected error starting process with missing binary")
	}
	_ = m
}

func TestManager_ShortLivedProcess_NoZombie(t *testing.T) {
	m := NewManager()
	repoPath := t.TempDir()
	// A script that exits immediately with status 0.
	writeTestScript(t, repoPath+"/ralph_loop.sh", "exit 0")

	if err := m.Start(repoPath); err != nil {
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
		m.StopAll()
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

	if err := m.Start(repoPath); err != nil {
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

	if err := m.Start(repoPath); err != nil {
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

	if err := m.Start(repoPath); err != nil {
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
}

func TestManager_KillTimeout_Custom(t *testing.T) {
	m := NewManager()
	m.KillTimeout = 10 * time.Second
	if m.killTimeout() != 10*time.Second {
		t.Errorf("expected custom KillTimeout 10s, got %v", m.killTimeout())
	}
}

func TestManager_KillTimeout_ZeroFallback(t *testing.T) {
	m := NewManager()
	m.KillTimeout = 0
	if m.killTimeout() != DefaultKillTimeout {
		t.Errorf("expected fallback to DefaultKillTimeout, got %v", m.killTimeout())
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

	if err := m.Start(repoPath); err != nil {
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
		m.StopAll()
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

	if err := m.Start(repoPath); err != nil {
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
		m.StopAll()
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

	if err := m.Start(repoPath); err != nil {
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
		m.StopAll()
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
