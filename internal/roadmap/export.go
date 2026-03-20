package roadmap

import (
	"encoding/json"
	"fmt"
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
	default:
		return "", fmt.Errorf("unknown format: %s (supported: rdcycle, fix_plan, progress)", format)
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
