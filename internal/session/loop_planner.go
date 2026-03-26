package session

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/roadmap"
)

// buildLoopPlannerPromptN builds a planner prompt requesting N parallel tasks.
func buildLoopPlannerPromptN(repoPath string, numTasks int, prev []LoopIteration) (string, error) {
	if numTasks <= 1 {
		return buildLoopPlannerPrompt(repoPath, prev)
	}
	prompt, err := buildLoopPlannerPrompt(repoPath, prev)
	if err != nil {
		return "", err
	}
	// Replace the single-task instruction with multi-task
	prompt = strings.Replace(prompt,
		`CRITICAL: Your ENTIRE response must be a single JSON object. No prose, no markdown fences, no explanation — just the JSON.

{"title":"short task title","prompt":"detailed implementation prompt for the worker"}

BAD (do NOT do this):
  Here's what I suggest: {"title":"...","prompt":"..."}

GOOD (do this):
  {"title":"add unit tests for error handling","prompt":"Add tests in internal/session/..."}

Constraints:
- Output ONLY the JSON object. Nothing before it, nothing after it.`,
		fmt.Sprintf(`CRITICAL: Your ENTIRE response must be a single JSON array. No prose, no markdown fences, no explanation — just the JSON.

Choose up to %d independent tasks that can run in parallel (no file conflicts).
[{"title":"task 1","prompt":"implementation prompt"},{"title":"task 2","prompt":"implementation prompt"}]

Each task runs in its own git worktree, so they must not modify the same files.

BAD (do NOT do this):
  Here are the tasks: [{"title":"...","prompt":"..."}]

GOOD (do this):
  [{"title":"add unit tests","prompt":"Add tests in..."},{"title":"fix lint warnings","prompt":"Fix..."}]

Constraints:
- Output ONLY the JSON array. Nothing before it, nothing after it.`, numTasks),
		1)
	return prompt, nil
}

func buildLoopPlannerPrompt(repoPath string, prevIterations []LoopIteration) (string, error) {
	var sections []string
	sections = append(sections, `You are the planner for a perpetual development loop.

CRITICAL: Your ENTIRE response must be a single JSON object. No prose, no markdown fences, no explanation — just the JSON.

{"title":"short task title","prompt":"detailed implementation prompt for the worker"}

BAD (do NOT do this):
  Here's what I suggest: {"title":"...","prompt":"..."}

GOOD (do this):
  {"title":"add unit tests for error handling","prompt":"Add tests in internal/session/..."}

Constraints:
- Output ONLY the JSON object. Nothing before it, nothing after it.
- Pick the highest-impact unfinished task that is safe to execute next.
- Keep the worker task concrete and implementation-focused.
- Assume verification will run after the worker finishes.
- Prefer variety in task types. If recent iterations were all bug fixes, choose a test, docs, or refactor task instead.`)

	roadmapPath := filepath.Join(repoPath, "ROADMAP.md")
	if _, err := os.Stat(roadmapPath); err == nil {
		if rm, err := roadmap.Parse(roadmapPath); err == nil {
			analysis, analyzeErr := roadmap.Analyze(rm, repoPath)
			if analyzeErr == nil {
				var ready []string
				for i, item := range analysis.Ready {
					if i >= 5 {
						break
					}
					ready = append(ready, fmt.Sprintf("- %s: %s", item.TaskID, item.Description))
				}
				sections = append(sections, fmt.Sprintf(
					"Roadmap summary:\n- Title: %s\n- Completion: %d/%d\n- Ready tasks:\n%s",
					rm.Title,
					rm.Stats.Completed,
					rm.Stats.Total,
					joinOrPlaceholder(ready, "- none detected"),
				))
			}
		}
	}

	issueLedgerPath := filepath.Join(repoPath, "docs", "issue-ledger.json")
	if data, err := os.ReadFile(issueLedgerPath); err == nil && len(data) > 0 {
		sections = append(sections, "Issue ledger:\n"+truncateForPrompt(string(data), 2500))
	}

	journal, err := ReadRecentJournal(repoPath, 5)
	if err == nil && len(journal) > 0 {
		sections = append(sections, "Recent journal context:\n"+SynthesizeContext(journal))
	}

	// Inject corrective guidance from previous iterations.
	if len(prevIterations) > 0 {
		last := prevIterations[len(prevIterations)-1]
		sections = append(sections, fmt.Sprintf(
			"Previous iteration: task=%q status=%s", last.Task.Title, last.Status))
		if last.HasQuestions {
			sections = append(sections,
				`IMPORTANT: The previous worker asked questions instead of acting autonomously.
In headless mode, no human will answer. Re-task with explicit instructions to make autonomous decisions using conservative defaults.`)
		}

		// Completed tasks dedup: list successful iterations so the planner avoids repeating them.
		var completedTitles []string
		for _, iter := range prevIterations {
			if iter.Status != "failed" && iter.Task.Title != "" {
				completedTitles = append(completedTitles, fmt.Sprintf("- %s", iter.Task.Title))
			}
		}
		if len(completedTitles) > 0 {
			sections = append(sections,
				"Completed tasks (DO NOT repeat these):\n"+strings.Join(completedTitles, "\n"))
		}

		// Inject recent task types for diversity steering.
		recentCount := 3
		if recentCount > len(prevIterations) {
			recentCount = len(prevIterations)
		}
		var recentTypes []string
		for _, iter := range prevIterations[len(prevIterations)-recentCount:] {
			recentTypes = append(recentTypes, fmt.Sprintf("- %s (status: %s)", iter.Task.Title, iter.Status))
		}
		sections = append(sections,
			"Recent task types (prefer a different kind of task):\n"+strings.Join(recentTypes, "\n"))
	}

	// Include recent git log subjects so the planner knows what was recently committed.
	if gitLog, err := recentGitLog(repoPath, 10); err == nil && gitLog != "" {
		sections = append(sections, "Recent git commits:\n"+gitLog)
	}

	return strings.Join(sections, "\n\n"), nil
}

// plannerTasksFromSession extracts up to maxTasks from the planner output.
// It tries to parse a JSON array first; if that fails, falls back to single task.
func plannerTasksFromSession(s *Session, maxTasks int) ([]LoopTask, string, error) {
	output := sessionOutputSummary(s)

	// Try multi-task parse first (JSON array)
	if maxTasks > 1 {
		tasks, err := parsePlannerTasks(output)
		if err == nil && len(tasks) > 0 {
			if len(tasks) > maxTasks {
				tasks = tasks[:maxTasks]
			}
			return tasks, output, nil
		}

		// Try from session fields
		s.mu.Lock()
		for _, candidate := range []string{s.LastOutput, strings.Join(s.OutputHistory, "\n")} {
			tasks, parseErr := parsePlannerTasks(candidate)
			if parseErr == nil && len(tasks) > 0 {
				s.mu.Unlock()
				if len(tasks) > maxTasks {
					tasks = tasks[:maxTasks]
				}
				return tasks, candidate, nil
			}
		}
		s.mu.Unlock()
	}

	// Fall back to single task
	task, out, err := plannerTaskFromSession(s)
	if err != nil {
		return nil, out, err
	}
	return []LoopTask{task}, out, nil
}

// parsePlannerTasks tries to parse a JSON array of tasks from planner output.
func parsePlannerTasks(text string) ([]LoopTask, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("empty output")
	}

	// Try direct parse
	var tasks []LoopTask
	for _, candidate := range plannerJSONArrayCandidates(text) {
		if err := json.Unmarshal([]byte(candidate), &tasks); err == nil && len(tasks) > 0 {
			valid := make([]LoopTask, 0, len(tasks))
			for _, t := range tasks {
				t.Title = sanitizeTaskTitle(t.Title)
				t.Prompt = strings.TrimSpace(t.Prompt)
				if t.Title != "" && t.Prompt != "" {
					valid = append(valid, t)
				}
			}
			if len(valid) > 0 {
				return valid, nil
			}
		}
	}

	return nil, errors.New("no task array found in planner output")
}

func plannerJSONArrayCandidates(text string) []string {
	var out []string
	out = append(out, text)

	reFence := regexp.MustCompile("(?s)```json\\s*(\\[.*?\\])\\s*```")
	if matches := reFence.FindStringSubmatch(text); len(matches) == 2 {
		out = append(out, matches[1])
	}

	start := strings.IndexByte(text, '[')
	end := strings.LastIndexByte(text, ']')
	if start >= 0 && end > start {
		out = append(out, text[start:end+1])
	}

	return dedupeStrings(out)
}

func plannerTaskFromSession(s *Session) (LoopTask, string, error) {
	output := sessionOutputSummary(s)
	task, err := parsePlannerTask(output)
	if err == nil {
		return task, output, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, candidate := range []string{s.LastOutput, strings.Join(s.OutputHistory, "\n")} {
		task, parseErr := parsePlannerTask(candidate)
		if parseErr == nil {
			return task, candidate, nil
		}
	}

	return LoopTask{}, output, err
}

func parsePlannerTask(text string) (LoopTask, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return LoopTask{}, errors.New("planner output is empty")
	}

	var task LoopTask
	for _, candidate := range plannerJSONCandidates(text) {
		if err := json.Unmarshal([]byte(candidate), &task); err == nil {
			task.Title = sanitizeTaskTitle(task.Title)
			task.Prompt = strings.TrimSpace(task.Prompt)
			if task.Title == "" && task.Prompt != "" {
				task.Title = sanitizeTaskTitle(firstLine(task.Prompt))
			}
			if task.Prompt == "" && task.Title != "" {
				task.Prompt = task.Title
			}
			if task.Title != "" && task.Prompt != "" {
				return task, nil
			}
		}
	}

	lines := nonEmptyLines(text)
	if len(lines) == 0 {
		return LoopTask{}, errors.New("planner output did not contain a task")
	}
	return LoopTask{
		Title:  sanitizeTaskTitle(firstLine(lines[0])),
		Prompt: strings.Join(lines, "\n"),
		Source: "fallback",
	}, nil
}

func plannerJSONCandidates(text string) []string {
	var out []string
	out = append(out, text)

	reFence := regexp.MustCompile("(?s)```json\\s*(\\{.*?\\})\\s*```")
	if matches := reFence.FindStringSubmatch(text); len(matches) == 2 {
		out = append(out, matches[1])
	}

	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start >= 0 && end > start {
		out = append(out, text[start:end+1])
	}

	return dedupeStrings(out)
}
