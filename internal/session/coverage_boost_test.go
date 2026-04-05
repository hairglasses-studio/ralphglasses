package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSuggestProvider_WithData(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)

	fa.mu.Lock()
	fa.promptProfiles["feature"] = &PromptProfile{
		TaskType:        "feature",
		SampleCount:     5,
		BestProvider:    "claude",
		SuggestedBudget: 2.0,
	}
	fa.mu.Unlock()

	prov, ok := fa.SuggestProvider("feature")
	if !ok {
		t.Error("expected SuggestProvider to return true")
	}
	if prov != ProviderClaude {
		t.Errorf("provider = %q, want claude", prov)
	}

	_, ok2 := fa.SuggestProvider("nonexistent")
	if ok2 {
		t.Error("expected false for unknown task type")
	}
}

func TestSuggestProvider_InsufficientSamples(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 10)

	fa.mu.Lock()
	fa.promptProfiles["bug_fix"] = &PromptProfile{
		TaskType:     "bug_fix",
		SampleCount:  3,
		BestProvider: "claude",
	}
	fa.mu.Unlock()

	_, ok := fa.SuggestProvider("bug_fix")
	if ok {
		t.Error("expected false when sample count < minSessions")
	}
}

func TestSuggestBudget_WithData(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)

	fa.mu.Lock()
	fa.promptProfiles["refactor"] = &PromptProfile{
		TaskType:        "refactor",
		SampleCount:     5,
		SuggestedBudget: 3.5,
	}
	fa.mu.Unlock()

	budget, ok := fa.SuggestBudget("refactor")
	if !ok {
		t.Error("expected SuggestBudget to return true")
	}
	if budget != 3.5 {
		t.Errorf("budget = %f, want 3.5", budget)
	}

	_, ok2 := fa.SuggestBudget("nonexistent")
	if ok2 {
		t.Error("expected false for unknown task type")
	}
}

func TestSuggestBudget_InsufficientSamples(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 10)

	fa.mu.Lock()
	fa.promptProfiles["test"] = &PromptProfile{
		TaskType:        "test",
		SampleCount:     2,
		SuggestedBudget: 1.0,
	}
	fa.mu.Unlock()

	_, ok := fa.SuggestBudget("test")
	if ok {
		t.Error("expected false when sample count < minSessions")
	}
}

func TestComputePercentile_MoreCases(t *testing.T) {
	// p0 edge case
	samples := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}
	if d := computePercentile(samples, 0); d != 10*time.Millisecond {
		t.Errorf("p0 = %v, want 10ms", d)
	}
	// p100
	if d := computePercentile(samples, 100); d != 20*time.Millisecond {
		t.Errorf("p100 = %v, want 20ms", d)
	}
}

func TestBlackboard_SaveAndReload(t *testing.T) {
	dir := t.TempDir()
	bb := NewBlackboard(dir)

	bb.Put("key1", "value1", "test")
	bb.Put("key2", 42, "test")

	bb.save()

	path := filepath.Join(dir, "blackboard.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty blackboard file")
	}

	bb2 := NewBlackboard(dir)
	_, ok := bb2.Get("key1")
	if !ok {
		t.Error("expected key1 to be found after reload")
	}
}

func TestBlackboard_SaveEmptyStateDir(t *testing.T) {
	bb := NewBlackboard("")
	bb.save()
}

func TestSuggestProvider_EmptyBestProvider(t *testing.T) {
	dir := t.TempDir()
	fa := NewFeedbackAnalyzer(dir, 1)

	fa.mu.Lock()
	fa.promptProfiles["empty_prov"] = &PromptProfile{
		TaskType:     "empty_prov",
		SampleCount:  5,
		BestProvider: "",
	}
	fa.mu.Unlock()

	_, ok := fa.SuggestProvider("empty_prov")
	if ok {
		t.Error("expected false when BestProvider is empty")
	}
}

func TestCostPredictor_SaveAndReload(t *testing.T) {
	dir := t.TempDir()
	cp := NewCostPredictor(dir)

	cp.Record(CostObservation{TaskType: "feature", Provider: "claude", CostUSD: 1.5})
	cp.Record(CostObservation{TaskType: "feature", Provider: "claude", CostUSD: 2.0})

	cp.save()

	cp2 := NewCostPredictor(dir)
	est := cp2.Predict("feature", "claude")
	if est <= 0 {
		t.Error("expected positive estimate after reload")
	}
}

func TestDecisionLog_RecentTruncation(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelAutoRecover)

	// Add more decisions than the limit
	for range 30 {
		dl.Propose(AutonomousDecision{
			Category:      DecisionRestart,
			RequiredLevel: LevelAutoRecover,
			Rationale:     "test",
			Action:        "test",
		})
	}

	// Request 5 most recent
	recent := dl.Recent(5)
	if len(recent) != 5 {
		t.Errorf("expected 5 recent decisions, got %d", len(recent))
	}

	// Request all
	all := dl.Recent(0) // 0 defaults to 20
	if len(all) != 20 {
		t.Errorf("expected 20 (default limit), got %d", len(all))
	}

	// Request more than available
	big := dl.Recent(100)
	if len(big) != 30 {
		t.Errorf("expected 30, got %d", len(big))
	}
}

func TestDecisionLog_RecentNegativeLimit(t *testing.T) {
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelAutoRecover)

	for range 5 {
		dl.Propose(AutonomousDecision{
			Category:      DecisionRestart,
			RequiredLevel: LevelAutoRecover,
			Rationale:     "test",
			Action:        "test",
		})
	}

	recent := dl.Recent(-1) // negative defaults to 20
	if len(recent) != 5 {
		t.Errorf("expected 5 (all), got %d", len(recent))
	}
}

func TestContextStore_SaveEmptyDir(t *testing.T) {
	cs := NewContextStore("")
	// Should not panic
	cs.save()
}

func TestCostPredictor_PredictNoData(t *testing.T) {
	cp := NewCostPredictor(t.TempDir())
	est := cp.Predict("unknown_type", "unknown_provider")
	// Should return a default (non-negative) even with no data
	if est < 0 {
		t.Errorf("expected non-negative estimate, got %f", est)
	}
}

func TestCostPredictor_ObservationCount(t *testing.T) {
	dir := t.TempDir()
	cp := NewCostPredictor(dir)
	if cp.ObservationCount() != 0 {
		t.Errorf("expected 0 observations, got %d", cp.ObservationCount())
	}
	cp.Record(CostObservation{TaskType: "feature", Provider: "claude", CostUSD: 1.0})
	if cp.ObservationCount() != 1 {
		t.Errorf("expected 1 observation, got %d", cp.ObservationCount())
	}
}

func TestMapProvider_AllProviders(t *testing.T) {
	tests := []struct {
		input Provider
		want  string
	}{
		{ProviderClaude, "claude"},
		{ProviderGemini, "gemini"},
		{ProviderCodex, "openai"},
		{Provider("unknown"), "claude"},
	}

	for _, tc := range tests {
		got := mapProvider(tc.input)
		if string(got) != tc.want {
			t.Errorf("mapProvider(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestContextStore_SaveAndReload(t *testing.T) {
	dir := t.TempDir()
	cs := NewContextStore(dir)

	cs.Register(ContextEntry{
		SessionID:   "sess-1",
		RepoPath:    "/tmp/repo",
		ActiveFiles: []string{"main.go"},
	})
	cs.save()

	cs2 := NewContextStore(dir)
	entries := cs2.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after reload, got %d", len(entries))
	}
	if entries[0].SessionID != "sess-1" {
		t.Errorf("session ID = %q", entries[0].SessionID)
	}
}
