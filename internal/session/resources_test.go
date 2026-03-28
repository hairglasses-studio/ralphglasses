package session

import (
	"strings"
	"testing"
)

func TestDiskSpaceWarning_EnoughSpace(t *testing.T) {
	// Current directory should have plenty of space — expect no warning.
	warn := DiskSpaceWarning(".", 1) // 1 byte threshold
	if warn != "" {
		t.Fatalf("expected no warning for minimal threshold, got: %s", warn)
	}
}

func TestDiskSpaceWarning_HighThreshold(t *testing.T) {
	// Use an absurdly high threshold (1 exabyte) — should trigger a warning.
	warn := DiskSpaceWarning(".", 1<<60)
	if warn == "" {
		t.Fatal("expected a warning for exabyte threshold")
	}
	if !strings.Contains(warn, "low disk space") {
		t.Fatalf("warning should mention low disk space, got: %s", warn)
	}
}

func TestDiskSpaceWarning_InvalidPath(t *testing.T) {
	// Non-existent path — should return empty (don't warn on error).
	warn := DiskSpaceWarning("/nonexistent/path/that/does/not/exist", 1)
	if warn != "" {
		t.Fatalf("expected no warning for invalid path, got: %s", warn)
	}
}

func TestMemoryPressureWarning_NoWarning(t *testing.T) {
	// Very generous thresholds — should not trigger.
	warn := MemoryPressureWarning(1<<62, 1.0) // 4 exabytes, 100% ratio
	if warn != "" {
		t.Fatalf("expected no warning with generous thresholds, got: %s", warn)
	}
}

func TestMemoryPressureWarning_HeapExceeded(t *testing.T) {
	// Threshold of 1 byte — any running Go process will exceed this.
	warn := MemoryPressureWarning(1, 1.0)
	if warn == "" {
		t.Fatal("expected a warning for 1-byte heap threshold")
	}
	if !strings.Contains(warn, "high memory pressure") {
		t.Fatalf("warning should mention high memory pressure, got: %s", warn)
	}
	if !strings.Contains(warn, "HeapAlloc") {
		t.Fatalf("warning should mention HeapAlloc, got: %s", warn)
	}
}

func TestMemoryPressureWarning_RatioExceeded(t *testing.T) {
	// Generous heap limit but impossible ratio (0.0) — should trigger ratio warning.
	warn := MemoryPressureWarning(1<<62, 0.0)
	if warn == "" {
		t.Fatal("expected a warning for 0.0 ratio threshold")
	}
	if !strings.Contains(warn, "HeapAlloc/HeapSys") {
		t.Fatalf("warning should mention HeapAlloc/HeapSys ratio, got: %s", warn)
	}
}

func TestDefaultConstants(t *testing.T) {
	// Verify the default constants have sensible values.
	if DefaultMinFreeDiskBytes != 5*OneGB {
		t.Errorf("DefaultMinFreeDiskBytes: got %d, want %d", DefaultMinFreeDiskBytes, 5*OneGB)
	}
	if DefaultMaxHeapBytes != 2*OneGB {
		t.Errorf("DefaultMaxHeapBytes: got %d, want %d", DefaultMaxHeapBytes, 2*OneGB)
	}
	if DefaultHeapUsageRatio != 0.9 {
		t.Errorf("DefaultHeapUsageRatio: got %f, want 0.9", DefaultHeapUsageRatio)
	}
}
