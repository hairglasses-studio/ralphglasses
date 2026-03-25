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
	if !strings.Contains(text, "Self-improvement loop started") {
		t.Errorf("expected success message, got: %s", text)
	}
	if !strings.Contains(text, "test-repo") {
		t.Errorf("expected repo name in result, got: %s", text)
	}
}

func TestHandleSelfImprove_CustomBudget(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo":       "test-repo",
		"budget_usd": float64(40),
	}

	result, err := srv.handleSelfImprove(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	if !strings.Contains(text, "budget=$40") {
		t.Errorf("expected budget=$40 in result, got: %s", text)
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
	if !strings.Contains(text, "max_iterations=10") {
		t.Errorf("expected max_iterations=10 in result, got: %s", text)
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
