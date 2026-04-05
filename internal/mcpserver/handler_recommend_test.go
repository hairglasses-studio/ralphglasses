package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
)

func TestHandleCostRecommend_NilPredictor(t *testing.T) {
	t.Parallel()
	srv := &Server{}
	result, err := srv.handleCostRecommend(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected 'not_configured' status, got: %s", text)
	}
}

func TestHandleCostRecommend_InsufficientData(t *testing.T) {
	t.Parallel()
	srv := &Server{
		CostPredictor: fleet.NewCostPredictor(2.5),
	}
	// Add 1 sample — need at least 2.
	srv.CostPredictor.Record(fleet.CostSample{
		Timestamp: time.Now(),
		CostUSD:   1.0,
		Provider:  "claude",
		TaskType:  "test",
	})

	result, err := srv.handleCostRecommend(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "insufficient_data") {
		t.Errorf("expected 'insufficient_data' status, got: %s", text)
	}
}

func TestHandleCostRecommend_WithData(t *testing.T) {
	t.Parallel()
	srv := &Server{
		CostPredictor: fleet.NewCostPredictor(2.5),
	}
	// Add enough samples.
	for i := 0; i < 5; i++ {
		srv.CostPredictor.Record(fleet.CostSample{
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
			CostUSD:   float64(i)*0.5 + 0.5,
			Provider:  "claude",
			TaskType:  "test",
		})
	}

	result, err := srv.handleCostRecommend(context.Background(), makeRequest(map[string]any{
		"budget_remaining": float64(10),
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
	if _, ok := data["recommendations"]; !ok {
		t.Error("expected 'recommendations' in result")
	}
	if _, ok := data["count"]; !ok {
		t.Error("expected 'count' in result")
	}
}

func TestHandleCostRecommend_TypeFilter(t *testing.T) {
	t.Parallel()
	srv := &Server{
		CostPredictor: fleet.NewCostPredictor(2.5),
	}
	for i := 0; i < 5; i++ {
		srv.CostPredictor.Record(fleet.CostSample{
			Timestamp: time.Now().Add(-time.Duration(i) * time.Minute),
			CostUSD:   float64(i)*0.5 + 0.5,
			Provider:  "claude",
			TaskType:  "test",
		})
	}

	result, err := srv.handleCostRecommend(context.Background(), makeRequest(map[string]any{
		"type": "nonexistent_type",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	var data map[string]any
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("failed to parse result JSON: %v", err)
	}
	count, _ := data["count"].(float64)
	if count != 0 {
		t.Errorf("expected 0 recommendations for nonexistent type, got %v", count)
	}
}
