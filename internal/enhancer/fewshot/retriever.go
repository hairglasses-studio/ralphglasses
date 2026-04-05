package fewshot

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

// Retriever finds similar high-quality prompts from the registry and formats
// them as few-shot examples for injection into agent prompts.
type Retriever struct {
	indexPath string
	config    Config
	weights   SimilarityWeights
	cache     *Cache
}

// NewRetriever creates a retriever that reads from the given JSONL index path.
func NewRetriever(indexPath string, cfg Config) *Retriever {
	return &Retriever{
		indexPath: indexPath,
		config:    cfg,
		weights:   DefaultWeights,
		cache:     NewCache(cfg.CacheMaxSize, cfg.CacheTTL),
	}
}

// Retrieve searches the registry for similar high-quality prompts matching the query.
func (r *Retriever) Retrieve(ctx context.Context, prompt, repo string) (*RetrievalResult, error) {
	if !r.config.Enabled {
		return &RetrievalResult{}, nil
	}

	taskType := enhancer.Classify(prompt)
	keywords := Tokenize(prompt)
	var tags []string // inferred from prompt domain

	query := Query{
		Prompt:   prompt,
		TaskType: taskType,
		Tags:     tags,
		Repo:     repo,
		Keywords: keywords,
	}

	// Check cache
	cacheKey := r.cacheKey(query)
	if cached, ok := r.cache.Get(cacheKey); ok {
		cached.CacheHit = true
		return cached, nil
	}

	start := time.Now()

	// Load index
	entries, err := r.loadIndex()
	if err != nil {
		return &RetrievalResult{}, nil // graceful no-op if index unavailable
	}

	if len(entries) == 0 {
		return &RetrievalResult{}, nil
	}

	// Quality filter
	filtered := r.filterByQuality(entries)
	if len(filtered) == 0 {
		return &RetrievalResult{Candidates: len(entries)}, nil
	}

	// Build IDF table
	idf := NewIDFTable(filtered)

	// Score all candidates
	scored := make([]scoredCandidate, 0, len(filtered))
	for _, entry := range filtered {
		score := CompositeScore(query, entry, idf, r.weights)
		scored = append(scored, scoredCandidate{entry: entry, score: score})
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Take top MaxCandidates
	maxCand := r.config.MaxCandidates
	if maxCand <= 0 {
		maxCand = r.config.K * 2
	}
	if len(scored) > maxCand {
		scored = scored[:maxCand]
	}

	// MMR re-rank for diversity
	selected := MMRRerank(scored, r.config.K, r.config.MMRLambda)

	// Build result
	result := &RetrievalResult{
		Candidates:   len(entries),
		SearchTimeMs: time.Since(start).Milliseconds(),
	}

	tokenBudget := r.config.MaxTokenBudget * 4 // chars (4 chars ≈ 1 token)
	usedChars := 0

	for _, sc := range selected {
		promptText := sc.entry.Prompt
		// Truncate to fit token budget
		perExampleBudget := tokenBudget / r.config.K
		if len(promptText) > perExampleBudget {
			promptText = promptText[:perExampleBudget] + "..."
		}
		usedChars += len(promptText)
		if usedChars > tokenBudget {
			break
		}

		result.Examples = append(result.Examples, RetrievedExample{
			Hash:       sc.entry.Hash,
			ShortHash:  sc.entry.ShortHash,
			Score:      sc.entry.Score,
			Grade:      sc.entry.Grade,
			TaskType:   sc.entry.TaskType,
			Tags:       sc.entry.Tags,
			Similarity: sc.score,
			Prompt:     promptText,
		})
	}

	// Format as XML block
	result.XMLBlock = FormatXML(result.Examples, string(taskType))

	// Cache the result
	r.cache.Put(cacheKey, result)

	return result, nil
}

// filterByQuality returns entries meeting quality and status requirements.
func (r *Retriever) filterByQuality(entries []PromptEntry) []PromptEntry {
	var filtered []PromptEntry
	for _, e := range entries {
		if e.Score < r.config.MinScore {
			continue
		}
		if e.Status != "scored" && e.Status != "improved" {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// loadIndex reads the JSONL file and parses entries.
func (r *Retriever) loadIndex() ([]PromptEntry, error) {
	data, err := os.ReadFile(r.indexPath)
	if err != nil {
		return nil, err
	}

	var entries []PromptEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var raw struct {
			Hash      string   `json:"hash"`
			ShortHash string   `json:"short_hash"`
			Prompt    string   `json:"prompt"`
			Score     int      `json:"score"`
			Grade     string   `json:"grade"`
			TaskType  string   `json:"task_type"`
			Tags      []string `json:"tags"`
			Repo      string   `json:"repo"`
			Status    string   `json:"status"`
			Timestamp string   `json:"timestamp"`
		}
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, raw.Timestamp)
		entries = append(entries, PromptEntry{
			Hash: raw.Hash, ShortHash: raw.ShortHash, Prompt: raw.Prompt,
			Score: raw.Score, Grade: raw.Grade, TaskType: raw.TaskType,
			Tags: raw.Tags, Repo: raw.Repo, Status: raw.Status, Timestamp: ts,
		})
	}
	return entries, nil
}

// cacheKey generates a deterministic cache key from query features.
func (r *Retriever) cacheKey(q Query) string {
	tags := make([]string, len(q.Tags))
	copy(tags, q.Tags)
	sort.Strings(tags)
	kw := make([]string, len(q.Keywords))
	copy(kw, q.Keywords)
	sort.Strings(kw)
	return fmt.Sprintf("%s|%s|%s|%s", q.TaskType, strings.Join(tags, ","),
		strings.Join(kw[:min(10, len(kw))], ","), q.Repo)
}
