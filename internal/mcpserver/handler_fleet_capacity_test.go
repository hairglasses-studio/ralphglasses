package mcpserver

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestFleetCapacityPlan(t *testing.T) {
	s := newTestServer(t.TempDir())

	tests := []struct {
		name    string
		args    map[string]any
		wantErr string
		check   func(t *testing.T, result map[string]any)
	}{
		{
			name: "basic_plan",
			args: map[string]any{
				"queue_depth":            float64(20),
				"available_budget":       float64(10.0),
				"target_completion_hours": float64(2),
				"avg_task_cost":          float64(0.10),
				"avg_task_duration_min":  float64(10),
				"utilization_factor":     float64(1.0), // 100% utilization for predictable math
			},
			check: func(t *testing.T, result map[string]any) {
				// 6 tasks/worker/hour * 1.0 util, 2 hours = 12 tasks/worker
				// 20 tasks / 12 = ceil(1.67) = 2 workers
				workers := result["recommended_workers"].(float64)
				if workers != 2 {
					t.Errorf("expected 2 workers, got %v", workers)
				}
				cost := result["estimated_cost"].(float64)
				if cost != 2.0 { // 20 * 0.10
					t.Errorf("expected cost 2.0, got %v", cost)
				}
				// Verify cost_per_worker_hour is present.
				if _, ok := result["cost_per_worker_hour"]; !ok {
					t.Error("missing cost_per_worker_hour in output")
				}
			},
		},
		{
			name: "budget_constrained",
			args: map[string]any{
				"queue_depth":          float64(100),
				"available_budget":     float64(2.0),
				"target_completion_hours": float64(1),
				"avg_task_cost":        float64(0.10),
				"avg_task_duration_min": float64(10),
			},
			check: func(t *testing.T, result map[string]any) {
				workers := result["recommended_workers"].(float64)
				budgetLimited := result["budget_limited_workers"].(float64)
				minWorkers := result["min_workers"].(float64)
				// Budget can afford: 2.0 / (6*0.10*1) = 3 workers
				// Need: 100 / 6 = 17 workers
				// Recommended should be budget-limited
				if workers != budgetLimited {
					t.Errorf("expected budget-limited workers, got recommended=%v budget=%v min=%v", workers, budgetLimited, minWorkers)
				}
			},
		},
		{
			name:    "zero_queue",
			args:    map[string]any{"queue_depth": float64(0), "available_budget": float64(10)},
			wantErr: "queue_depth must be > 0",
		},
		{
			name:    "zero_budget",
			args:    map[string]any{"queue_depth": float64(10), "available_budget": float64(0)},
			wantErr: "available_budget must be > 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = tt.args

			res, err := s.handleFleetCapacityPlan(nil, req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			text := res.Content[0].(mcp.TextContent).Text
			if tt.wantErr != "" {
				if res.IsError {
					return // error response, OK
				}
				t.Fatalf("expected error containing %q", tt.wantErr)
			}

			var result map[string]any
			if err := json.Unmarshal([]byte(text), &result); err != nil {
				t.Fatalf("unmarshal result: %v", err)
			}

			if tt.check != nil {
				tt.check(t, result)
			}
		})
	}
}
