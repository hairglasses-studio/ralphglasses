package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFeedbackAnalyzer_Ingest(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 2) // low min for testing

	entries := []JournalEntry{
		{Timestamp: time.Now(), Provider: "claude", TaskFocus: "fix broken login", SpentUSD: 1.5, TurnCount: 10, ExitReason: "completed"},
		{Timestamp: time.Now(), Provider: "claude", TaskFocus: "fix lint error", SpentUSD: 0.8, TurnCount: 5, ExitReason: "completed"},
		{Timestamp: time.Now(), Provider: "gemini", TaskFocus: "fix null pointer crash", SpentUSD: 0.3, TurnCount: 8, ExitReason: "completed"},
		{Timestamp: time.Now(), Provider: "claude", TaskFocus: "add new feature", SpentUSD: 5.0, TurnCount: 30, ExitReason: "completed"},
		{Timestamp: time.Now(), Provider: "gemini", TaskFocus: "add login feature", SpentUSD: 2.0, TurnCount: 20, ExitReason: "errored"},
	}

	fa.Ingest(entries)

	// All 3 "fix ..." entries classify as bug_fix
	bugProfile, ok := fa.GetPromptProfile("bug_fix")
	if !ok {
		t.Fatal("expected bug_fix prompt profile")
	}
	if bugProfile.SampleCount != 3 {
		t.Errorf("bug_fix samples: got %d, want 3", bugProfile.SampleCount)
	}
	if bugProfile.CompletionRate != 100 {
		t.Errorf("bug_fix completion: got %.0f%%, want 100%%", bugProfile.CompletionRate)
	}

	featureProfile, ok := fa.GetPromptProfile("feature")
	if !ok {
		t.Fatal("expected feature prompt profile")
	}
	if featureProfile.SampleCount != 2 {
		t.Errorf("feature samples: got %d, want 2", featureProfile.SampleCount)
	}
}

func TestFeedbackAnalyzer_SuggestProvider(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 2)

	entries := []JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 2.0, TurnCount: 10, ExitReason: "completed"},
		{Provider: "gemini", TaskFocus: "fix error", SpentUSD: 0.5, TurnCount: 5, ExitReason: "completed"},
		{Provider: "gemini", TaskFocus: "fix lint", SpentUSD: 0.3, TurnCount: 3, ExitReason: "completed"},
	}

	fa.Ingest(entries)

	provider, ok := fa.SuggestProvider("bug_fix")
	if !ok {
		t.Fatal("expected provider suggestion")
	}
	// Gemini should be suggested (cheaper with same completion rate)
	if provider != ProviderGemini {
		t.Errorf("got provider %q, want gemini (cheaper)", provider)
	}
}

func TestFeedbackAnalyzer_SuggestBudget(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 2)

	entries := []JournalEntry{
		{Provider: "claude", TaskFocus: "add feature", SpentUSD: 3.0, TurnCount: 15, ExitReason: "completed"},
		{Provider: "claude", TaskFocus: "add button", SpentUSD: 2.0, TurnCount: 10, ExitReason: "completed"},
	}

	fa.Ingest(entries)

	budget, ok := fa.SuggestBudget("feature")
	if !ok {
		t.Fatal("expected budget suggestion")
	}
	if budget <= 0 {
		t.Errorf("budget should be positive, got $%.2f", budget)
	}
}

func TestFeedbackAnalyzer_ProviderProfiles(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)

	entries := []JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 2.0, TurnCount: 10, ExitReason: "completed"},
		{Provider: "gemini", TaskFocus: "fix error", SpentUSD: 0.5, TurnCount: 8, ExitReason: "completed"},
	}

	fa.Ingest(entries)

	profile, ok := fa.GetProviderProfile("claude", "bug_fix")
	if !ok {
		t.Fatal("expected claude bug_fix profile")
	}
	if profile.AvgCostUSD != 2.0 {
		t.Errorf("avg cost: got $%.2f, want $2.00", profile.AvgCostUSD)
	}
	if profile.CostPerTurn != 0.2 {
		t.Errorf("cost per turn: got $%.3f, want $0.200", profile.CostPerTurn)
	}
}

func TestFeedbackAnalyzer_Persistence(t *testing.T) {
	dir := t.TempDir()

	fa1 := NewFeedbackAnalyzer(dir, 1)
	fa1.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 1.0, TurnCount: 5, ExitReason: "completed"},
	})

	// Verify file exists
	path := filepath.Join(dir, "feedback_profiles.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("profiles file not created: %v", err)
	}

	// Reload
	fa2 := NewFeedbackAnalyzer(dir, 1)
	profiles := fa2.AllPromptProfiles()
	if len(profiles) == 0 {
		t.Error("expected profiles after reload")
	}
}

func TestClassifyTask(t *testing.T) {
	tests := []struct {
		focus string
		want  string
	}{
		{"fix broken login", "bug_fix"},
		{"add new feature", "feature"},
		{"refactor auth module", "refactor"},
		{"write unit tests", "test"},
		{"update README", "docs"},
		{"configure CI/CD pipeline", "config"},
		{"review pull request", "review"},
		{"optimize database queries", "optimization"},
		{"something random", "general"},
	}
	for _, tt := range tests {
		got := classifyTask(tt.focus)
		if got != tt.want {
			t.Errorf("classifyTask(%q): got %q, want %q", tt.focus, got, tt.want)
		}
	}
}

func TestFeedbackAnalyzer_AllProviderProfiles(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)
	fa.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 0.50, TurnCount: 10, ExitReason: "completed"},
		{Provider: "gemini", TaskFocus: "fix crash", SpentUSD: 0.10, TurnCount: 5, ExitReason: "completed"},
	})

	profiles := fa.AllProviderProfiles()
	if len(profiles) == 0 {
		t.Fatal("expected non-empty provider profiles")
	}
	for _, p := range profiles {
		if p.Provider == "" {
			t.Error("empty provider in profile")
		}
	}
}

func TestFeedbackAnalyzer_SuggestEnhancementMode(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)

	// No data — returns auto
	if got := fa.SuggestEnhancementMode("bug_fix"); got != "auto" {
		t.Errorf("expected auto, got %s", got)
	}

	// Ingest local enhancement entries
	fa.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 0.10, ExitReason: "completed", EnhancementSource: "local"},
	})
	if got := fa.SuggestEnhancementMode("bug_fix"); got != "local" {
		t.Errorf("expected local with only local data, got %s", got)
	}
}

func TestFeedbackAnalyzer_SuggestEnhancementMode_Both(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)

	// Ingest both local and llm entries for the same task type
	fa.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 0.10, ExitReason: "completed", EnhancementSource: "local"},
		{Provider: "claude", TaskFocus: "fix crash", SpentUSD: 5.00, ExitReason: "errored", EnhancementSource: "llm"},
	})

	mode := fa.SuggestEnhancementMode("bug_fix")
	// local completed at low cost, llm errored at high cost -> local should win
	if mode != "local" {
		t.Errorf("expected local (better effectiveness), got %s", mode)
	}
}

func TestFeedbackAnalyzer_AllEnhancementProfiles(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)
	fa.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 0.10, ExitReason: "completed", EnhancementSource: "local"},
		{Provider: "claude", TaskFocus: "add feature", SpentUSD: 0.30, ExitReason: "completed", EnhancementSource: "llm"},
	})

	profiles := fa.AllEnhancementProfiles()
	if len(profiles) == 0 {
		t.Fatal("expected non-empty enhancement profiles")
	}
}

func TestFeedbackAnalyzer_SuggestEnhancementMode_Equal(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)

	// Both similar effectiveness => auto
	fa.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 0.10, ExitReason: "completed", EnhancementSource: "local"},
		{Provider: "claude", TaskFocus: "fix crash", SpentUSD: 0.10, ExitReason: "completed", EnhancementSource: "llm"},
	})

	mode := fa.SuggestEnhancementMode("bug_fix")
	// With similar stats, should return "auto"
	if mode != "auto" && mode != "local" && mode != "llm" {
		t.Errorf("unexpected mode: %s", mode)
	}
}

func TestFeedbackAnalyzer_SuggestEnhancementMode_OnlyLLM(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)

	fa.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 0.10, ExitReason: "completed", EnhancementSource: "llm"},
	})

	if got := fa.SuggestEnhancementMode("bug_fix"); got != "llm" {
		t.Errorf("expected llm with only llm data, got %s", got)
	}
}

func TestBuildEnhancementProfile(t *testing.T) {
	entries := []JournalEntry{
		{SpentUSD: 0.10, ExitReason: "completed"},
		{SpentUSD: 0.20, ExitReason: "errored"},
		{SpentUSD: 0.30, ExitReason: ""},
	}
	p := buildEnhancementProfile("local", "bug_fix", entries)
	if p.SampleCount != 3 {
		t.Errorf("SampleCount = %d, want 3", p.SampleCount)
	}
	if p.Source != "local" {
		t.Errorf("Source = %q, want local", p.Source)
	}
}

func TestFeedbackAnalyzer_MinSessions(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 5) // requires 5 sessions

	// Only 2 entries — should not be trusted
	fa.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 1.0},
		{Provider: "claude", TaskFocus: "fix error", SpentUSD: 2.0},
	})

	_, ok := fa.GetPromptProfile("bug_fix")
	if ok {
		t.Error("profile should not be trusted with only 2 samples (min 5)")
	}
}

// --- SeedProfilesFromObservations tests ---

func TestSeedProfilesFromObservations_Empty(t *testing.T) {
	t.Parallel()
	profiles, err := SeedProfilesFromObservations(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles.PromptProfiles) != 0 {
		t.Errorf("expected 0 prompt profiles, got %d", len(profiles.PromptProfiles))
	}
	if len(profiles.ProviderProfiles) != 0 {
		t.Errorf("expected 0 provider profiles, got %d", len(profiles.ProviderProfiles))
	}
}

func TestSeedProfilesFromObservations_SingleProvider(t *testing.T) {
	t.Parallel()
	obs := []LoopObservation{
		{WorkerProvider: "claude", TaskType: "bug_fix", TotalCostUSD: 1.0, TotalLatencyMs: 5000, VerifyPassed: true},
		{WorkerProvider: "claude", TaskType: "bug_fix", TotalCostUSD: 2.0, TotalLatencyMs: 3000, VerifyPassed: true},
	}
	profiles, err := SeedProfilesFromObservations(obs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles.ProviderProfiles) != 1 {
		t.Fatalf("expected 1 provider profile, got %d", len(profiles.ProviderProfiles))
	}
	pp := profiles.ProviderProfiles[0]
	if pp.Provider != "claude" {
		t.Errorf("provider = %q, want claude", pp.Provider)
	}
	if pp.SampleCount != 2 {
		t.Errorf("sample count = %d, want 2", pp.SampleCount)
	}
	if pp.CompletionRate != 100 {
		t.Errorf("completion rate = %.0f%%, want 100%%", pp.CompletionRate)
	}
}

func TestSeedProfilesFromObservations_MultiProvider(t *testing.T) {
	t.Parallel()
	obs := []LoopObservation{
		{WorkerProvider: "claude", TaskType: "bug_fix", TotalCostUSD: 2.0, VerifyPassed: true},
		{WorkerProvider: "gemini", TaskType: "bug_fix", TotalCostUSD: 0.5, VerifyPassed: true},
		{WorkerProvider: "gemini", TaskType: "feature", TotalCostUSD: 1.0, VerifyPassed: false},
	}
	profiles, err := SeedProfilesFromObservations(obs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles.ProviderProfiles) != 2 {
		t.Fatalf("expected 2 provider profiles, got %d", len(profiles.ProviderProfiles))
	}
	// Find gemini profile
	var gemini *ProviderProfile
	for i := range profiles.ProviderProfiles {
		if profiles.ProviderProfiles[i].Provider == "gemini" {
			gemini = &profiles.ProviderProfiles[i]
		}
	}
	if gemini == nil {
		t.Fatal("expected gemini provider profile")
	}
	if gemini.SampleCount != 2 {
		t.Errorf("gemini samples = %d, want 2", gemini.SampleCount)
	}
	if gemini.CompletionRate != 50 {
		t.Errorf("gemini completion = %.0f%%, want 50%%", gemini.CompletionRate)
	}
}

func TestSeedProfilesFromObservations_TaskTypeGrouping(t *testing.T) {
	t.Parallel()
	obs := []LoopObservation{
		{WorkerProvider: "claude", TaskType: "bug_fix", TotalCostUSD: 1.0, VerifyPassed: true},
		{WorkerProvider: "claude", TaskType: "bug_fix", TotalCostUSD: 3.0, VerifyPassed: false},
		{WorkerProvider: "claude", TaskType: "feature", TotalCostUSD: 5.0, VerifyPassed: true},
	}
	profiles, err := SeedProfilesFromObservations(obs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles.PromptProfiles) != 2 {
		t.Fatalf("expected 2 task profiles, got %d", len(profiles.PromptProfiles))
	}
	var bugFix *PromptProfile
	for i := range profiles.PromptProfiles {
		if profiles.PromptProfiles[i].TaskType == "bug_fix" {
			bugFix = &profiles.PromptProfiles[i]
		}
	}
	if bugFix == nil {
		t.Fatal("expected bug_fix prompt profile")
	}
	if bugFix.SampleCount != 2 {
		t.Errorf("bug_fix samples = %d, want 2", bugFix.SampleCount)
	}
	if bugFix.CompletionRate != 50 {
		t.Errorf("bug_fix completion = %.0f%%, want 50%%", bugFix.CompletionRate)
	}
}

func TestSeedProfilesFromObservations_SkipsMalformed(t *testing.T) {
	t.Parallel()
	obs := []LoopObservation{
		{WorkerProvider: "", PlannerProvider: "", TaskType: "bug_fix", TotalCostUSD: 1.0}, // no provider — skipped for provider profiles
		{WorkerProvider: "claude", TaskType: "feature", TotalCostUSD: 2.0, VerifyPassed: true},
	}
	profiles, err := SeedProfilesFromObservations(obs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only claude should appear in provider profiles (the empty-provider obs is skipped).
	if len(profiles.ProviderProfiles) != 1 {
		t.Fatalf("expected 1 provider profile, got %d", len(profiles.ProviderProfiles))
	}
	if profiles.ProviderProfiles[0].Provider != "claude" {
		t.Errorf("provider = %q, want claude", profiles.ProviderProfiles[0].Provider)
	}
	// But both show up in prompt profiles (task_type grouping uses "general" default).
	if len(profiles.PromptProfiles) != 2 {
		t.Errorf("expected 2 prompt profiles, got %d", len(profiles.PromptProfiles))
	}
}

func TestSeedProfilesFromObservations_AverageCalculations(t *testing.T) {
	t.Parallel()
	obs := []LoopObservation{
		{WorkerProvider: "claude", TaskType: "bug_fix", TotalCostUSD: 1.0, TotalLatencyMs: 2000, VerifyPassed: true, WorkerTokensOut: 10},
		{WorkerProvider: "claude", TaskType: "bug_fix", TotalCostUSD: 3.0, TotalLatencyMs: 4000, VerifyPassed: true, WorkerTokensOut: 20},
	}
	profiles, err := SeedProfilesFromObservations(obs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Check provider profile averages.
	if len(profiles.ProviderProfiles) != 1 {
		t.Fatalf("expected 1 provider profile, got %d", len(profiles.ProviderProfiles))
	}
	pp := profiles.ProviderProfiles[0]
	if pp.AvgCostUSD != 2.0 {
		t.Errorf("avg cost = $%.2f, want $2.00", pp.AvgCostUSD)
	}
	if pp.AvgTurns != 15 {
		t.Errorf("avg turns = %d, want 15", pp.AvgTurns)
	}

	// Check prompt profile averages.
	if len(profiles.PromptProfiles) != 1 {
		t.Fatalf("expected 1 prompt profile, got %d", len(profiles.PromptProfiles))
	}
	tp := profiles.PromptProfiles[0]
	if tp.AvgCostUSD != 2.0 {
		t.Errorf("prompt avg cost = $%.2f, want $2.00", tp.AvgCostUSD)
	}
	// AvgDurationSec = avg(2000, 4000) ms / 1000 = 3.0 sec
	if tp.AvgDurationSec != 3.0 {
		t.Errorf("prompt avg duration = %.1fs, want 3.0s", tp.AvgDurationSec)
	}
}

func TestFeedbackAnalyzer_SeedFromObservations_Idempotent(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)

	obs := []LoopObservation{
		{WorkerProvider: "claude", TaskType: "bug_fix", TotalCostUSD: 1.0, VerifyPassed: true},
	}

	// First seed should populate.
	if err := fa.SeedFromObservations(obs); err != nil {
		t.Fatalf("seed error: %v", err)
	}
	profiles1 := fa.AllPromptProfiles()
	if len(profiles1) == 0 {
		t.Fatal("expected seeded prompt profiles")
	}

	// Second seed should be a no-op (profiles already exist).
	obs2 := []LoopObservation{
		{WorkerProvider: "gemini", TaskType: "feature", TotalCostUSD: 5.0, VerifyPassed: true},
	}
	if err := fa.SeedFromObservations(obs2); err != nil {
		t.Fatalf("second seed error: %v", err)
	}
	profiles2 := fa.AllPromptProfiles()
	if len(profiles2) != len(profiles1) {
		t.Errorf("seed was not idempotent: got %d profiles, want %d", len(profiles2), len(profiles1))
	}
}

func TestFeedbackAnalyzer_SeedDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)

	// Manually ingest entries first.
	fa.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 1.0, TurnCount: 5, ExitReason: "completed"},
	})

	// Seed should not overwrite existing profiles.
	obs := []LoopObservation{
		{WorkerProvider: "gemini", TaskType: "feature", TotalCostUSD: 10.0, VerifyPassed: true},
	}
	if err := fa.SeedFromObservations(obs); err != nil {
		t.Fatalf("seed error: %v", err)
	}
	// Should still only have the original ingested profiles.
	profiles := fa.AllPromptProfiles()
	for _, p := range profiles {
		if p.TaskType == "feature" {
			t.Error("seed should not have added feature profile when profiles already existed")
		}
	}
}

func TestSeedProfilesFromJSONL_MissingFile(t *testing.T) {
	t.Parallel()
	profiles, err := SeedProfilesFromJSONL("/nonexistent/path/observations.jsonl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles.PromptProfiles) != 0 || len(profiles.ProviderProfiles) != 0 {
		t.Error("expected empty profiles for missing file")
	}
}

func TestSeedProfilesFromJSONL_ValidFile(t *testing.T) {
	dir := t.TempDir()
	obsPath := filepath.Join(dir, "observations.jsonl")

	// Write some observations.
	obs := []LoopObservation{
		{WorkerProvider: "claude", TaskType: "bug_fix", TotalCostUSD: 1.5, VerifyPassed: true},
		{WorkerProvider: "claude", TaskType: "feature", TotalCostUSD: 3.0, VerifyPassed: false},
	}
	for _, o := range obs {
		if err := WriteObservation(obsPath, o); err != nil {
			t.Fatalf("write observation: %v", err)
		}
	}

	profiles, err := SeedProfilesFromJSONL(obsPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles.PromptProfiles) != 2 {
		t.Errorf("expected 2 prompt profiles, got %d", len(profiles.PromptProfiles))
	}
	if len(profiles.ProviderProfiles) != 1 {
		t.Errorf("expected 1 provider profile, got %d", len(profiles.ProviderProfiles))
	}
}

func TestSeedProfilesFromObservations_FallbackToPlannerProvider(t *testing.T) {
	t.Parallel()
	obs := []LoopObservation{
		{PlannerProvider: "gemini", WorkerProvider: "", TaskType: "feature", TotalCostUSD: 1.0, VerifyPassed: true},
	}
	profiles, err := SeedProfilesFromObservations(obs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(profiles.ProviderProfiles) != 1 {
		t.Fatalf("expected 1 provider profile, got %d", len(profiles.ProviderProfiles))
	}
	if profiles.ProviderProfiles[0].Provider != "gemini" {
		t.Errorf("provider = %q, want gemini (fallback from planner)", profiles.ProviderProfiles[0].Provider)
	}
}
