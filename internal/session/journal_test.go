package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteJournalEntry(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	now := time.Now()
	s := &Session{
		ID:         "sess-001",
		Provider:   ProviderClaude,
		RepoPath:   dir,
		RepoName:   "test-repo",
		Model:      "sonnet",
		SpentUSD:   1.23,
		TurnCount:  10,
		Prompt:     "Fix the parser bug",
		ExitReason: "completed normally",
		LaunchedAt: now.Add(-5 * time.Minute),
	}
	ended := now
	s.EndedAt = &ended

	if err := WriteJournalEntry(s); err != nil {
		t.Fatalf("WriteJournalEntry: %v", err)
	}

	// Read back
	data, err := os.ReadFile(filepath.Join(dir, journalFile))
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}

	var entry JournalEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if entry.SessionID != "sess-001" {
		t.Errorf("session_id = %q, want sess-001", entry.SessionID)
	}
	if entry.Provider != "claude" {
		t.Errorf("provider = %q, want claude", entry.Provider)
	}
	if entry.SpentUSD != 1.23 {
		t.Errorf("spent_usd = %f, want 1.23", entry.SpentUSD)
	}
	if entry.TaskFocus != "Fix the parser bug" {
		t.Errorf("task_focus = %q, want 'Fix the parser bug'", entry.TaskFocus)
	}
	if entry.DurationSec < 299 || entry.DurationSec > 301 {
		t.Errorf("duration_sec = %f, want ~300", entry.DurationSec)
	}
}

func TestReadRecentJournal(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	// Write 20 entries
	for i := 0; i < 20; i++ {
		entry := JournalEntry{
			Timestamp: time.Now(),
			SessionID: "sess-" + string(rune('A'+i)),
			Provider:  "claude",
			RepoName:  "test-repo",
			TurnCount: i + 1,
		}
		if err := WriteJournalEntryManual(dir, entry); err != nil {
			t.Fatalf("write entry %d: %v", i, err)
		}
	}

	entries, err := ReadRecentJournal(dir, 5)
	if err != nil {
		t.Fatalf("ReadRecentJournal: %v", err)
	}
	if len(entries) != 5 {
		t.Fatalf("got %d entries, want 5", len(entries))
	}

	// Should be the last 5 (turn counts 16-20)
	if entries[0].TurnCount != 16 {
		t.Errorf("first entry turn_count = %d, want 16", entries[0].TurnCount)
	}
	if entries[4].TurnCount != 20 {
		t.Errorf("last entry turn_count = %d, want 20", entries[4].TurnCount)
	}
}

func TestReadRecentJournal_Empty(t *testing.T) {
	dir := t.TempDir()

	entries, err := ReadRecentJournal(dir, 5)
	if err != nil {
		t.Fatalf("ReadRecentJournal on missing file: %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil, got %d entries", len(entries))
	}
}

func TestSynthesizeContext(t *testing.T) {
	entries := []JournalEntry{
		{
			Worked:  []string{"Used incremental builds", "Ran tests first"},
			Failed:  []string{"Forgot to check go vet"},
			Suggest: []string{"old suggestion"},
		},
		{
			Worked:  []string{"Used incremental builds", "Good commit messages"},
			Failed:  []string{"Forgot to check go vet", "Missed edge case"},
			Suggest: []string{"Run go vet before commit"},
		},
		{
			Worked:  []string{"Ran tests first"},
			Failed:  []string{"Broke import cycle"},
			Suggest: []string{"Check imports early"},
		},
	}

	ctx := SynthesizeContext(entries)

	if !strings.Contains(ctx, "## Session Improvement Context") {
		t.Error("missing header")
	}
	if !strings.Contains(ctx, "### Reinforce") {
		t.Error("missing Reinforce section")
	}
	if !strings.Contains(ctx, "### Avoid") {
		t.Error("missing Avoid section")
	}
	if !strings.Contains(ctx, "### Apply Now") {
		t.Error("missing Apply Now section")
	}

	// Deduplication: "Used incremental builds" should appear once
	if strings.Count(strings.ToLower(ctx), "used incremental builds") != 1 {
		t.Error("expected deduplication of 'Used incremental builds'")
	}

	// Suggestions should come from last 2 sessions
	if !strings.Contains(ctx, "Run go vet before commit") {
		t.Error("missing recent suggestion")
	}
	if !strings.Contains(ctx, "Check imports early") {
		t.Error("missing recent suggestion")
	}

	// 2000 char cap
	if len(ctx) > 2000 {
		t.Errorf("context length %d exceeds 2000 cap", len(ctx))
	}
}

func TestSynthesizeContext_Empty(t *testing.T) {
	ctx := SynthesizeContext(nil)
	if ctx != "" {
		t.Errorf("expected empty string, got %q", ctx)
	}
}

func TestConsolidatePatterns(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	// Write entries with repeated items
	for i := 0; i < 5; i++ {
		entry := JournalEntry{
			Timestamp: time.Now(),
			SessionID: "sess",
			Worked:    []string{"Good pattern"},
			Failed:    []string{"Bad pattern"},
			Suggest:   []string{"Always do X"},
		}
		if i < 2 {
			// Only 2 occurrences of "rare item" — should not be consolidated
			entry.Worked = append(entry.Worked, "Rare item")
		}
		_ = WriteJournalEntryManual(dir, entry)
	}

	if err := ConsolidatePatterns(dir); err != nil {
		t.Fatalf("ConsolidatePatterns: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, patternsFile))
	if err != nil {
		t.Fatalf("read patterns: %v", err)
	}

	var patterns ConsolidatedPatterns
	if err := json.Unmarshal(data, &patterns); err != nil {
		t.Fatalf("unmarshal patterns: %v", err)
	}

	if len(patterns.Positive) != 1 {
		t.Errorf("expected 1 positive pattern, got %d", len(patterns.Positive))
	}
	if len(patterns.Positive) > 0 && patterns.Positive[0].Count != 5 {
		t.Errorf("positive count = %d, want 5", patterns.Positive[0].Count)
	}
	if len(patterns.Negative) != 1 {
		t.Errorf("expected 1 negative pattern, got %d", len(patterns.Negative))
	}
	if len(patterns.Rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(patterns.Rules))
	}
}

func TestPruneJournal(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	// Write 200 entries
	for i := 0; i < 200; i++ {
		entry := JournalEntry{
			Timestamp: time.Now(),
			SessionID: "sess",
			TurnCount: i + 1,
			Worked:    []string{"Repeated item"},
		}
		_ = WriteJournalEntryManual(dir, entry)
	}

	pruned, err := PruneJournal(dir, 50)
	if err != nil {
		t.Fatalf("PruneJournal: %v", err)
	}
	if pruned != 150 {
		t.Errorf("pruned = %d, want 150", pruned)
	}

	// Verify remaining count
	entries, err := ReadRecentJournal(dir, 10000)
	if err != nil {
		t.Fatalf("ReadRecentJournal: %v", err)
	}
	if len(entries) != 50 {
		t.Errorf("remaining entries = %d, want 50", len(entries))
	}

	// Should be the last 50 (turn counts 151-200)
	if entries[0].TurnCount != 151 {
		t.Errorf("first remaining turn_count = %d, want 151", entries[0].TurnCount)
	}

	// Patterns file should have been written
	if _, err := os.Stat(filepath.Join(dir, patternsFile)); err != nil {
		t.Error("expected patterns file after prune")
	}
}

func TestPruneJournal_NoPruneNeeded(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	for i := 0; i < 10; i++ {
		_ = WriteJournalEntryManual(dir, JournalEntry{Timestamp: time.Now(), SessionID: "s"})
	}

	pruned, err := PruneJournal(dir, 100)
	if err != nil {
		t.Fatalf("PruneJournal: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0", pruned)
	}
}

func TestParseImprovementMarkers(t *testing.T) {
	history := []string{
		"some output",
		"---RALPH_STATUS---",
		"WORKED:",
		"- Fast builds",
		"- Clean tests",
		"FAILED:",
		"- Broke CI",
		"SUGGEST:",
		"- Run CI locally",
		"---END_STATUS---",
		"more output",
	}

	worked, failed, suggest := parseImprovementMarkers(history)

	if len(worked) != 2 {
		t.Errorf("worked count = %d, want 2", len(worked))
	}
	if len(failed) != 1 {
		t.Errorf("failed count = %d, want 1", len(failed))
	}
	if len(suggest) != 1 {
		t.Errorf("suggest count = %d, want 1", len(suggest))
	}
}

func TestExtractSignalsFromOutput(t *testing.T) {
	t.Run("error_patterns", func(t *testing.T) {
		history := []string{
			"Starting build...",
			"error: undefined variable foo",
			"FAIL main_test.go",
			"Build completed with errors",
		}
		worked, failed, suggest := extractSignalsFromOutput(history, StatusErrored, 0, 0)
		if len(failed) < 2 {
			t.Errorf("expected at least 2 failed items, got %d: %v", len(failed), failed)
		}
		if len(worked) > 0 {
			t.Errorf("expected no worked items for error output, got %v", worked)
		}
		_ = suggest
	})

	t.Run("success_patterns", func(t *testing.T) {
		history := []string{
			"PASS",
			"ok  \tgithub.com/example/pkg\t0.5s",
			"created new file handler.go",
			"fixed validation bug in parser",
		}
		worked, failed, _ := extractSignalsFromOutput(history, StatusCompleted, 0.05, 3)
		if len(worked) < 3 {
			t.Errorf("expected at least 3 worked items, got %d: %v", len(worked), worked)
		}
		if len(failed) != 0 {
			t.Errorf("expected no failed items, got %v", failed)
		}
	})

	t.Run("friction_repeated_errors", func(t *testing.T) {
		history := []string{
			"error: connection refused",
			"error: connection refused",
			"error: connection refused",
			"error: connection refused",
		}
		_, _, suggest := extractSignalsFromOutput(history, StatusErrored, 0, 0)
		if len(suggest) == 0 {
			t.Fatal("expected friction suggestion for repeated errors")
		}
		found := false
		for _, s := range suggest {
			if strings.Contains(s, "repeated error") && strings.Contains(s, "4x") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected suggestion mentioning repeated error count, got %v", suggest)
		}
	})

	t.Run("cost_anomaly", func(t *testing.T) {
		history := []string{"some output"}
		_, _, suggest := extractSignalsFromOutput(history, StatusCompleted, 2.0, 5)
		if len(suggest) == 0 {
			t.Fatal("expected cost anomaly suggestion")
		}
		found := false
		for _, s := range suggest {
			if strings.Contains(s, "cost per turn") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected cost per turn suggestion, got %v", suggest)
		}
	})

	t.Run("empty_history", func(t *testing.T) {
		worked, failed, suggest := extractSignalsFromOutput(nil, StatusCompleted, 0, 0)
		if len(worked) != 0 || len(failed) != 0 || len(suggest) != 0 {
			t.Errorf("expected empty results for nil history, got worked=%v failed=%v suggest=%v", worked, failed, suggest)
		}
	})

	t.Run("caps_results", func(t *testing.T) {
		var history []string
		for i := 0; i < 20; i++ {
			history = append(history, fmt.Sprintf("error: failure %d", i))
			history = append(history, fmt.Sprintf("created file_%d.go", i))
		}
		worked, failed, _ := extractSignalsFromOutput(history, StatusErrored, 0, 0)
		if len(worked) > 5 {
			t.Errorf("worked should be capped at 5, got %d", len(worked))
		}
		if len(failed) > 5 {
			t.Errorf("failed should be capped at 5, got %d", len(failed))
		}
	})
}

func TestWriteJournalEntryManual_Timestamp(t *testing.T) {
	dir := t.TempDir()

	// Zero timestamp gets auto-populated
	entry := JournalEntry{
		SessionID: "manual-001",
		Provider:  "claude",
	}
	if err := WriteJournalEntryManual(dir, entry); err != nil {
		t.Fatalf("WriteJournalEntryManual: %v", err)
	}

	entries, err := ReadRecentJournal(dir, 10)
	if err != nil {
		t.Fatalf("ReadRecentJournal: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Timestamp.IsZero() {
		t.Error("expected timestamp to be auto-populated")
	}
}

func TestWriteJournalEntryManual_MultipleEntries(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 10; i++ {
		entry := JournalEntry{
			Timestamp: time.Now(),
			SessionID: fmt.Sprintf("sess-%d", i),
			Provider:  "claude",
			TurnCount: i + 1,
		}
		if err := WriteJournalEntryManual(dir, entry); err != nil {
			t.Fatalf("write entry %d: %v", i, err)
		}
	}

	entries, err := ReadRecentJournal(dir, 10000)
	if err != nil {
		t.Fatalf("ReadRecentJournal: %v", err)
	}
	if len(entries) != 10 {
		t.Errorf("expected 10 entries, got %d", len(entries))
	}
}

func TestReadRecentJournal_DefaultLimit(t *testing.T) {
	dir := t.TempDir()

	for i := 0; i < 10; i++ {
		_ = WriteJournalEntryManual(dir, JournalEntry{
			Timestamp: time.Now(),
			SessionID: fmt.Sprintf("s%d", i),
			TurnCount: i + 1,
		})
	}

	// limit <= 0 defaults to 5
	entries, err := ReadRecentJournal(dir, 0)
	if err != nil {
		t.Fatalf("ReadRecentJournal: %v", err)
	}
	if len(entries) != 5 {
		t.Errorf("expected 5 entries (default limit), got %d", len(entries))
	}
	// Should be the last 5
	if entries[0].TurnCount != 6 {
		t.Errorf("first entry turn_count = %d, want 6", entries[0].TurnCount)
	}
}

func TestExtractTaskFocus(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{
			name:   "single_line",
			prompt: "Fix the parser bug",
			want:   "Fix the parser bug",
		},
		{
			name:   "multi_line",
			prompt: "Fix the bug\nMore details here\nExtra context",
			want:   "Fix the bug",
		},
		{
			name:   "long_prompt",
			prompt: strings.Repeat("a", 300),
			want:   strings.Repeat("a", 200),
		},
		{
			name:   "empty",
			prompt: "",
			want:   "",
		},
		{
			name:   "whitespace_only",
			prompt: "   ",
			want:   "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTaskFocus(tt.prompt)
			if got != tt.want {
				t.Errorf("extractTaskFocus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWriteJournalEntry_StatusFallbacks(t *testing.T) {
	dir := t.TempDir()

	// Completed session with task focus should auto-populate Worked
	now := time.Now()
	ended := now
	s := &Session{
		ID:         "complete-001",
		Provider:   ProviderClaude,
		RepoPath:   dir,
		RepoName:   "test-repo",
		Status:     StatusCompleted,
		Prompt:     "Implement feature X",
		LaunchedAt: now.Add(-1 * time.Minute),
		EndedAt:    &ended,
	}

	if err := WriteJournalEntry(s); err != nil {
		t.Fatalf("WriteJournalEntry: %v", err)
	}

	entries, err := ReadRecentJournal(dir, 10)
	if err != nil {
		t.Fatalf("ReadRecentJournal: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].Worked) == 0 {
		t.Error("expected Worked to be auto-populated for completed session")
	}
}

func TestWriteJournalEntry_ErroredSession(t *testing.T) {
	dir := t.TempDir()

	now := time.Now()
	s := &Session{
		ID:         "error-001",
		Provider:   ProviderGemini,
		RepoPath:   dir,
		RepoName:   "test-repo",
		Status:     StatusErrored,
		Prompt:     "Build the module",
		Error:      "API timeout after 30s",
		LaunchedAt: now.Add(-2 * time.Minute),
	}

	if err := WriteJournalEntry(s); err != nil {
		t.Fatalf("WriteJournalEntry: %v", err)
	}

	entries, err := ReadRecentJournal(dir, 10)
	if err != nil {
		t.Fatalf("ReadRecentJournal: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].Failed) == 0 {
		t.Error("expected Failed to be auto-populated for errored session")
	}
}

func TestWriteJournalEntry_WithMarkers(t *testing.T) {
	dir := t.TempDir()

	now := time.Now()
	ended := now
	s := &Session{
		ID:         "markers-001",
		Provider:   ProviderClaude,
		RepoPath:   dir,
		RepoName:   "test-repo",
		Status:     StatusCompleted,
		Prompt:     "Fix stuff",
		LaunchedAt: now.Add(-30 * time.Second),
		EndedAt:    &ended,
		OutputHistory: []string{
			"Working on it...",
			"---RALPH_STATUS---",
			"WORKED:",
			"- Fast iteration",
			"FAILED:",
			"- Missed edge case",
			"SUGGEST:",
			"- Add more tests",
			"---END_STATUS---",
		},
	}

	if err := WriteJournalEntry(s); err != nil {
		t.Fatalf("WriteJournalEntry: %v", err)
	}

	entries, err := ReadRecentJournal(dir, 10)
	if err != nil {
		t.Fatalf("ReadRecentJournal: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].SignalSource != "markers" {
		t.Errorf("signal_source = %q, want markers", entries[0].SignalSource)
	}
	if len(entries[0].Worked) != 1 || entries[0].Worked[0] != "Fast iteration" {
		t.Errorf("Worked = %v, want [Fast iteration]", entries[0].Worked)
	}
}

func TestPruneJournal_DefaultKeepN(t *testing.T) {
	dir := t.TempDir()

	// Write 150 entries
	for i := 0; i < 150; i++ {
		_ = WriteJournalEntryManual(dir, JournalEntry{
			Timestamp: time.Now(),
			SessionID: "s",
			TurnCount: i + 1,
			Worked:    []string{"item"},
		})
	}

	// keepN <= 0 defaults to 100
	pruned, err := PruneJournal(dir, 0)
	if err != nil {
		t.Fatalf("PruneJournal: %v", err)
	}
	if pruned != 50 {
		t.Errorf("pruned = %d, want 50", pruned)
	}

	entries, err := ReadRecentJournal(dir, 10000)
	if err != nil {
		t.Fatalf("ReadRecentJournal: %v", err)
	}
	if len(entries) != 100 {
		t.Errorf("remaining = %d, want 100", len(entries))
	}
}

func TestConsolidatePatterns_Empty(t *testing.T) {
	dir := t.TempDir()

	// No journal — should return nil
	err := ConsolidatePatterns(dir)
	if err != nil {
		t.Fatalf("ConsolidatePatterns on empty: %v", err)
	}

	// Patterns file should not be created
	_, err = os.Stat(filepath.Join(dir, patternsFile))
	if err == nil {
		t.Error("patterns file should not exist for empty journal")
	}
}

func TestConcurrentJournalWrites(t *testing.T) {
	dir := t.TempDir()

	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(idx int) {
			defer func() { done <- struct{}{} }()
			entry := JournalEntry{
				Timestamp: time.Now(),
				SessionID: fmt.Sprintf("concurrent-%d", idx),
				Provider:  "claude",
				TurnCount: idx,
			}
			_ = WriteJournalEntryManual(dir, entry)
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	entries, err := ReadRecentJournal(dir, 10000)
	if err != nil {
		t.Fatalf("ReadRecentJournal: %v", err)
	}
	if len(entries) != 10 {
		t.Errorf("expected 10 entries from concurrent writes, got %d", len(entries))
	}
}

func TestSynthesizeContext_LongOutput(t *testing.T) {
	// Create entries that would generate > 2000 chars
	var entries []JournalEntry
	for i := 0; i < 50; i++ {
		entries = append(entries, JournalEntry{
			Worked:  []string{fmt.Sprintf("Very long worked item number %d with extra text to make it verbose", i)},
			Failed:  []string{fmt.Sprintf("Very long failed item number %d with extra text to make it verbose", i)},
			Suggest: []string{fmt.Sprintf("Very long suggestion number %d with extra text to make it verbose", i)},
		})
	}

	ctx := SynthesizeContext(entries)
	if len(ctx) > 2000 {
		t.Errorf("context length %d exceeds 2000 cap", len(ctx))
	}
	if !strings.HasSuffix(ctx, "...") {
		t.Error("expected truncated output to end with ...")
	}
}

func TestExtractSection_NoMatch(t *testing.T) {
	block := "WORKED:\n- item1\nSUGGEST:\n- suggestion"
	failed := extractSection(block, "FAILED:")
	if failed != nil {
		t.Errorf("expected nil for missing section, got %v", failed)
	}
}

func TestFindLastSeen(t *testing.T) {
	ts1 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ts2 := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	entries := []JournalEntry{
		{Timestamp: ts1, Worked: []string{"pattern A"}},
		{Timestamp: ts2, Worked: []string{"pattern A", "pattern B"}},
	}

	got := findLastSeen(entries, "pattern A", func(e JournalEntry) []string { return e.Worked })
	if !got.Equal(ts2) {
		t.Errorf("findLastSeen = %v, want %v", got, ts2)
	}

	got = findLastSeen(entries, "nonexistent", func(e JournalEntry) []string { return e.Worked })
	if !got.IsZero() {
		t.Errorf("findLastSeen for missing = %v, want zero", got)
	}
}

func TestDedup(t *testing.T) {
	items := []string{"Hello", "hello", "HELLO", "world", "World", "", "  "}
	result := dedup(items)
	if len(result) != 2 {
		t.Errorf("dedup result = %v, want 2 items", result)
	}
}

func TestCountItems(t *testing.T) {
	items := []string{"a", "A", "b", "a", "  A  ", ""}
	counts := countItems(items)
	if counts["a"] != 4 {
		t.Errorf("count of 'a' = %d, want 4", counts["a"])
	}
	if counts["b"] != 1 {
		t.Errorf("count of 'b' = %d, want 1", counts["b"])
	}
}
