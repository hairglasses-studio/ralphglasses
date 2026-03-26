package session

import (
	"math"
	"testing"
)

func TestJaccardSimilarity_ExactMatch(t *testing.T) {
	sim := JaccardSimilarity("add logging", "add logging")
	if sim != 1.0 {
		t.Errorf("exact match: got %f, want 1.0", sim)
	}
}

func TestJaccardSimilarity_NearMatch(t *testing.T) {
	// "add tests for session loop" vs "add test coverage for session loop"
	// Words A: {add, tests, for, session, loop} = 5
	// Words B: {add, test, coverage, for, session, loop} = 6
	// Intersection: {add, for, session, loop} = 4
	// Union: {add, tests, test, coverage, for, session, loop} = 7
	// Jaccard = 4/7 ≈ 0.571
	sim := JaccardSimilarity("add tests for session loop", "add test coverage for session loop")
	if sim < 0.5 || sim > 0.6 {
		t.Errorf("near match: got %f, want ~0.571", sim)
	}
}

func TestJaccardSimilarity_HighOverlap(t *testing.T) {
	// "add unit tests for parser" vs "add unit tests for the parser"
	// Words A: {add, unit, tests, for, parser} = 5
	// Words B: {add, unit, tests, for, the, parser} = 6
	// Intersection: {add, unit, tests, for, parser} = 5
	// Union: {add, unit, tests, for, the, parser} = 6
	// Jaccard = 5/6 ≈ 0.833
	sim := JaccardSimilarity("add unit tests for parser", "add unit tests for the parser")
	if sim < 0.8 {
		t.Errorf("high overlap: got %f, want >= 0.8", sim)
	}
}

func TestJaccardSimilarity_Distinct(t *testing.T) {
	sim := JaccardSimilarity("fix build error", "add new MCP tool")
	if sim > 0.2 {
		t.Errorf("distinct: got %f, want < 0.2", sim)
	}
}

func TestJaccardSimilarity_CaseInsensitive(t *testing.T) {
	sim := JaccardSimilarity("Add Logging", "add logging")
	if sim != 1.0 {
		t.Errorf("case insensitive: got %f, want 1.0", sim)
	}
}

func TestJaccardSimilarity_BothEmpty(t *testing.T) {
	sim := JaccardSimilarity("", "")
	if sim != 1.0 {
		t.Errorf("both empty: got %f, want 1.0", sim)
	}
}

func TestJaccardSimilarity_OneEmpty(t *testing.T) {
	sim := JaccardSimilarity("add logging", "")
	if sim != 0.0 {
		t.Errorf("one empty: got %f, want 0.0", sim)
	}

	sim = JaccardSimilarity("", "add logging")
	if sim != 0.0 {
		t.Errorf("other empty: got %f, want 0.0", sim)
	}
}

func TestJaccardSimilarity_SingleWord(t *testing.T) {
	sim := JaccardSimilarity("fix", "fix")
	if sim != 1.0 {
		t.Errorf("same single word: got %f, want 1.0", sim)
	}

	sim = JaccardSimilarity("fix", "add")
	if sim != 0.0 {
		t.Errorf("different single word: got %f, want 0.0", sim)
	}
}

func TestJaccardSimilarity_WhitespaceVariations(t *testing.T) {
	sim := JaccardSimilarity("  add  logging  ", "add logging")
	if sim != 1.0 {
		t.Errorf("whitespace variations: got %f, want 1.0", sim)
	}
}

func TestJaccardSimilarity_DuplicateWords(t *testing.T) {
	// Duplicate words in input should not affect result (set semantics).
	sim := JaccardSimilarity("add add logging", "add logging")
	if sim != 1.0 {
		t.Errorf("duplicate words: got %f, want 1.0", sim)
	}
}

func TestJaccardSimilarity_Symmetric(t *testing.T) {
	a := "fix parser error handling"
	b := "fix error in parser handling module"
	sim1 := JaccardSimilarity(a, b)
	sim2 := JaccardSimilarity(b, a)
	if math.Abs(sim1-sim2) > 1e-10 {
		t.Errorf("not symmetric: %f != %f", sim1, sim2)
	}
}

func TestIsSimilarTask_Found(t *testing.T) {
	completed := []string{
		"add unit tests for parser",
		"fix CI pipeline timeout",
		"refactor session manager",
	}

	found, matched := IsSimilarTask("add unit tests for the parser", completed, 0.8)
	if !found {
		t.Error("expected to find similar task")
	}
	if matched != "add unit tests for parser" {
		t.Errorf("matched = %q, want %q", matched, "add unit tests for parser")
	}
}

func TestIsSimilarTask_NotFound(t *testing.T) {
	completed := []string{
		"add unit tests for parser",
		"fix CI pipeline timeout",
	}

	found, matched := IsSimilarTask("add new MCP tool for fleet management", completed, 0.8)
	if found {
		t.Errorf("unexpected match: %q", matched)
	}
}

func TestIsSimilarTask_EmptyCompleted(t *testing.T) {
	found, _ := IsSimilarTask("any task", nil, 0.8)
	if found {
		t.Error("empty completed list should not match")
	}
}

func TestIsSimilarTask_EmptyTitle(t *testing.T) {
	completed := []string{"add logging"}
	found, _ := IsSimilarTask("", completed, 0.8)
	if found {
		t.Error("empty title should not match non-empty completed")
	}
}

func TestFilterDuplicateTasks_ExactMatch(t *testing.T) {
	tasks := []LoopTask{
		{Title: "add logging"},
		{Title: "fix tests"},
	}
	completed := []string{"add logging"}

	result := filterDuplicateTasks(tasks, completed, DefaultSimilarityThreshold)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0].Title != "fix tests" {
		t.Errorf("title = %q, want %q", result[0].Title, "fix tests")
	}
}

func TestFilterDuplicateTasks_NearDuplicate(t *testing.T) {
	tasks := []LoopTask{
		{Title: "add unit tests for the parser"},
		{Title: "implement new fleet dashboard"},
	}
	completed := []string{"add unit tests for parser"}

	result := filterDuplicateTasks(tasks, completed, DefaultSimilarityThreshold)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
	if result[0].Title != "implement new fleet dashboard" {
		t.Errorf("title = %q, want %q", result[0].Title, "implement new fleet dashboard")
	}
}

func TestFilterDuplicateTasks_NoCompleted(t *testing.T) {
	tasks := []LoopTask{
		{Title: "add logging"},
		{Title: "fix tests"},
	}

	result := filterDuplicateTasks(tasks, nil, DefaultSimilarityThreshold)
	if len(result) != 2 {
		t.Fatalf("len = %d, want 2", len(result))
	}
}

func TestFilterDuplicateTasks_AllDuplicates(t *testing.T) {
	tasks := []LoopTask{
		{Title: "add logging"},
		{Title: "add logging to session"},
	}
	// "add logging" is exact match; "add logging to session" has Jaccard
	// with "add logging to the session module" = {add,logging,to,session}/{add,logging,to,the,session,module} = 4/6 ≈ 0.667
	// so only the exact match is caught at threshold 0.8.
	completed := []string{"add logging"}

	result := filterDuplicateTasks(tasks, completed, DefaultSimilarityThreshold)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1 (only exact dup removed)", len(result))
	}
}

func TestFilterDuplicateTasks_CaseInsensitive(t *testing.T) {
	tasks := []LoopTask{
		{Title: "Add Logging"},
	}
	completed := []string{"add logging"}

	result := filterDuplicateTasks(tasks, completed, DefaultSimilarityThreshold)
	if len(result) != 0 {
		t.Fatalf("len = %d, want 0 (case-insensitive exact match)", len(result))
	}
}
