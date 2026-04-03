package patterns

import (
	"strings"
	"testing"
)

func TestMultiReflexion_AddReview(t *testing.T) {
	mr := NewMultiReflexion([]ReflexionPerspective{
		{Name: "security"},
		{Name: "performance"},
	})

	err := mr.AddReview("security", PerspectiveReview{
		Verdict: "approve", Score: 0.9, Comments: []string{"looks safe"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if mr.AllReviewed() {
		t.Error("should not be all reviewed with 1/2")
	}

	mr.AddReview("performance", PerspectiveReview{
		Verdict: "approve", Score: 0.8, Comments: []string{"fast enough"},
	})
	if !mr.AllReviewed() {
		t.Error("should be all reviewed")
	}
}

func TestMultiReflexion_UnknownPerspective(t *testing.T) {
	mr := NewMultiReflexion([]ReflexionPerspective{{Name: "sec"}})
	err := mr.AddReview("unknown", PerspectiveReview{Verdict: "approve"})
	if err == nil {
		t.Error("expected error for unknown perspective")
	}
}

func TestMultiReflexion_Aggregate_Unanimous(t *testing.T) {
	mr := NewMultiReflexion([]ReflexionPerspective{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	})
	for _, name := range []string{"a", "b", "c"} {
		mr.AddReview(name, PerspectiveReview{Verdict: "approve", Score: 0.9})
	}

	agg := mr.Aggregate()
	if agg.Verdict != "approve" {
		t.Errorf("verdict = %q, want approve", agg.Verdict)
	}
	if !agg.Unanimous {
		t.Error("expected unanimous")
	}
	if agg.Score < 0.89 || agg.Score > 0.91 {
		t.Errorf("score = %.2f, want ~0.9", agg.Score)
	}
}

func TestMultiReflexion_Aggregate_MajorityVote(t *testing.T) {
	mr := NewMultiReflexion([]ReflexionPerspective{
		{Name: "a"}, {Name: "b"}, {Name: "c"},
	})
	mr.AddReview("a", PerspectiveReview{Verdict: "approve", Score: 0.9})
	mr.AddReview("b", PerspectiveReview{Verdict: "request_changes", Score: 0.4})
	mr.AddReview("c", PerspectiveReview{Verdict: "request_changes", Score: 0.5})

	agg := mr.Aggregate()
	if agg.Verdict != "request_changes" {
		t.Errorf("verdict = %q, want request_changes (majority)", agg.Verdict)
	}
	if agg.Unanimous {
		t.Error("should not be unanimous")
	}
}

func TestMultiReflexion_Aggregate_TieBreakConservative(t *testing.T) {
	mr := NewMultiReflexion([]ReflexionPerspective{
		{Name: "a"}, {Name: "b"},
	})
	mr.AddReview("a", PerspectiveReview{Verdict: "approve", Score: 0.9})
	mr.AddReview("b", PerspectiveReview{Verdict: "request_changes", Score: 0.5})

	agg := mr.Aggregate()
	// On tie, should prefer conservative (request_changes)
	if agg.Verdict != "request_changes" {
		t.Errorf("verdict = %q, want request_changes (conservative tie-break)", agg.Verdict)
	}
}

func TestMultiReflexion_MergedComments(t *testing.T) {
	mr := NewMultiReflexion([]ReflexionPerspective{{Name: "a"}, {Name: "b"}})
	mr.AddReview("a", PerspectiveReview{
		Verdict: "approve", Score: 0.8,
		Comments: []string{"fix typo", "add test"},
	})
	mr.AddReview("b", PerspectiveReview{
		Verdict: "approve", Score: 0.7,
		Comments: []string{"add test", "improve docs"}, // "add test" is duplicate
	})

	agg := mr.Aggregate()
	if len(agg.Comments) != 3 { // fix typo, add test, improve docs (deduplicated)
		t.Errorf("comments = %d, want 3 (deduplicated)", len(agg.Comments))
	}
}

func TestMultiReflexion_Reset(t *testing.T) {
	mr := NewMultiReflexion([]ReflexionPerspective{{Name: "a"}})
	mr.AddReview("a", PerspectiveReview{Verdict: "approve", Score: 1.0})

	mr.Reset()
	if mr.AllReviewed() {
		t.Error("should not be reviewed after reset")
	}
	agg := mr.Aggregate()
	if agg.Verdict != "pending" {
		t.Errorf("verdict after reset = %q, want pending", agg.Verdict)
	}
}

func TestThreePerspectiveReview(t *testing.T) {
	mr := ThreePerspectiveReview()
	perspectives := mr.Perspectives()
	if len(perspectives) != 3 {
		t.Fatalf("expected 3 perspectives, got %d", len(perspectives))
	}

	names := map[string]bool{}
	for _, p := range perspectives {
		names[p.Name] = true
		if p.SystemPrompt == "" {
			t.Errorf("perspective %s has empty system prompt", p.Name)
		}
	}

	for _, expected := range []string{"AX", "UX", "DX"} {
		if !names[expected] {
			t.Errorf("missing perspective %s", expected)
		}
	}
}

func TestMultiReflexion_FormatReport(t *testing.T) {
	mr := ThreePerspectiveReview()
	mr.AddReview("AX", PerspectiveReview{Verdict: "approve", Score: 0.9, Comments: []string{"good"}})
	mr.AddReview("UX", PerspectiveReview{Verdict: "approve", Score: 0.8})
	mr.AddReview("DX", PerspectiveReview{Verdict: "request_changes", Score: 0.6, Comments: []string{"needs tests"}})

	report := mr.FormatReport()
	if !strings.Contains(report, "Multi-Perspective Review") {
		t.Error("expected header in report")
	}
	if !strings.Contains(report, "AX") || !strings.Contains(report, "DX") {
		t.Error("expected perspective names in report")
	}
}
