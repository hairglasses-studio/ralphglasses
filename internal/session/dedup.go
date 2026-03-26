package session

import (
	"strings"
)

// DefaultSimilarityThreshold is the Jaccard similarity threshold above which
// two task titles are considered near-duplicates.
const DefaultSimilarityThreshold = 0.8

// JaccardSimilarity computes word-level Jaccard similarity between two strings.
// Both strings are lowercased and split on whitespace. Returns a value between
// 0.0 (no word overlap) and 1.0 (identical word sets).
func JaccardSimilarity(a, b string) float64 {
	wordsA := wordSet(a)
	wordsB := wordSet(b)

	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 1.0 // two empty strings are identical
	}
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0.0
	}

	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}

	union := len(wordsA)
	for w := range wordsB {
		if !wordsA[w] {
			union++
		}
	}

	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// IsSimilarTask checks whether title is similar to any entry in completed
// above the given threshold. Returns whether a match was found and the
// matched title.
func IsSimilarTask(title string, completed []string, threshold float64) (bool, string) {
	for _, c := range completed {
		if JaccardSimilarity(title, c) >= threshold {
			return true, c
		}
	}
	return false, ""
}

// filterDuplicateTasks removes tasks whose titles are exact or near-duplicate
// matches against a list of completed task titles. It returns the filtered
// task slice (which may be shorter than the input).
func filterDuplicateTasks(tasks []LoopTask, completedTitles []string, threshold float64) []LoopTask {
	if len(completedTitles) == 0 {
		return tasks
	}

	// Build a set of lowercased completed titles for fast exact matching.
	exactSet := make(map[string]struct{}, len(completedTitles))
	for _, t := range completedTitles {
		exactSet[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
	}

	filtered := make([]LoopTask, 0, len(tasks))
	for _, task := range tasks {
		lower := strings.ToLower(strings.TrimSpace(task.Title))

		// Exact match check.
		if _, ok := exactSet[lower]; ok {
			continue
		}

		// Near-duplicate check via Jaccard similarity.
		if similar, _ := IsSimilarTask(task.Title, completedTitles, threshold); similar {
			continue
		}

		filtered = append(filtered, task)
	}
	return filtered
}

// wordSet is defined in episodic.go — reused here for Jaccard computation.
