package fewshot

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRetriever_EmptyIndex(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, ".prompt-index.jsonl")
	os.WriteFile(indexPath, []byte{}, 0644)

	r := NewRetriever(indexPath, DefaultConfig())
	result, err := r.Retrieve(context.Background(), "Write a Go function", "test-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Examples) != 0 {
		t.Errorf("expected 0 examples from empty index, got %d", len(result.Examples))
	}
}

func TestRetriever_MissingIndex(t *testing.T) {
	r := NewRetriever("/nonexistent/path/.prompt-index.jsonl", DefaultConfig())
	result, err := r.Retrieve(context.Background(), "Write a Go function", "test-repo")
	if err != nil {
		t.Fatalf("should gracefully handle missing index, got error: %v", err)
	}
	if len(result.Examples) != 0 {
		t.Errorf("expected 0 examples, got %d", len(result.Examples))
	}
}

func TestRetriever_WithQualityFilter(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, ".prompt-index.jsonl")

	entries := []map[string]any{
		{"hash": "aaa", "short_hash": "aaa", "prompt": "High quality Go implementation with error handling", "score": 85, "grade": "B", "task_type": "code", "tags": []string{"go"}, "repo": "test", "status": "scored", "timestamp": "2026-04-05T12:00:00Z"},
		{"hash": "bbb", "short_hash": "bbb", "prompt": "Low quality do stuff", "score": 30, "grade": "F", "task_type": "general", "tags": []string{}, "repo": "test", "status": "unsorted", "timestamp": "2026-04-05T12:00:00Z"},
		{"hash": "ccc", "short_hash": "ccc", "prompt": "Medium quality analysis of database performance", "score": 75, "grade": "C", "task_type": "analysis", "tags": []string{"database"}, "repo": "test", "status": "scored", "timestamp": "2026-04-05T12:00:00Z"},
	}

	f, _ := os.Create(indexPath)
	for _, e := range entries {
		line, _ := json.Marshal(e)
		f.Write(line)
		f.WriteString("\n")
	}
	f.Close()

	cfg := DefaultConfig()
	cfg.MinScore = 70
	cfg.K = 5 // request more than available

	r := NewRetriever(indexPath, cfg)
	result, err := r.Retrieve(context.Background(), "Go function with error handling", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only get entries with score >= 70 and status in {scored, improved}
	// Entry "aaa" (85, scored) and "ccc" (75, scored) qualify
	// Entry "bbb" (30, unsorted) should be filtered out
	if len(result.Examples) > 2 {
		t.Errorf("expected at most 2 qualifying examples, got %d", len(result.Examples))
	}
	for _, ex := range result.Examples {
		if ex.Score < 70 {
			t.Errorf("example %s has score %d < minimum 70", ex.ShortHash, ex.Score)
		}
	}
}

func TestRetriever_CacheHit(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, ".prompt-index.jsonl")

	entries := []map[string]any{
		{"hash": "ddd", "short_hash": "ddd", "prompt": "Implement a REST API endpoint", "score": 80, "grade": "B", "task_type": "code", "tags": []string{"go", "api"}, "repo": "test", "status": "scored", "timestamp": "2026-04-05T12:00:00Z"},
	}
	f, _ := os.Create(indexPath)
	for _, e := range entries {
		line, _ := json.Marshal(e)
		f.Write(line)
		f.WriteString("\n")
	}
	f.Close()

	r := NewRetriever(indexPath, DefaultConfig())

	// First call: cache miss
	r1, _ := r.Retrieve(context.Background(), "REST API with auth", "test")
	if r1.CacheHit {
		t.Error("first call should be cache miss")
	}

	// Second call: cache hit
	r2, _ := r.Retrieve(context.Background(), "REST API with auth", "test")
	if !r2.CacheHit {
		t.Error("second call should be cache hit")
	}
}

func TestFormatXML(t *testing.T) {
	examples := []RetrievedExample{
		{ShortHash: "abc123", Score: 85, Grade: "B", TaskType: "code", Similarity: 0.92, Prompt: "Test prompt"},
	}
	xml := FormatXML(examples, "code")
	if xml == "" {
		t.Fatal("expected non-empty XML")
	}
	if !contains(xml, "few_shot_examples") {
		t.Error("expected few_shot_examples tag")
	}
	if !contains(xml, "abc123") {
		t.Error("expected hash in XML")
	}
}

func TestFormatMarkdown(t *testing.T) {
	examples := []RetrievedExample{
		{ShortHash: "def456", Score: 90, Grade: "A", TaskType: "analysis", Similarity: 0.85, Prompt: "Analyze this"},
	}
	md := FormatMarkdown(examples, "analysis")
	if md == "" {
		t.Fatal("expected non-empty markdown")
	}
	if !contains(md, "Example 1") {
		t.Error("expected 'Example 1' in markdown")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
