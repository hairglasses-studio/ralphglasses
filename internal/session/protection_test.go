package session

import (
	"testing"
)

func TestClassifyDiffPaths(t *testing.T) {
	paths := []string{
		"internal/session/loop.go",
		"internal/session/cascade.go",
		"internal/session/manager.go",
		"internal/tui/view.go",
		"internal/session/selftest.go",
		"README.md",
	}

	safe, needsReview := ClassifyDiffPaths(paths)

	// loop.go, manager.go, selftest.go are forbidden
	if len(needsReview) != 3 {
		t.Errorf("needsReview = %d, want 3: %v", len(needsReview), needsReview)
	}

	// cascade.go, view.go, README.md are safe
	if len(safe) != 3 {
		t.Errorf("safe = %d, want 3: %v", len(safe), safe)
	}
}

func TestClassifyDiffPaths_Empty(t *testing.T) {
	safe, needsReview := ClassifyDiffPaths(nil)
	if safe != nil {
		t.Errorf("expected nil safe, got %v", safe)
	}
	if needsReview != nil {
		t.Errorf("expected nil needsReview, got %v", needsReview)
	}
}

func TestClassifyDiffPaths_AllSafe(t *testing.T) {
	paths := []string{"foo.go", "bar.go"}
	safe, needsReview := ClassifyDiffPaths(paths)
	if len(safe) != 2 {
		t.Errorf("safe = %d, want 2", len(safe))
	}
	if needsReview != nil {
		t.Errorf("expected nil needsReview, got %v", needsReview)
	}
}
