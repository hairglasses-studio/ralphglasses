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
