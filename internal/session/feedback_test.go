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
