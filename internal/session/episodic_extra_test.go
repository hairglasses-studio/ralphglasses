package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRewriteFile_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	em := NewEpisodicMemory(dir, 100, 0)

	now := time.Now()
	em.mu.Lock()
	em.episodes = []Episode{
		{Timestamp: now, TaskType: "feature", TaskTitle: "add login", Provider: "claude", CostUSD: 0.5},
		{Timestamp: now, TaskType: "bug_fix", TaskTitle: "fix crash", Provider: "gemini", CostUSD: 0.3},
		{Timestamp: now, TaskType: "test", TaskTitle: "add tests", Provider: "claude", CostUSD: 0.1},
	}
	em.mu.Unlock()

	// Force a rewrite
	em.rewriteFile()

	// Reload from the rewritten file
	em2 := NewEpisodicMemory(dir, 100, 0)
	em2.mu.Lock()
	count := len(em2.episodes)
	em2.mu.Unlock()

	if count != 3 {
		t.Fatalf("expected 3 episodes after rewrite+reload, got %d", count)
	}

	em2.mu.Lock()
	if em2.episodes[0].TaskTitle != "add login" {
		t.Errorf("episode 0 title = %q", em2.episodes[0].TaskTitle)
	}
	if em2.episodes[2].Provider != "claude" {
		t.Errorf("episode 2 provider = %q", em2.episodes[2].Provider)
	}
	em2.mu.Unlock()
}

func TestRewriteFile_EmptyStateDir(t *testing.T) {
	em := &EpisodicMemory{stateDir: "", maxSize: 100}
	// Should not panic
	em.rewriteFile()
}

func TestRewriteFile_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "nested", "episodic")

	em := &EpisodicMemory{
		stateDir: subDir,
		maxSize:  100,
		episodes: []Episode{
			{TaskType: "feature", TaskTitle: "test"},
		},
	}
	em.rewriteFile()

	// File should exist
	path := filepath.Join(subDir, "episodes.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty file after rewrite")
	}
}

func TestEpisodicMemory_DefaultK(t *testing.T) {
	em := NewEpisodicMemory("", 0, 0)
	if em.DefaultK != 5 {
		t.Errorf("DefaultK = %d, want 5", em.DefaultK)
	}
	if em.maxSize != 500 {
		t.Errorf("maxSize = %d, want 500", em.maxSize)
	}
}

func TestEpisodicMemory_CustomK(t *testing.T) {
	em := NewEpisodicMemory("", 200, 10)
	if em.DefaultK != 10 {
		t.Errorf("DefaultK = %d, want 10", em.DefaultK)
	}
	if em.maxSize != 200 {
		t.Errorf("maxSize = %d, want 200", em.maxSize)
	}
}

func TestFindSimilarEpisodes_Interface(t *testing.T) {
	dir := t.TempDir()
	em := NewEpisodicMemory(dir, 100, 0)

	now := time.Now()
	em.mu.Lock()
	em.episodes = []Episode{
		{Timestamp: now, TaskType: "feature", TaskTitle: "add auth", Prompt: "add auth", Provider: "claude", TurnCount: 10, CostUSD: 0.5, Worked: []string{"done"}},
		{Timestamp: now, TaskType: "feature", TaskTitle: "add api", Prompt: "add api", Provider: "gemini", TurnCount: 5, CostUSD: 0.3, Worked: []string{"done"}},
	}
	em.mu.Unlock()

	// This is the CurriculumSorter interface method
	results := em.FindSimilarEpisodes("feature", "add auth", 2)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].TurnCount == 0 {
		t.Error("expected non-zero TurnCount")
	}
}

func TestTruncate(t *testing.T) {
	if s := truncate("hello world", 5); s != "hello" {
		t.Errorf("truncate(hello world, 5) = %q", s)
	}
	if s := truncate("hi", 10); s != "hi" {
		t.Errorf("truncate(hi, 10) = %q", s)
	}
	if s := truncate("", 5); s != "" {
		t.Errorf("truncate('', 5) = %q", s)
	}
}

func TestWordSet(t *testing.T) {
	set := wordSet("Hello World hello")
	if len(set) != 2 {
		t.Errorf("expected 2 unique words, got %d", len(set))
	}
	if !set["hello"] {
		t.Error("missing 'hello'")
	}
	if !set["world"] {
		t.Error("missing 'world'")
	}
}

func TestLoad_EmptyStateDir(t *testing.T) {
	em := &EpisodicMemory{stateDir: ""}
	// Should not panic
	em.load()
	if len(em.episodes) != 0 {
		t.Error("expected empty episodes")
	}
}

func TestLoad_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	// Write corrupt JSONL
	os.WriteFile(filepath.Join(dir, "episodes.jsonl"), []byte("not json\n{also bad\n"), 0644)

	em := NewEpisodicMemory(dir, 100, 0)
	em.mu.Lock()
	count := len(em.episodes)
	em.mu.Unlock()

	if count != 0 {
		t.Errorf("expected 0 episodes from corrupt file, got %d", count)
	}
}

func TestRecordSuccess_MultipleInSequence(t *testing.T) {
	dir := t.TempDir()
	em := NewEpisodicMemory(dir, 100, 0)

	for i := 0; i < 5; i++ {
		em.RecordSuccess(JournalEntry{
			TaskFocus:   "task",
			Provider:    "claude",
			ExitReason:  "completed",
			Worked:      []string{"something"},
			DurationSec: 60,
			TurnCount:   10,
		})
	}

	em.mu.Lock()
	count := len(em.episodes)
	em.mu.Unlock()

	if count != 5 {
		t.Errorf("expected 5 episodes, got %d", count)
	}

	// Verify all persisted
	data, err := os.ReadFile(filepath.Join(dir, "episodes.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 5 {
		t.Errorf("expected 5 lines in JSONL, got %d", len(lines))
	}
}
