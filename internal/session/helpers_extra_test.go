package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestClassifyDiffPaths_MixedPaths(t *testing.T) {
	paths := []string{
		"internal/session/loop.go",
		"internal/tui/views/main.go",
		"internal/session/manager.go",
		"README.md",
		"internal/session/selftest.go",
	}

	safe, needsReview := ClassifyDiffPaths(paths)

	if len(safe) != 2 {
		t.Errorf("safe count = %d, want 2, got: %v", len(safe), safe)
	}
	if len(needsReview) != 3 {
		t.Errorf("needsReview count = %d, want 3, got: %v", len(needsReview), needsReview)
	}
}

func TestClassifyDiffPaths_AllSafePaths(t *testing.T) {
	paths := []string{
		"internal/tui/views/main.go",
		"docs/README.md",
		"go.mod",
	}

	safe, needsReview := ClassifyDiffPaths(paths)
	if len(safe) != 3 {
		t.Errorf("safe count = %d, want 3", len(safe))
	}
	if len(needsReview) != 0 {
		t.Errorf("needsReview count = %d, want 0", len(needsReview))
	}
}

func TestClassifyDiffPaths_AllForbidden(t *testing.T) {
	paths := []string{
		"internal/session/loop.go",
		"internal/session/manager.go",
	}

	safe, needsReview := ClassifyDiffPaths(paths)
	if len(safe) != 0 {
		t.Errorf("safe count = %d, want 0", len(safe))
	}
	if len(needsReview) != 2 {
		t.Errorf("needsReview count = %d, want 2", len(needsReview))
	}
}

func TestClassifyDiffPaths_NilInput(t *testing.T) {
	safe, needsReview := ClassifyDiffPaths(nil)
	if len(safe) != 0 || len(needsReview) != 0 {
		t.Errorf("expected both empty for nil input")
	}
}

func TestDecisionLog_AppendToFile(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelAutoOptimize)

	// Propose a decision that should be executed
	allowed := dl.Propose(AutonomousDecision{
		Category:      DecisionRestart,
		RequiredLevel: LevelAutoRecover,
		Rationale:     "test decision",
		Action:        "restart session",
	})
	if !allowed {
		t.Error("expected decision to be allowed at level 2 (requires level 1)")
	}

	// Verify the JSONL file was written
	path := filepath.Join(dir, "decisions.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), "test decision") {
		t.Error("expected decision rationale in file")
	}
}

func TestDecisionLog_LoadRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Create a decision log and add some decisions
	dl := NewDecisionLog(dir, LevelFullAutonomy)
	dl.Propose(AutonomousDecision{
		ID:            "dec-1",
		Category:      DecisionRestart,
		RequiredLevel: LevelAutoRecover,
		Rationale:     "first decision",
		Action:        "restart",
	})
	dl.Propose(AutonomousDecision{
		ID:            "dec-2",
		Category:      DecisionBudgetAdjust,
		RequiredLevel: LevelAutoOptimize,
		Rationale:     "second decision",
		Action:        "adjust budget",
	})

	// Create a new decision log from the same directory — should load from file
	dl2 := NewDecisionLog(dir, LevelFullAutonomy)
	recent := dl2.Recent(10)
	if len(recent) != 2 {
		t.Fatalf("expected 2 decisions loaded, got %d", len(recent))
	}
	if recent[0].ID != "dec-1" {
		t.Errorf("decision[0].ID = %q, want dec-1", recent[0].ID)
	}
}

func TestDecisionLog_EmptyStateDir(t *testing.T) {
	dl := NewDecisionLog("", LevelObserve)

	// Propose should not panic with empty state dir
	dl.Propose(AutonomousDecision{
		Category:      DecisionRestart,
		RequiredLevel: LevelObserve,
		Rationale:     "no-op",
		Action:        "nothing",
	})

	recent := dl.Recent(10)
	if len(recent) != 1 {
		t.Errorf("expected 1 decision, got %d", len(recent))
	}
}

func TestFeedbackAnalyzer_SaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()

	fa := NewFeedbackAnalyzer(dir, 1)
	fa.Ingest([]JournalEntry{
		{
			Provider:   "claude",
			TaskFocus:  "add unit tests",
			SpentUSD:   0.5,
			TurnCount:  10,
			ExitReason: "completed",
		},
		{
			Provider:   "gemini",
			TaskFocus:  "add integration tests",
			SpentUSD:   0.2,
			TurnCount:  5,
			ExitReason: "completed",
		},
	})

	// Load into a new analyzer
	fa2 := NewFeedbackAnalyzer(dir, 1)
	profiles := fa2.AllPromptProfiles()
	if len(profiles) == 0 {
		t.Fatal("expected loaded profiles to be non-empty")
	}

	// Check test task type exists
	p, ok := fa2.GetPromptProfile("test")
	if !ok {
		t.Fatal("expected 'test' profile to exist")
	}
	if p.SampleCount != 2 {
		t.Errorf("sample count = %d, want 2", p.SampleCount)
	}
}

func TestFeedbackAnalyzer_EmptyStateDir(t *testing.T) {
	fa := NewFeedbackAnalyzer("", 5)
	// Should not panic
	fa.Ingest([]JournalEntry{{Provider: "claude", TaskFocus: "test thing"}})
	profiles := fa.AllPromptProfiles()
	if len(profiles) == 0 {
		t.Error("expected at least one profile after ingest")
	}
}

func TestSuggestEnhancementMode_NoData(t *testing.T) {
	fa := NewFeedbackAnalyzer("", 5)
	mode := fa.SuggestEnhancementMode("test")
	if mode != "auto" {
		t.Errorf("mode = %q, want auto (no data)", mode)
	}
}

func TestSuggestEnhancementMode_LocalOnly(t *testing.T) {
	fa := NewFeedbackAnalyzer("", 1)
	// Inject local enhancement data directly
	fa.mu.Lock()
	fa.enhancementProfiles["local:test"] = &EnhancementProfile{
		Source:        "local",
		TaskType:      "test",
		SampleCount:   5,
		Effectiveness: 10.0,
	}
	fa.mu.Unlock()

	mode := fa.SuggestEnhancementMode("test")
	if mode != "local" {
		t.Errorf("mode = %q, want local (only local data)", mode)
	}
}

func TestSuggestEnhancementMode_LLMOnly(t *testing.T) {
	fa := NewFeedbackAnalyzer("", 1)
	fa.mu.Lock()
	fa.enhancementProfiles["llm:test"] = &EnhancementProfile{
		Source:        "llm",
		TaskType:      "test",
		SampleCount:   5,
		Effectiveness: 10.0,
	}
	fa.mu.Unlock()

	mode := fa.SuggestEnhancementMode("test")
	if mode != "llm" {
		t.Errorf("mode = %q, want llm (only llm data)", mode)
	}
}

func TestSuggestEnhancementMode_BothAutoTie(t *testing.T) {
	fa := NewFeedbackAnalyzer("", 1)
	fa.mu.Lock()
	fa.enhancementProfiles["local:test"] = &EnhancementProfile{
		Source: "local", TaskType: "test", SampleCount: 5, Effectiveness: 10.0,
	}
	fa.enhancementProfiles["llm:test"] = &EnhancementProfile{
		Source: "llm", TaskType: "test", SampleCount: 5, Effectiveness: 10.0,
	}
	fa.mu.Unlock()

	mode := fa.SuggestEnhancementMode("test")
	if mode != "auto" {
		t.Errorf("mode = %q, want auto (tied effectiveness)", mode)
	}
}

func TestClassifyTask_Categories(t *testing.T) {
	tests := []struct {
		focus string
		want  string
	}{
		{"refactor the auth module", "refactor"},
		{"add unit tests for parser", "test"},
		{"write documentation for API", "docs"},
		{"deploy to staging", "config"},
		{"review PR #123", "review"},
		{"optimize database queries", "optimization"},
		{"fix broken login page", "bug_fix"},
		{"implement new feature X", "feature"},
		{"something completely unrelated", "general"},
	}
	for _, tt := range tests {
		got := classifyTask(tt.focus)
		if got != tt.want {
			t.Errorf("classifyTask(%q) = %q, want %q", tt.focus, got, tt.want)
		}
	}
}

func TestDedupeStrings(t *testing.T) {
	input := []string{"a", "b", "a", "c", "", "  ", "b"}
	got := dedupeStrings(input)
	if len(got) != 3 {
		t.Errorf("dedupeStrings returned %d items, want 3: %v", len(got), got)
	}
}

func TestNormalizeLoopProfile_Defaults(t *testing.T) {
	profile := LoopProfile{} // all defaults
	got, err := normalizeLoopProfile(profile)
	if err != nil {
		t.Fatalf("normalizeLoopProfile: %v", err)
	}
	if got.PlannerProvider == "" {
		t.Error("expected PlannerProvider to be filled")
	}
	if got.WorkerProvider == "" {
		t.Error("expected WorkerProvider to be filled")
	}
	if got.MaxConcurrentWorkers <= 0 {
		t.Error("expected MaxConcurrentWorkers > 0")
	}
}

func TestNormalizeLoopProfile_InvalidRetryLimit(t *testing.T) {
	profile := LoopProfile{RetryLimit: -1}
	_, err := normalizeLoopProfile(profile)
	if err == nil {
		t.Error("expected error for negative retry limit")
	}
}

func TestNormalizeLoopProfile_ExcessiveWorkers(t *testing.T) {
	profile := LoopProfile{MaxConcurrentWorkers: 20}
	_, err := normalizeLoopProfile(profile)
	if err == nil {
		t.Error("expected error for >8 concurrent workers")
	}
}

func TestNormalizeLoopProfile_BadWorktreePolicy(t *testing.T) {
	profile := LoopProfile{WorktreePolicy: "docker"}
	_, err := normalizeLoopProfile(profile)
	if err == nil {
		t.Error("expected error for unsupported worktree policy")
	}
}

func TestNormalizeLoopProfile_CompactionThreshold(t *testing.T) {
	profile := LoopProfile{CompactionEnabled: true}
	got, err := normalizeLoopProfile(profile)
	if err != nil {
		t.Fatalf("normalizeLoopProfile: %v", err)
	}
	if got.CompactionThreshold != 10 {
		t.Errorf("CompactionThreshold = %d, want 10 (default when enabled)", got.CompactionThreshold)
	}
}

func TestWriteAndReadImprovementNote(t *testing.T) {
	dir := t.TempDir()

	note := ImprovementNote{
		ID:          "note-1",
		Timestamp:   time.Now(),
		Category:    "config",
		Priority:    2,
		Title:       "test note",
		Description: "a test improvement note",
		Source:      "test",
		AutoApply:   true,
		Status:      "pending",
	}

	if err := WriteImprovementNote(dir, note); err != nil {
		t.Fatalf("WriteImprovementNote: %v", err)
	}

	notes, err := ReadPendingNotes(dir)
	if err != nil {
		t.Fatalf("ReadPendingNotes: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].ID != "note-1" {
		t.Errorf("note ID = %q, want note-1", notes[0].ID)
	}
}

func TestReadPendingNotes_NotFound(t *testing.T) {
	dir := t.TempDir()
	notes, err := ReadPendingNotes(dir)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if notes != nil {
		t.Errorf("expected nil notes, got: %v", notes)
	}
}

func TestSanitizeTaskTitle_OutputPrefixes(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"All tests pass", "self-improvement iteration"},
		{"I've completed the task", "self-improvement iteration"},
		{"Successfully deployed", "self-improvement iteration"},
		{"Normal task title", "Normal task title"},
	}
	for _, tt := range tests {
		got := sanitizeTaskTitle(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeTaskTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeTaskTitle_JSONObject(t *testing.T) {
	input := `{"title": "my task", "prompt": "do it"}`
	got := sanitizeTaskTitle(input)
	if got != "my task" {
		t.Errorf("sanitizeTaskTitle(json) = %q, want 'my task'", got)
	}
}

func TestSanitizeTaskTitle_Multiline(t *testing.T) {
	input := "First line\nSecond line\nThird line"
	got := sanitizeTaskTitle(input)
	if got != "First line" {
		t.Errorf("sanitizeTaskTitle(multiline) = %q, want 'First line'", got)
	}
}

func TestSanitizeTaskTitle_Truncate(t *testing.T) {
	input := strings.Repeat("x", 200)
	got := sanitizeTaskTitle(input)
	if len(got) != 120 {
		t.Errorf("sanitizeTaskTitle length = %d, want 120", len(got))
	}
}

func TestSanitizeTaskTitle_FencedJSON(t *testing.T) {
	input := "```json\n{\"title\": \"fenced task\"}\n```"
	got := sanitizeTaskTitle(input)
	if got != "fenced task" {
		t.Errorf("sanitizeTaskTitle(fenced) = %q, want 'fenced task'", got)
	}
}

func TestExpandHome(t *testing.T) {
	// Non-home path should remain unchanged
	got := expandHome("/tmp/sessions")
	if got != "/tmp/sessions" {
		t.Errorf("expandHome('/tmp/sessions') = %q", got)
	}

	// Home path should expand
	got = expandHome("~/test")
	if strings.HasPrefix(got, "~/") {
		t.Errorf("expandHome('~/test') should expand home, got %q", got)
	}
}

func TestPersistOrWarn(t *testing.T) {
	m := NewManager()
	m.SetStateDir("")

	s := &Session{ID: "warn-test"}
	// Should not panic with empty state dir
	m.persistOrWarn(s, "test context")
}

func TestBuildEnhancementProfile_Effectiveness(t *testing.T) {
	entries := []JournalEntry{
		{SpentUSD: 0.5, ExitReason: "completed"},
		{SpentUSD: 0.3, ExitReason: "completed"},
		{SpentUSD: 0.8, ExitReason: "error"},
	}

	p := buildEnhancementProfile("local", "test", entries)
	if p.SampleCount != 3 {
		t.Errorf("sample count = %d, want 3", p.SampleCount)
	}
	if p.Source != "local" {
		t.Errorf("source = %q, want local", p.Source)
	}
	// 2 out of 3 completed = 66.7%
	if p.CompletionRate < 66.0 || p.CompletionRate > 67.0 {
		t.Errorf("completion rate = %.1f, want ~66.7", p.CompletionRate)
	}
}

func TestBuildProviderProfile(t *testing.T) {
	entries := []JournalEntry{
		{SpentUSD: 1.0, TurnCount: 5, ExitReason: "completed"},
		{SpentUSD: 2.0, TurnCount: 10, ExitReason: ""},
	}

	p := buildProviderProfile("claude", "feature", entries)
	if p.Provider != "claude" {
		t.Errorf("provider = %q, want claude", p.Provider)
	}
	if p.SampleCount != 2 {
		t.Errorf("sample count = %d, want 2", p.SampleCount)
	}
	if p.AvgCostUSD != 1.5 {
		t.Errorf("avg cost = %f, want 1.5", p.AvgCostUSD)
	}
	if p.CostPerTurn == 0 {
		t.Error("expected non-zero CostPerTurn")
	}
}

func TestAutoOptimizerGenerateNotes_NilPatterns(t *testing.T) {
	ao := NewAutoOptimizer(nil, nil, nil, nil)
	notes := ao.GenerateNotes(nil)
	if notes != nil {
		t.Errorf("expected nil notes for nil patterns, got %v", notes)
	}
}

func TestAutoOptimizerGenerateNotes_ProviderRule(t *testing.T) {
	ao := NewAutoOptimizer(nil, nil, nil, nil)
	patterns := &ConsolidatedPatterns{
		Rules: []string{
			"use gemini for test tasks",
		},
	}
	notes := ao.GenerateNotes(patterns)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Category != "config" {
		t.Errorf("category = %q, want config", notes[0].Category)
	}
	if !notes[0].AutoApply {
		t.Error("expected AutoApply=true for provider rule")
	}
}

func TestAutoOptimizerGenerateNotes_NegativePattern(t *testing.T) {
	ao := NewAutoOptimizer(nil, nil, nil, nil)
	patterns := &ConsolidatedPatterns{
		Negative: []ConsolidatedItem{
			{Text: "timeout on large repos", Count: 5, Category: "performance", LastSeen: time.Now()},
		},
	}
	notes := ao.GenerateNotes(patterns)
	if len(notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(notes))
	}
	if notes[0].Priority != 2 {
		t.Errorf("priority = %d, want 2", notes[0].Priority)
	}
}

func TestGateChange_Disabled(t *testing.T) {
	ao := NewAutoOptimizer(nil, nil, nil, nil)
	// GateEnabled is false by default
	change := GatedChange{
		ChangeType: "config",
	}
	result := ao.GateChange("/tmp/repo", change)
	if result.Verdict != "skip" {
		t.Errorf("verdict = %q, want skip (gate disabled)", result.Verdict)
	}
}

func TestApplyPendingNotes_NoFile(t *testing.T) {
	ao := NewAutoOptimizer(nil, nil, nil, nil)
	dir := t.TempDir()

	applied, rejected, err := ao.ApplyPendingNotes(dir)
	if err != nil {
		t.Fatalf("ApplyPendingNotes: %v", err)
	}
	if applied != 0 || rejected != 0 {
		t.Errorf("applied=%d, rejected=%d, want 0,0", applied, rejected)
	}
}

func TestApplyPendingNotes_WithNotes(t *testing.T) {
	dir := t.TempDir()

	// Write some notes
	note := ImprovementNote{
		ID:        "note-apply",
		Category:  "config",
		Title:     "use gemini for test tasks",
		AutoApply: true,
		Status:    "pending",
	}
	WriteImprovementNote(dir, note)

	// Write a non-auto-apply note
	note2 := ImprovementNote{
		ID:        "note-manual",
		Category:  "code",
		Title:     "needs manual review",
		AutoApply: false,
		Status:    "pending",
	}
	WriteImprovementNote(dir, note2)

	ao := NewAutoOptimizer(nil, nil, nil, nil)
	applied, rejected, err := ao.ApplyPendingNotes(dir)
	if err != nil {
		t.Fatalf("ApplyPendingNotes: %v", err)
	}

	// GateEnabled is false, so gate returns "skip" (not rolled back) => applied
	if applied != 1 {
		t.Errorf("applied = %d, want 1", applied)
	}
	if rejected != 0 {
		t.Errorf("rejected = %d, want 0", rejected)
	}

	// Verify the notes file was updated
	notes, _ := ReadPendingNotes(dir)
	for _, n := range notes {
		if n.ID == "note-apply" && n.Status != "applied" {
			t.Errorf("note-apply status = %q, want applied", n.Status)
		}
		if n.ID == "note-manual" && n.Status != "pending" {
			t.Errorf("note-manual status = %q, want pending", n.Status)
		}
	}
}

func TestWriteImprovementNotes_Overwrite(t *testing.T) {
	dir := t.TempDir()

	notes := []ImprovementNote{
		{ID: "n1", Title: "first", Status: "applied"},
		{ID: "n2", Title: "second", Status: "pending"},
	}

	if err := writeImprovementNotes(dir, notes); err != nil {
		t.Fatalf("writeImprovementNotes: %v", err)
	}

	loaded, err := ReadPendingNotes(dir)
	if err != nil {
		t.Fatalf("ReadPendingNotes: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 notes, got %d", len(loaded))
	}
}

func TestIngestSessionJournal(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)
	dl := NewDecisionLog("", LevelObserve)
	ao := NewAutoOptimizer(fa, dl, nil, nil)

	ended := time.Now()
	s := &Session{
		ID:         "journal-test",
		Provider:   ProviderClaude,
		RepoName:   "test-repo",
		Model:      "sonnet",
		SpentUSD:   1.5,
		TurnCount:  20,
		ExitReason: "completed",
		Prompt:     "add unit tests for auth",
		LaunchedAt: time.Now().Add(-5 * time.Minute),
		EndedAt:    &ended,
	}

	ao.IngestSessionJournal(s)

	// Check that profiles were updated
	profiles := fa.AllPromptProfiles()
	if len(profiles) == 0 {
		t.Error("expected profiles to be populated after ingest")
	}
}

func TestIngestSessionJournal_NilFeedback(t *testing.T) {
	ao := NewAutoOptimizer(nil, nil, nil, nil)
	s := &Session{ID: "nil-test"}
	// Should not panic
	ao.IngestSessionJournal(s)
}

func TestIngestSessionJournal_NilSession(t *testing.T) {
	ao := NewAutoOptimizer(nil, nil, nil, nil)
	// Should not panic
	ao.IngestSessionJournal(nil)
}

// ConsolidatedPatterns is needed for GenerateNotes — let's verify we can access it
func TestConsolidatedPatternsType(t *testing.T) {
	// Just verify the type is accessible and usable
	p := &ConsolidatedPatterns{
		Rules:    []string{"rule 1"},
		Negative: []ConsolidatedItem{{Text: "neg", Count: 1}},
	}
	_, _ = json.Marshal(p)
}
