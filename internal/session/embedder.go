package session

import (
	"hash/fnv"
	"math"
	"strings"
)

// Embedder produces vector embeddings from text for similarity search.
type Embedder interface {
	Embed(text string) ([]float64, error)
}

// TrigramEmbedder implements Embedder using character trigram frequency vectors
// hashed to a fixed number of dimensions. Pure Go, no GPU needed.
type TrigramEmbedder struct {
	Dimensions int // default 128
}

// NewTrigramEmbedder creates an embedder with the given dimensions.
// If dimensions <= 0, defaults to 128.
func NewTrigramEmbedder(dimensions int) *TrigramEmbedder {
	if dimensions <= 0 {
		dimensions = 128
	}
	return &TrigramEmbedder{Dimensions: dimensions}
}

// Embed converts text into a normalized vector of character trigram frequencies.
//  1. Lowercase the text
//  2. Extract all character trigrams (e.g., "hello" -> "hel", "ell", "llo")
//  3. Hash each trigram to a bucket [0, Dimensions) using FNV-1a
//  4. Increment the count in that bucket
//  5. L2-normalize the resulting vector
func (e *TrigramEmbedder) Embed(text string) ([]float64, error) {
	vec := make([]float64, e.Dimensions)

	lower := strings.ToLower(text)
	runes := []rune(lower)

	if len(runes) < 3 {
		return vec, nil
	}

	for i := 0; i <= len(runes)-3; i++ {
		trigram := string(runes[i : i+3])
		bucket := hashTrigram(trigram, e.Dimensions)
		vec[bucket]++
	}

	// L2-normalize
	l2Normalize(vec)

	return vec, nil
}

// hashTrigram hashes a trigram string to a bucket index using FNV-1a.
func hashTrigram(trigram string, dimensions int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(trigram))
	return int(h.Sum32()) % dimensions
}

// l2Normalize normalizes a vector in-place to unit length.
func l2Normalize(vec []float64) {
	var sumSq float64
	for _, v := range vec {
		sumSq += v * v
	}
	norm := math.Sqrt(sumSq)
	if norm == 0 {
		return
	}
	for i := range vec {
		vec[i] /= norm
	}
}

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns 0 if either vector is zero-length or all zeros.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}

	minLen := min(len(b), len(a))

	var dot, normA, normB float64
	for i := 0; i < minLen; i++ {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	// Account for remaining elements in the longer vector
	for i := minLen; i < len(a); i++ {
		normA += a[i] * a[i]
	}
	for i := minLen; i < len(b); i++ {
		normB += b[i] * b[i]
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
