package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

func (s *Server) handlePromptABTest(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	promptA := getStringArg(req, "prompt_a")
	if promptA == "" {
		return codedError(ErrInvalidParams, "prompt_a required"), nil
	}
	promptB := getStringArg(req, "prompt_b")
	if promptB == "" {
		return codedError(ErrInvalidParams, "prompt_b required"), nil
	}

	targetProvider := getStringArg(req, "target_provider")
	if targetProvider == "" {
		targetProvider = "openai"
	}

	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	// Map target provider name.
	var tp enhancer.ProviderName
	switch strings.ToLower(targetProvider) {
	case "claude":
		tp = enhancer.ProviderClaude
	case "gemini":
		tp = enhancer.ProviderGemini
	case "openai", "codex":
		tp = enhancer.ProviderOpenAI
	default:
		tp = enhancer.ProviderOpenAI
	}

	// Score both prompts.
	taskType := enhancer.Classify(promptA)
	lintsA := enhancer.Lint(promptA)
	arA := enhancer.Analyze(promptA)
	scoreA := enhancer.Score(promptA, taskType, lintsA, &arA, tp)

	taskTypeB := enhancer.Classify(promptB)
	lintsB := enhancer.Lint(promptB)
	arB := enhancer.Analyze(promptB)
	scoreB := enhancer.Score(promptB, taskTypeB, lintsB, &arB, tp)

	type promptMetrics struct {
		Prompt       string `json:"prompt_preview"`
		OverallScore int    `json:"overall_score"`
		OverallGrade string `json:"overall_grade"`
		LintWarnings int    `json:"lint_warnings"`
		LintErrors   int    `json:"lint_errors"`
		WordCount    int    `json:"word_count"`
		TaskType     string `json:"task_type"`
		Dimensions   int    `json:"dimensions_scored"`
	}

	metricsA := promptMetrics{
		Prompt:       truncatePrompt(promptA, 100),
		OverallScore: scoreA.Overall,
		OverallGrade: scoreA.Grade,
		LintWarnings: countSeverity(lintsA, "warning"),
		LintErrors:   countSeverity(lintsA, "error"),
		WordCount:    len(strings.Fields(promptA)),
		TaskType:     string(taskType),
		Dimensions:   len(scoreA.Dimensions),
	}

	metricsB := promptMetrics{
		Prompt:       truncatePrompt(promptB, 100),
		OverallScore: scoreB.Overall,
		OverallGrade: scoreB.Grade,
		LintWarnings: countSeverity(lintsB, "warning"),
		LintErrors:   countSeverity(lintsB, "error"),
		WordCount:    len(strings.Fields(promptB)),
		TaskType:     string(taskTypeB),
		Dimensions:   len(scoreB.Dimensions),
	}

	// Determine winner. Check high thresholds first to avoid shadowing.
	diff := scoreA.Overall - scoreB.Overall
	absDiff := int(math.Abs(float64(diff)))
	winner := "tie"
	confidence := "low"
	switch {
	case absDiff <= 5:
		winner = "tie"
		confidence = "low"
	case absDiff <= 15:
		if diff > 0 {
			winner = "A"
		} else {
			winner = "B"
		}
		confidence = "medium"
	default: // absDiff > 15
		if diff > 0 {
			winner = "A"
		} else {
			winner = "B"
		}
		confidence = "high"
	}

	// Collect suggestions from the winner's dimensions.
	var winnerSuggestions []string
	winnerScore := scoreB
	if winner == "A" {
		winnerScore = scoreA
	}
	if winner != "tie" {
		for _, d := range winnerScore.Dimensions {
			winnerSuggestions = append(winnerSuggestions, d.Suggestions...)
		}
	}

	// Dimension comparison.
	type dimDiff struct {
		Dimension string `json:"dimension"`
		ScoreA    int    `json:"score_a"`
		ScoreB    int    `json:"score_b"`
		Winner    string `json:"winner"`
	}
	var dimDiffs []dimDiff
	dimMapA := make(map[string]int)
	for _, d := range scoreA.Dimensions {
		dimMapA[d.Name] = d.Score
	}
	for _, d := range scoreB.Dimensions {
		dd := dimDiff{
			Dimension: d.Name,
			ScoreA:    dimMapA[d.Name],
			ScoreB:    d.Score,
		}
		if dd.ScoreA > dd.ScoreB {
			dd.Winner = "A"
		} else if dd.ScoreB > dd.ScoreA {
			dd.Winner = "B"
		} else {
			dd.Winner = "tie"
		}
		dimDiffs = append(dimDiffs, dd)
	}

	testID := fmt.Sprintf("ab-%d", time.Now().Unix())

	result := map[string]any{
		"test_id":            testID,
		"target_provider":    targetProvider,
		"prompt_a":           metricsA,
		"prompt_b":           metricsB,
		"winner":             winner,
		"score_diff":         absDiff,
		"confidence":         confidence,
		"dimensions":         dimDiffs,
		"winner_suggestions": winnerSuggestions,
	}

	// Save results.
	abDir := filepath.Join(repoPath, ".ralph", "ab_tests")
	os.MkdirAll(abDir, 0o755)
	if data, err := json.MarshalIndent(result, "", "  "); err == nil {
		os.WriteFile(filepath.Join(abDir, testID+".json"), data, 0o644)
	}

	return jsonResult(result), nil
}

func truncatePrompt(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func countSeverity(lints []enhancer.LintResult, severity string) int {
	count := 0
	for _, l := range lints {
		if l.Severity == severity {
			count++
		}
	}
	return count
}
