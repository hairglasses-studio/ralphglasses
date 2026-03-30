package resource

import (
	"testing"
)

func TestCheck(t *testing.T) {
	s := Check("/tmp")
	if s.NumGoroutine <= 0 {
		t.Error("expected positive goroutine count")
	}
	if s.MemAllocMB <= 0 {
		t.Error("expected positive memory allocation")
	}
	// /tmp should exist and have disk stats on any Unix system
	if s.DiskTotalBytes == 0 {
		t.Error("expected non-zero disk total")
	}
	if s.DiskUsedPct < 0 || s.DiskUsedPct > 100 {
		t.Errorf("disk used pct out of range: %f", s.DiskUsedPct)
	}
}

func TestCheckNonexistentPath(t *testing.T) {
	s := Check("/nonexistent/path/that/does/not/exist")
	// Should not panic, just have zero disk stats
	if s.DiskTotalBytes != 0 {
		t.Error("expected zero disk total for nonexistent path")
	}
}

func TestIsHealthy(t *testing.T) {
	s := Status{}
	if !s.IsHealthy() {
		t.Error("empty status should be healthy")
	}
	s.Warnings = append(s.Warnings, "test warning")
	if s.IsHealthy() {
		t.Error("status with warnings should not be healthy")
	}
}
