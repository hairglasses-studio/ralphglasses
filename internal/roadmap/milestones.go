package roadmap

import (
	"math"
	"sort"
)

// PhaseProgress holds completion metrics for a single phase.
type PhaseProgress struct {
	Name              string  `json:"name"`
	Total             int     `json:"total"`
	Completed         int     `json:"completed"`
	Remaining         int     `json:"remaining"`
	CompletionPercent float64 `json:"completion_percent"`
}

// Velocity captures historical throughput for effort estimation.
type Velocity struct {
	// TasksPerPeriod is the number of tasks completed per time period
	// (the unit is caller-defined: day, sprint, week, etc.).
	TasksPerPeriod float64
}

// EffortEstimate holds the estimated remaining effort for a phase.
type EffortEstimate struct {
	PhaseName        string  `json:"phase_name"`
	RemainingTasks   int     `json:"remaining_tasks"`
	PeriodsRemaining float64 `json:"periods_remaining"` // NaN/Inf when velocity is 0
}

// CriticalPathItem is an incomplete task that blocks the most downstream work.
type CriticalPathItem struct {
	TaskID         string   `json:"task_id"`
	Description    string   `json:"description"`
	Phase          string   `json:"phase"`
	Section        string   `json:"section"`
	BlockedCount   int      `json:"blocked_count"`   // direct + transitive dependents
	BlockedTaskIDs []string `json:"blocked_task_ids"` // IDs of tasks this blocks
}

// GroupByPhase returns per-phase progress summaries.
func GroupByPhase(rm *Roadmap) []PhaseProgress {
	out := make([]PhaseProgress, 0, len(rm.Phases))
	for _, p := range rm.Phases {
		pp := PhaseProgress{
			Name:      p.Name,
			Total:     p.Stats.Total,
			Completed: p.Stats.Completed,
			Remaining: p.Stats.Remaining,
		}
		if pp.Total > 0 {
			pp.CompletionPercent = float64(pp.Completed) / float64(pp.Total) * 100
		}
		out = append(out, pp)
	}
	return out
}

// EstimateEffort projects how many periods each phase needs at the given velocity.
// Returns one entry per phase that still has remaining work.
func EstimateEffort(rm *Roadmap, v Velocity) []EffortEstimate {
	var out []EffortEstimate
	for _, p := range rm.Phases {
		if p.Stats.Remaining == 0 {
			continue
		}
		est := EffortEstimate{
			PhaseName:      p.Name,
			RemainingTasks: p.Stats.Remaining,
		}
		if v.TasksPerPeriod > 0 {
			est.PeriodsRemaining = float64(p.Stats.Remaining) / v.TasksPerPeriod
		} else {
			est.PeriodsRemaining = math.Inf(1)
		}
		out = append(out, est)
	}
	return out
}

// CriticalPath identifies incomplete tasks that block the most downstream work,
// sorted descending by blocked count. It considers both direct and transitive
// dependents across all phases.
func CriticalPath(rm *Roadmap) []CriticalPathItem {
	type taskInfo struct {
		id      string
		desc    string
		phase   string
		section string
		done    bool
	}

	// Index every task by ID and build a reverse dependency graph
	// (parent -> children that depend on parent).
	taskByID := make(map[string]taskInfo)
	// reverseDeps: taskID -> list of task IDs that directly depend on it.
	reverseDeps := make(map[string][]string)

	for _, p := range rm.Phases {
		for _, s := range p.Sections {
			for _, t := range s.Tasks {
				if t.ID == "" {
					continue
				}
				taskByID[t.ID] = taskInfo{
					id:      t.ID,
					desc:    t.Description,
					phase:   p.Name,
					section: s.Name,
					done:    t.Done,
				}
				for _, dep := range t.DependsOn {
					reverseDeps[dep] = append(reverseDeps[dep], t.ID)
				}
			}
		}
	}

	// For each incomplete task, compute the transitive set of tasks it blocks.
	// BFS from the task through reverseDeps.
	transitiveBlocked := func(rootID string) []string {
		visited := map[string]bool{rootID: true}
		queue := []string{rootID}
		var blocked []string
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, child := range reverseDeps[cur] {
				if visited[child] {
					continue
				}
				visited[child] = true
				info, ok := taskByID[child]
				if ok && !info.done {
					blocked = append(blocked, child)
				}
				queue = append(queue, child)
			}
		}
		return blocked
	}

	var items []CriticalPathItem
	for id, info := range taskByID {
		if info.done {
			continue
		}
		blocked := transitiveBlocked(id)
		if len(blocked) == 0 {
			continue
		}
		sort.Strings(blocked)
		items = append(items, CriticalPathItem{
			TaskID:         id,
			Description:    info.desc,
			Phase:          info.phase,
			Section:        info.section,
			BlockedCount:   len(blocked),
			BlockedTaskIDs: blocked,
		})
	}

	// Sort descending by blocked count, then by task ID for determinism.
	sort.Slice(items, func(i, j int) bool {
		if items[i].BlockedCount != items[j].BlockedCount {
			return items[i].BlockedCount > items[j].BlockedCount
		}
		return items[i].TaskID < items[j].TaskID
	})

	return items
}
