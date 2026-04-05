package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// writeTestObservation writes a minimal observation file so handlers can load data.
func writeTestObservation(t *testing.T, repoDir string) {
	t.Helper()
	obsDir := filepath.Join(repoDir, ".ralph", "logs")
	if err := os.MkdirAll(obsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	obs := session.LoopObservation{
		Timestamp:      time.Now(),
		WorkerProvider: "claude",
		VerifyPassed:   true,
		TotalCostUSD:   0.50,
	}
	data, _ := json.Marshal(obs)
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(obsDir, "loop_observations.jsonl"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHandleEvalSignificance_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleEvalSignificance(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo required") {
		t.Errorf("expected 'repo required', got: %s", text)
	}
}

func TestHandleEvalSignificance_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleEvalSignificance(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
}

func TestHandleEvalSignificance_NoObservations(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleEvalSignificance(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
}

func TestHandleEvalSignificance_InvalidMode(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	writeTestObservation(t, filepath.Join(root, "test-repo"))

	result, err := srv.handleEvalSignificance(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"mode": "invalid_mode",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid mode")
	}
	text := getResultText(result)
	if !strings.Contains(text, "unknown mode") {
		t.Errorf("expected 'unknown mode' in error, got: %s", text)
	}
}

func TestHandleEvalSignificance_ProvidersMode_MissingProviders(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	writeTestObservation(t, filepath.Join(root, "test-repo"))

	result, err := srv.handleEvalSignificance(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"mode": "providers",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing provider_a/provider_b")
	}
	text := getResultText(result)
	if !strings.Contains(text, "provider_a and provider_b") {
		t.Errorf("expected 'provider_a and provider_b' in error, got: %s", text)
	}
}

func TestHandleEvalSignificance_PeriodsMode_MissingSplit(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	writeTestObservation(t, filepath.Join(root, "test-repo"))

	result, err := srv.handleEvalSignificance(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"mode": "periods",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing split_hours_ago")
	}
	text := getResultText(result)
	if !strings.Contains(text, "split_hours_ago") {
		t.Errorf("expected 'split_hours_ago' in error, got: %s", text)
	}
}

func TestHandleEvalSignificance_CostMode_MissingProviders(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	writeTestObservation(t, filepath.Join(root, "test-repo"))

	result, err := srv.handleEvalSignificance(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"mode": "cost",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing provider_a/provider_b")
	}
}

func TestHandleEvalDefine_MissingYAML(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleEvalDefine(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing yaml_content")
	}
	text := getResultText(result)
	if !strings.Contains(text, "yaml_content required") {
		t.Errorf("expected 'yaml_content required', got: %s", text)
	}
}

func TestHandleEvalDefine_InvalidYAML(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	result, err := srv.handleEvalDefine(context.Background(), makeRequest(map[string]any{
		"yaml_content": "not: valid: yaml: content: [[[",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid YAML")
	}
}

func TestHandleEvalABTest_InvalidMode(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	writeTestObservation(t, filepath.Join(root, "test-repo"))

	result, err := srv.handleEvalABTest(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"mode": "invalid_mode",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid mode")
	}
	text := getResultText(result)
	if !strings.Contains(text, "unknown mode") {
		t.Errorf("expected 'unknown mode' in error, got: %s", text)
	}
}
