package session

import (
	"encoding/json"
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
