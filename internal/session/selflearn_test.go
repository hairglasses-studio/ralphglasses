package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Cross-subsystem integration tests
// ---------------------------------------------------------------------------

// TestCascadeUsesUncertainty verifies that EvaluateCheapResult uses
// ExtractConfidence to compute the confidence score and compares it
// against the configured threshold.
func TestCascadeUsesUncertainty(t *testing.T) {
	dir := t.TempDir()
	cr := NewCascadeRouter(CascadeConfig{
		CheapProvider:       ProviderGemini,
		ExpensiveProvider:   ProviderClaude,
		ConfidenceThreshold: 0.7,
		MaxCheapBudgetUSD:   0.50,
		MaxCheapTurns:       10,
	}, nil, nil, dir)

	// High-confidence session: clean, no errors, verification passes
	highConf := &Session{
		Status:    StatusCompleted,
		TurnCount: 5,
	}
	passVerify := []LoopVerification{{Status: "passed", ExitCode: 0}}
	escalate, conf, reason := cr.EvaluateCheapResult(highConf, 5, passVerify)
	if escalate {
		t.Errorf("expected no escalation for high-confidence session, got reason=%q conf=%.2f", reason, conf)
	}
	if conf < 0.5 {
		t.Errorf("expected confidence > 0.5, got %.2f", conf)
	}

	// Low-confidence session: many hedging words, no verification
	lowConf := &Session{
		Status:        StatusCompleted,
		TurnCount:     20,
		OutputHistory: []string{"I might be wrong", "maybe this could work", "perhaps try this", "not sure about this", "I think maybe", "possibly could be", "uncertain about", "might need to", "could be wrong", "perhaps not"},
	}
	failVerify := []LoopVerification{{Status: "failed", ExitCode: 1}}
	escalate, conf, reason = cr.EvaluateCheapResult(lowConf, 5, failVerify)
	if !escalate {
		t.Error("expected escalation for low-confidence session")
	}
	if reason == "" {
		t.Error("expected non-empty escalation reason")
	}
}

// TestCurriculumWithEpisodicMemory verifies that CurriculumSorter uses
// EpisodicMemory data via the EpisodicSource interface when scoring tasks.
func TestCurriculumWithEpisodicMemory(t *testing.T) {
	dir := t.TempDir()
	em := NewEpisodicMemory(dir, 100, 0)

	// Record some episodes with high turn counts (suggesting difficulty)
	for i := 0; i < 5; i++ {
		em.RecordSuccess(JournalEntry{
			TaskFocus:   "debug memory leak in server",
			Provider:    "claude",
			Model:       "opus",
			SpentUSD:    2.50,
			TurnCount:   25,
			DurationSec: 300,
			Worked:      []string{"identified leak", "fixed allocation"},
			ExitReason:  "completed",
		})
	}

	cs := NewCurriculumSorter(nil, em)

	// Task similar to stored episodes should score higher difficulty
	hardTask := LoopTask{Title: "debug memory leak in parser", Prompt: "Fix the memory leak in the parser server component"}
	hardDiff := cs.ScoreTask(hardTask)

	// Task unrelated to episodes should score differently
	easyTask := LoopTask{Title: "add unit test", Prompt: "Add a simple test"}
	easyDiff := cs.ScoreTask(easyTask)

	if hardDiff.DifficultyScore <= easyDiff.DifficultyScore {
		t.Errorf("expected hard task (%.2f) to score higher than easy task (%.2f)",
			hardDiff.DifficultyScore, easyDiff.DifficultyScore)
	}
}

// TestReflexionTriggeredByUncertainty verifies that low-confidence signals
// correctly trigger reflexion via ShouldTriggerReflexion.
func TestReflexionTriggeredByUncertainty(t *testing.T) {
	thresholds := DefaultConfidenceThresholds()

	// Low confidence signals
	low := ConfidenceSignals{
		Overall:        0.15,
		TurnEfficiency: 0.2,
		ErrorFree:      false,
		VerifyPassed:   false,
		HedgeCount:     8,
		QuestionCount:  4,
	}
	if !ShouldTriggerReflexion(low, thresholds) {
		t.Error("expected reflexion trigger for low-confidence signals")
	}

	// Medium confidence signals
	med := ConfidenceSignals{
		Overall:        0.55,
		TurnEfficiency: 0.7,
		ErrorFree:      true,
		VerifyPassed:   false,
		HedgeCount:     2,
		QuestionCount:  1,
	}
	if ShouldTriggerReflexion(med, thresholds) {
		t.Error("did not expect reflexion trigger for medium-confidence signals")
	}
}

// TestEpisodicAdapterSatisfiesCurriculumInterface verifies that
// EpisodicMemory.FindSimilarEpisodes implements EpisodicSource.
func TestEpisodicAdapterSatisfiesCurriculumInterface(t *testing.T) {
	dir := t.TempDir()
	em := NewEpisodicMemory(dir, 100, 0)
	em.RecordSuccess(JournalEntry{
		TaskFocus:   "add validation",
		Provider:    "claude",
		Model:       "opus",
		SpentUSD:    0.30,
		TurnCount:   4,
		DurationSec: 60,
		Worked:      []string{"added input validation"},
		ExitReason:  "completed",
	})

	// Verify it satisfies the interface
	var src EpisodicSource = em
	results := src.FindSimilarEpisodes("feature", "add validation", 3)
	if len(results) == 0 {
		t.Fatal("expected at least one CurriculumEpisode from adapter")
	}
	if results[0].TurnCount != 4 {
		t.Errorf("expected TurnCount=4, got %d", results[0].TurnCount)
	}
}

// TestManagerSubsystemSetters verifies all new setter methods work.
func TestManagerSubsystemSetters(t *testing.T) {
	m := NewManager()
	m.SetStateDir(t.TempDir())

	dir := t.TempDir()
	rs := NewReflexionStore(dir)
	em := NewEpisodicMemory(dir, 100, 0)
	cr := NewCascadeRouter(DefaultCascadeConfig(), nil, nil, dir)
	cs := NewCurriculumSorter(nil, nil)

	m.SetReflexionStore(rs)
	m.SetEpisodicMemory(em)
	m.SetCascadeRouter(cr)
	m.SetCurriculumSorter(cs)

	// Verify fields are set (access through the struct)
	if m.reflexion != rs {
		t.Error("reflexion store not set")
	}
	if m.episodic != em {
		t.Error("episodic memory not set")
	}
	if m.cascade != cr {
		t.Error("cascade router not set")
	}
	if m.curriculum != cs {
		t.Error("curriculum sorter not set")
	}
}

// ---------------------------------------------------------------------------
// Edge case tests: Reflexion
// ---------------------------------------------------------------------------

func TestReflexion_EmptyErrorString(t *testing.T) {
	rs := NewReflexionStore(t.TempDir())
	iter := LoopIteration{
		Status: "failed",
		Error:  "",
		Verification: []LoopVerification{
			{Status: "failed", ExitCode: 1, Output: ""},
		},
	}
	r := rs.ExtractReflection("loop-1", iter)
	if r == nil {
		t.Fatal("expected reflection even with empty error")
	}
	if r.FailureMode != "verify_failed" {
		t.Errorf("expected verify_failed, got %s", r.FailureMode)
	}
}

func TestReflexion_LongErrorOutput(t *testing.T) {
	rs := NewReflexionStore(t.TempDir())
	// 20KB error output
	longErr := ""
	for i := 0; i < 200; i++ {
		longErr += "error: something went wrong at line " + string(rune('0'+i%10)) + "\n"
	}
	iter := LoopIteration{
		Status:       "failed",
		Error:        longErr,
		WorkerOutput: longErr,
	}
	r := rs.ExtractReflection("loop-1", iter)
	if r == nil {
		t.Fatal("expected reflection for long error output")
	}
	// Correction should not be excessively long
	if len(r.Correction) > 1000 {
		t.Errorf("correction too long: %d chars", len(r.Correction))
	}
}

func TestReflexion_RecentForTaskNoMatch(t *testing.T) {
	rs := NewReflexionStore(t.TempDir())
	rs.Store(Reflection{
		Timestamp: time.Now(),
		LoopID:    "loop-1",
		TaskTitle: "completely unrelated task about databases",
	})
	results := rs.RecentForTask("fix CSS styling in header component", 5)
	// "completely unrelated task about databases" has no keyword overlap with "fix CSS styling..."
	// depending on implementation, this might or might not match
	// The point is it doesn't panic
	_ = results
}

// ---------------------------------------------------------------------------
// Edge case tests: Episodic Memory
// ---------------------------------------------------------------------------

func TestEpisodicMemory_MaxSizeOne(t *testing.T) {
	dir := t.TempDir()
	em := NewEpisodicMemory(dir, 1, 0)

	em.RecordSuccess(JournalEntry{
		TaskFocus: "task one", Provider: "claude", TurnCount: 3,
		Worked: []string{"done"}, ExitReason: "completed",
	})
	em.RecordSuccess(JournalEntry{
		TaskFocus: "task two", Provider: "claude", TurnCount: 5,
		Worked: []string{"done"}, ExitReason: "completed",
	})

	em.mu.Lock()
	count := len(em.episodes)
	em.mu.Unlock()

	if count > 1 {
		t.Errorf("expected maxSize=1 to prune to 1, got %d", count)
	}
}

func TestEpisodicMemory_FindSimilarEmpty(t *testing.T) {
	em := NewEpisodicMemory(t.TempDir(), 100, 0)
	results := em.FindSimilar("feature", "add something", 3)
	if len(results) != 0 {
		t.Errorf("expected empty results from empty store, got %d", len(results))
	}
}

func TestEpisodicMemory_AllSameType(t *testing.T) {
	dir := t.TempDir()
	em := NewEpisodicMemory(dir, 5, 0)

	for i := 0; i < 10; i++ {
		em.RecordSuccess(JournalEntry{
			TaskFocus: "add test", Provider: "claude", TurnCount: 3,
			Worked: []string{"added"}, ExitReason: "completed",
		})
	}

	em.mu.Lock()
	count := len(em.episodes)
	em.mu.Unlock()

	if count > 5 {
		t.Errorf("expected pruning to 5, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Edge case tests: Cascade
// ---------------------------------------------------------------------------

func TestCascade_ThresholdZero(t *testing.T) {
	dir := t.TempDir()
	cr := NewCascadeRouter(CascadeConfig{
		CheapProvider:       ProviderGemini,
		ExpensiveProvider:   ProviderClaude,
		ConfidenceThreshold: 0.0, // never escalate on confidence
		MaxCheapBudgetUSD:   1.0,
		MaxCheapTurns:       20,
	}, nil, nil, dir)

	sess := &Session{Status: StatusCompleted, TurnCount: 5}
	verify := []LoopVerification{{Status: "passed", ExitCode: 0}}
	escalate, _, _ := cr.EvaluateCheapResult(sess, 5, verify)
	if escalate {
		t.Error("threshold=0.0 should never escalate on confidence")
	}
}

func TestCascade_ThresholdOne(t *testing.T) {
	dir := t.TempDir()
	cr := NewCascadeRouter(CascadeConfig{
		CheapProvider:       ProviderGemini,
		ExpensiveProvider:   ProviderClaude,
		ConfidenceThreshold: 1.0, // extremely high threshold
		MaxCheapBudgetUSD:   1.0,
		MaxCheapTurns:       20,
	}, nil, nil, dir)

	// Session with some hedging language should not reach 1.0
	sess := &Session{
		Status:    StatusCompleted,
		TurnCount: 5,
		LastOutput: "I think this might work but I'm not sure",
	}
	verify := []LoopVerification{{Status: "passed", ExitCode: 0}}
	escalate, _, _ := cr.EvaluateCheapResult(sess, 5, verify)
	if !escalate {
		t.Error("threshold=1.0 with hedging output should escalate")
	}
}

func TestCascade_NilDecisionLog(t *testing.T) {
	dir := t.TempDir()
	cr := NewCascadeRouter(DefaultCascadeConfig(), nil, nil, dir)
	// Should not panic
	if !cr.ShouldCascade("feature", "add a thing") {
		t.Error("expected ShouldCascade=true with nil feedback")
	}
}

// ---------------------------------------------------------------------------
// Edge case tests: Uncertainty
// ---------------------------------------------------------------------------

func TestUncertainty_NilOutputHistory(t *testing.T) {
	sess := &Session{
		Status:        StatusCompleted,
		TurnCount:     5,
		OutputHistory: nil,
	}
	signals := ExtractConfidence(sess, 5, []LoopVerification{{Status: "passed", ExitCode: 0}})
	if signals.Overall < 0 || signals.Overall > 1 {
		t.Errorf("Overall out of range: %.2f", signals.Overall)
	}
	if signals.HedgeCount != 0 {
		t.Errorf("expected 0 hedges for nil output, got %d", signals.HedgeCount)
	}
}

func TestUncertainty_MixedVerification(t *testing.T) {
	sess := &Session{Status: StatusCompleted, TurnCount: 5}
	mixed := []LoopVerification{
		{Status: "passed", ExitCode: 0},
		{Status: "failed", ExitCode: 1},
	}
	signals := ExtractConfidence(sess, 5, mixed)
	if signals.VerifyPassed {
		t.Error("expected VerifyPassed=false with mixed verification")
	}
}

// ---------------------------------------------------------------------------
// Edge case tests: Curriculum
// ---------------------------------------------------------------------------

func TestCurriculum_EmptyPrompt(t *testing.T) {
	cs := NewCurriculumSorter(nil, nil)
	task := LoopTask{Title: "do something", Prompt: ""}
	diff := cs.ScoreTask(task)
	if diff.DifficultyScore < 0 || diff.DifficultyScore > 1 {
		t.Errorf("score out of range for empty prompt: %.2f", diff.DifficultyScore)
	}
}

func TestCurriculum_WhitespaceTitle(t *testing.T) {
	cs := NewCurriculumSorter(nil, nil)
	task := LoopTask{Title: "   ", Prompt: "some prompt"}
	diff := cs.ScoreTask(task)
	if diff.DifficultyScore < 0 || diff.DifficultyScore > 1 {
		t.Errorf("score out of range for whitespace title: %.2f", diff.DifficultyScore)
	}
}

func TestCurriculum_SortStability(t *testing.T) {
	cs := NewCurriculumSorter(nil, nil)
	tasks := []LoopTask{
		{Title: "add test A", Prompt: "add test A"},
		{Title: "add test B", Prompt: "add test B"},
		{Title: "add test C", Prompt: "add test C"},
	}
	sorted := cs.SortTasks(tasks)
	if len(sorted) != len(tasks) {
		t.Errorf("sort changed length: %d → %d", len(tasks), len(sorted))
	}
	// Original should not be mutated
	if tasks[0].Title != "add test A" {
		t.Error("SortTasks mutated input slice")
	}
}

// ---------------------------------------------------------------------------
// Persistence round-trip tests
// ---------------------------------------------------------------------------

func TestReflexionStore_ConcurrentAccess(t *testing.T) {
	dir := t.TempDir()
	rs := NewReflexionStore(dir)

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			rs.Store(Reflection{
				Timestamp:   time.Now(),
				LoopID:      "loop-1",
				TaskTitle:   "concurrent task",
				FailureMode: "test",
				RootCause:   "test",
			})
			_ = rs.RecentForTask("concurrent task", 5)
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify file is readable
	data, err := os.ReadFile(filepath.Join(dir, "reflections.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty reflections file after concurrent writes")
	}
}

func TestCascadeRouter_Persistence(t *testing.T) {
	dir := t.TempDir()
	cr := NewCascadeRouter(DefaultCascadeConfig(), nil, nil, dir)
	cr.RecordResult(CascadeResult{
		Timestamp:    time.Now(),
		UsedProvider: ProviderGemini,
		Escalated:    false,
		TotalCostUSD: 0.10,
	})
	cr.RecordResult(CascadeResult{
		Timestamp:    time.Now(),
		UsedProvider: ProviderClaude,
		Escalated:    true,
		CheapCostUSD: 0.05,
		TotalCostUSD: 0.45,
		Reason:       "low_confidence",
	})

	// Reload
	cr2 := NewCascadeRouter(DefaultCascadeConfig(), nil, nil, dir)
	stats := cr2.Stats()
	if stats.TotalDecisions != 2 {
		t.Errorf("expected 2 decisions after reload, got %d", stats.TotalDecisions)
	}
	if stats.Escalations != 1 {
		t.Errorf("expected 1 escalation, got %d", stats.Escalations)
	}
}

// ---------------------------------------------------------------------------
// Property / invariant tests
// ---------------------------------------------------------------------------

func TestProperty_JaccardSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		fn   func(float64) bool
		desc string
	}{
		{"identical", "foo bar baz", "foo bar baz", func(v float64) bool { return v == 1.0 }, "identical strings must yield 1.0"},
		{"empty_both", "", "", func(v float64) bool { return v == 0.0 }, "empty strings must yield 0.0"},
		{"empty_one", "hello world", "", func(v float64) bool { return v == 0.0 }, "one empty must yield 0.0"},
		{"disjoint", "alpha beta", "gamma delta", func(v float64) bool { return v == 0.0 }, "no overlap must yield 0.0"},
		{"subset", "a b", "a b c d", func(v float64) bool { return v > 0.0 && v <= 1.0 }, "subset must yield (0, 1]"},
		{"symmetric", "x y z", "y z w", func(v float64) bool {
			return jaccardSimilarity("x y z", "y z w") == jaccardSimilarity("y z w", "x y z")
		}, "must be symmetric"},
		{"bounded", "the quick brown fox", "lazy dog jumps over", func(v float64) bool { return v >= 0.0 && v <= 1.0 }, "must be in [0, 1]"},
		{"case_insensitive", "Hello World", "hello world", func(v float64) bool { return v == 1.0 }, "case should not matter"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := jaccardSimilarity(tc.a, tc.b)
			if !tc.fn(result) {
				t.Errorf("jaccardSimilarity(%q, %q) = %f: %s", tc.a, tc.b, result, tc.desc)
			}
		})
	}
}

func TestProperty_ExtractConfidence(t *testing.T) {
	// Invariant: Overall is always in [0.0, 1.0]
	cases := []struct {
		name   string
		verify bool
		output string
		turns  int
		errMsg string
	}{
		{"perfect", true, "Done. All tests pass.", 3, ""},
		{"hedging", false, "I think maybe this might possibly work but I'm not sure", 20, ""},
		{"error", false, "", 0, "segfault"},
		{"empty", false, "", 0, ""},
		{"questions", false, "Should I use slog? What about the interface?", 1, ""},
		{"long_output", true, string(make([]byte, 10000)), 5, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var verification []LoopVerification
			if tc.verify {
				verification = []LoopVerification{{ExitCode: 0}}
			} else {
				verification = []LoopVerification{{ExitCode: 1}}
			}
			signals := ExtractConfidence(&Session{
				Status:    StatusCompleted,
				TurnCount: tc.turns,
				Error:     tc.errMsg,
				LastOutput: tc.output,
			}, 5, verification)

			if signals.Overall < 0.0 || signals.Overall > 1.0 {
				t.Errorf("Overall = %f, must be in [0, 1]", signals.Overall)
			}
			if signals.TurnEfficiency < 0.0 {
				t.Errorf("TurnEfficiency = %f, must be >= 0", signals.TurnEfficiency)
			}
		})
	}

	// Invariant: verify_passed=true always scores higher than verify_passed=false
	// for otherwise identical sessions.
	sess := &Session{Status: StatusCompleted, TurnCount: 5, LastOutput: "done"}
	passedSignals := ExtractConfidence(sess, 5, []LoopVerification{{ExitCode: 0}})
	failedSignals := ExtractConfidence(sess, 5, []LoopVerification{{ExitCode: 1}})
	if passedSignals.Overall <= failedSignals.Overall {
		t.Errorf("verify_passed=true (%f) should score higher than false (%f)",
			passedSignals.Overall, failedSignals.Overall)
	}
}

func TestProperty_ScoreTaskDifficulty(t *testing.T) {
	cs := NewCurriculumSorter(nil, nil)

	cases := []struct {
		name  string
		title string
		prompt string
	}{
		{"simple_test", "Add test for utils", "Write a unit test"},
		{"complex_redesign", "Architecture redesign of the entire system", "Rewrite the complex core system with breaking changes across multiple files"},
		{"fix_typo", "Fix simple typo in README", "rename"},
		{"empty", "", ""},
		{"long_prompt", "Feature", string(make([]byte, 2000))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			td := cs.ScoreTask(LoopTask{Title: tc.title, Prompt: tc.prompt})

			// Invariant: DifficultyScore in [0, 1]
			if td.DifficultyScore < 0.0 || td.DifficultyScore > 1.0 {
				t.Errorf("DifficultyScore = %f, must be in [0, 1]", td.DifficultyScore)
			}

			// Invariant: Recommendation is always set
			switch td.Recommendation {
			case "cheap_provider", "expensive_provider", "decompose":
				// OK
			default:
				t.Errorf("unexpected recommendation: %q", td.Recommendation)
			}
		})
	}

	// Invariant: "simple typo rename" should score easier than "complex architecture redesign"
	easy := cs.ScoreTask(LoopTask{Title: "Fix simple typo", Prompt: "rename a variable"})
	hard := cs.ScoreTask(LoopTask{Title: "Architecture redesign", Prompt: "Rewrite the complex core with breaking changes across multiple files"})
	if easy.DifficultyScore >= hard.DifficultyScore {
		t.Errorf("easy (%f) should score lower than hard (%f)", easy.DifficultyScore, hard.DifficultyScore)
	}
}

func TestProperty_SortTasksStability(t *testing.T) {
	cs := NewCurriculumSorter(nil, nil)

	// Tasks with identical difficulty should preserve original order (stable sort).
	tasks := []LoopTask{
		{Title: "Test A", Prompt: "write test"},
		{Title: "Test B", Prompt: "write test"},
		{Title: "Test C", Prompt: "write test"},
	}

	sorted := cs.SortTasks(tasks)
	if len(sorted) != len(tasks) {
		t.Fatalf("sorted length %d != input length %d", len(sorted), len(tasks))
	}

	// Same difficulty → order preserved
	for i, s := range sorted {
		if s.Title != tasks[i].Title {
			t.Errorf("position %d: got %q, want %q (sort not stable)", i, s.Title, tasks[i].Title)
		}
	}
}

func TestProperty_ReflexionExtractNilOnSuccess(t *testing.T) {
	rs := NewReflexionStore("")

	// Invariant: ExtractReflection returns nil for non-failed iterations.
	for _, status := range []string{"idle", "planning", "executing", "verifying"} {
		iter := LoopIteration{Number: 1, Status: status, Task: LoopTask{Title: "test"}}
		if ref := rs.ExtractReflection("loop-1", iter); ref != nil {
			t.Errorf("status=%q: expected nil reflection, got %+v", status, ref)
		}
	}

	// Invariant: ExtractReflection returns non-nil for failed iterations.
	failedIter := LoopIteration{
		Number: 1,
		Status: "failed",
		Error:  "verify command failed",
		Task:   LoopTask{Title: "fix bug"},
	}
	if ref := rs.ExtractReflection("loop-1", failedIter); ref == nil {
		t.Error("expected non-nil reflection for failed iteration")
	}
}

func TestProperty_EpisodicPruneNeverExceedsMax(t *testing.T) {
	maxSize := 5
	em := NewEpisodicMemory("", maxSize, 0)

	// Add more episodes than maxSize
	for i := 0; i < maxSize*3; i++ {
		em.RecordSuccess(JournalEntry{
			Provider:  "claude",
			TaskFocus: "task",
			Worked:    []string{"ok"},
		})
	}

	em.mu.Lock()
	count := len(em.episodes)
	em.mu.Unlock()

	if count > maxSize {
		t.Errorf("episode count %d exceeds maxSize %d after pruning", count, maxSize)
	}
}
