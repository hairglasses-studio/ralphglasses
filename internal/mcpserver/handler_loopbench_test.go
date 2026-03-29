package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// writeObservationsJSONL writes LoopObservation records as JSONL to the canonical path.
func writeObservationsJSONL(t *testing.T, repoPath string, observations []session.LoopObservation) {
	t.Helper()
	obsPath := session.ObservationPath(repoPath)
	if err := os.MkdirAll(filepath.Dir(obsPath), 0755); err != nil {
		t.Fatal(err)
	}
	var lines []byte
	for _, obs := range observations {
		data, err := json.Marshal(obs)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, data...)
		lines = append(lines, '\n')
	}
	if err := os.WriteFile(obsPath, lines, 0644); err != nil {
		t.Fatal(err)
	}
}

// writeBaselineFile writes a LoopBaseline to the canonical path.
func writeBaselineFile(t *testing.T, repoPath string, bl *e2e.LoopBaseline) {
	t.Helper()
	blPath := e2e.BaselinePath(repoPath)
	if err := os.MkdirAll(filepath.Dir(blPath), 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(bl)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(blPath, data, 0644); err != nil {
		t.Fatal(err)
	}
}

// --- handleLoopBenchmark tests ---

func TestHandleLoopBenchmark_NoRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopBenchmark(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleLoopBenchmark_NoObservations(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleLoopBenchmark(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["message"] != "no observations in window" {
		t.Fatalf("expected 'no observations in window', got: %v", data["message"])
	}
	if data["observations"] != float64(0) {
		t.Fatalf("expected observations=0, got: %v", data["observations"])
	}
}

func TestHandleLoopBenchmark_InvalidRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleLoopBenchmark(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrRepoNotFound)) {
		t.Fatalf("expected REPO_NOT_FOUND error code, got: %s", text)
	}
}

func TestHandleLoopBenchmark_WithObservations(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repo := srv.findRepo("test-repo")
	if repo == nil {
		t.Fatal("test-repo not found after scan")
	}

	// Write JSONL observation file with valid observations.
	writeObservationsJSONL(t, repo.Path, []session.LoopObservation{
		{
			Timestamp:      time.Now(),
			LoopID:         "loop-1",
			RepoName:       "test-repo",
			TotalLatencyMs: 1500,
			TotalCostUSD:   0.05,
			Status:         "completed",
			VerifyPassed:   true,
			TaskType:       "build",
		},
	})

	result, err := srv.handleLoopBenchmark(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"hours": float64(48),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got: %s", getResultText(result))
	}

	var out map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out["observation_count"] == float64(0) {
		t.Fatal("expected observation_count > 0")
	}
}

// --- handleLoopBaseline tests ---

func TestHandleLoopBaseline_NoRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopBaseline(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleLoopBaseline_ViewAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Default action is "view", which calls LoadBaseline.
	// No baseline file exists, so it should return a filesystem error.
	result, err := srv.handleLoopBaseline(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// LoadBaseline on a non-existent file returns an error,
	// which the handler wraps with ErrFilesystem.
	if !result.IsError {
		// If the implementation returns an empty baseline for missing files,
		// that is also acceptable.
		text := getResultText(result)
		if text == "" {
			t.Fatal("expected non-empty response")
		}
	} else {
		text := getResultText(result)
		if !strings.Contains(text, string(ErrFilesystem)) {
			t.Fatalf("expected FILESYSTEM error code, got: %s", text)
		}
	}
}

func TestHandleLoopBaseline_ViewWithBaseline(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repo := srv.findRepo("test-repo")
	if repo == nil {
		t.Fatal("test-repo not found after scan")
	}

	writeBaselineFile(t, repo.Path, &e2e.LoopBaseline{
		Aggregate: &e2e.BaselineStats{
			SampleCount: 5,
			CostP50:     0.03,
			CostP95:     0.10,
			LatencyP50:  1200,
			LatencyP95:  3500,
		},
	})

	result, err := srv.handleLoopBaseline(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"action": "view",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getResultText(result))
	}

	text := getResultText(result)
	if text == "" {
		t.Fatal("expected non-empty baseline response")
	}
}

func TestHandleLoopBaseline_InvalidAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleLoopBaseline(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"action": "invalid-action",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for invalid action")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
	if !strings.Contains(text, "invalid-action") {
		t.Fatalf("expected error to mention the invalid action, got: %s", text)
	}
}

func TestHandleLoopBaseline_InvalidRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleLoopBaseline(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrRepoNotFound)) {
		t.Fatalf("expected REPO_NOT_FOUND error code, got: %s", text)
	}
}

// --- handleLoopGates tests ---

func TestHandleLoopGates_NoRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopGates(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Fatalf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleLoopGates_NoBaseline(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// No baseline and no observations — gates still evaluate with nil baseline.
	result, err := srv.handleLoopGates(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result (gates evaluate even without baseline), got: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil gate report")
	}
}

func TestHandleLoopGates_InvalidRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleLoopGates(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for nonexistent repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrRepoNotFound)) {
		t.Fatalf("expected REPO_NOT_FOUND error code, got: %s", text)
	}
}

func TestHandleLoopGates_EvaluatesGates(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repo := srv.findRepo("test-repo")
	if repo == nil {
		t.Fatal("test-repo not found after scan")
	}

	// Write JSONL observations.
	writeObservationsJSONL(t, repo.Path, []session.LoopObservation{
		{Timestamp: time.Now(), LoopID: "loop-1", RepoName: "test-repo", TotalLatencyMs: 1000, TotalCostUSD: 0.03, Status: "completed", VerifyPassed: true, TaskType: "build"},
		{Timestamp: time.Now(), LoopID: "loop-2", RepoName: "test-repo", TotalLatencyMs: 2000, TotalCostUSD: 0.05, Status: "completed", VerifyPassed: true, TaskType: "build"},
		{Timestamp: time.Now(), LoopID: "loop-3", RepoName: "test-repo", TotalLatencyMs: 1500, TotalCostUSD: 0.04, Status: "completed", VerifyPassed: false, TaskType: "test"},
	})

	// Write a baseline.
	writeBaselineFile(t, repo.Path, &e2e.LoopBaseline{
		Aggregate: &e2e.BaselineStats{
			SampleCount: 10,
			CostP50:     0.04,
			CostP95:     0.08,
			LatencyP50:  1500,
			LatencyP95:  3000,
		},
	})

	result, err := srv.handleLoopGates(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"hours": float64(24),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got: %s", getResultText(result))
	}

	var report map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &report); err != nil {
		t.Fatalf("unmarshal gate report: %v", err)
	}
	if report == nil {
		t.Fatal("expected non-nil gate report")
	}
}

// --- QW-6 (FINDING-226/238): Loop gates baseline auto-initialization ---

func TestHandleLoopGates_AutoInitBaseline(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repo := srv.findRepo("test-repo")
	if repo == nil {
		t.Fatal("test-repo not found after scan")
	}

	// Write observations but NO baseline file — the handler should auto-create one.
	now := time.Now()
	writeObservationsJSONL(t, repo.Path, []session.LoopObservation{
		{Timestamp: now, LoopID: "loop-1", RepoName: "test-repo", TotalLatencyMs: 1000, TotalCostUSD: 0.03, Status: "idle", VerifyPassed: true, TaskType: "build", PlannerProvider: "claude"},
		{Timestamp: now, LoopID: "loop-2", RepoName: "test-repo", TotalLatencyMs: 2000, TotalCostUSD: 0.05, Status: "idle", VerifyPassed: true, TaskType: "build", PlannerProvider: "claude"},
		{Timestamp: now, LoopID: "loop-3", RepoName: "test-repo", TotalLatencyMs: 1500, TotalCostUSD: 0.04, Status: "idle", VerifyPassed: true, TaskType: "test", PlannerProvider: "claude"},
		{Timestamp: now, LoopID: "loop-4", RepoName: "test-repo", TotalLatencyMs: 1200, TotalCostUSD: 0.04, Status: "idle", VerifyPassed: true, TaskType: "build", PlannerProvider: "claude"},
		{Timestamp: now, LoopID: "loop-5", RepoName: "test-repo", TotalLatencyMs: 1100, TotalCostUSD: 0.03, Status: "idle", VerifyPassed: true, TaskType: "test", PlannerProvider: "claude"},
	})

	// Verify no baseline file exists before the call.
	blPath := e2e.BaselinePath(repo.Path)
	if _, err := os.Stat(blPath); err == nil {
		t.Fatal("precondition failed: baseline file should not exist yet")
	}

	result, err := srv.handleLoopGates(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"hours": float64(24),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected non-error result, got: %s", getResultText(result))
	}

	// After the call, a baseline file should have been auto-created.
	if _, err := os.Stat(blPath); err != nil {
		t.Fatalf("expected baseline file to be auto-created at %s, got error: %v", blPath, err)
	}

	// Load the auto-created baseline and verify it has meaningful (non-zero) values.
	bl, err := e2e.LoadBaseline(blPath)
	if err != nil {
		t.Fatalf("failed to load auto-created baseline: %v", err)
	}
	if bl.Aggregate == nil {
		t.Fatal("expected auto-created baseline to have non-nil Aggregate")
	}
	if bl.Aggregate.CostP95 == 0 {
		t.Error("expected auto-created baseline CostP95 > 0 (not zero-initialized)")
	}
	if bl.Aggregate.LatencyP95 == 0 {
		t.Error("expected auto-created baseline LatencyP95 > 0 (not zero-initialized)")
	}
	if bl.Aggregate.SampleCount == 0 {
		t.Error("expected auto-created baseline SampleCount > 0")
	}
}

func TestHandleLoopGates_SaveErrorPropagated(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repo := srv.findRepo("test-repo")
	if repo == nil {
		t.Fatal("test-repo not found after scan")
	}

	// Write observations.
	now := time.Now()
	writeObservationsJSONL(t, repo.Path, []session.LoopObservation{
		{Timestamp: now, LoopID: "loop-1", RepoName: "test-repo", TotalLatencyMs: 1000, TotalCostUSD: 0.03, Status: "idle", VerifyPassed: true, TaskType: "build", PlannerProvider: "claude"},
	})

	// Make the baseline directory read-only so SaveBaseline fails.
	ralphDir := filepath.Join(repo.Path, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Create a directory at the baseline path to cause a write error.
	blPath := e2e.BaselinePath(repo.Path)
	if err := os.MkdirAll(blPath, 0755); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleLoopGates(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"hours": float64(24),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The result should be an error because SaveBaseline failed.
	if !result.IsError {
		t.Error("expected error result when baseline save fails, but got success")
	}
}

// --- FINDING-90 / FINDING-91 tests ---

func TestHandleLoopBaseline_RefreshNeverReturnsWindowHoursZero(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repo := srv.findRepo("test-repo")
	if repo == nil {
		t.Fatal("test-repo not found after scan")
	}

	now := time.Now()
	writeObservationsJSONL(t, repo.Path, []session.LoopObservation{
		{Timestamp: now.Add(-6 * time.Hour), LoopID: "loop-1", RepoName: "test-repo", TotalLatencyMs: 1000, TotalCostUSD: 0.03, Status: "completed", VerifyPassed: true, TaskType: "build"},
		{Timestamp: now.Add(-3 * time.Hour), LoopID: "loop-2", RepoName: "test-repo", TotalLatencyMs: 1500, TotalCostUSD: 0.04, Status: "completed", VerifyPassed: true, TaskType: "build"},
		{Timestamp: now, LoopID: "loop-3", RepoName: "test-repo", TotalLatencyMs: 2000, TotalCostUSD: 0.05, Status: "completed", VerifyPassed: true, TaskType: "test"},
	})

	result, err := srv.handleLoopBaseline(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"action": "refresh",
		"hours":  float64(48),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	wh, ok := data["window_hours"]
	if !ok {
		t.Fatal("expected window_hours field in response")
	}
	if wh == float64(0) {
		t.Fatal("window_hours must not be 0 — FINDING-90")
	}

	wt, ok := data["window_type"]
	if !ok {
		t.Fatal("expected window_type field in response")
	}
	if wt != "rolling" {
		t.Fatalf("expected window_type=rolling, got: %v", wt)
	}
}

func TestHandleLoopBenchmark_IncludesObservationCount(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repo := srv.findRepo("test-repo")
	if repo == nil {
		t.Fatal("test-repo not found after scan")
	}

	now := time.Now()
	writeObservationsJSONL(t, repo.Path, []session.LoopObservation{
		{Timestamp: now.Add(-2 * time.Hour), LoopID: "loop-1", RepoName: "test-repo", TotalLatencyMs: 1000, TotalCostUSD: 0.03, Status: "completed", VerifyPassed: true, TaskType: "build"},
		{Timestamp: now, LoopID: "loop-2", RepoName: "test-repo", TotalLatencyMs: 2000, TotalCostUSD: 0.05, Status: "completed", VerifyPassed: true, TaskType: "build"},
	})

	result, err := srv.handleLoopBenchmark(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"hours": float64(48),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	oc, ok := data["observation_count"]
	if !ok {
		t.Fatal("expected observation_count field — FINDING-91")
	}
	if oc != float64(2) {
		t.Fatalf("expected observation_count=2, got: %v", oc)
	}

	wt, ok := data["window_type"]
	if !ok {
		t.Fatal("expected window_type field — FINDING-91")
	}
	if wt != "rolling" {
		t.Fatalf("expected window_type=rolling, got: %v", wt)
	}

	ws, ok := data["window_size"]
	if !ok {
		t.Fatal("expected window_size field — FINDING-91")
	}
	if ws.(float64) <= 0 {
		t.Fatalf("expected window_size > 0, got: %v", ws)
	}
}

func TestHandleLoopBenchmark_DivergenceWarnings(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repo := srv.findRepo("test-repo")
	if repo == nil {
		t.Fatal("test-repo not found after scan")
	}

	now := time.Now()
	// Write observations with high cost (diverges from baseline).
	writeObservationsJSONL(t, repo.Path, []session.LoopObservation{
		{Timestamp: now.Add(-1 * time.Hour), LoopID: "loop-1", RepoName: "test-repo", TotalLatencyMs: 5000, TotalCostUSD: 0.50, Status: "completed", VerifyPassed: true, TaskType: "build"},
		{Timestamp: now, LoopID: "loop-2", RepoName: "test-repo", TotalLatencyMs: 6000, TotalCostUSD: 0.60, Status: "completed", VerifyPassed: false, TaskType: "build"},
	})

	// Write a baseline with much lower cost/latency — >20% divergence expected.
	writeBaselineFile(t, repo.Path, &e2e.LoopBaseline{
		Aggregate: &e2e.BaselineStats{
			SampleCount: 10,
			CostP50:     0.03,
			CostP95:     0.08,
			LatencyP50:  1200,
			LatencyP95:  2500,
		},
		Rates: &e2e.BaselineRates{
			CompletionRate: 0.90,
			VerifyPassRate: 0.95,
			ErrorRate:      0.05,
		},
	})

	result, err := srv.handleLoopBenchmark(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"hours": float64(48),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	dw, ok := data["divergence_warnings"]
	if !ok {
		t.Fatal("expected divergence_warnings when baseline and benchmark differ significantly — FINDING-91")
	}
	warnings, ok := dw.([]any)
	if !ok || len(warnings) == 0 {
		t.Fatal("expected non-empty divergence_warnings array")
	}

	// Verify at least one cost-related warning is present.
	found := false
	for _, w := range warnings {
		if s, ok := w.(string); ok && (strings.Contains(s, "cost_p50") || strings.Contains(s, "cost_p95")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected cost-related divergence warning, got: %v", warnings)
	}
}
