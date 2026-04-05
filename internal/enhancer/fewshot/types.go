// Package fewshot provides a few-shot example retriever that searches the prompt
// registry for similar high-quality prompts and formats them as injection context.
// Uses BM25-lite keyword scoring, Jaccard tag similarity, and MMR re-ranking.
// No external dependencies — pure Go lexical matching.
package fewshot

import (
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

// Config controls retriever behavior.
type Config struct {
	K              int           `json:"k"`                // number of examples to return (default 3)
	MinScore       int           `json:"min_score"`        // minimum quality score for candidates (default 70)
	MaxCandidates  int           `json:"max_candidates"`   // pre-filter pool size (default 2*K)
	MMRLambda      float64       `json:"mmr_lambda"`       // relevance vs diversity tradeoff (default 0.7)
	MaxTokenBudget int           `json:"max_token_budget"` // max tokens for all examples (default 2000)
	CacheTTL       time.Duration `json:"cache_ttl"`        // cache entry TTL (default 5 min)
	CacheMaxSize   int           `json:"cache_max_size"`   // cache capacity (default 256)
	Enabled        bool          `json:"enabled"`
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		K:              3,
		MinScore:       70,
		MaxCandidates:  6,
		MMRLambda:      0.7,
		MaxTokenBudget: 2000,
		CacheTTL:       5 * time.Minute,
		CacheMaxSize:   256,
		Enabled:        true,
	}
}

// Query represents a retrieval query derived from the user's prompt.
type Query struct {
	Prompt   string
	TaskType enhancer.TaskType
	Tags     []string
	Repo     string
	Keywords []string // extracted from prompt
}

// PromptEntry is a candidate from the prompt registry index.
type PromptEntry struct {
	Hash      string
	ShortHash string
	Prompt    string
	Score     int
	Grade     string
	TaskType  string
	Tags      []string
	Repo      string
	Status    string
	Timestamp time.Time
}

// RetrievedExample is a single selected example with similarity metadata.
type RetrievedExample struct {
	Hash       string   `json:"hash"`
	ShortHash  string   `json:"short_hash"`
	Score      int      `json:"score"`
	Grade      string   `json:"grade"`
	TaskType   string   `json:"task_type"`
	Tags       []string `json:"tags"`
	Similarity float64  `json:"similarity"`
	Prompt     string   `json:"prompt"`
}

// RetrievalResult is the output of a retrieval operation.
type RetrievalResult struct {
	Examples     []RetrievedExample `json:"examples"`
	XMLBlock     string             `json:"xml_block"`
	CacheHit     bool               `json:"cache_hit"`
	SearchTimeMs int64              `json:"search_time_ms"`
	Candidates   int                `json:"candidates"`
}

// Weights for composite similarity scoring.
var DefaultWeights = SimilarityWeights{
	TaskType: 0.30,
	TagOverlap: 0.25,
	Keyword:  0.25,
	Repo:     0.10,
	Recency:  0.10,
}

// SimilarityWeights controls the relative importance of each signal.
type SimilarityWeights struct {
	TaskType   float64
	TagOverlap float64
	Keyword    float64
	Repo       float64
	Recency    float64
}
