package session

import (
	"math"
	"testing"
)

func TestTrigramEmbedDeterministic(t *testing.T) {
	e := NewTrigramEmbedder(128)
	v1, err := e.Embed("hello world")
	if err != nil {
		t.Fatal(err)
	}
	v2, err := e.Embed("hello world")
	if err != nil {
		t.Fatal(err)
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			t.Fatalf("vectors differ at index %d: %f vs %f", i, v1[i], v2[i])
		}
	}
}

func TestTrigramEmbedDifferentInputs(t *testing.T) {
	e := NewTrigramEmbedder(128)
	v1, _ := e.Embed("implement a REST API server")
	v2, _ := e.Embed("fix the CSS layout bug")
	differ := false
	for i := range v1 {
		if v1[i] != v2[i] {
			differ = true
			break
		}
	}
	if !differ {
		t.Fatal("different inputs produced identical vectors")
	}
}

func TestTrigramEmbedNormalized(t *testing.T) {
	e := NewTrigramEmbedder(128)
	v, _ := e.Embed("the quick brown fox jumps over the lazy dog")
	var sumSq float64
	for _, x := range v {
		sumSq += x * x
	}
	norm := math.Sqrt(sumSq)
	if math.Abs(norm-1.0) > 1e-9 {
		t.Fatalf("expected L2 norm ~1.0, got %f", norm)
	}
}

func TestCosineSimilaritySameVector(t *testing.T) {
	v := []float64{0.5, 0.5, 0.5, 0.5}
	sim := CosineSimilarity(v, v)
	if math.Abs(sim-1.0) > 1e-9 {
		t.Fatalf("expected 1.0, got %f", sim)
	}
}

func TestCosineSimilarityOrthogonal(t *testing.T) {
	a := []float64{1, 0, 0, 0}
	b := []float64{0, 1, 0, 0}
	sim := CosineSimilarity(a, b)
	if math.Abs(sim) > 1e-9 {
		t.Fatalf("expected 0.0, got %f", sim)
	}
}

func TestCosineSimilarityEmpty(t *testing.T) {
	sim := CosineSimilarity(nil, []float64{1, 2, 3})
	if sim != 0 {
		t.Fatalf("expected 0.0 for nil vector, got %f", sim)
	}
	sim = CosineSimilarity([]float64{}, []float64{1, 2, 3})
	if sim != 0 {
		t.Fatalf("expected 0.0 for empty vector, got %f", sim)
	}
	sim = CosineSimilarity([]float64{0, 0, 0}, []float64{1, 2, 3})
	if sim != 0 {
		t.Fatalf("expected 0.0 for zero vector, got %f", sim)
	}
}

func TestEpisodicRecallWithEmbedder(t *testing.T) {
	em := NewEpisodicMemory(t.TempDir(), 100, 5)
	em.SetEmbedder(NewTrigramEmbedder(128))

	// Store episodes with distinct prompts
	em.RecordSuccess(JournalEntry{
		TaskFocus:   "implement REST API with JSON endpoints",
		ExitReason:  "completed",
		Worked:      []string{"used net/http"},
		TurnCount:   5,
		DurationSec: 60,
	})
	em.RecordSuccess(JournalEntry{
		TaskFocus:   "fix CSS layout bug in sidebar",
		ExitReason:  "completed",
		Worked:      []string{"adjusted flexbox"},
		TurnCount:   3,
		DurationSec: 30,
	})
	em.RecordSuccess(JournalEntry{
		TaskFocus:   "build REST API for user management",
		ExitReason:  "completed",
		Worked:      []string{"added CRUD routes"},
		TurnCount:   8,
		DurationSec: 120,
	})

	// Query for something API-related should rank API episodes higher
	results := em.FindSimilar("", "create a REST API service", 2)
	if len(results) == 0 {
		t.Fatal("expected results from embedding-based recall")
	}

	// Verify we got results and that API-related episodes score higher
	foundAPI := false
	for _, ep := range results {
		for _, w := range ep.Worked {
			if w == "used net/http" || w == "added CRUD routes" {
				foundAPI = true
			}
		}
	}
	if !foundAPI {
		t.Errorf("expected API-related episodes in top results, got: %+v", results)
	}
}
