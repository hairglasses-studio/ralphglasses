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
	var emptyRes map[string]any
	if err := json.Unmarshal([]byte(text), &emptyRes); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	if emptyRes["status"] != "empty" {
		t.Errorf("expected status=empty, got %v", emptyRes["status"])
	}
	if emptyRes["item_type"] != "observations" {
		t.Errorf("expected item_type=observations, got %v", emptyRes["item_type"])
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

	// Verify percentile fields are populated.
	if summary.LatencyP50 == 0 {
		t.Error("expected non-zero LatencyP50")
	}
	if summary.LatencyP95 == 0 {
		t.Error("expected non-zero LatencyP95")
	}
	if summary.LatencyP99 == 0 {
		t.Error("expected non-zero LatencyP99")
	}
}

func TestHandleObservationSummaryPercentiles(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	repoPath := root + "/test-repo"
	obsPath := session.ObservationPath(repoPath)
	// Write 10 observations with varying latencies and costs.
	for i := 0; i < 10; i++ {
		obs := session.LoopObservation{
			Timestamp:      time.Now().Add(-time.Duration(i) * time.Minute),
			LoopID:         "loop-p",
			RepoName:       "test-repo",
			Status:         "idle",
			TotalLatencyMs: int64((i + 1) * 1000), // 1s, 2s, ..., 10s
			TotalCostUSD:   float64(i+1) * 0.01,   // $0.01 .. $0.10
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

	if summary.TotalIterations != 10 {
		t.Errorf("TotalIterations = %d, want 10", summary.TotalIterations)
	}

	// With values 1..10 sorted, p50 should be ~5.5s.
	if summary.LatencyP50 < 5.0 || summary.LatencyP50 > 6.0 {
		t.Errorf("LatencyP50 = %f, want ~5.5", summary.LatencyP50)
	}
	// p95 should be close to 9.55.
	if summary.LatencyP95 < 9.0 || summary.LatencyP95 > 10.0 {
		t.Errorf("LatencyP95 = %f, want ~9.55", summary.LatencyP95)
	}
	// p99 should be close to 9.91.
	if summary.LatencyP99 < 9.5 || summary.LatencyP99 > 10.0 {
		t.Errorf("LatencyP99 = %f, want ~9.91", summary.LatencyP99)
	}

	// Cost percentiles.
	if summary.CostP50 < 0.05 || summary.CostP50 > 0.06 {
		t.Errorf("CostP50 = %f, want ~0.055", summary.CostP50)
	}
}

func TestHandleObservationQueryTimeRange(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	repoPath := root + "/test-repo"
	obsPath := session.ObservationPath(repoPath)

	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		obs := session.LoopObservation{
			Timestamp:      baseTime.Add(time.Duration(i) * time.Hour),
			LoopID:         "loop-tr",
			RepoName:       "test-repo",
			Status:         "idle",
			TotalLatencyMs: 1000,
		}
		if err := session.WriteObservation(obsPath, obs); err != nil {
			t.Fatalf("write observation: %v", err)
		}
	}

	// Query with since + until to get a window of 3 observations (hours 1..3).
	sinceStr := baseTime.Add(1 * time.Hour).Format(time.RFC3339)
	untilStr := baseTime.Add(3 * time.Hour).Format(time.RFC3339)

	result, err := srv.handleObservationQuery(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"since": sinceStr,
		"until": untilStr,
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
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(obs) != 3 {
		t.Errorf("expected 3 observations in time range, got %d", len(obs))
	}
}

func TestHandleObservationSummaryTimeRange(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	repoPath := root + "/test-repo"
	obsPath := session.ObservationPath(repoPath)

	baseTime := time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		obs := session.LoopObservation{
			Timestamp:      baseTime.Add(time.Duration(i) * time.Hour),
			LoopID:         "loop-tr2",
			RepoName:       "test-repo",
			Status:         "idle",
			TotalLatencyMs: 1000,
			TotalCostUSD:   0.05,
		}
		if err := session.WriteObservation(obsPath, obs); err != nil {
			t.Fatalf("write observation: %v", err)
		}
	}

	sinceStr := baseTime.Add(1 * time.Hour).Format(time.RFC3339)
	untilStr := baseTime.Add(2 * time.Hour).Format(time.RFC3339)

	result, err := srv.handleObservationSummary(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"since": sinceStr,
		"until": untilStr,
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
	if summary.TotalIterations != 2 {
		t.Errorf("expected 2 iterations in time range, got %d", summary.TotalIterations)
	}
}

func TestHandleObservationSummaryBackfillProviderCounts(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	repoPath := root + "/test-repo"
	obsPath := session.ObservationPath(repoPath)
	// Write observations with provider fields but no model/acceptance fields
	// to exercise the backfill logic in the handler.
	for i := 0; i < 4; i++ {
		obs := session.LoopObservation{
			Timestamp:       time.Now().Add(-time.Duration(i) * time.Minute),
			LoopID:          "loop-backfill",
			RepoName:        "test-repo",
			IterationNumber: i + 1,
			Status:          "idle",
			VerifyPassed:    i < 3, // first 3 pass, last one doesn't
			TotalLatencyMs:  1000,
			TotalCostUSD:    0.05,
			PlannerProvider: "claude",
			WorkerProvider:  "gemini",
		}
		if i == 3 {
			obs.Status = "failed"
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

	// Verify acceptance_counts is populated via backfill.
	if len(summary.AcceptanceCounts) == 0 {
		t.Error("expected acceptance_counts to be populated via backfill")
	}
	if summary.AcceptanceCounts["auto_merge"] != 3 {
		t.Errorf("expected 3 auto_merge, got %d", summary.AcceptanceCounts["auto_merge"])
	}
	if summary.AcceptanceCounts["rejected"] != 1 {
		t.Errorf("expected 1 rejected, got %d", summary.AcceptanceCounts["rejected"])
	}

	// Verify model_usage is populated via backfill (using provider names).
	if len(summary.ModelUsage) == 0 {
		t.Error("expected model_usage to be populated via backfill")
	}
	if summary.ModelUsage["claude"] != 4 {
		t.Errorf("expected claude count 4, got %d", summary.ModelUsage["claude"])
	}
	if summary.ModelUsage["gemini"] != 4 {
		t.Errorf("expected gemini count 4, got %d", summary.ModelUsage["gemini"])
	}
}

func TestHandleObservationQueryInvalidSince(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleObservationQuery(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"since": "not-a-timestamp",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid since")
	}
	text := getResultText(result)
	if !strings.Contains(text, "invalid since timestamp") {
		t.Errorf("expected 'invalid since timestamp' in error, got: %s", text)
	}
}
