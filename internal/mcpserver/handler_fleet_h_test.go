package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/blackboard"
)

// --- handleBlackboardQuery ---

func TestHandleBlackboardQuery_NilBlackboard(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.Blackboard = nil

	result, err := srv.handleBlackboardQuery(context.Background(), makeRequest(map[string]any{
		"namespace": "test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected 'not_configured' in result, got: %s", text)
	}
}

func TestHandleBlackboardQuery_MissingNamespace(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.Blackboard = nil

	// When Blackboard is nil, it returns not_configured before checking params.
	// To test missing namespace validation, we need a non-nil Blackboard.
	// Since nil returns early, verify that behavior.
	result, err := srv.handleBlackboardQuery(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected 'not_configured' for nil blackboard, got: %s", text)
	}
}

// --- handleBlackboardPut ---

func TestHandleBlackboardPut_NilBlackboard(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.Blackboard = nil

	result, err := srv.handleBlackboardPut(context.Background(), makeRequest(map[string]any{
		"namespace": "test",
		"key":       "k1",
		"value":     `{"foo":"bar"}`,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected 'not_configured' in result, got: %s", text)
	}
}

func TestHandleBlackboardPut_MissingNamespace(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.Blackboard = nil

	// nil Blackboard returns not_configured before param validation.
	result, err := srv.handleBlackboardPut(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected 'not_configured' for nil blackboard, got: %s", text)
	}
}

// --- handleA2AOffers ---

func TestHandleA2AOffers_NilA2A(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.A2A = nil

	result, err := srv.handleA2AOffers(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected 'not_configured' in result, got: %s", text)
	}
	if !strings.Contains(text, "a2a adapter not initialized") {
		t.Errorf("expected 'a2a adapter not initialized' in result, got: %s", text)
	}
}

// --- handleCostForecast ---

func TestHandleCostForecast_NilCostPredictor(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.CostPredictor = nil

	result, err := srv.handleCostForecast(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected 'not_configured' in result, got: %s", text)
	}
	if !strings.Contains(text, "cost predictor not initialized") {
		t.Errorf("expected 'cost predictor not initialized' in result, got: %s", text)
	}
}

// --- handleBlackboardQuery with real blackboard ---

func TestHandleBlackboardQuery_WithBlackboard_MissingNamespace(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.Blackboard = blackboard.NewBlackboard(t.TempDir())

	result, err := srv.handleBlackboardQuery(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleBlackboardQuery", result, "INVALID_PARAMS")
}

func TestHandleBlackboardQuery_WithBlackboard_ValidNamespace(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.Blackboard = blackboard.NewBlackboard(t.TempDir())

	result, err := srv.handleBlackboardQuery(context.Background(), makeRequest(map[string]any{
		"namespace": "test-ns",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "test-ns") {
		t.Errorf("expected namespace in result, got: %s", text)
	}
	if !strings.Contains(text, `"count":0`) {
		t.Errorf("expected count 0 for empty namespace, got: %s", text)
	}
}

// --- handleBlackboardPut with real blackboard ---

func TestHandleBlackboardPut_WithBlackboard_MissingNamespace(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.Blackboard = blackboard.NewBlackboard(t.TempDir())

	result, err := srv.handleBlackboardPut(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleBlackboardPut", result, "INVALID_PARAMS")
}

func TestHandleBlackboardPut_WithBlackboard_MissingKey(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.Blackboard = blackboard.NewBlackboard(t.TempDir())

	result, err := srv.handleBlackboardPut(context.Background(), makeRequest(map[string]any{
		"namespace": "test-ns",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleBlackboardPut", result, "INVALID_PARAMS")
}

func TestHandleBlackboardPut_WithBlackboard_MissingValue(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.Blackboard = blackboard.NewBlackboard(t.TempDir())

	result, err := srv.handleBlackboardPut(context.Background(), makeRequest(map[string]any{
		"namespace": "test-ns",
		"key":       "k1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleBlackboardPut", result, "INVALID_PARAMS")
}

func TestHandleBlackboardPut_WithBlackboard_InvalidJSON(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.Blackboard = blackboard.NewBlackboard(t.TempDir())

	result, err := srv.handleBlackboardPut(context.Background(), makeRequest(map[string]any{
		"namespace": "test-ns",
		"key":       "k1",
		"value":     "not valid json{{{",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleBlackboardPut", result, "INVALID_PARAMS")
}

func TestHandleBlackboardPut_WithBlackboard_ValidPut(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.Blackboard = blackboard.NewBlackboard(t.TempDir())

	result, err := srv.handleBlackboardPut(context.Background(), makeRequest(map[string]any{
		"namespace": "test-ns",
		"key":       "k1",
		"value":     `{"foo":"bar"}`,
		"writer_id": "test-writer",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "test-ns") {
		t.Errorf("expected namespace in result, got: %s", text)
	}
}

func TestHandleBlackboardPut_WithBlackboard_WithTTL(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.Blackboard = blackboard.NewBlackboard(t.TempDir())

	result, err := srv.handleBlackboardPut(context.Background(), makeRequest(map[string]any{
		"namespace":   "test-ns",
		"key":         "k2",
		"value":       `{"temp":true}`,
		"ttl_seconds": float64(60),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
}

func TestHandleCostForecast_NilCostPredictor_WithBudget(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.CostPredictor = nil

	result, err := srv.handleCostForecast(context.Background(), makeRequest(map[string]any{
		"budget_remaining": float64(100),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected 'not_configured' even with budget param, got: %s", text)
	}
}
