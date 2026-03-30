package session

import (
	"fmt"
	"log/slog"
	"sort"
)

// SprintPlanner wraps RoadmapToTasks with priority/size-aware sprint batching.
// It reads a ROADMAP.md, extracts unchecked items with priority and size annotations,
// and produces a CycleRun with a bounded batch of tasks sorted by priority.
type SprintPlanner struct {
	RoadmapPath       string
	MaxItemsPerSprint int      // default 5
	MaxSizePoints     int      // S=1, M=2, L=3; default 6
	TargetPriorities  []string // default ["P0", "P1", "P2"]
}

// sizePoints maps size labels to point values for budget enforcement.
var sizePoints = map[string]int{
	"S": 1,
	"M": 2,
	"L": 3,
}

// NewSprintPlanner creates a SprintPlanner with sensible defaults.
func NewSprintPlanner(roadmapPath string) *SprintPlanner {
	return &SprintPlanner{
		RoadmapPath:       roadmapPath,
		MaxItemsPerSprint: 5,
		MaxSizePoints:     6,
		TargetPriorities:  []string{"P0", "P1", "P2"},
	}
}

// PlanNextSprint reads ROADMAP.md, extracts unchecked items with priority/size,
// and returns a CycleRun with up to MaxItemsPerSprint tasks, sorted by priority
// and bounded by MaxSizePoints. Returns nil if no unchecked items remain.
func (sp *SprintPlanner) PlanNextSprint(repoPath string) *CycleRun {
	tasks, err := RoadmapToTasks(sp.RoadmapPath, 0)
	if err != nil {
		slog.Warn("sprint_planner: failed to parse roadmap", "error", err)
		return nil
	}
	if len(tasks) == 0 {
		return nil
	}

	// Filter to target priorities if set.
	if len(sp.TargetPriorities) > 0 {
		tasks = sp.filterByPriority(tasks)
	}
	if len(tasks) == 0 {
		return nil
	}

	// Sort by priority descending (highest first).
	sort.SliceStable(tasks, func(i, j int) bool {
		return tasks[i].Priority > tasks[j].Priority
	})

	// Select tasks within item count and size budget.
	selected := sp.selectBatch(tasks)
	if len(selected) == 0 {
		return nil
	}

	// Build objective from first few task titles.
	objective := sp.buildObjective(selected)

	cycle := NewCycleRun(
		fmt.Sprintf("sprint-%d", len(selected)),
		repoPath,
		objective,
		[]string{"Tests pass", "No regressions"},
	)
	cycle.Tasks = selected

	return cycle
}

// filterByPriority keeps only tasks whose priority matches one of the target
// priority levels. Priority mapping: P0→1.0, P1→0.8, P2→0.5.
func (sp *SprintPlanner) filterByPriority(tasks []CycleTask) []CycleTask {
	allowed := make(map[float64]bool, len(sp.TargetPriorities))
	for _, p := range sp.TargetPriorities {
		switch p {
		case "P0":
			allowed[1.0] = true
		case "P1":
			allowed[0.8] = true
		case "P2":
			allowed[0.5] = true
		}
	}

	var filtered []CycleTask
	for _, t := range tasks {
		if allowed[t.Priority] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// selectBatch picks tasks up to MaxItemsPerSprint and MaxSizePoints.
// Tasks should already be sorted by priority descending.
func (sp *SprintPlanner) selectBatch(tasks []CycleTask) []CycleTask {
	maxItems := sp.MaxItemsPerSprint
	if maxItems <= 0 {
		maxItems = 5
	}
	maxPoints := sp.MaxSizePoints
	if maxPoints <= 0 {
		maxPoints = 6
	}

	var selected []CycleTask
	usedPoints := 0

	for _, t := range tasks {
		if len(selected) >= maxItems {
			break
		}
		pts := taskSizePoints(t.Size)
		if usedPoints+pts > maxPoints {
			continue // skip this item, try smaller ones
		}
		selected = append(selected, t)
		usedPoints += pts
	}

	return selected
}

// taskSizePoints returns the point value for a task size.
// Unknown or empty sizes default to 2 (medium).
func taskSizePoints(size string) int {
	if pts, ok := sizePoints[size]; ok {
		return pts
	}
	return 2 // default to medium
}

// buildObjective creates a sprint objective from the selected tasks.
func (sp *SprintPlanner) buildObjective(tasks []CycleTask) string {
	if len(tasks) == 0 {
		return "Sprint: no tasks"
	}
	if len(tasks) == 1 {
		return fmt.Sprintf("Sprint: %s", tasks[0].Title)
	}
	titles := make([]string, 0, min(3, len(tasks)))
	for i := 0; i < len(tasks) && i < 3; i++ {
		titles = append(titles, tasks[i].Title)
	}
	obj := fmt.Sprintf("Sprint (%d tasks): %s", len(tasks), titles[0])
	for i := 1; i < len(titles); i++ {
		obj += ", " + titles[i]
	}
	if len(tasks) > 3 {
		obj += fmt.Sprintf(" (+%d more)", len(tasks)-3)
	}
	return obj
}
