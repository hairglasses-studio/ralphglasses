package session

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ObservationsToTasks converts recent loop observations into prioritized cycle tasks.
// It examines failed, regressed, stalled, and noop observations and generates
// actionable tasks. Tasks are deduplicated by Jaccard similarity on titles.
func ObservationsToTasks(observations []LoopObservation) []CycleTask {
	if len(observations) == 0 {
		return nil
	}

	var tasks []CycleTask

	// Count recurrences per error pattern for priority scoring.
	errorCounts := map[string]int{}
	for _, obs := range observations {
		if obs.Error != "" {
			errorCounts[obs.Error]++
		}
	}

	for _, obs := range observations {
		var task *CycleTask

		switch obs.Status {
		case "failed":
			title := fmt.Sprintf("Fix failure: %s", cycleTruncate(obs.TaskTitle, 80))
			prompt := fmt.Sprintf("Fix the failing task %q. Error: %s", obs.TaskTitle, obs.Error)
			priority := 0.8
			if count := errorCounts[obs.Error]; count > 1 {
				priority += float64(count) * 0.05
			}
			priority += obs.TotalCostUSD * 0.1 // cost impact
			task = &CycleTask{
				Title:    title,
				Prompt:   prompt,
				Source:   "finding",
				Priority: clampPriority(priority),
				Status:   "pending",
			}

		case "noop":
			title := fmt.Sprintf("Investigate no-op: %s", cycleTruncate(obs.TaskTitle, 80))
			prompt := fmt.Sprintf("Investigate why task %q produced no changes. Loop ID: %s, iteration: %d",
				obs.TaskTitle, obs.LoopID, obs.IterationNumber)
			task = &CycleTask{
				Title:    title,
				Prompt:   prompt,
				Source:   "finding",
				Priority: clampPriority(0.5),
				Status:   "pending",
			}

		case "regressed":
			title := fmt.Sprintf("Fix regression: %s", cycleTruncate(obs.TaskTitle, 80))
			prompt := fmt.Sprintf("Fix regression in task %q. The verification previously passed but now fails. Error: %s",
				obs.TaskTitle, obs.Error)
			priority := 0.9
			if count := errorCounts[obs.Error]; count > 1 {
				priority += float64(count) * 0.05
			}
			task = &CycleTask{
				Title:    title,
				Prompt:   prompt,
				Source:   "finding",
				Priority: clampPriority(priority),
				Status:   "pending",
			}

		case "stalled":
			title := fmt.Sprintf("Unstall: %s", cycleTruncate(obs.TaskTitle, 80))
			prompt := fmt.Sprintf("Investigate and fix stalled task %q in loop %s", obs.TaskTitle, obs.LoopID)
			task = &CycleTask{
				Title:    title,
				Prompt:   prompt,
				Source:   "finding",
				Priority: clampPriority(0.6),
				Status:   "pending",
			}
		}

		if task != nil {
			tasks = append(tasks, *task)
		}
	}

	// Deduplicate by Jaccard similarity on titles (threshold 0.7).
	tasks = deduplicateTasks(tasks, 0.7)

	// Sort by priority descending.
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Priority > tasks[j].Priority
	})

	return tasks
}

// RoadmapToTasks parses ROADMAP.md open items into cycle tasks.
// It finds unchecked `- [ ]` items, extracts the title and section context,
// and returns them as tasks with source="roadmap".
func RoadmapToTasks(roadmapPath string, maxTasks int) ([]CycleTask, error) {
	f, err := os.Open(roadmapPath)
	if err != nil {
		return nil, fmt.Errorf("open roadmap: %w", err)
	}
	defer f.Close()

	var tasks []CycleTask
	var currentSection string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Track section headers.
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "### ") {
			currentSection = strings.TrimSpace(strings.TrimLeft(line, "#"))
			continue
		}

		// Find unchecked items.
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- [ ] ") {
			continue
		}

		// Extract title: everything after "- [ ] " up to backtick or end.
		titlePart := strings.TrimPrefix(trimmed, "- [ ] ")
		// Strip markdown bold wrapper if present.
		title := titlePart
		if idx := strings.Index(title, "**"); idx >= 0 {
			end := strings.Index(title[idx+2:], "**")
			if end >= 0 {
				title = title[idx+2 : idx+2+end]
			}
		}
		title = strings.TrimSpace(title)
		if title == "" {
			continue
		}

		prompt := fmt.Sprintf("Implement: %s", titlePart)
		if currentSection != "" {
			prompt = fmt.Sprintf("[%s] %s", currentSection, prompt)
		}

		tasks = append(tasks, CycleTask{
			Title:    title,
			Prompt:   prompt,
			Source:   "roadmap",
			Priority: 0.5, // default; caller can re-prioritize
			Status:   "pending",
		})

		if maxTasks > 0 && len(tasks) >= maxTasks {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan roadmap: %w", err)
	}

	return tasks, nil
}

// cycleTruncate shortens s to maxLen, appending "..." if truncated.
func cycleTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// clampPriority clamps a priority value to [0, 1].
func clampPriority(p float64) float64 {
	if p < 0 {
		return 0
	}
	if p > 1 {
		return 1
	}
	return p
}

// cycleJaccardSimilarity computes the Jaccard index between two sets of words.
func cycleJaccardSimilarity(a, b string) float64 {
	wordsA := strings.Fields(strings.ToLower(a))
	wordsB := strings.Fields(strings.ToLower(b))

	setA := make(map[string]bool, len(wordsA))
	for _, w := range wordsA {
		setA[w] = true
	}
	setB := make(map[string]bool, len(wordsB))
	for _, w := range wordsB {
		setB[w] = true
	}

	if len(setA) == 0 && len(setB) == 0 {
		return 1.0
	}

	intersection := 0
	for w := range setA {
		if setB[w] {
			intersection++
		}
	}

	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

// deduplicateTasks removes tasks whose titles are too similar (above threshold).
// Keeps the higher-priority task when duplicates are found.
func deduplicateTasks(tasks []CycleTask, threshold float64) []CycleTask {
	if len(tasks) <= 1 {
		return tasks
	}

	// Sort by priority descending so we keep higher-priority tasks.
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].Priority > tasks[j].Priority
	})

	var result []CycleTask
	for _, t := range tasks {
		duplicate := false
		for _, kept := range result {
			if cycleJaccardSimilarity(t.Title, kept.Title) >= threshold {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, t)
		}
	}
	return result
}
