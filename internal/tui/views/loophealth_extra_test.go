package views

import (
	"strings"
	"testing"
)

func TestRenderTaskDistribution_Empty(t *testing.T) {
	t.Parallel()
	result := renderTaskDistribution(nil, 120)
	if result != "" {
		t.Errorf("expected empty string for nil dist, got %q", result)
	}

	result = renderTaskDistribution(map[string]int{}, 120)
	if result != "" {
		t.Errorf("expected empty string for empty dist, got %q", result)
	}
}

func TestRenderTaskDistribution_SingleType(t *testing.T) {
	t.Parallel()
	dist := map[string]int{"bugfix": 5}
	result := renderTaskDistribution(dist, 120)

	if !strings.Contains(result, "bugfix") {
		t.Error("should contain task type name 'bugfix'")
	}
	if !strings.Contains(result, "100%") {
		t.Error("single task type should show 100%")
	}
	if !strings.Contains(result, "(5)") {
		t.Error("should show count (5)")
	}
	if !strings.Contains(result, "Task Type Distribution") {
		t.Error("should have section header")
	}
}

func TestRenderTaskDistribution_MultipleTypes(t *testing.T) {
	t.Parallel()
	dist := map[string]int{
		"bugfix":  10,
		"feature": 5,
		"refactor": 3,
	}
	result := renderTaskDistribution(dist, 80)

	for _, name := range []string{"bugfix", "feature", "refactor"} {
		if !strings.Contains(result, name) {
			t.Errorf("should contain task type %q", name)
		}
	}
	// Verify sorted by count descending: bugfix (10) should appear before feature (5).
	bugfixIdx := strings.Index(result, "bugfix")
	featureIdx := strings.Index(result, "feature")
	if bugfixIdx > featureIdx {
		t.Error("types should be sorted by count descending")
	}
}

func TestRenderTaskDistribution_WideWidth(t *testing.T) {
	t.Parallel()
	dist := map[string]int{"code": 3}

	// Width > 80 should use wider bar (30 chars).
	resultWide := renderTaskDistribution(dist, 100)
	resultNarrow := renderTaskDistribution(dist, 60)

	// Both should render successfully without panicking.
	if resultWide == "" || resultNarrow == "" {
		t.Error("renderTaskDistribution should return non-empty for non-empty dist")
	}
}
