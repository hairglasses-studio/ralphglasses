package session

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
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
			prompt := buildFailedTaskPrompt(obs)
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
			prompt := buildNoopTaskPrompt(obs)
			task = &CycleTask{
				Title:    title,
				Prompt:   prompt,
				Source:   "finding",
				Priority: clampPriority(0.5),
				Status:   "pending",
			}

		case "regressed":
			title := fmt.Sprintf("Fix regression: %s", cycleTruncate(obs.TaskTitle, 80))
			prompt := buildRegressedTaskPrompt(obs)
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
			prompt := buildStalledTaskPrompt(obs)
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

		priority, size := parseRoadmapAnnotations(trimmed)

		tasks = append(tasks, CycleTask{
			Title:    title,
			Prompt:   prompt,
			Source:   "roadmap",
			Priority: priority,
			Size:     size,
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

// buildFailedTaskPrompt creates a structured prompt for a failed observation.
func buildFailedTaskPrompt(obs LoopObservation) string {
	var b strings.Builder
	b.WriteString("<context>\n")
	fmt.Fprintf(&b, "A %s-provider session in repo %q failed.\n", obs.WorkerProvider, obs.RepoName)
	if obs.Error != "" {
		b.WriteString("\nError output:\n```\n")
		b.WriteString(obs.Error)
		b.WriteString("\n```\n")
	}
	if len(obs.DiffPaths) > 0 {
		b.WriteString("\nFiles touched before failure: ")
		b.WriteString(strings.Join(obs.DiffPaths, ", "))
		b.WriteString("\n")
	}
	b.WriteString("</context>\n\n")

	b.WriteString("<instructions>\n")
	fmt.Fprintf(&b, "1. Diagnose the root cause of the failure in task %q\n", obs.TaskTitle)
	b.WriteString("2. Apply a minimal fix targeting only the root cause\n")
	b.WriteString("3. Verify the fix resolves the error\n")
	b.WriteString("</instructions>\n\n")

	b.WriteString("<constraints>\n")
	b.WriteString("- Make the smallest change that fixes the issue\n")
	b.WriteString("- Do not refactor unrelated code\n")
	b.WriteString("</constraints>\n\n")

	b.WriteString("<verification>\n")
	b.WriteString("- go vet ./...\n")
	b.WriteString("- go test ./... -count=1\n")
	b.WriteString("</verification>")
	return b.String()
}

// buildNoopTaskPrompt creates a structured prompt for a no-op observation.
func buildNoopTaskPrompt(obs LoopObservation) string {
	var b strings.Builder
	b.WriteString("<context>\n")
	fmt.Fprintf(&b, "A %s-provider session in repo %q completed but produced zero file changes.\n",
		obs.WorkerProvider, obs.RepoName)
	fmt.Fprintf(&b, "Loop ID: %s, iteration: %d\n", obs.LoopID, obs.IterationNumber)
	b.WriteString("</context>\n\n")

	b.WriteString("<instructions>\n")
	fmt.Fprintf(&b, "1. Read the original task: %q\n", obs.TaskTitle)
	b.WriteString("2. Determine why no files were changed — was the task already done, was the prompt unclear, or was there a blocker?\n")
	b.WriteString("3. If work is still needed, implement it. If the task is already complete, confirm with a verification check.\n")
	b.WriteString("</instructions>\n\n")

	b.WriteString("<verification>\n")
	b.WriteString("- go test ./... -count=1\n")
	b.WriteString("</verification>")
	return b.String()
}

// buildRegressedTaskPrompt creates a structured prompt for a regressed observation.
func buildRegressedTaskPrompt(obs LoopObservation) string {
	var b strings.Builder
	b.WriteString("<context>\n")
	fmt.Fprintf(&b, "A previously-passing task in repo %q has regressed — verification now fails.\n", obs.RepoName)
	if obs.Error != "" {
		b.WriteString("\nRegression error:\n```\n")
		b.WriteString(obs.Error)
		b.WriteString("\n```\n")
	}
	if len(obs.DiffPaths) > 0 {
		b.WriteString("\nFiles changed in the regressing session: ")
		b.WriteString(strings.Join(obs.DiffPaths, ", "))
		b.WriteString("\n")
	}
	b.WriteString("</context>\n\n")

	b.WriteString("<instructions>\n")
	fmt.Fprintf(&b, "1. Identify what changed to cause the regression in task %q\n", obs.TaskTitle)
	b.WriteString("2. Fix the regression while preserving the original intended behavior\n")
	b.WriteString("3. Verify both the original task and the regression fix pass\n")
	b.WriteString("</instructions>\n\n")

	b.WriteString("<verification>\n")
	b.WriteString("- go vet ./...\n")
	b.WriteString("- go test ./... -count=1\n")
	b.WriteString("</verification>")
	return b.String()
}

// buildStalledTaskPrompt creates a structured prompt for a stalled observation.
func buildStalledTaskPrompt(obs LoopObservation) string {
	var b strings.Builder
	b.WriteString("<context>\n")
	fmt.Fprintf(&b, "A session in repo %q stalled — it stopped making progress.\n", obs.RepoName)
	fmt.Fprintf(&b, "Loop ID: %s, provider: %s\n", obs.LoopID, obs.WorkerProvider)
	if obs.TotalLatencyMs > 0 {
		fmt.Fprintf(&b, "Elapsed before stall: %.0fs\n", float64(obs.TotalLatencyMs)/1000)
	}
	b.WriteString("</context>\n\n")

	b.WriteString("<instructions>\n")
	fmt.Fprintf(&b, "1. Investigate why task %q stalled\n", obs.TaskTitle)
	b.WriteString("2. Check for infinite loops, missing inputs, or blocked dependencies\n")
	b.WriteString("3. Apply a fix and verify the task can complete\n")
	b.WriteString("</instructions>\n\n")

	b.WriteString("<verification>\n")
	b.WriteString("- go test ./... -count=1\n")
	b.WriteString("</verification>")
	return b.String()
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

// priorityTagRe matches backtick-wrapped priority annotations like `P0`, `P1`, `P2`.
var priorityTagRe = regexp.MustCompile("`(P[0-2])`")

// sizeTagRe matches backtick-wrapped size annotations like `S`, `M`, `L`.
var sizeTagRe = regexp.MustCompile("`([SML])`")

// parseRoadmapAnnotations extracts priority and size from backtick annotations
// in a ROADMAP line. Returns (priority float64, size string).
// Priority mapping: P0→1.0, P1→0.8, P2→0.5, missing→0.5.
// Size: "S", "M", "L", or "" if missing.
func parseRoadmapAnnotations(line string) (float64, string) {
	priority := 0.5
	if m := priorityTagRe.FindStringSubmatch(line); len(m) > 1 {
		switch m[1] {
		case "P0":
			priority = 1.0
		case "P1":
			priority = 0.8
		case "P2":
			priority = 0.5
		}
	}

	var size string
	if m := sizeTagRe.FindStringSubmatch(line); len(m) > 1 {
		size = m[1]
	}

	return priority, size
}

// EnhanceCycleTasks runs the local deterministic enhancement pipeline on each
// task's prompt. This applies specificity, structure, self-check injection, and
// other stages from the enhancer package (~100ms per prompt, no LLM calls).
func EnhanceCycleTasks(tasks []CycleTask) []CycleTask {
	for i := range tasks {
		result := enhancer.Enhance(tasks[i].Prompt, enhancer.TaskTypeTroubleshooting)
		if result.Enhanced != tasks[i].Prompt && len(result.StagesRun) > 0 {
			tasks[i].Prompt = result.Enhanced
		}
	}
	return tasks
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
