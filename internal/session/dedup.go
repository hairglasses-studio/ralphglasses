package session

import (
	"regexp"
	"strings"
)

// DefaultSimilarityThreshold is the Jaccard similarity threshold above which
// two task titles are considered near-duplicates. Lowered from 0.8 to 0.7
// to catch rephrasings like "add tests for session" vs "write test coverage
// for session package".
const DefaultSimilarityThreshold = 0.7

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

// filePathPattern matches Go source file paths in task prompts. It captures
// paths starting with common Go project directories.
var filePathPattern = regexp.MustCompile(`(?:internal|cmd|pkg)/\S+\.go`)

// extractFilePathsFromText returns unique file paths found in text matching the
// standard Go project layout (internal/, cmd/, pkg/ prefixed .go files).
func extractFilePathsFromText(text string) []string {
	matches := filePathPattern.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	unique := make([]string, 0, len(matches))
	for _, m := range matches {
		if _, ok := seen[m]; !ok {
			seen[m] = struct{}{}
			unique = append(unique, m)
		}
	}
	return unique
}

// fileOverlapRatio computes the fraction of paths in "a" that also appear in
// "b". Returns 0 if a is empty.
func fileOverlapRatio(a, b []string) float64 {
	if len(a) == 0 {
		return 0
	}
	bSet := make(map[string]struct{}, len(b))
	for _, p := range b {
		bSet[p] = struct{}{}
	}
	overlap := 0
	for _, p := range a {
		if _, ok := bSet[p]; ok {
			overlap++
		}
	}
	return float64(overlap) / float64(len(a))
}

// ContentOverlapThreshold is the minimum fraction of a proposed task's file
// paths that must overlap with a completed task's file paths for the proposed
// task to be considered a content duplicate.
const ContentOverlapThreshold = 0.5

// filterDuplicateTasksByContent removes proposed tasks whose file paths
// overlap significantly with completed tasks. A proposed task is rejected if
// >50% of its referenced file paths appear in any single completed task's
// prompt. Tasks with no extractable file paths are always kept.
func filterDuplicateTasksByContent(proposed []LoopTask, completed []LoopTask) []LoopTask {
	if len(completed) == 0 {
		return proposed
	}

	// Pre-extract file paths from all completed tasks.
	completedPaths := make([][]string, len(completed))
	for i, c := range completed {
		completedPaths[i] = extractFilePathsFromText(c.Prompt)
	}

	filtered := make([]LoopTask, 0, len(proposed))
	for _, p := range proposed {
		paths := extractFilePathsFromText(p.Prompt)
		if len(paths) == 0 {
			// No file paths to compare — keep the task.
			filtered = append(filtered, p)
			continue
		}

		isDup := false
		for _, cp := range completedPaths {
			if len(cp) == 0 {
				continue
			}
			if fileOverlapRatio(paths, cp) > ContentOverlapThreshold {
				isDup = true
				break
			}
		}
		if !isDup {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// wordSet is defined in episodic.go — reused here for Jaccard computation.
