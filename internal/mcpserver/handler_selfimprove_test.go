package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleSelfImprove_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleSelfImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
}

func TestHandleSelfImprove_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo": "nonexistent-repo",
	}

	result, err := srv.handleSelfImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
}

func TestHandleSelfImprove_ValidRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo": "test-repo",
	}

	result, err := srv.handleSelfImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"message"`) {
		t.Errorf("expected message field in result, got: %s", text)
	}
	if !strings.Contains(text, `"repo":"test-repo"`) {
		t.Errorf("expected repo name in result, got: %s", text)
	}
	if !strings.Contains(text, `"trace"`) {
		t.Errorf("expected trace in default response, got: %s", text)
	}
	if !strings.Contains(text, `"productivity"`) {
		t.Errorf("expected productivity block in response, got: %s", text)
	}
}

func TestHandleSelfImprove_CustomBudget(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Budget <= $20 uses the standard profile with 1/4 + 3/4 split.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo":       "test-repo",
		"budget_usd": float64(16),
	}

	result, err := srv.handleSelfImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"applied_budget_usd":16`) {
		t.Errorf("expected applied_budget_usd:16 in result, got: %s", text)
	}
}

func TestHandleSelfImprove_OptimizedBudget(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Budget > $20 uses the cost-optimized Sonnet-only profile.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo":       "test-repo",
		"budget_usd": float64(100),
	}

	result, err := srv.handleSelfImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	// Optimized profile: planner=$1.50, worker=$3.00, total=$4.50 per iteration
	if !strings.Contains(text, `"applied_budget_usd":4.5`) {
		t.Errorf("expected applied_budget_usd:4.5 in result, got: %s", text)
	}
	// Trace should show Sonnet-level per-session budgets (not Opus-level $5/$15)
	if !strings.Contains(text, `"planner_budget_usd":1.5`) {
		t.Errorf("expected planner_budget_usd:1.5 for optimized profile, got: %s", text)
	}
}

func TestHandleSelfImprove_CustomIterations(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo":           "test-repo",
		"max_iterations": float64(10),
	}

	result, err := srv.handleSelfImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, `"max_iterations":10`) {
		t.Errorf("expected max_iterations:10 in result, got: %s", text)
	}
}

func TestHandleSelfImprove_InvalidRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo": "../../../etc/passwd",
	}

	result, err := srv.handleSelfImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid repo name")
	}
}
