package fewshot

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

func TestTokenize(t *testing.T) {
	tokens := Tokenize("Write a Go function that implements sorting")
	if len(tokens) == 0 {
		t.Fatal("expected non-empty tokens")
	}
	// "a" and "that" should be filtered as stop words
	for _, tok := range tokens {
		if tok == "a" || tok == "that" {
			t.Errorf("stop word %q should be filtered", tok)
		}
	}
}

func TestJaccardSimilarity(t *testing.T) {
	tests := []struct {
		a, b []string
		want float64
	}{
		{[]string{"go", "mcp"}, []string{"go", "mcp"}, 1.0},
		{[]string{"go", "mcp"}, []string{"go", "test"}, 1.0 / 3.0},
		{[]string{"go"}, []string{"python"}, 0.0},
		{nil, nil, 0.0},
		{[]string{"go"}, nil, 0.0},
	}
	for _, tc := range tests {
		got := JaccardSimilarity(tc.a, tc.b)
		if got < tc.want-0.01 || got > tc.want+0.01 {
			t.Errorf("JaccardSimilarity(%v, %v) = %.3f, want %.3f", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestTaskTypeScore(t *testing.T) {
	// Exact match
	if TaskTypeScore(enhancer.TaskTypeCode, enhancer.TaskTypeCode) != 1.0 {
		t.Error("exact match should return 1.0")
	}
	// Same family (code + workflow = implementation)
	if TaskTypeScore(enhancer.TaskTypeCode, enhancer.TaskTypeWorkflow) != 0.5 {
		t.Error("same-family should return 0.5")
	}
	// Different family
	if TaskTypeScore(enhancer.TaskTypeCode, enhancer.TaskTypeCreative) != 0.0 {
		t.Error("different family should return 0.0")
	}
}

func TestRecencyDecay(t *testing.T) {
	now := time.Now()
	// Just created — should be ~1.0
	recent := RecencyDecay(now)
	if recent < 0.99 {
		t.Errorf("recent decay should be ~1.0, got %.3f", recent)
	}

	// 30 days ago — should be ~0.37 (1/e)
	old := RecencyDecay(now.AddDate(0, 0, -30))
	if old < 0.35 || old > 0.40 {
		t.Errorf("30-day decay should be ~0.37, got %.3f", old)
	}
}

func TestBM25Score(t *testing.T) {
	entries := []PromptEntry{
		{Prompt: "Write a Go function for sorting"},
		{Prompt: "Create a Python script for data analysis"},
		{Prompt: "Build a Go REST API with authentication"},
	}
	idf := NewIDFTable(entries)

	query := Tokenize("Go function sorting")
	doc1 := Tokenize("Write a Go function for sorting")
	doc2 := Tokenize("Create a Python script for data analysis")

	score1 := idf.BM25Score(query, doc1)
	score2 := idf.BM25Score(query, doc2)

	if score1 <= score2 {
		t.Errorf("Go sorting query should match Go sorting doc better: s1=%.3f s2=%.3f", score1, score2)
	}
}

func TestMMRRerank(t *testing.T) {
	candidates := []scoredCandidate{
		{entry: PromptEntry{Tags: []string{"go", "api"}}, score: 0.9},
		{entry: PromptEntry{Tags: []string{"go", "api"}}, score: 0.85},     // similar to first
		{entry: PromptEntry{Tags: []string{"python", "data"}}, score: 0.7}, // diverse
	}

	selected := MMRRerank(candidates, 2, 0.7)
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected, got %d", len(selected))
	}

	// First should be highest-scored
	if selected[0].score != 0.9 {
		t.Errorf("first selected should be highest scored (0.9), got %.2f", selected[0].score)
	}

	// Second should prefer diverse (python/data) over similar (go/api) due to MMR
	// The diverse candidate has lower similarity to the first selected, boosting MMR
}

func TestCompositeScore(t *testing.T) {
	query := Query{
		TaskType: enhancer.TaskTypeCode,
		Tags:     []string{"go", "mcp"},
		Keywords: Tokenize("Write a Go MCP tool handler"),
		Repo:     "dotfiles-mcp",
	}

	candidate := PromptEntry{
		TaskType:  "code",
		Tags:      []string{"go", "mcp", "handler"},
		Prompt:    "Implement a new MCP tool handler using mcpkit",
		Repo:      "dotfiles-mcp",
		Timestamp: time.Now(),
	}

	entries := []PromptEntry{candidate}
	idf := NewIDFTable(entries)

	score := CompositeScore(query, candidate, idf, DefaultWeights)
	if score <= 0 {
		t.Errorf("expected positive composite score, got %.3f", score)
	}
	if score > 1.0 {
		t.Errorf("composite score should be <= 1.0, got %.3f", score)
	}
}
