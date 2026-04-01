package process

import (
	"os"
	"testing"
)

func TestCollectChildPIDsByPgid_CurrentProcess(t *testing.T) {
	// Call with the current process's PID.
	// On macOS, /proc doesn't exist so it returns []int{}.
	// On Linux, it returns children in the same pgid.
	pid := os.Getpid()
	pids := CollectChildPIDsByPgid(pid)
	// Just verify no panic and returns a slice (may be empty or non-nil).
	_ = pids
}

func TestCollectChildPIDsByPgid_InvalidPID(t *testing.T) {
	// PID -1 should fail Getpgid and return empty slice.
	pids := CollectChildPIDsByPgid(-1)
	// Should return empty slice (not nil).
	if pids == nil {
		t.Error("CollectChildPIDsByPgid(-1) returned nil, expected non-nil empty slice")
	}
}
