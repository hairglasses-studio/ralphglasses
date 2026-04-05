package promptdj

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBatchAnalysis_Empty(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, ".prompt-index.jsonl")
	os.WriteFile(indexPath, []byte{}, 0644)

	result, err := RunBatchAnalysis(BatchAnalysisConfig{IndexPath: indexPath})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalPrompts != 0 {
		t.Errorf("expected 0 prompts, got %d", result.TotalPrompts)
	}
}

func TestBatchAnalysis_WithData(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, ".prompt-index.jsonl")

	entries := []map[string]any{
		{"hash": "aaa", "short_hash": "aaa", "prompt": "Write a Go function that implements a concurrent-safe LRU cache with TTL-based expiration and automatic cleanup goroutine for stale entries", "repo": "test", "word_count": 22},
		{"hash": "bbb", "short_hash": "bbb", "prompt": "Create a REST API endpoint in Go that validates JSON payloads against a schema and returns appropriate HTTP status codes", "repo": "test", "word_count": 18},
		{"hash": "ccc", "short_hash": "ccc", "prompt": "ok", "repo": "test", "word_count": 1},
	}

	f, _ := os.Create(indexPath)
	for _, e := range entries {
		line, _ := json.Marshal(e)
		f.Write(line)
		f.WriteString("\n")
	}
	f.Close()

	result, err := RunBatchAnalysis(BatchAnalysisConfig{IndexPath: indexPath, MinWords: 5})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalPrompts != 3 {
		t.Errorf("expected 3 total, got %d", result.TotalPrompts)
	}
	if result.AnalyzedCount != 2 {
		t.Errorf("expected 2 analyzed (1 too short), got %d", result.AnalyzedCount)
	}
	if result.SkippedCount != 1 {
		t.Errorf("expected 1 skipped, got %d", result.SkippedCount)
	}
	if result.AvgScore <= 0 {
		t.Error("expected positive average score")
	}
	if len(result.ByTaskType) == 0 {
		t.Error("expected non-empty task type distribution")
	}
}

func TestBatchAnalysis_Recommendations(t *testing.T) {
	result := &BatchAnalysisResult{
		AnalyzedCount:     5,
		AvgScore:          35,
		ScoreDistribution: map[string]int{"F": 3, "D": 2},
		AntiPatterns:      map[string]int{"aggressive-emphasis": 5},
	}
	recs := generateRecommendations(result)
	if len(recs) == 0 {
		t.Error("expected recommendations for low-quality corpus")
	}
}
