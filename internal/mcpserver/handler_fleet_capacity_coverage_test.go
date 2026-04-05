package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestHandleFleetCapacityPlan_MissingQueueDepth(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleFleetCapacityPlan(context.Background(), makeRequest(map[string]any{
		"available_budget": float64(100),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing queue_depth")
	}
	text := getResultText(result)
	if !strings.Contains(text, "queue_depth") {
		t.Errorf("expected 'queue_depth' in error, got: %s", text)
	}
}

func TestHandleFleetCapacityPlan_MissingBudget(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleFleetCapacityPlan(context.Background(), makeRequest(map[string]any{
		"queue_depth": float64(10),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing available_budget")
	}
	text := getResultText(result)
	if !strings.Contains(text, "available_budget") {
		t.Errorf("expected 'available_budget' in error, got: %s", text)
	}
}

func TestHandleFleetCapacityPlan_ValidInputs(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleFleetCapacityPlan(context.Background(), makeRequest(map[string]any{
		"queue_depth":      float64(20),
		"available_budget": float64(50),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	text := getResultText(result)
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}
	if _, ok := data["recommended_workers"]; !ok {
		t.Error("expected 'recommended_workers' in result")
	}
	if _, ok := data["estimated_cost"]; !ok {
		t.Error("expected 'estimated_cost' in result")
	}
}

func TestHandleFleetCapacityPlan_ClampUtilization(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		utilizationFactor float64
	}{
		{name: "too_low", utilizationFactor: 0.01},
		{name: "too_high", utilizationFactor: 2.0},
		{name: "normal", utilizationFactor: 0.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := &Server{}
			result, err := srv.handleFleetCapacityPlan(context.Background(), makeRequest(map[string]any{
				"queue_depth":        float64(10),
				"available_budget":   float64(50),
				"utilization_factor": tt.utilizationFactor,
			}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsError {
				t.Fatalf("unexpected error result: %s", getResultText(result))
			}
		})
	}
}

func TestHandleFleetCapacityPlan_ZeroTaskDuration(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleFleetCapacityPlan(context.Background(), makeRequest(map[string]any{
		"queue_depth":           float64(10),
		"available_budget":      float64(50),
		"avg_task_duration_min": float64(0),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
}
