package mcpserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/eval"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- handleEvalCounterfactual ---

func TestHandleEvalCounterfactual_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleEvalCounterfactual(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo required") {
		t.Errorf("expected 'repo required' in error, got: %s", text)
	}
}

func TestHandleEvalCounterfactual_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleEvalCounterfactual(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo not found") {
		t.Errorf("expected 'repo not found' in error, got: %s", text)
	}
}

// --- handleEvalABTest ---

func TestHandleEvalABTest_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleEvalABTest(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo required") {
		t.Errorf("expected 'repo required' in error, got: %s", text)
	}
}

func TestHandleEvalABTest_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleEvalABTest(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
}

// --- handleEvalChangepoints ---

func TestHandleEvalChangepoints_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleEvalChangepoints(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo required") {
		t.Errorf("expected 'repo required' in error, got: %s", text)
	}
}

func TestHandleEvalChangepoints_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleEvalChangepoints(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
}

// --- eval handlers with valid repo but no observations ---

func TestHandleEvalCounterfactual_NoObservations(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleEvalCounterfactual(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"status":"empty"`) && !strings.Contains(text, "no observations") {
		t.Errorf("expected empty result or 'no observations' message, got: %s", text)
	}
}

func TestHandleEvalABTest_NoObservations(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleEvalABTest(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"status":"empty"`) && !strings.Contains(text, "no observations") {
		t.Errorf("expected empty result or 'no observations' message, got: %s", text)
	}
}

func TestHandleEvalChangepoints_NoObservations(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleEvalChangepoints(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"status":"empty"`) && !strings.Contains(text, "no observations") {
		t.Errorf("expected empty result or 'no observations' message, got: %s", text)
	}
}

// --- handleBanditStatus ---

func TestHandleBanditStatus_NilSessionManager(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())
	srv.SessMgr = nil

	result, err := srv.handleBanditStatus(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nil session manager")
	}
	text := getResultText(result)
	if !strings.Contains(text, "session manager not initialized") {
		t.Errorf("expected 'session manager not initialized' in error, got: %s", text)
	}
}

func TestHandleBanditStatus_NoCascadeRouter(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleBanditStatus(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return not_configured (not an error, just status).
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected 'not_configured' in result, got: %s", text)
	}
}

// --- handleConfidenceCalibration ---

func TestHandleConfidenceCalibration_NilSessionManager(t *testing.T) {
	t.Parallel()
	srv := NewServer(t.TempDir())
	srv.SessMgr = nil

	result, err := srv.handleConfidenceCalibration(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nil session manager")
	}
	text := getResultText(result)
	if !strings.Contains(text, "session manager not initialized") {
		t.Errorf("expected 'session manager not initialized' in error, got: %s", text)
	}
}

func TestHandleConfidenceCalibration_NoCascadeRouter(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleConfidenceCalibration(context.Background(), makeRequest(map[string]any{}))
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

// --- FINDING-105: A/B test insufficient data ---

func TestHandleEvalABTest_ProvidersInsufficientData(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	obsPath := session.ObservationPath(repoPath)

	// Write 3 observations for provider_a (below minimum of 5) and 6 for provider_b.
	now := time.Now()
	for i := 0; i < 3; i++ {
		_ = session.WriteObservation(obsPath, session.LoopObservation{
			Timestamp:      now.Add(-time.Duration(i) * time.Minute),
			WorkerProvider: "claude",
			VerifyPassed:   true,
			TotalCostUSD:   1.0,
		})
	}
	for i := 0; i < 6; i++ {
		_ = session.WriteObservation(obsPath, session.LoopObservation{
			Timestamp:      now.Add(-time.Duration(i) * time.Minute),
			WorkerProvider: "gemini",
			VerifyPassed:   true,
			TotalCostUSD:   0.5,
		})
	}

	result, err := srv.handleEvalABTest(context.Background(), makeRequest(map[string]any{
		"repo":       "test-repo",
		"mode":       "providers",
		"provider_a": "claude",
		"provider_b": "gemini",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "insufficient_data") {
		t.Errorf("expected 'insufficient_data' status, got: %s", text)
	}
	if !strings.Contains(text, "minimum") {
		t.Errorf("expected minimum_required in result, got: %s", text)
	}
	// Should NOT contain posteriors/prob_a_better.
	if strings.Contains(text, "prob_a_better") {
		t.Errorf("should not compute posteriors with insufficient data, got: %s", text)
	}
}

func TestHandleEvalABTest_ProvidersZeroGroupReturnsInsufficientData(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	obsPath := session.ObservationPath(repoPath)

	// Write observations only for provider_a; provider_b has 0.
	now := time.Now()
	for i := 0; i < 10; i++ {
		_ = session.WriteObservation(obsPath, session.LoopObservation{
			Timestamp:      now.Add(-time.Duration(i) * time.Minute),
			WorkerProvider: "claude",
			VerifyPassed:   true,
			TotalCostUSD:   1.0,
		})
	}

	result, err := srv.handleEvalABTest(context.Background(), makeRequest(map[string]any{
		"repo":       "test-repo",
		"mode":       "providers",
		"provider_a": "claude",
		"provider_b": "gemini",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "insufficient_data") {
		t.Errorf("expected 'insufficient_data' for zero-size group, got: %s", text)
	}
	if !strings.Contains(text, `"group_b_count":0`) {
		t.Errorf("expected group_b_count of 0, got: %s", text)
	}
}

func TestHandleEvalABTest_PeriodsInsufficientData(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	obsPath := session.ObservationPath(repoPath)

	// Write 8 observations all recent (after the split point), leaving 0 before.
	now := time.Now()
	for i := 0; i < 8; i++ {
		_ = session.WriteObservation(obsPath, session.LoopObservation{
			Timestamp:      now.Add(-time.Duration(i) * time.Minute),
			WorkerProvider: "claude",
			VerifyPassed:   true,
			TotalCostUSD:   1.0,
		})
	}

	result, err := srv.handleEvalABTest(context.Background(), makeRequest(map[string]any{
		"repo":            "test-repo",
		"mode":            "periods",
		"split_hours_ago": float64(24), // all obs are within last hour, so "before" is empty
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "insufficient_data") {
		t.Errorf("expected 'insufficient_data' for periods with empty before group, got: %s", text)
	}
}

func TestHandleEvalABTest_ProvidersSufficientData(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	obsPath := session.ObservationPath(repoPath)

	// Write 5+ observations for each provider.
	now := time.Now()
	for i := 0; i < 6; i++ {
		_ = session.WriteObservation(obsPath, session.LoopObservation{
			Timestamp:      now.Add(-time.Duration(i) * time.Minute),
			WorkerProvider: "claude",
			VerifyPassed:   true,
			TotalCostUSD:   1.0,
		})
		_ = session.WriteObservation(obsPath, session.LoopObservation{
			Timestamp:      now.Add(-time.Duration(i) * time.Minute),
			WorkerProvider: "gemini",
			VerifyPassed:   i%2 == 0,
			TotalCostUSD:   0.5,
		})
	}

	result, err := srv.handleEvalABTest(context.Background(), makeRequest(map[string]any{
		"repo":       "test-repo",
		"mode":       "providers",
		"provider_a": "claude",
		"provider_b": "gemini",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	// Should contain comparison data, not insufficient_data.
	if strings.Contains(text, "insufficient_data") {
		t.Errorf("should not return insufficient_data with 6 obs per group, got: %s", text)
	}
	if !strings.Contains(text, "comparison") {
		t.Errorf("expected comparison in result, got: %s", text)
	}
}

// --- FINDING-106: Changepoint burn-in ---

func TestFilterChangepointBurnIn(t *testing.T) {
	t.Parallel()

	cps := []eval.Changepoint{
		{Index: 0, MetricName: "cost", Significance: 0.9},
		{Index: 2, MetricName: "cost", Significance: 0.8},
		{Index: 5, MetricName: "cost", Significance: 0.7},
		{Index: 10, MetricName: "cost", Significance: 0.6},
	}

	filtered := filterChangepointBurnIn(cps, 5)
	if len(filtered) != 2 {
		t.Fatalf("expected 2 changepoints after burn-in filter, got %d", len(filtered))
	}
	if filtered[0].Index != 5 {
		t.Errorf("expected first changepoint at index 5, got %d", filtered[0].Index)
	}
	if filtered[1].Index != 10 {
		t.Errorf("expected second changepoint at index 10, got %d", filtered[1].Index)
	}
}

func TestFilterChangepointBurnIn_AllFiltered(t *testing.T) {
	t.Parallel()

	cps := []eval.Changepoint{
		{Index: 0, MetricName: "cost"},
		{Index: 3, MetricName: "cost"},
	}

	filtered := filterChangepointBurnIn(cps, 5)
	if filtered == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(filtered) != 0 {
		t.Errorf("expected 0 changepoints, got %d", len(filtered))
	}
}

func TestFilterChangepointBurnIn_Empty(t *testing.T) {
	t.Parallel()

	filtered := filterChangepointBurnIn(nil, 5)
	if filtered == nil {
		t.Fatal("expected non-nil empty slice, got nil")
	}
	if len(filtered) != 0 {
		t.Errorf("expected 0 changepoints, got %d", len(filtered))
	}
}
