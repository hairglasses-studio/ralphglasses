package bandit

import "sort"

// KnapsackTask represents a task to be assigned to a provider arm.
type KnapsackTask struct {
	ID         string  `json:"id"`
	Complexity float64 `json:"complexity"` // -1.0=simple, 1.0=complex (matches FeatureComplexity)
}

// TaskAssignment records a single task-to-arm assignment from the knapsack solver.
type TaskAssignment struct {
	TaskID  string  `json:"task_id"`
	ArmID   string  `json:"arm_id"`   // provider/model
	Cost    float64 `json:"cost"`
	Quality float64 `json:"quality"`
}

// taskArmCandidate holds a scored (task, arm) pair for greedy ranking.
type taskArmCandidate struct {
	taskIdx int
	armIdx  int
	cost    float64
	quality float64
	ratio   float64 // quality / cost (efficiency)
}

// SolveBudgetKnapsack assigns tasks to provider arms to maximize total quality
// within a budget constraint. Uses a greedy approximation inspired by
// PILOT (ArXiv 2508.21141): rank all (task, arm) pairs by quality/cost
// efficiency, then greedily assign the best feasible pair until the budget is
// exhausted or all tasks are assigned.
//
// predictor returns (cost, quality) estimates for a given (task, arm) pair.
// Each task is assigned to at most one arm. Pairs where cost <= 0 are skipped.
// If budget <= 0, no assignments are made.
func SolveBudgetKnapsack(
	tasks []KnapsackTask,
	arms []Arm,
	budget float64,
	predictor func(task KnapsackTask, arm Arm) (cost, quality float64),
) []TaskAssignment {
	if len(tasks) == 0 || len(arms) == 0 || budget <= 0 || predictor == nil {
		return nil
	}

	// Score every (task, arm) pair.
	candidates := make([]taskArmCandidate, 0, len(tasks)*len(arms))
	for ti, task := range tasks {
		for ai, arm := range arms {
			cost, quality := predictor(task, arm)
			if cost <= 0 || quality <= 0 {
				continue
			}
			candidates = append(candidates, taskArmCandidate{
				taskIdx: ti,
				armIdx:  ai,
				cost:    cost,
				quality: quality,
				ratio:   quality / cost,
			})
		}
	}

	// Sort by efficiency (quality/cost) descending.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].ratio > candidates[j].ratio
	})

	// Greedy assignment: pick the best feasible candidate for each unassigned task.
	assigned := make(map[int]bool, len(tasks))
	remaining := budget
	var result []TaskAssignment

	for _, c := range candidates {
		if assigned[c.taskIdx] {
			continue
		}
		if c.cost > remaining {
			continue
		}
		assigned[c.taskIdx] = true
		remaining -= c.cost
		result = append(result, TaskAssignment{
			TaskID:  tasks[c.taskIdx].ID,
			ArmID:   arms[c.armIdx].ID,
			Cost:    c.cost,
			Quality: c.quality,
		})
		if len(assigned) == len(tasks) {
			break // all tasks assigned
		}
	}

	return result
}
