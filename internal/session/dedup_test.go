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

// --- extractFilePathsFromText tests ---

func TestExtractFilePaths_GoFiles(t *testing.T) {
	text := "Add tests in internal/session/manager.go and fix internal/session/dedup.go"
	paths := extractFilePathsFromText(text)
	if len(paths) != 2 {
		t.Fatalf("len = %d, want 2; got %v", len(paths), paths)
	}
	if paths[0] != "internal/session/manager.go" {
		t.Errorf("paths[0] = %q, want %q", paths[0], "internal/session/manager.go")
	}
	if paths[1] != "internal/session/dedup.go" {
		t.Errorf("paths[1] = %q, want %q", paths[1], "internal/session/dedup.go")
	}
}

func TestExtractFilePaths_CmdAndPkg(t *testing.T) {
	text := "Update cmd/main.go and pkg/util/helper.go"
	paths := extractFilePathsFromText(text)
	if len(paths) != 2 {
		t.Fatalf("len = %d, want 2; got %v", len(paths), paths)
	}
}

func TestExtractFilePaths_NoPaths(t *testing.T) {
	paths := extractFilePathsFromText("add logging to the system")
	if len(paths) != 0 {
		t.Fatalf("expected no paths, got %v", paths)
	}
}

func TestExtractFilePaths_EmptyString(t *testing.T) {
	paths := extractFilePathsFromText("")
	if len(paths) != 0 {
		t.Fatalf("expected no paths, got %v", paths)
	}
}

func TestExtractFilePaths_Deduplicates(t *testing.T) {
	text := "Fix internal/session/manager.go then test internal/session/manager.go"
	paths := extractFilePathsFromText(text)
	if len(paths) != 1 {
		t.Fatalf("len = %d, want 1 (deduplicated); got %v", len(paths), paths)
	}
}

// --- filterDuplicateTasksByContent tests ---

func TestFilterDuplicateTasksByContent_FileOverlapRejects(t *testing.T) {
	proposed := []LoopTask{
		{Title: "improve manager", Prompt: "Refactor internal/session/manager.go for clarity"},
	}
	completed := []LoopTask{
		{Title: "add tests for manager", Prompt: "Add tests for internal/session/manager.go"},
	}

	result := filterDuplicateTasksByContent(proposed, completed)
	if len(result) != 0 {
		t.Fatalf("len = %d, want 0 (file overlap should reject)", len(result))
	}
}

func TestFilterDuplicateTasksByContent_DifferentFilesPasses(t *testing.T) {
	proposed := []LoopTask{
		{Title: "improve planner", Prompt: "Refactor internal/session/loop_planner.go"},
	}
	completed := []LoopTask{
		{Title: "add tests for manager", Prompt: "Add tests for internal/session/manager.go"},
	}

	result := filterDuplicateTasksByContent(proposed, completed)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1 (different files should pass)", len(result))
	}
}

func TestFilterDuplicateTasksByContent_NoPathsInProposed(t *testing.T) {
	proposed := []LoopTask{
		{Title: "add logging", Prompt: "Add logging throughout the codebase"},
	}
	completed := []LoopTask{
		{Title: "fix manager", Prompt: "Fix internal/session/manager.go"},
	}

	result := filterDuplicateTasksByContent(proposed, completed)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1 (no paths in proposed = keep)", len(result))
	}
}

func TestFilterDuplicateTasksByContent_NoPathsInCompleted(t *testing.T) {
	proposed := []LoopTask{
		{Title: "fix manager", Prompt: "Fix internal/session/manager.go"},
	}
	completed := []LoopTask{
		{Title: "add logging", Prompt: "Add logging throughout the codebase"},
	}

	result := filterDuplicateTasksByContent(proposed, completed)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1 (no paths in completed = keep)", len(result))
	}
}

func TestFilterDuplicateTasksByContent_EmptyCompleted(t *testing.T) {
	proposed := []LoopTask{
		{Title: "fix it", Prompt: "Fix internal/session/manager.go"},
	}

	result := filterDuplicateTasksByContent(proposed, nil)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1", len(result))
	}
}

func TestFilterDuplicateTasksByContent_EmptyPrompts(t *testing.T) {
	proposed := []LoopTask{
		{Title: "fix it", Prompt: ""},
	}
	completed := []LoopTask{
		{Title: "add tests", Prompt: ""},
	}

	result := filterDuplicateTasksByContent(proposed, completed)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1 (empty prompts = keep)", len(result))
	}
}

func TestFilterDuplicateTasksByContent_PartialOverlapBelow50Passes(t *testing.T) {
	// Proposed has 3 files, only 1 overlaps = 33% < 50%
	proposed := []LoopTask{
		{Title: "refactor session", Prompt: "Refactor internal/session/manager.go internal/session/dedup.go internal/session/loop_planner.go"},
	}
	completed := []LoopTask{
		{Title: "fix manager", Prompt: "Fix internal/session/manager.go"},
	}

	result := filterDuplicateTasksByContent(proposed, completed)
	if len(result) != 1 {
		t.Fatalf("len = %d, want 1 (33%% overlap < 50%% threshold)", len(result))
	}
}

func TestFilterDuplicateTasksByContent_MixedFileOverlapDifferentTitle(t *testing.T) {
	// Different title but same file — content dedup should still catch it.
	proposed := []LoopTask{
		{Title: "completely new feature", Prompt: "Implement feature in internal/session/manager.go"},
	}
	completed := []LoopTask{
		{Title: "add tests for manager", Prompt: "Add tests for internal/session/manager.go"},
	}

	result := filterDuplicateTasksByContent(proposed, completed)
	if len(result) != 0 {
		t.Fatalf("len = %d, want 0 (same files, caught by content dedup)", len(result))
	}
}

// --- Threshold 0.7 rephrasing tests ---

func TestSimilarityThreshold07_CatchesRephrasings(t *testing.T) {
	// "add tests for session" vs "write test coverage for session package"
	// Words A: {add, tests, for, session} = 4
	// Words B: {write, test, coverage, for, session, package} = 6
	// Intersection: {for, session} = 2
	// Union: {add, tests, write, test, coverage, for, session, package} = 8
	// Jaccard = 2/8 = 0.25 — not caught even at 0.7
	//
	// But: "add tests for session" vs "add test for session"
	// Words A: {add, tests, for, session} = 4
	// Words B: {add, test, for, session} = 4
	// Intersection: {add, for, session} = 3
	// Union: {add, tests, test, for, session} = 5
	// Jaccard = 3/5 = 0.6 — not caught at 0.8 but close.
	//
	// "add tests for session package" vs "add tests for the session package"
	// Words A: {add, tests, for, session, package} = 5
	// Words B: {add, tests, for, the, session, package} = 6
	// Intersection: 5, Union: 6
	// Jaccard = 5/6 ≈ 0.833 — caught at both 0.7 and 0.8
	//
	// "add tests for session" vs "add tests session"
	// Words A: {add, tests, for, session} = 4
	// Words B: {add, tests, session} = 3
	// Intersection: 3, Union: 4
	// Jaccard = 3/4 = 0.75 — caught at 0.7 but NOT at 0.8
	sim := JaccardSimilarity("add tests for session", "add tests session")
	if sim < 0.7 {
		t.Errorf("got %f, want >= 0.7 (should be caught at new threshold)", sim)
	}
	if sim >= 0.8 {
		t.Errorf("got %f, want < 0.8 (should NOT have been caught at old threshold)", sim)
	}

	// Verify the default threshold is now 0.7
	if DefaultSimilarityThreshold != 0.7 {
		t.Errorf("DefaultSimilarityThreshold = %f, want 0.7", DefaultSimilarityThreshold)
	}

	// This rephrasing should now be caught by filterDuplicateTasks
	tasks := []LoopTask{{Title: "add tests for session"}}
	completed := []string{"add tests session"}
	result := filterDuplicateTasks(tasks, completed, DefaultSimilarityThreshold)
	if len(result) != 0 {
		t.Fatalf("len = %d, want 0 (rephrasing should be caught at 0.7 threshold)", len(result))
	}
}
