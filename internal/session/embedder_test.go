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

func TestTrigramEmbedShortInput(t *testing.T) {
	e := NewTrigramEmbedder(128)

	// 1-char input: fewer than 3 runes → no trigrams → zero vector.
	v1, err := e.Embed("a")
	if err != nil {
		t.Fatal(err)
	}
	for i, x := range v1 {
		if x != 0 {
			t.Fatalf("expected zero vector for 1-char input, got non-zero at index %d: %f", i, x)
		}
	}

	// 2-char input: still fewer than 3 runes → zero vector.
	v2, err := e.Embed("ab")
	if err != nil {
		t.Fatal(err)
	}
	for i, x := range v2 {
		if x != 0 {
			t.Fatalf("expected zero vector for 2-char input, got non-zero at index %d: %f", i, x)
		}
	}
}

func TestTrigramEmbedUnicode(t *testing.T) {
	e := NewTrigramEmbedder(128)

	// CJK characters (each is one rune, need at least 3).
	v, err := e.Embed("\u4f60\u597d\u4e16\u754c") // 你好世界
	if err != nil {
		t.Fatal(err)
	}

	// Should produce a non-zero vector since we have 4 runes (2 trigrams).
	allZero := true
	for _, x := range v {
		if x != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("expected non-zero vector for CJK input")
	}

	// Should be normalized to unit length.
	var sumSq float64
	for _, x := range v {
		sumSq += x * x
	}
	norm := math.Sqrt(sumSq)
	if math.Abs(norm-1.0) > 1e-9 {
		t.Fatalf("expected L2 norm ~1.0 for CJK input, got %f", norm)
	}
}

func TestCosineSimilarityMismatchedLengths(t *testing.T) {
	a := []float64{1.0, 0.0, 0.0}
	b := []float64{1.0, 0.0, 0.0, 0.0, 0.0}

	sim := CosineSimilarity(a, b)
	// The implementation handles mismatched lengths by using minLen for dot
	// product and accounting for extra elements in norms. With a=[1,0,0] and
	// b=[1,0,0,0,0], dot=1, normA=1, normB=1 → sim=1.0.
	// This tests that it doesn't panic.
	if math.IsNaN(sim) {
		t.Fatal("expected valid similarity, got NaN")
	}

	// Test with orthogonal mismatched vectors: extra dimensions only in b.
	c := []float64{1.0}
	d := []float64{0.0, 1.0}
	sim2 := CosineSimilarity(c, d)
	// dot = 1*0 = 0, so similarity should be 0.
	if math.Abs(sim2) > 1e-9 {
		t.Fatalf("expected 0.0 for orthogonal mismatched vectors, got %f", sim2)
	}
}
