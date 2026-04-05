package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleSessionTriage_Defaults(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleSessionTriage(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "total_sessions") {
		t.Errorf("expected 'total_sessions' in result, got: %s", text)
	}
}

func TestHandleSessionTriage_InvalidSince(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleSessionTriage(context.Background(), makeRequest(map[string]any{
		"since": "not-a-time",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid since parameter")
	}
}

func TestHandleSessionSalvage_MissingID(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleSessionSalvage(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing id")
	}
	text := getResultText(result)
	if !strings.Contains(text, "id required") {
		t.Errorf("expected 'id required', got: %s", text)
	}
}

func TestHandleSessionSalvage_NotFound(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleSessionSalvage(context.Background(), makeRequest(map[string]any{
		"id": "nonexistent-session-id",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
	text := getResultText(result)
	if !strings.Contains(text, "not found") {
		t.Errorf("expected 'not found', got: %s", text)
	}
}

func TestHandleRecoveryPlan_EmptySessions(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleRecoveryPlan(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "rec-empty") {
		t.Errorf("expected empty plan, got: %s", text)
	}
}

func TestHandleRecoveryExecute_MissingPlanJSON(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleRecoveryExecute(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing plan_json")
	}
	text := getResultText(result)
	if !strings.Contains(text, "plan_json required") {
		t.Errorf("expected 'plan_json required', got: %s", text)
	}
}

func TestHandleRecoveryExecute_InvalidJSON(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	result, err := srv.handleRecoveryExecute(context.Background(), makeRequest(map[string]any{
		"plan_json": "not valid json",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid JSON")
	}
	text := getResultText(result)
	if !strings.Contains(text, "invalid plan_json") {
		t.Errorf("expected 'invalid plan_json', got: %s", text)
	}
}

func TestHandleRecoveryExecute_BudgetExceeded(t *testing.T) {
	t.Parallel()
	srv := &Server{SessMgr: session.NewManager()}
	// Create a plan with high retry costs.
	result, err := srv.handleRecoveryExecute(context.Background(), makeRequest(map[string]any{
		"plan_json":      `[{"action":"retry","budget_usd":100,"repo":"test","prompt":"do work","provider":"claude","model":"sonnet"}]`,
		"budget_cap_usd": float64(5),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for budget exceeded")
	}
	text := getResultText(result)
	if !strings.Contains(text, "exceeds budget cap") {
		t.Errorf("expected 'exceeds budget cap', got: %s", text)
	}
}

func TestHandleRecoveryExecute_NoSessMgr(t *testing.T) {
	t.Parallel()
	srv := &Server{} // No SessMgr
	result, err := srv.handleRecoveryExecute(context.Background(), makeRequest(map[string]any{
		"plan_json": `[{"action":"retry","budget_usd":1,"repo":"test","prompt":"do work"}]`,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nil session manager")
	}
	text := getResultText(result)
	if !strings.Contains(text, "session manager") {
		t.Errorf("expected 'session manager' in error, got: %s", text)
	}
}
