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

// DedupSkip records why a proposed task was filtered by deduplication.
type DedupSkip struct {
	TaskTitle   string  `json:"task_title"`
	DedupReason string  `json:"dedup_reason"`  // "exact_match", "near_duplicate", "content_overlap"
	MatchedWith string  `json:"matched_with,omitempty"` // title or description of the matched completed task
	Similarity  float64 `json:"similarity,omitempty"`   // Jaccard score for near-duplicate matches
}

// filterDuplicateTasks removes tasks whose titles are exact or near-duplicate
// matches against a list of completed task titles. It returns the filtered
// task slice (which may be shorter than the input).
func filterDuplicateTasks(tasks []LoopTask, completedTitles []string, threshold float64) []LoopTask {
	kept, _ := filterDuplicateTasksWithReason(tasks, completedTitles, threshold)
	return kept
}

// filterDuplicateTasksWithReason removes tasks whose titles are exact or
// near-duplicate matches against completed task titles, and returns both the
// filtered tasks and the list of skipped tasks with their dedup reasons.
func filterDuplicateTasksWithReason(tasks []LoopTask, completedTitles []string, threshold float64) ([]LoopTask, []DedupSkip) {
	if len(completedTitles) == 0 {
		return tasks, nil
	}

	// Build a set of lowercased completed titles for fast exact matching.
	exactSet := make(map[string]struct{}, len(completedTitles))
	for _, t := range completedTitles {
		exactSet[strings.ToLower(strings.TrimSpace(t))] = struct{}{}
	}

	filtered := make([]LoopTask, 0, len(tasks))
	var skipped []DedupSkip
	for _, task := range tasks {
		lower := strings.ToLower(strings.TrimSpace(task.Title))

		// Exact match check.
		if _, ok := exactSet[lower]; ok {
			skipped = append(skipped, DedupSkip{
				TaskTitle:   task.Title,
				DedupReason: "exact_match",
				MatchedWith: lower,
				Similarity:  1.0,
			})
			continue
		}

		// Near-duplicate check via Jaccard similarity.
		if similar, matched := IsSimilarTask(task.Title, completedTitles, threshold); similar {
			skipped = append(skipped, DedupSkip{
				TaskTitle:   task.Title,
				DedupReason: "near_duplicate",
				MatchedWith: matched,
				Similarity:  JaccardSimilarity(task.Title, matched),
			})
			continue
		}

		filtered = append(filtered, task)
	}
	return filtered, skipped
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
	kept, _ := filterDuplicateTasksByContentWithReason(proposed, completed)
	return kept
}

// filterDuplicateTasksByContentWithReason is like filterDuplicateTasksByContent
// but also returns DedupSkip entries for rejected tasks.
func filterDuplicateTasksByContentWithReason(proposed []LoopTask, completed []LoopTask) ([]LoopTask, []DedupSkip) {
	if len(completed) == 0 {
		return proposed, nil
	}

	// Pre-extract file paths from all completed tasks.
	completedPaths := make([][]string, len(completed))
	for i, c := range completed {
		completedPaths[i] = extractFilePathsFromText(c.Prompt)
	}

	filtered := make([]LoopTask, 0, len(proposed))
	var skipped []DedupSkip
	for _, p := range proposed {
		paths := extractFilePathsFromText(p.Prompt)
		if len(paths) == 0 {
			// No file paths to compare — keep the task.
			filtered = append(filtered, p)
			continue
		}

		isDup := false
		matchIdx := -1
		for j, cp := range completedPaths {
			if len(cp) == 0 {
				continue
			}
			if fileOverlapRatio(paths, cp) > ContentOverlapThreshold {
				isDup = true
				matchIdx = j
				break
			}
		}
		if isDup {
			matched := ""
			if matchIdx >= 0 && matchIdx < len(completed) {
				matched = completed[matchIdx].Title
			}
			skipped = append(skipped, DedupSkip{
				TaskTitle:   p.Title,
				DedupReason: "content_overlap",
				MatchedWith: matched,
			})
		} else {
			filtered = append(filtered, p)
		}
	}
	return filtered, skipped
}

// wordSet is defined in episodic.go — reused here for Jaccard computation.
