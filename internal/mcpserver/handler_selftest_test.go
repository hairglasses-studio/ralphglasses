package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestHandleSelfTest_ValidRepo(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo": repoPath,
	}

	result, err := srv.handleSelfTest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	// Parse JSON response
	text := result.Content[0].(mcp.TextContent).Text
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	// Check defaults
	if resp["repo"] != repoPath {
		t.Errorf("repo = %v, want %v", resp["repo"], repoPath)
	}
	if resp["iterations"].(float64) != 3 {
		t.Errorf("iterations = %v, want 3", resp["iterations"])
	}
	if resp["budget_usd"].(float64) != 5.0 {
		t.Errorf("budget_usd = %v, want 5.0", resp["budget_usd"])
	}
	if resp["use_snapshot"] != true {
		t.Errorf("use_snapshot = %v, want true", resp["use_snapshot"])
	}
	if resp["status"] != "prepared" {
		t.Errorf("status = %v, want prepared", resp["status"])
	}
}

func TestHandleSelfTest_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleSelfTest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
}

func TestHandleSelfTest_NonexistentRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo": "/nonexistent/path/that/does/not/exist",
	}

	result, err := srv.handleSelfTest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
}

func TestHandleSelfTest_RepoIsFile(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Create a file (not directory) to pass as repo
	tmpFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(tmpFile, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo": tmpFile,
	}

	result, err := srv.handleSelfTest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result when repo path is a file")
	}
}

func TestHandleSelfTest_CustomIterationsAndBudget(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo":         repoPath,
		"iterations":   float64(10),
		"budget_usd":   float64(25.0),
		"use_snapshot": false,
	}

	result, err := srv.handleSelfTest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %v", result.Content)
	}

	text := result.Content[0].(mcp.TextContent).Text
	var resp map[string]any
	if err := json.Unmarshal([]byte(text), &resp); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}

	if resp["iterations"].(float64) != 10 {
		t.Errorf("iterations = %v, want 10", resp["iterations"])
	}
	if resp["budget_usd"].(float64) != 25.0 {
		t.Errorf("budget_usd = %v, want 25.0", resp["budget_usd"])
	}
	if resp["use_snapshot"] != false {
		t.Errorf("use_snapshot = %v, want false", resp["use_snapshot"])
	}
}

func TestHandleSelfTest_InvalidIterations(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo":       repoPath,
		"iterations": float64(0),
	}

	result, err := srv.handleSelfTest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for iterations=0")
	}
}

func TestHandleSelfTest_InvalidBudget(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"repo":       repoPath,
		"budget_usd": float64(-1),
	}

	result, err := srv.handleSelfTest(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for negative budget")
	}
}
