package session

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestLearningTransfer_Empty(t *testing.T) {
	lt := NewLearningTransfer("")
	insights := lt.TransferInsights(nil, "session-new")
	if insights != nil {
		t.Errorf("expected nil insights from empty store, got %d", len(insights))
	}
}

func TestLearningTransfer_RecordAndRetrieve(t *testing.T) {
	lt := NewLearningTransfer("")

	// Record 3 successful sessions with common "worked" pattern.
	for i := 0; i < 3; i++ {
		lt.RecordSession(SessionLearning{
			SessionID:   "sess-" + ltItoa(i),
			TaskType:    "feature",
			Provider:    "claude",
			Success:     true,
			CostUSD:     0.50,
			TurnCount:   5,
			DurationSec: 120,
			Worked:      []string{"added validation", "wrote tests"},
			RecordedAt:  time.Now(),
		})
	}

	insights := lt.TransferInsights(nil, "sess-new")
	if len(insights) == 0 {
		t.Fatal("expected insights after 3 sessions with common patterns")
	}

	// At least one should be a success_pattern.
	foundSuccess := false
	for _, ins := range insights {
		if ins.Type == "success_pattern" {
			foundSuccess = true
			break
		}
	}
	if !foundSuccess {
		t.Error("expected at least one success_pattern insight")
	}
}

func TestLearningTransfer_FilterByFromSessions(t *testing.T) {
	lt := NewLearningTransfer("")

	for i := 0; i < 5; i++ {
		lt.RecordSession(SessionLearning{
			SessionID:  "sess-" + ltItoa(i),
			TaskType:   "bug_fix",
			Provider:   "claude",
			Success:    true,
			CostUSD:    0.30,
			Worked:     []string{"fixed null pointer"},
			RecordedAt: time.Now(),
		})
	}

	// Filter to only 2 sessions.
	insights := lt.TransferInsights([]string{"sess-0", "sess-1"}, "sess-new")
	// With only 2 sessions, "fixed null pointer" appears 2 times and should generate an insight.
	if insights == nil {
		t.Error("expected some insights when filtering to 2 sessions with same worked pattern")
	}
}

func TestLearningTransfer_FilterNoMatch(t *testing.T) {
	lt := NewLearningTransfer("")

	lt.RecordSession(SessionLearning{
		SessionID: "sess-A",
		TaskType:  "test",
		Provider:  "claude",
		Success:   true,
		Worked:    []string{"wrote tests"},
	})

	// Ask for sessions that don't exist.
	insights := lt.TransferInsights([]string{"nonexistent-1", "nonexistent-2"}, "sess-new")
	if insights != nil {
		t.Errorf("expected nil insights for non-matching fromSessions, got %d", len(insights))
	}
}

func TestLearningTransfer_ProviderHint(t *testing.T) {
	lt := NewLearningTransfer("")

	// Record several successful sessions with the same provider + task type.
	for i := 0; i < 4; i++ {
		lt.RecordSession(SessionLearning{
			SessionID: "sess-" + ltItoa(i),
			TaskType:  "test",
			Provider:  "gemini",
			Success:   true,
			CostUSD:   0.10,
		})
	}

	insights := lt.AllInsights()
	foundProviderHint := false
	for _, ins := range insights {
		if ins.Type == "provider_hint" && ins.Provider == "gemini" && ins.TaskType == "test" {
			foundProviderHint = true
			break
		}
	}
	if !foundProviderHint {
		t.Error("expected provider_hint insight for gemini+test after 4 successful sessions")
	}
}

func TestLearningTransfer_BudgetHint(t *testing.T) {
	lt := NewLearningTransfer("")

	for i := 0; i < 3; i++ {
		lt.RecordSession(SessionLearning{
			SessionID: "sess-" + ltItoa(i),
			TaskType:  "refactor",
			Provider:  "claude",
			Success:   true,
			CostUSD:   1.00,
		})
	}

	insights := lt.AllInsights()
	foundBudget := false
	for _, ins := range insights {
		if ins.Type == "budget_hint" && ins.TaskType == "refactor" {
			foundBudget = true
			break
		}
	}
	if !foundBudget {
		t.Error("expected budget_hint insight for refactor tasks after 3 sessions")
	}
}

func TestLearningTransfer_RecordFromJournalEntry(t *testing.T) {
	lt := NewLearningTransfer("")

	entry := JournalEntry{
		Timestamp:  time.Now(),
		SessionID:  "j-sess-1",
		Provider:   "claude",
		TaskFocus:  "fix bug in parser",
		ExitReason: "completed",
		SpentUSD:   0.25,
		TurnCount:  4,
		Worked:     []string{"identified root cause"},
	}
	lt.RecordFromJournalEntry(entry)

	sessions := lt.AllSessions()
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session after recording from journal, got %d", len(sessions))
	}
	if sessions[0].SessionID != "j-sess-1" {
		t.Errorf("session ID mismatch: got %q", sessions[0].SessionID)
	}
	if !sessions[0].Success {
		t.Error("expected session to be marked successful for exit_reason=completed")
	}
	if sessions[0].TaskType != "bug_fix" {
		t.Errorf("expected task_type=bug_fix, got %q", sessions[0].TaskType)
	}
}

func TestLearningTransfer_Persistence(t *testing.T) {
	dir := t.TempDir()
	lt := NewLearningTransfer(dir)

	for i := 0; i < 3; i++ {
		lt.RecordSession(SessionLearning{
			SessionID: "sess-" + ltItoa(i),
			TaskType:  "docs",
			Provider:  "claude",
			Success:   true,
			CostUSD:   0.05,
			Worked:    []string{"wrote readme"},
		})
	}

	// File should exist.
	path := filepath.Join(dir, "learnings.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected learnings.json to exist: %v", err)
	}

	// Reload and verify.
	lt2 := NewLearningTransfer(dir)
	sessions := lt2.AllSessions()
	if len(sessions) != 3 {
		t.Errorf("expected 3 sessions after reload, got %d", len(sessions))
	}
}

func TestLearningTransfer_ConcurrentSafety(t *testing.T) {
	lt := NewLearningTransfer("")
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			lt.RecordSession(SessionLearning{
				SessionID: "sess-" + ltItoa(n),
				TaskType:  "feature",
				Provider:  "claude",
				Success:   n%2 == 0,
				CostUSD:   0.10,
				Worked:    []string{"concurrent work"},
			})
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lt.TransferInsights(nil, "reader")
		}()
	}

	wg.Wait()
}

func TestLearningTransfer_HashStr(t *testing.T) {
	// Same input should produce same hash.
	h1 := hashStr("hello world")
	h2 := hashStr("hello world")
	if h1 != h2 {
		t.Errorf("hashStr not deterministic: %q != %q", h1, h2)
	}

	// Different input should produce different hash (probabilistic).
	h3 := hashStr("something else")
	if h1 == h3 {
		t.Errorf("hashStr collision for different inputs: %q", h1)
	}
}

func TestLearningTransfer_FailurePatterns(t *testing.T) {
	lt := NewLearningTransfer("")

	for i := 0; i < 3; i++ {
		lt.RecordSession(SessionLearning{
			SessionID: "fail-" + ltItoa(i),
			TaskType:  "config",
			Provider:  "claude",
			Success:   false,
			Failed:    []string{"provider timeout"},
		})
	}

	insights := lt.AllInsights()
	foundFailure := false
	for _, ins := range insights {
		if ins.Type == "failure_pattern" {
			foundFailure = true
			break
		}
	}
	if !foundFailure {
		t.Error("expected failure_pattern insight after 3 sessions with same failure")
	}
}
