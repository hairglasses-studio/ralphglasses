package session

import (
	"runtime"
	"testing"
)

func TestCollectSessionChildPIDs_InvalidPID(t *testing.T) {
	t.Parallel()
	// PID -1 should fail the Getpgid call and return empty slice.
	children := collectSessionChildPIDs(-1)
	if children == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(children) != 0 {
		t.Errorf("expected 0 children for invalid PID, got %d", len(children))
	}
}

func TestCollectSessionChildPIDs_CurrentProcess(t *testing.T) {
	t.Parallel()
	// On macOS, /proc doesn't exist, so this should return an empty slice.
	// On Linux, it should return children in our process group (likely none in test).
	children := collectSessionChildPIDs(1) // PID 1 (init/launchd)
	if children == nil {
		t.Fatal("expected non-nil slice")
	}
	// On macOS /proc is unavailable, so we expect empty.
	if runtime.GOOS == "darwin" {
		if len(children) != 0 {
			t.Errorf("expected 0 children on macOS (no /proc), got %d", len(children))
		}
	}
	// On Linux, just verify it doesn't panic or return nil.
}

func TestCollectSessionChildPIDs_ZeroPID(t *testing.T) {
	t.Parallel()
	children := collectSessionChildPIDs(0)
	// PID 0 has special meaning; Getpgid(0) returns the calling process's group.
	// Either way, the result should be a non-nil slice.
	if children == nil {
		t.Fatal("expected non-nil slice for PID 0")
	}
}
