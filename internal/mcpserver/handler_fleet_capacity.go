package mcpserver

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleFleetCapacityPlan(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	queueDepth := int(getNumberArg(req, "queue_depth", 0))
	if queueDepth <= 0 {
		return codedError(ErrInvalidParams, "queue_depth must be > 0"), nil
	}
	availableBudget := getNumberArg(req, "available_budget", 0)
	if availableBudget <= 0 {
		return codedError(ErrInvalidParams, "available_budget must be > 0"), nil
	}
	targetHours := getNumberArg(req, "target_completion_hours", 4.0)
	avgTaskCost := getNumberArg(req, "avg_task_cost", 0)
	avgTaskDurationMin := getNumberArg(req, "avg_task_duration_min", 10.0)

	// If avg_task_cost not provided, try to read from cost observations.
	if avgTaskCost <= 0 {
		avgTaskCost = s.estimateAvgTaskCost()
	}

	tasksPerWorkerPerHour := 60.0 / avgTaskDurationMin
	estimatedCost := float64(queueDepth) * avgTaskCost

	// Minimum workers needed to complete in target time.
	minWorkers := int(math.Ceil(float64(queueDepth) / (tasksPerWorkerPerHour * targetHours)))

	// Budget-limited workers: how many can we afford?
	costPerWorkerHour := tasksPerWorkerPerHour * avgTaskCost
	budgetLimitedWorkers := int(math.Floor(availableBudget / (costPerWorkerHour * targetHours)))
	if budgetLimitedWorkers < 1 {
		budgetLimitedWorkers = 1
	}

	recommendedWorkers := minWorkers
	if budgetLimitedWorkers < minWorkers {
		recommendedWorkers = budgetLimitedWorkers
	}

	// Estimate completion time with recommended workers.
	estimatedCompletionHours := float64(queueDepth) / (float64(recommendedWorkers) * tasksPerWorkerPerHour)

	budgetHeadroom := availableBudget - estimatedCost

	result := map[string]any{
		"recommended_workers":         recommendedWorkers,
		"min_workers":                 minWorkers,
		"budget_limited_workers":      budgetLimitedWorkers,
		"estimated_cost":              math.Round(estimatedCost*100) / 100,
		"estimated_completion_hours":  math.Round(estimatedCompletionHours*100) / 100,
		"budget_headroom":             math.Round(budgetHeadroom*100) / 100,
		"inputs": map[string]any{
			"queue_depth":           queueDepth,
			"available_budget":      availableBudget,
			"target_hours":          targetHours,
			"avg_task_cost":         avgTaskCost,
			"avg_task_duration_min": avgTaskDurationMin,
		},
	}
	return jsonResult(result), nil
}

// estimateAvgTaskCost reads cost observations and computes the mean.
func (s *Server) estimateAvgTaskCost() float64 {
	defaultCost := 0.10

	s.mu.RLock()
	repos := s.Repos
	s.mu.RUnlock()

	for _, r := range repos {
		obsPath := filepath.Join(r.Path, ".ralph", "cost_observations.json")
		data, err := os.ReadFile(obsPath)
		if err != nil {
			continue
		}
		var observations []struct {
			Cost float64 `json:"cost"`
		}
		if err := json.Unmarshal(data, &observations); err != nil {
			continue
		}
		if len(observations) == 0 {
			continue
		}
		var total float64
		for _, o := range observations {
			total += o.Cost
		}
		return total / float64(len(observations))
	}
	return defaultCost
}
