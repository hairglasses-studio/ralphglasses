package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

func TestHandleCircuitReset_MissingService(t *testing.T) {
	t.Parallel()
	srv := &Server{ScanPath: t.TempDir()}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := srv.handleCircuitReset(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	if !json.Valid([]byte(text)) {
		t.Fatalf("invalid JSON: %s", text)
	}
	var body map[string]any
	json.Unmarshal([]byte(text), &body)
	if body["error_code"] != "INVALID_PARAMS" {
		t.Errorf("expected INVALID_PARAMS, got %v", body["error_code"])
	}
}

func TestHandleCircuitReset_Enhancer_NoEngine(t *testing.T) {
	// Clear all API keys so getEngine() returns nil (sync.Once not yet called).
	for _, key := range []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GOOGLE_API_KEY", "GEMINI_API_KEY"} {
		t.Setenv(key, "")
	}

	srv := &Server{ScanPath: t.TempDir()}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"service": "enhancer"}

	result, err := srv.handleCircuitReset(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	// Without an API key, getEngine() returns nil → provider-unavailable error.
	if body["error_code"] != "PROVIDER_UNAVAILABLE" {
		t.Errorf("expected error_code=PROVIDER_UNAVAILABLE, got %v", body["error_code"])
	}
}

func TestHandleCircuitReset_UnknownService(t *testing.T) {
	t.Parallel()
	srv := &Server{ScanPath: t.TempDir()}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"service": "nonexistent"}

	result, err := srv.handleCircuitReset(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	var body map[string]any
	json.Unmarshal([]byte(text), &body)
	if body["error_code"] != "SERVICE_NOT_FOUND" {
		t.Errorf("expected SERVICE_NOT_FOUND, got %v", body["error_code"])
	}
}

func TestHandleCircuitReset_RepoCircuitBreaker(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	repoPath := filepath.Join(root, "testrepo")
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}

	srv := &Server{
		ScanPath: root,
		Repos:    []*model.Repo{{Name: "testrepo", Path: repoPath}},
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"service": "testrepo"}

	result, err := srv.handleCircuitReset(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := result.Content[0].(mcp.TextContent).Text
	var body map[string]any
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "reset" {
		t.Errorf("expected status=reset, got %v", body["status"])
	}
	if body["service"] != "testrepo" {
		t.Errorf("expected service=testrepo, got %v", body["service"])
	}

	// Verify file was written.
	cbPath := filepath.Join(ralphDir, ".circuit_breaker_state")
	if _, err := os.Stat(cbPath); os.IsNotExist(err) {
		t.Error("circuit breaker state file not created")
	}
}
