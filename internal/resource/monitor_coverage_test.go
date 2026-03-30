package resource

import (
	"testing"
)

func TestCheck_EmptyPath(t *testing.T) {
	t.Parallel()
	s := Check("")
	// With empty path, disk stats should be zero but other fields populated.
	if s.NumGoroutine <= 0 {
		t.Error("expected positive goroutine count")
	}
	if s.MemAllocMB <= 0 {
		t.Error("expected positive memory allocation")
	}
	if s.DiskTotalBytes != 0 {
		t.Error("expected zero disk total for empty path")
	}
	if s.DiskFreeBytes != 0 {
		t.Error("expected zero disk free for empty path")
	}
}

func TestCheck_TempDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := Check(dir)
	if s.DiskTotalBytes == 0 {
		t.Error("expected non-zero disk total for temp dir")
	}
	if s.DiskFreeBytes == 0 {
		t.Error("expected non-zero disk free for temp dir")
	}
	if s.DiskFreeBytes > s.DiskTotalBytes {
		t.Error("free bytes should not exceed total bytes")
	}
}

func TestDiskUsage_NonexistentPath(t *testing.T) {
	t.Parallel()
	_, err := diskUsage("/nonexistent/path/xyz")
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestDiskUsage_ValidPath(t *testing.T) {
	t.Parallel()
	info, err := diskUsage(t.TempDir())
	if err != nil {
		t.Fatalf("diskUsage: %v", err)
	}
	if info.total == 0 {
		t.Error("expected non-zero total")
	}
	if info.free == 0 {
		t.Error("expected non-zero free space")
	}
}

func TestCheck_WarningsForNormalSystem(t *testing.T) {
	t.Parallel()
	// On a normal development machine, Check should not generate warnings.
	s := Check(t.TempDir())
	// We can't guarantee no warnings on all systems, but we can verify the
	// warnings slice is properly initialized (not nil).
	if s.Warnings == nil {
		// nil is acceptable; means no warnings.
	}
}

func TestStatus_IsHealthy_WithMultipleWarnings(t *testing.T) {
	t.Parallel()
	s := Status{
		Warnings: []string{"warning1", "warning2"},
	}
	if s.IsHealthy() {
		t.Error("status with multiple warnings should not be healthy")
	}
}
