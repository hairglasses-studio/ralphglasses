package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordSuccess(t *testing.T) {
	dir := t.TempDir()
	em := NewEpisodicMemory(dir, 100)

	journal := JournalEntry{
		TaskFocus:   "add user authentication",
		Provider:    "claude",
		Model:       "opus-4",
		SpentUSD:    0.50,
		TurnCount:   10,
		DurationSec: 120,
		ExitReason:  "completed",
		Worked:      []string{"added tests", "clean implementation"},
		Suggest:     []string{"use table-driven tests"},
	}

	em.RecordSuccess(journal)

	em.mu.Lock()
	count := len(em.episodes)
	em.mu.Unlock()

	if count != 1 {
		t.Fatalf("expected 1 episode, got %d", count)
	}

	// Verify persisted to file
	data, err := os.ReadFile(filepath.Join(dir, "episodes.jsonl"))
	if err != nil {
		t.Fatalf("failed to read episodes file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("episodes file is empty")
	}

	// Verify episode fields
	em.mu.Lock()
	ep := em.episodes[0]
	em.mu.Unlock()

	if ep.TaskType != "feature" {
		t.Errorf("expected task_type 'feature', got %q", ep.TaskType)
	}
	if ep.TaskTitle != "add user authentication" {
		t.Errorf("expected task_title 'add user authentication', got %q", ep.TaskTitle)
	}
	if ep.Provider != "claude" {
		t.Errorf("expected provider 'claude', got %q", ep.Provider)
	}
	if len(ep.Worked) != 2 {
		t.Errorf("expected 2 worked items, got %d", len(ep.Worked))
	}
	if len(ep.KeyInsights) != 1 {
		t.Errorf("expected 1 key insight, got %d", len(ep.KeyInsights))
	}
}

func TestRecordSuccess_SkipsFailures(t *testing.T) {
	dir := t.TempDir()
	em := NewEpisodicMemory(dir, 100)

	// Budget exceeded — should not record
	em.RecordSuccess(JournalEntry{
		TaskFocus:  "fix bug",
		ExitReason: "budget_exceeded",
		Worked:     []string{"some work"},
	})

	em.mu.Lock()
	count1 := len(em.episodes)
	em.mu.Unlock()
	if count1 != 0 {
		t.Errorf("expected 0 episodes for budget_exceeded, got %d", count1)
	}

	// Empty Worked — should not record
	em.RecordSuccess(JournalEntry{
		TaskFocus:  "fix bug",
		ExitReason: "completed",
		Worked:     nil,
	})

	em.mu.Lock()
	count2 := len(em.episodes)
	em.mu.Unlock()
	if count2 != 0 {
		t.Errorf("expected 0 episodes for empty Worked, got %d", count2)
	}
}

func TestFindSimilar(t *testing.T) {
	dir := t.TempDir()
	em := NewEpisodicMemory(dir, 100)

	now := time.Now()

	em.mu.Lock()
	em.episodes = []Episode{
		{Timestamp: now, TaskType: "feature", TaskTitle: "add user login", Prompt: "add user login", Provider: "claude"},
		{Timestamp: now, TaskType: "bug_fix", TaskTitle: "fix parser crash", Prompt: "fix parser crash", Provider: "gemini"},
		{Timestamp: now, TaskType: "feature", TaskTitle: "add admin panel", Prompt: "add admin panel", Provider: "claude"},
		{Timestamp: now, TaskType: "test", TaskTitle: "add unit tests", Prompt: "add unit tests", Provider: "claude"},
		{Timestamp: now, TaskType: "refactor", TaskTitle: "refactor auth module", Prompt: "refactor auth module", Provider: "openai"},
	}
	em.mu.Unlock()

	// Query with matching task type
	results := em.FindSimilar("feature", "add new feature", 2)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Both should be feature type
	for _, r := range results {
		if r.TaskType != "feature" {
			t.Errorf("expected task_type 'feature', got %q", r.TaskType)
		}
	}

	// Query with keyword overlap
	results2 := em.FindSimilar("general", "add user auth", 3)
	if len(results2) == 0 {
		t.Fatal("expected at least 1 result from keyword matching")
	}
	// "add user login" should rank high due to word overlap with "add user auth"
	if results2[0].TaskTitle != "add user login" {
		t.Errorf("expected 'add user login' as top result, got %q", results2[0].TaskTitle)
	}
}

func TestFindSimilar_RecencyBonus(t *testing.T) {
	dir := t.TempDir()
	em := NewEpisodicMemory(dir, 100)

	now := time.Now()
	recent := now.Add(-24 * time.Hour)
	old := now.Add(-60 * 24 * time.Hour)

	em.mu.Lock()
	em.episodes = []Episode{
		{Timestamp: old, TaskType: "feature", TaskTitle: "add login", Prompt: "add login", Provider: "claude"},
		{Timestamp: recent, TaskType: "feature", TaskTitle: "add login", Prompt: "add login", Provider: "gemini"},
	}
	em.mu.Unlock()

	results := em.FindSimilar("feature", "add login", 2)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Recent one should rank first due to recency bonus
	if results[0].Provider != "gemini" {
		t.Errorf("expected recent episode (gemini) first, got %q", results[0].Provider)
	}
}

func TestFormatExamples(t *testing.T) {
	em := NewEpisodicMemory("", 100)

	episodes := []Episode{
		{
			TaskType:    "feature",
			TaskTitle:   "add user login",
			Provider:    "claude",
			CostUSD:     0.50,
			TurnCount:   10,
			Worked:      []string{"clean code", "good tests"},
			KeyInsights: []string{"use interfaces"},
		},
		{
			TaskType:  "bug_fix",
			TaskTitle: "fix crash on startup",
			Provider:  "gemini",
			CostUSD:   0.25,
			TurnCount: 5,
			Worked:    []string{"root cause found quickly"},
		},
	}

	output := em.FormatExamples(episodes)

	if output == "" {
		t.Fatal("expected non-empty output")
	}
	if !episodicContains(output, "Successful Approaches for Similar Tasks") {
		t.Error("missing header")
	}
	if !episodicContains(output, "Example 1") {
		t.Error("missing Example 1")
	}
	if !episodicContains(output, "Example 2") {
		t.Error("missing Example 2")
	}
	if !episodicContains(output, "$0.50") {
		t.Error("missing cost")
	}
	if !episodicContains(output, "Key insights: use interfaces") {
		t.Error("missing key insights")
	}
	if !episodicContains(output, "Use these examples to guide your approach.") {
		t.Error("missing closing instruction")
	}
}

func TestFormatExamples_Empty(t *testing.T) {
	em := NewEpisodicMemory("", 100)
	if output := em.FormatExamples(nil); output != "" {
		t.Errorf("expected empty string for nil episodes, got %q", output)
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	em := NewEpisodicMemory(dir, 5)

	now := time.Now()

	em.mu.Lock()
	for i := 0; i < 10; i++ {
		taskType := "feature"
		if i >= 5 {
			taskType = "bug_fix"
		}
		em.episodes = append(em.episodes, Episode{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			TaskType:  taskType,
			TaskTitle: "task",
			Prompt:    "task",
			Provider:  "claude",
		})
	}
	em.mu.Unlock()

	em.Prune()

	em.mu.Lock()
	count := len(em.episodes)
	em.mu.Unlock()

	if count > 5 {
		t.Errorf("expected at most 5 episodes after prune, got %d", count)
	}

	// Verify both types are represented
	em.mu.Lock()
	types := make(map[string]int)
	for _, ep := range em.episodes {
		types[ep.TaskType]++
	}
	em.mu.Unlock()

	if types["feature"] == 0 {
		t.Error("expected some 'feature' episodes after prune")
	}
	if types["bug_fix"] == 0 {
		t.Error("expected some 'bug_fix' episodes after prune")
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		a, b string
		low  float64
		high float64
	}{
		{"add user login", "add user auth", 0.3, 0.6},
		{"refactor parser", "add user login", 0.0, 0.1},
		{"", "", 0.0, 0.0},
		{"hello world", "hello world", 1.0, 1.0},
	}

	for _, tc := range tests {
		score := jaccardSimilarity(tc.a, tc.b)
		if score < tc.low || score > tc.high {
			t.Errorf("jaccardSimilarity(%q, %q) = %f, expected [%f, %f]", tc.a, tc.b, score, tc.low, tc.high)
		}
	}
}

func TestEpisodicPersistence(t *testing.T) {
	dir := t.TempDir()

	// Store episodes
	em1 := NewEpisodicMemory(dir, 100)
	em1.RecordSuccess(JournalEntry{
		TaskFocus:   "implement feature X",
		Provider:    "claude",
		Model:       "opus-4",
		SpentUSD:    1.00,
		TurnCount:   20,
		DurationSec: 300,
		ExitReason:  "completed",
		Worked:      []string{"clean implementation"},
		Suggest:     []string{"add more tests"},
	})
	em1.RecordSuccess(JournalEntry{
		TaskFocus:   "fix login bug",
		Provider:    "gemini",
		Model:       "pro-2",
		SpentUSD:    0.25,
		TurnCount:   5,
		DurationSec: 60,
		ExitReason:  "",
		Worked:      []string{"found root cause"},
	})

	// Load from same directory
	em2 := NewEpisodicMemory(dir, 100)

	em2.mu.Lock()
	count := len(em2.episodes)
	em2.mu.Unlock()

	if count != 2 {
		t.Fatalf("expected 2 episodes after reload, got %d", count)
	}

	em2.mu.Lock()
	ep0 := em2.episodes[0]
	ep1 := em2.episodes[1]
	em2.mu.Unlock()

	if ep0.TaskTitle != "implement feature X" {
		t.Errorf("expected first episode title 'implement feature X', got %q", ep0.TaskTitle)
	}
	if ep1.Provider != "gemini" {
		t.Errorf("expected second episode provider 'gemini', got %q", ep1.Provider)
	}
}

func episodicContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
