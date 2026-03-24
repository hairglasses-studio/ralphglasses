package session

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// TaskDifficulty captures the estimated difficulty of a task.
type TaskDifficulty struct {
	TaskType              string  `json:"task_type"`
	Title                 string  `json:"title"`
	DifficultyScore       float64 `json:"difficulty"`
	HistoricalSuccessRate float64 `json:"success_rate"`
	AvgTurns              float64 `json:"avg_turns"`
	AvgCostUSD            float64 `json:"avg_cost_usd"`
	SampleCount           int     `json:"sample_count"`
	Recommendation        string  `json:"recommendation"` // "cheap_provider", "expensive_provider", "decompose"
}

// CurriculumEpisode is the subset of episode data needed by the curriculum sorter.
type CurriculumEpisode struct {
	TurnCount int
	CostUSD   float64
	Worked    []string
}

// EpisodicSource retrieves similar episodes for curriculum scoring.
// Implemented by EpisodicMemory when available.
type EpisodicSource interface {
	FindSimilarEpisodes(taskType string, prompt string, k int) []CurriculumEpisode
}

// CurriculumSorter scores and sorts tasks by estimated difficulty.
type CurriculumSorter struct {
	feedback *FeedbackAnalyzer
	episodic EpisodicSource // optional, for richer history
}

// NewCurriculumSorter creates a curriculum sorter. Both arguments can be nil for
// graceful degradation (defaults are used for missing signals).
func NewCurriculumSorter(feedback *FeedbackAnalyzer, episodic EpisodicSource) *CurriculumSorter {
	return &CurriculumSorter{
		feedback: feedback,
		episodic: episodic,
	}
}

// ScoreTask computes a multi-signal difficulty score for a task.
// The score ranges from 0.0 (trivial) to 1.0 (very hard).
func (cs *CurriculumSorter) ScoreTask(task LoopTask) TaskDifficulty {
	td := TaskDifficulty{
		Title: task.Title,
	}

	// 1. Task type difficulty (weight 0.15)
	td.TaskType = classifyTask(task.Title)
	taskTypeScore := taskTypeDifficulty(task.Title)

	// 2. Historical success rate (weight 0.30)
	historicalScore := 0.5
	if cs.feedback != nil {
		if profile, ok := cs.feedback.GetPromptProfile(td.TaskType); ok && profile.SampleCount >= 3 {
			historicalScore = 1.0 - profile.CompletionRate/100.0
			td.AvgTurns = float64(profile.AvgTurns)
			td.AvgCostUSD = profile.AvgCostUSD
			td.HistoricalSuccessRate = profile.CompletionRate / 100.0
			td.SampleCount = profile.SampleCount
		}
	}

	// 3. Prompt complexity (weight 0.25)
	complexityScore := promptComplexity(task.Prompt)

	// 4. Episodic evidence (weight 0.20)
	episodicScore := 0.5
	if cs.episodic != nil {
		episodes := cs.episodic.FindSimilarEpisodes(td.TaskType, task.Title, 5)
		if len(episodes) > 0 {
			totalTurns := 0
			for _, ep := range episodes {
				totalTurns += ep.TurnCount
			}
			avgTurns := float64(totalTurns) / float64(len(episodes))
			switch {
			case avgTurns > 20:
				episodicScore = 0.8
			case avgTurns > 10:
				episodicScore = 0.6
			case avgTurns > 5:
				episodicScore = 0.4
			default:
				episodicScore = 0.2
			}
		}
	}

	// 5. Keyword indicators (weight 0.10)
	keywordScore := keywordDifficulty(task.Title, task.Prompt)

	// Combine weighted signals
	td.DifficultyScore = 0.15*taskTypeScore +
		0.30*historicalScore +
		0.25*complexityScore +
		0.20*episodicScore +
		0.10*keywordScore

	// Clamp final score
	td.DifficultyScore = math.Max(0, math.Min(1, td.DifficultyScore))

	// Set recommendation
	switch {
	case td.DifficultyScore < 0.35:
		td.Recommendation = "cheap_provider"
	case td.DifficultyScore > 0.8:
		td.Recommendation = "decompose"
	default:
		td.Recommendation = "expensive_provider"
	}

	return td
}

// SortTasks scores and sorts tasks by difficulty ascending (easy first).
// Returns a sorted copy; the input slice is not mutated.
func (cs *CurriculumSorter) SortTasks(tasks []LoopTask) []LoopTask {
	type scored struct {
		task  LoopTask
		score float64
	}

	items := make([]scored, len(tasks))
	for i, t := range tasks {
		items[i] = scored{task: t, score: cs.ScoreTask(t).DifficultyScore}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].score < items[j].score
	})

	result := make([]LoopTask, len(items))
	for i, item := range items {
		result[i] = item.task
	}
	return result
}

// ShouldDecompose returns true if a task is too complex for a single worker
// and should be broken into sub-tasks.
func (cs *CurriculumSorter) ShouldDecompose(difficulty TaskDifficulty) bool {
	return difficulty.DifficultyScore > 0.8 &&
		(difficulty.HistoricalSuccessRate < 0.5 || difficulty.SampleCount < 3)
}

// DecompositionPrompt generates a prompt asking an LLM to break a complex task
// into smaller, independently verifiable sub-tasks.
func (cs *CurriculumSorter) DecompositionPrompt(task LoopTask) string {
	return fmt.Sprintf(`The following task appears too complex for a single worker. Break it into 2-3 smaller, independently verifiable sub-tasks:

Task: %s
%s

Requirements for sub-tasks:
- Each sub-task should be completable in under 15 turns
- Each sub-task should be independently testable
- Order sub-tasks from easiest to hardest`, task.Title, task.Prompt)
}

// taskTypeDifficulty returns a base difficulty score using keyword scanning on
// the original title text. This is separate from classifyTask categories and
// provides finer-grained scoring.
func taskTypeDifficulty(title string) float64 {
	lower := strings.ToLower(title)

	// Ordered from easiest to hardest; first match wins.
	keywords := []struct {
		score float64
		words []string
	}{
		{0.2, []string{"test", "lint", "format"}},
		{0.25, []string{"docs", "comment"}},
		{0.5, []string{"fix", "bug"}},
		{0.6, []string{"feature", "add", "implement"}},
		{0.65, []string{"refactor"}},
		{0.7, []string{"debug"}},
		{0.8, []string{"architecture", "design"}},
	}

	for _, kw := range keywords {
		for _, w := range kw.words {
			if strings.Contains(lower, w) {
				return kw.score
			}
		}
	}
	return 0.5
}

// promptComplexity computes a heuristic complexity score from prompt text.
func promptComplexity(prompt string) float64 {
	wc := wordCount(prompt)

	var score float64
	switch {
	case wc < 20:
		score = 0.2
	case wc < 50:
		score = 0.4
	case wc < 100:
		score = 0.6
	case wc < 200:
		score = 0.7
	default:
		score = 0.8
	}

	lower := strings.ToLower(prompt)
	if strings.Contains(lower, "multiple files") ||
		strings.Contains(lower, "across") ||
		strings.Contains(lower, "several") {
		score += 0.1
	}
	if strings.Contains(lower, "breaking change") ||
		strings.Contains(lower, "backward compat") ||
		strings.Contains(lower, "migration") {
		score += 0.1
	}

	return math.Max(0, math.Min(1, score))
}

// keywordDifficulty scans title and prompt for explicit difficulty indicators.
func keywordDifficulty(title, prompt string) float64 {
	combined := strings.ToLower(title + " " + prompt)

	easyKeywords := []string{"simple", "trivial", "minor", "typo", "rename"}
	for _, kw := range easyKeywords {
		if strings.Contains(combined, kw) {
			return 0.15
		}
	}

	hardKeywords := []string{"complex", "critical", "overhaul", "rewrite", "redesign"}
	for _, kw := range hardKeywords {
		if strings.Contains(combined, kw) {
			return 0.85
		}
	}

	return 0.5
}

// wordCount returns the number of whitespace-separated words in s.
func wordCount(s string) int {
	return len(strings.Fields(s))
}
