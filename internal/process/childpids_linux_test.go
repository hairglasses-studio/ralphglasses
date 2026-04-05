//go:build linux

package process

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestCollectChildPIDs_SameProcessGroup verifies that CollectChildPIDs finds
// processes in the same process group as the parent.
func TestCollectChildPIDs_SameProcessGroup(t *testing.T) {
	// Start a bash script that spawns two sleep children, then waits.
	cmd := exec.Command("bash", "-c", "sleep 60 & sleep 60 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()

	// Give the shell time to spawn its children.
	time.Sleep(100 * time.Millisecond)

	pids := CollectChildPIDs(cmd.Process.Pid)
	if len(pids) < 2 {
		t.Errorf("expected at least 2 child PIDs (the two sleep processes), got %d: %v", len(pids), pids)
	}

	// All returned PIDs must be positive and distinct from the parent.
	seen := make(map[int]bool)
	for _, p := range pids {
		if p <= 0 {
			t.Errorf("got non-positive PID %d", p)
		}
		if p == cmd.Process.Pid {
			t.Errorf("parent PID %d must not appear in child list", p)
		}
		if seen[p] {
			t.Errorf("duplicate PID %d in result", p)
		}
		seen[p] = true
	}
}

// TestCollectChildPIDs_DeadPID returns nil for a PID that does not exist.
func TestCollectChildPIDs_DeadPID(t *testing.T) {
	pids := CollectChildPIDs(1_000_000_000)
	if len(pids) != 0 {
		t.Errorf("expected empty for non-existent PID, got %v", pids)
	}
}

// TestCollectChildPIDs_SelfProcess collects children of the test process itself.
// The test process should have no additional process-group siblings injected,
// so the result may be empty — but the call must not error.
func TestCollectChildPIDs_SelfProcess(t *testing.T) {
	// This must not panic.
	_ = CollectChildPIDs(os.Getpid())
}
