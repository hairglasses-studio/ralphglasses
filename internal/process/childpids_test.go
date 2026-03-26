package process

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestCollectChildPIDs_RealSubprocess(t *testing.T) {
	// Start a real subprocess.
	cmd := exec.Command("sleep", "10")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start subprocess: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	pid := cmd.Process.Pid
	if pid <= 0 {
		t.Fatalf("expected positive PID, got %d", pid)
	}

	result := collectChildPIDs(pid)
	if result == nil {
		t.Fatal("collectChildPIDs returned nil; expected non-nil slice")
	}
	// sleep typically has no children, so we just verify non-nil []int.
	// The slice may be empty — that's fine.
}

func TestCollectChildPIDs_InvalidPID(t *testing.T) {
	result := collectChildPIDs(999999999)
	if result == nil {
		t.Fatal("collectChildPIDs returned nil for invalid PID; expected empty slice")
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice for invalid PID, got %v", result)
	}
}

func TestCollectChildPIDsFromProc_OwnPID(t *testing.T) {
	// Our own PID should not error out.
	result := collectChildPIDsFromProc(os.Getpid())
	if result == nil {
		t.Fatal("collectChildPIDsFromProc returned nil; expected non-nil slice")
	}
}

func TestManager_Start_RecordsChildPids(t *testing.T) {
	m := NewManager()
	repoPath := setupRepoWithRalphDir(t)
	defer m.StopAll(context.Background())

	if err := m.Start(context.Background(), repoPath); err != nil {
		t.Fatalf("Start: %v", err)
	}

	m.mu.Lock()
	mp, ok := m.procs[repoPath]
	m.mu.Unlock()
	if !ok {
		t.Fatal("expected process to be tracked")
	}

	if mp.PID <= 0 {
		t.Errorf("expected positive PID, got %d", mp.PID)
	}
	if mp.ChildPids == nil {
		t.Fatal("ChildPids should be non-nil (empty slice, not nil)")
	}

	// Give it a moment and verify PID is still correct.
	time.Sleep(50 * time.Millisecond)
	pid := m.PidForRepo(repoPath)
	if pid != mp.PID {
		t.Errorf("PidForRepo returned %d, expected %d", pid, mp.PID)
	}
}
