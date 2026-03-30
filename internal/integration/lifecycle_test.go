//go:build integration

package integration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
)

// TestLifecycle_ScanStartStatusStop exercises the full lifecycle:
// scan a repo directory, start a loop, verify running, poll status, stop, verify stopped.
func TestLifecycle_ScanStartStatusStop(t *testing.T) {
	// Create a parent directory containing one "repo" subdirectory.
	parentDir := t.TempDir()
	repoName := "test-repo"
	repoPath := filepath.Join(parentDir, repoName)
	if err := os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}

	// Write a .ralphrc so the scanner finds it.
	if err := os.WriteFile(filepath.Join(repoPath, ".ralphrc"), []byte("# config\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a ralph_loop.sh that writes status.json, then sleeps.
	scriptBody := `
cat > .ralph/status.json <<'STATUSEOF'
{"status":"running","loop_count":1,"last_action":"integration-test"}
STATUSEOF
sleep 30
`
	scriptPath := filepath.Join(repoPath, "ralph_loop.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\n"+scriptBody), 0755); err != nil {
		t.Fatal(err)
	}

	// --- Step 1: Scan ---
	ctx := context.Background()
	repos, err := discovery.Scan(ctx, parentDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Name != repoName {
		t.Errorf("repo name = %q, want %q", repos[0].Name, repoName)
	}
	if repos[0].Path != repoPath {
		t.Errorf("repo path = %q, want %q", repos[0].Path, repoPath)
	}
	if !repos[0].HasRalph {
		t.Error("expected HasRalph=true")
	}
	if !repos[0].HasRC {
		t.Error("expected HasRC=true")
	}

	// --- Step 2: Start ---
	mgr := process.NewManager()
	mgr.KillTimeout = 200 * time.Millisecond // fast kills for testing
	defer mgr.StopAll(context.Background())

	if err := mgr.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// --- Step 3: Assert running ---
	if !mgr.IsRunning(repoPath) {
		t.Fatal("expected IsRunning=true after Start")
	}

	pid := mgr.PidForRepo(repoPath)
	if pid <= 0 {
		t.Fatalf("expected positive PID, got %d", pid)
	}

	// PID file should exist.
	if !pidFileExists(repoPath) {
		t.Error("expected PID file to exist after Start")
	}

	// --- Step 4: Poll for status.json ---
	waitForCondition(t, 3*time.Second, "status.json written by script", func() bool {
		_, err := os.Stat(filepath.Join(repoPath, ".ralph", "status.json"))
		return err == nil
	})

	status, err := model.LoadStatus(context.Background(), repoPath)
	if err != nil {
		t.Fatalf("LoadStatus: %v", err)
	}
	if status.Status != "running" {
		t.Errorf("status.Status = %q, want running", status.Status)
	}
	if status.LoopCount != 1 {
		t.Errorf("status.LoopCount = %d, want 1", status.LoopCount)
	}

	// --- Step 5: Stop ---
	if err := mgr.Stop(context.Background(), repoPath); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Wait for the process to be cleaned up.
	waitForCondition(t, 3*time.Second, "IsRunning=false after Stop", func() bool {
		return !mgr.IsRunning(repoPath)
	})

	// --- Step 6: Verify stopped ---
	if mgr.IsRunning(repoPath) {
		t.Error("expected IsRunning=false after Stop")
	}

	// PID file should be cleaned up.
	waitForCondition(t, 2*time.Second, "PID file removed after Stop", func() bool {
		return !pidFileExists(repoPath)
	})
}

// TestLifecycle_AutoRestart verifies that a crashing process is automatically
// restarted and eventually succeeds.
func TestLifecycle_AutoRestart(t *testing.T) {
	repoPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}

	counterFile := filepath.Join(repoPath, ".ralph", "run_counter")

	// Script exits 1 on runs 1-2, exits 0 on run 3.
	scriptBody := `COUNTER_FILE="` + counterFile + `"
if [ ! -f "$COUNTER_FILE" ]; then
  echo 1 > "$COUNTER_FILE"
else
  COUNT=$(cat "$COUNTER_FILE")
  echo $((COUNT + 1)) > "$COUNTER_FILE"
fi
COUNT=$(cat "$COUNTER_FILE")
if [ "$COUNT" -le 2 ]; then
  exit 1
fi
exit 0
`
	scriptPath := filepath.Join(repoPath, "ralph_loop.sh")
	if err := os.WriteFile(scriptPath, []byte("#!/bin/bash\n"+scriptBody), 0755); err != nil {
		t.Fatal(err)
	}

	mgr := process.NewManager()
	mgr.AutoRestart = true
	mgr.MaxRestarts = 2
	mgr.KillTimeout = 200 * time.Millisecond

	if err := mgr.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Wait for the process to finish all restarts and exit cleanly.
	waitForCondition(t, 30*time.Second, "process finishes after restarts", func() bool {
		return !mgr.IsRunning(repoPath)
	})

	// Verify 3 total runs.
	data, err := os.ReadFile(counterFile)
	if err != nil {
		t.Fatalf("read counter: %v", err)
	}
	count := string(data)
	// Trim whitespace.
	for len(count) > 0 && (count[len(count)-1] == '\n' || count[len(count)-1] == ' ') {
		count = count[:len(count)-1]
	}
	if count != "3" {
		t.Errorf("expected 3 invocations, got %q", count)
	}

	// Final exit should be clean.
	code, _, ok := mgr.LastExitStatus(repoPath)
	if !ok {
		t.Fatal("expected LastExitStatus to be recorded")
	}
	if code != 0 {
		t.Errorf("expected final exit code 0, got %d", code)
	}

	// PID file should be cleaned up.
	if pidFileExists(repoPath) {
		t.Error("PID file should be removed after process exits")
	}
}

// TestLifecycle_DoubleStop verifies that stopping an already-stopped process
// returns ErrNotRunning.
func TestLifecycle_DoubleStop(t *testing.T) {
	repoPath := setupTestRepo(t, "sleep 30")

	mgr := process.NewManager()
	mgr.KillTimeout = 200 * time.Millisecond

	if err := mgr.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !mgr.IsRunning(repoPath) {
		t.Fatal("expected running after Start")
	}

	// First stop should succeed.
	if err := mgr.Stop(context.Background(), repoPath); err != nil {
		t.Fatalf("first Stop: %v", err)
	}

	// Wait for cleanup.
	waitForCondition(t, 3*time.Second, "IsRunning=false after first Stop", func() bool {
		return !mgr.IsRunning(repoPath)
	})

	// Second stop should return ErrNotRunning.
	err := mgr.Stop(context.Background(), repoPath)
	if err == nil {
		t.Fatal("expected error on second Stop")
	}
	if !errors.Is(err, process.ErrNotRunning) {
		t.Errorf("expected ErrNotRunning, got: %v", err)
	}
}

// TestLifecycle_StopBeforeStart verifies that stopping an unmanaged repo
// returns ErrNotRunning.
func TestLifecycle_StopBeforeStart(t *testing.T) {
	repoPath := setupTestRepo(t, "sleep 30")

	mgr := process.NewManager()

	err := mgr.Stop(context.Background(), repoPath)
	if err == nil {
		t.Fatal("expected error when stopping unmanaged repo")
	}
	if !errors.Is(err, process.ErrNotRunning) {
		t.Errorf("expected ErrNotRunning, got: %v", err)
	}
}

// TestLifecycle_ScanFindsMultipleRepos verifies the scanner finds multiple
// repos in a parent directory and that they can be started independently.
func TestLifecycle_ScanFindsMultipleRepos(t *testing.T) {
	parentDir := t.TempDir()

	// Create 3 repos.
	for _, name := range []string{"alpha", "beta", "gamma"} {
		rp := filepath.Join(parentDir, name)
		if err := os.MkdirAll(filepath.Join(rp, ".ralph"), 0755); err != nil {
			t.Fatal(err)
		}
		script := filepath.Join(rp, "ralph_loop.sh")
		if err := os.WriteFile(script, []byte("#!/bin/bash\nsleep 30\n"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Also create a non-ralph directory that should be ignored.
	if err := os.MkdirAll(filepath.Join(parentDir, "not-a-repo"), 0755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	repos, err := discovery.Scan(ctx, parentDir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(repos) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(repos))
	}

	// Start all, verify running, stop all.
	mgr := process.NewManager()
	mgr.KillTimeout = 200 * time.Millisecond
	defer mgr.StopAll(context.Background())

	for _, r := range repos {
		if err := mgr.Start(context.Background(), r.Path); err != nil {
			t.Fatalf("Start(%s): %v", r.Name, err)
		}
	}

	paths := mgr.RunningPaths()
	if len(paths) != 3 {
		t.Errorf("expected 3 running, got %d", len(paths))
	}

	mgr.StopAll(context.Background())

	if len(mgr.RunningPaths()) != 0 {
		t.Error("expected 0 running after StopAll")
	}
}

// TestLifecycle_ProcessExitChannel verifies the exit channel delivers messages.
func TestLifecycle_ProcessExitChannel(t *testing.T) {
	repoPath := setupTestRepo(t, "exit 0")

	mgr := process.NewManager()
	mgr.KillTimeout = 200 * time.Millisecond

	if err := mgr.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case msg := <-mgr.ExitChan():
		if msg.RepoPath != repoPath {
			t.Errorf("ExitMsg.RepoPath = %q, want %q", msg.RepoPath, repoPath)
		}
		if msg.ExitCode != 0 {
			t.Errorf("ExitMsg.ExitCode = %d, want 0", msg.ExitCode)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for ProcessExitMsg")
	}
}
