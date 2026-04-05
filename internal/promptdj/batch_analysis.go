package promptdj

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

// BatchAnalysisConfig controls the batch analysis pipeline.
type BatchAnalysisConfig struct {
	IndexPath  string // path to .prompt-index.jsonl
	MinWords   int    // minimum word count to analyze (default 5)
	MaxPrompts int    // max prompts to process (0 = unlimited)
}

// BatchAnalysisResult holds aggregate analysis of registry prompts.
type BatchAnalysisResult struct {
	TotalPrompts    int                    `json:"total_prompts"`
	AnalyzedCount   int                    `json:"analyzed_count"`
	SkippedCount    int                    `json:"skipped_count"`
	AvgScore        float64                `json:"avg_score"`
	ScoreDistribution map[string]int       `json:"score_distribution"` // grade -> count
	ByTaskType      map[string]int         `json:"by_task_type"`
	ByDomain        map[string]int         `json:"by_domain"`
	ByRepo          map[string]int         `json:"by_repo"`
	TopPrompts      []AnalyzedPrompt       `json:"top_prompts"`       // top 10 by score
	AntiPatterns    map[string]int         `json:"anti_patterns"`     // lint category -> count
	ProcessingMs    int64                  `json:"processing_ms"`
	Recommendations []string              `json:"recommendations"`
}

// AnalyzedPrompt is a single prompt with its analysis results.
type AnalyzedPrompt struct {
	Hash      string   `json:"hash"`
	ShortHash string   `json:"short_hash"`
	Score     int      `json:"score"`
	Grade     string   `json:"grade"`
	TaskType  string   `json:"task_type"`
	Tags      []string `json:"tags"`
	Repo      string   `json:"repo"`
	WordCount int      `json:"word_count"`
	LintCount int      `json:"lint_count"`
	Preview   string   `json:"preview"`
}

// RunBatchAnalysis processes all prompts in the registry index, scoring,
// classifying, and tagging each one. Returns aggregate statistics.
func RunBatchAnalysis(cfg BatchAnalysisConfig) (*BatchAnalysisResult, error) {
	start := time.Now()

	data, err := os.ReadFile(cfg.IndexPath)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}

	result := &BatchAnalysisResult{
		ScoreDistribution: make(map[string]int),
		ByTaskType:        make(map[string]int),
		ByDomain:          make(map[string]int),
		ByRepo:            make(map[string]int),
		AntiPatterns:      make(map[string]int),
	}

	minWords := cfg.MinWords
	if minWords <= 0 {
		minWords = 5
	}

	var totalScore int
	var analyzed []AnalyzedPrompt

	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var rec struct {
			Hash      string `json:"hash"`
			ShortHash string `json:"short_hash"`
			Prompt    string `json:"prompt"`
			Repo      string `json:"repo"`
			WordCount int    `json:"word_count"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}

		result.TotalPrompts++

		if cfg.MaxPrompts > 0 && result.AnalyzedCount >= cfg.MaxPrompts {
			result.SkippedCount++
			continue
		}

		if rec.WordCount < minWords || len(rec.Prompt) < 10 {
			result.SkippedCount++
			continue
		}

		// Score
		ar := enhancer.Analyze(rec.Prompt)
		score := ar.Score
		grade := "F"
		if ar.ScoreReport != nil {
			score = ar.ScoreReport.Overall
			grade = ar.ScoreReport.Grade
		}

		// Classify
		taskType := enhancer.Classify(rec.Prompt)

		// Auto-tag
		tagResult := enhancer.AutoTag(rec.Prompt)

		// Lint
		lints := enhancer.Lint(rec.Prompt)
		for _, l := range lints {
			result.AntiPatterns[l.Category]++
		}

		result.AnalyzedCount++
		totalScore += score
		result.ScoreDistribution[grade]++
		result.ByTaskType[string(taskType)]++
		result.ByRepo[rec.Repo]++
		for _, tag := range tagResult.Tags {
			result.ByDomain[tag]++
		}

		preview := rec.Prompt
		if len(preview) > 150 {
			preview = preview[:150] + "..."
		}

		analyzed = append(analyzed, AnalyzedPrompt{
			Hash:      rec.Hash,
			ShortHash: rec.ShortHash,
			Score:     score,
			Grade:     grade,
			TaskType:  string(taskType),
			Tags:      tagResult.Tags,
			Repo:      rec.Repo,
			WordCount: rec.WordCount,
			LintCount: len(lints),
			Preview:   preview,
		})
	}

	if result.AnalyzedCount > 0 {
		result.AvgScore = float64(totalScore) / float64(result.AnalyzedCount)
	}

	// Sort by score descending, take top 10
	sort.Slice(analyzed, func(i, j int) bool {
		return analyzed[i].Score > analyzed[j].Score
	})
	if len(analyzed) > 10 {
		result.TopPrompts = analyzed[:10]
	} else {
		result.TopPrompts = analyzed
	}

	// Generate recommendations
	result.Recommendations = generateRecommendations(result)
	result.ProcessingMs = time.Since(start).Milliseconds()

	return result, nil
}

func generateRecommendations(r *BatchAnalysisResult) []string {
	var recs []string

	if r.AvgScore < 50 {
		recs = append(recs, fmt.Sprintf("Average score %.0f/100 is low. Run prompt_improve on unsorted prompts to boost quality.", r.AvgScore))
	} else if r.AvgScore < 70 {
		recs = append(recs, fmt.Sprintf("Average score %.0f/100 is moderate. Focus on Structure and Examples dimensions.", r.AvgScore))
	}

	if fCount, ok := r.ScoreDistribution["F"]; ok && fCount > r.AnalyzedCount/3 {
		recs = append(recs, fmt.Sprintf("%d/%d prompts grade F. Consider auto-enhancement via prompt_improve.", fCount, r.AnalyzedCount))
	}

	if agg, ok := r.AntiPatterns["aggressive-emphasis"]; ok && agg > 3 {
		recs = append(recs, fmt.Sprintf("Found %d aggressive emphasis patterns (ALL-CAPS). Claude 4.x performs better with calm instructions.", agg))
	}

	if neg, ok := r.AntiPatterns["negative-framing"]; ok && neg > 2 {
		recs = append(recs, fmt.Sprintf("Found %d negative framing patterns. Rewrite 'don't do X' as 'do Y instead'.", neg))
	}

	if r.AnalyzedCount < 20 {
		recs = append(recs, "Registry has fewer than 20 prompts. Continue capturing to build a representative corpus.")
	}

	return recs
}
