package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleObservationQueryNoRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleObservationQuery(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo name required") {
		t.Errorf("expected 'repo name required' in error, got: %s", text)
	}
}

func TestHandleObservationQueryEmpty(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleObservationQuery(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}

	text := getResultText(result)
	var obs []session.LoopObservation
	if err := json.Unmarshal([]byte(text), &obs); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if len(obs) != 0 {
		t.Errorf("expected empty array, got %d observations", len(obs))
	}
}

func TestHandleObservationSummary(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	// Write some test observations.
	repoPath := root + "/test-repo"
	obsPath := session.ObservationPath(repoPath)
	for i := 0; i < 3; i++ {
		obs := session.LoopObservation{
			Timestamp:       time.Now().Add(-time.Duration(i) * time.Minute),
			LoopID:          "loop-1",
			RepoName:        "test-repo",
			IterationNumber: i + 1,
			Status:          "idle",
			TotalLatencyMs:  1000,
			TotalCostUSD:    0.05,
			FilesChanged:    2,
			LinesAdded:      10,
			LinesRemoved:    5,
		}
		if err := session.WriteObservation(obsPath, obs); err != nil {
			t.Fatalf("write observation: %v", err)
		}
	}

	result, err := srv.handleObservationSummary(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}

	text := getResultText(result)
	var summary session.IterationSummary
	if err := json.Unmarshal([]byte(text), &summary); err != nil {
		t.Fatalf("failed to unmarshal summary: %v", err)
	}
	if summary.TotalIterations != 3 {
		t.Errorf("expected 3 total iterations, got %d", summary.TotalIterations)
	}
	if summary.CompletedCount != 3 {
		t.Errorf("expected 3 completed, got %d", summary.CompletedCount)
	}
}
