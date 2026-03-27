package roadmap

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// TaskSpec is the rdcycle JSON format for ralph loop consumption.
type TaskSpec struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Completion  string     `json:"completion"`
	Tasks       []SpecTask `json:"tasks"`
}

// SpecTask is an individual task in a spec.
type SpecTask struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Done        bool     `json:"done"`
	DependsOn   []string `json:"depends_on,omitempty"`
}

// Export converts a roadmap into a consumable format for ralph loops.
func Export(rm *Roadmap, format, phase, section string, maxTasks int, respectDeps bool) (string, error) {
	if maxTasks <= 0 {
		maxTasks = 20
	}

	tasks := collectTasks(rm, phase, section, maxTasks, respectDeps)

	switch format {
	case "rdcycle", "":
		return exportRDCycle(rm, tasks)
	case "fix_plan":
		return exportFixPlan(tasks), nil
	case "progress":
		return exportProgress(rm, tasks)
	case "launch_ready":
		return exportLaunchReady(rm, tasks)
	default:
		return "", fmt.Errorf("unknown format: %s (supported: rdcycle, fix_plan, progress, launch_ready)", format)
	}
}

func collectTasks(rm *Roadmap, phaseFilter, sectionFilter string, maxTasks int, respectDeps bool) []taskWithContext {
	var tasks []taskWithContext

	completedIDs := make(map[string]struct{})
	if respectDeps {
		for _, p := range rm.Phases {
			for _, s := range p.Sections {
				for _, t := range s.Tasks {
					if t.Done && t.ID != "" {
						completedIDs[t.ID] = struct{}{}
					}
				}
			}
		}
	}

	for _, p := range rm.Phases {
		if phaseFilter != "" && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(phaseFilter)) {
			continue
		}
		for _, s := range p.Sections {
			if sectionFilter != "" && !strings.Contains(strings.ToLower(s.Name), strings.ToLower(sectionFilter)) {
				continue
			}
			for _, t := range s.Tasks {
				if respectDeps && !depsReady(t, completedIDs) {
					continue
				}
				tasks = append(tasks, taskWithContext{
					Task:    t,
					Phase:   p.Name,
					Section: s.Name,
				})
				if len(tasks) >= maxTasks {
					return tasks
				}
			}
		}
	}

	return tasks
}

type taskWithContext struct {
	Task
	Phase   string
	Section string
}

func depsReady(t Task, completed map[string]struct{}) bool {
	for _, dep := range t.DependsOn {
		if _, ok := completed[dep]; !ok {
			return false
		}
	}
	return true
}

func exportRDCycle(rm *Roadmap, tasks []taskWithContext) (string, error) {
	spec := TaskSpec{
		Name:        rm.Title,
		Description: fmt.Sprintf("Auto-exported from %s (%d tasks)", rm.Title, len(tasks)),
		Completion:  fmt.Sprintf("%d/%d", rm.Stats.Completed, rm.Stats.Total),
	}

	for _, tc := range tasks {
		id := tc.Task.ID
		if id == "" {
			id = fmt.Sprintf("%s/%s", tc.Phase, tc.Section)
		}
		spec.Tasks = append(spec.Tasks, SpecTask{
			ID:          id,
			Description: tc.Task.Description,
			Done:        tc.Task.Done,
			DependsOn:   tc.Task.DependsOn,
		})
	}

	data, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal rdcycle spec: %w", err)
	}
	return string(data), nil
}

func exportFixPlan(tasks []taskWithContext) string {
	var b strings.Builder
	b.WriteString("# Fix Plan\n\n")

	currentPhase := ""
	for _, tc := range tasks {
		if tc.Phase != currentPhase {
			currentPhase = tc.Phase
			b.WriteString(fmt.Sprintf("## %s\n\n", currentPhase))
		}
		check := " "
		if tc.Task.Done {
			check = "x"
		}
		desc := tc.Task.Description
		if tc.Task.ID != "" {
			desc = tc.Task.ID + " — " + desc
		}
		b.WriteString(fmt.Sprintf("- [%s] %s\n", check, desc))
	}

	return b.String()
}

func exportProgress(rm *Roadmap, tasks []taskWithContext) (string, error) {
	type progressEntry struct {
		Iteration    int      `json:"iteration"`
		Status       string   `json:"status"`
		CompletedIDs []string `json:"completed_ids"`
		TotalTasks   int      `json:"total_tasks"`
		Source       string   `json:"source"`
	}

	var completedIDs []string
	for _, tc := range tasks {
		if tc.Task.Done && tc.Task.ID != "" {
			completedIDs = append(completedIDs, tc.Task.ID)
		}
	}

	prog := progressEntry{
		Iteration:    0,
		Status:       "initialized",
		CompletedIDs: completedIDs,
		TotalTasks:   len(tasks),
		Source:       rm.Title,
	}

	data, err := json.MarshalIndent(prog, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal progress: %w", err)
	}
	return string(data), nil
}

// LaunchTask is a task enriched with metadata for session_launch consumption.
type LaunchTask struct {
	ID                string  `json:"id"`
	Prompt            string  `json:"prompt"`
	Provider          string  `json:"provider"`
	BudgetUSD         float64 `json:"budget_usd"`
	Repo              string  `json:"repo"`
	DifficultyScore   float64 `json:"difficulty_score"`
	SuggestedProvider string  `json:"suggested_provider"`
	EstimatedBudget   float64 `json:"estimated_budget_usd"`
	Phase             string  `json:"phase"`
	Section           string  `json:"section"`
	DependsOn         []string `json:"depends_on,omitempty"`
}

// highDifficultyKeywords indicate complex tasks (score contribution: +0.25).
var highDifficultyKeywords = []string{
	"refactor", "architecture", "migration", "redesign", "rewrite", "overhaul",
}

// medDifficultyKeywords indicate medium tasks (score contribution: +0.15).
var medDifficultyKeywords = []string{
	"implement", "integrate", "test", "fix", "update", "add", "build",
}

// lowDifficultyKeywords indicate simple tasks (score contribution: +0.05).
var lowDifficultyKeywords = []string{
	"docs", "config", "lint", "format", "rename", "typo", "comment",
}

// ComputeDifficulty scores a task from 0.0 to 1.0 based on description length,
// dependency count, section depth (presence of section context), and keywords.
func ComputeDifficulty(description string, depCount int, hasSection bool) float64 {
	score := 0.0

	// Description length: longer descriptions tend to be more complex.
	// 0-50 chars: 0.05, 50-120: 0.10, 120-250: 0.15, 250+: 0.25
	descLen := len(description)
	switch {
	case descLen > 250:
		score += 0.25
	case descLen > 120:
		score += 0.15
	case descLen > 50:
		score += 0.10
	default:
		score += 0.05
	}

	// Dependency count: more deps = harder coordination.
	// 0: 0, 1: 0.10, 2: 0.20, 3+: 0.25
	switch {
	case depCount >= 3:
		score += 0.25
	case depCount >= 2:
		score += 0.20
	case depCount >= 1:
		score += 0.10
	}

	// Section depth: tasks in a named section (###) are more specific = slightly easier.
	// Tasks at phase level (no section) are broader = harder.
	if !hasSection {
		score += 0.10
	}

	// Keyword matching: scan description for difficulty indicators.
	lower := strings.ToLower(description)
	keywordScore := 0.0
	for _, kw := range highDifficultyKeywords {
		if strings.Contains(lower, kw) {
			keywordScore = math.Max(keywordScore, 0.25)
			break
		}
	}
	if keywordScore == 0 {
		for _, kw := range medDifficultyKeywords {
			if strings.Contains(lower, kw) {
				keywordScore = math.Max(keywordScore, 0.15)
				break
			}
		}
	}
	if keywordScore == 0 {
		for _, kw := range lowDifficultyKeywords {
			if strings.Contains(lower, kw) {
				keywordScore = math.Max(keywordScore, 0.05)
				break
			}
		}
	}
	score += keywordScore

	// Clamp to [0.0, 1.0]
	if score > 1.0 {
		score = 1.0
	}
	return math.Round(score*100) / 100
}

// SuggestedProvider maps a difficulty score to the recommended provider string.
func SuggestedProvider(difficulty float64) string {
	switch {
	case difficulty < 0.3:
		return "gemini/flash"
	case difficulty <= 0.7:
		return "claude/sonnet"
	default:
		return "claude/opus"
	}
}

// EstimatedBudget maps a difficulty score to a budget in USD.
func EstimatedBudget(difficulty float64) float64 {
	switch {
	case difficulty < 0.3:
		// Simple: $0.25 base + scale to $0.50
		return math.Round((0.25+difficulty*0.83)*100) / 100
	case difficulty <= 0.7:
		// Medium: $0.50 base + scale to $2.00
		return math.Round((0.50+(difficulty-0.3)*3.75)*100) / 100
	default:
		// Complex: $2.00 base + scale to $5.00
		return math.Round((2.00+(difficulty-0.7)*10.0)*100) / 100
	}
}

func exportLaunchReady(rm *Roadmap, tasks []taskWithContext) (string, error) {
	launchTasks := make([]LaunchTask, 0, len(tasks))

	for _, tc := range tasks {
		id := tc.Task.ID
		if id == "" {
			id = fmt.Sprintf("%s/%s", tc.Phase, tc.Section)
		}

		hasSection := tc.Section != "" && tc.Section != tc.Phase
		difficulty := ComputeDifficulty(tc.Task.Description, len(tc.Task.DependsOn), hasSection)
		provider := SuggestedProvider(difficulty)
		budget := EstimatedBudget(difficulty)

		launchTasks = append(launchTasks, LaunchTask{
			ID:                id,
			Prompt:            tc.Task.Description,
			Provider:          provider,
			BudgetUSD:         budget,
			Repo:              rm.Title,
			DifficultyScore:   difficulty,
			SuggestedProvider: provider,
			EstimatedBudget:   budget,
			Phase:             tc.Phase,
			Section:           tc.Section,
			DependsOn:         tc.Task.DependsOn,
		})
	}

	data, err := json.MarshalIndent(launchTasks, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal launch_ready: %w", err)
	}
	return string(data), nil
}
