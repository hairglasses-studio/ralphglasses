package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// writeRepoStatus writes status.json to the .ralph dir for a test repo.
func writeRepoStatus(t *testing.T, repoPath string, status model.LoopStatus) {
	t.Helper()
	data, _ := json.Marshal(status)
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", "status.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// writeRepoCircuit writes .circuit_breaker_state to the .ralph dir for a test repo.
func writeRepoCircuit(t *testing.T, repoPath string, cb model.CircuitBreakerState) {
	t.Helper()
	data, _ := json.Marshal(cb)
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", ".circuit_breaker_state"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRepoHealth_HighScoreForHealthyRepo(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")

	// Write a CLAUDE.md so the health check can inspect it.
	_ = os.WriteFile(filepath.Join(repoPath, "CLAUDE.md"), []byte("# Test\nSome instructions.\n"), 0644)

	// Write healthy state to disk (RefreshRepo reads from disk).
	writeRepoCircuit(t, repoPath, model.CircuitBreakerState{State: "CLOSED"})
	writeRepoStatus(t, repoPath, model.LoopStatus{
		Timestamp: time.Now(),
		LoopCount: 5,
		Status:    "running",
	})

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRepoHealth returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	score := data["health_score"].(float64)
	if score < 80 {
		t.Errorf("expected high health score (>=80) for healthy repo, got %v", score)
	}
	if data["repo"] != "test-repo" {
		t.Errorf("expected repo=test-repo, got %v", data["repo"])
	}
	if data["circuit_breaker"] != "CLOSED" {
		t.Errorf("expected circuit_breaker=CLOSED, got %v", data["circuit_breaker"])
	}
}

func TestRepoHealth_MissingRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}
	assertErrorCode(t, "handleRepoHealth", result, "INVALID_PARAMS")
}

func TestRepoHealth_InvalidRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "../../etc/passwd",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}
	assertErrorCode(t, "handleRepoHealth", result, "INVALID_PARAMS")
}

func TestRepoHealth_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}
	assertErrorCode(t, "handleRepoHealth", result, "REPO_NOT_FOUND")
}

func TestRepoHealth_CircuitBreakerOpenPenalty(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	writeRepoCircuit(t, repoPath, model.CircuitBreakerState{
		State:  "OPEN",
		Reason: "too many failures",
	})
	writeRepoStatus(t, repoPath, model.LoopStatus{Timestamp: time.Now()})

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRepoHealth returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	score := data["health_score"].(float64)
	if score > 70 {
		t.Errorf("expected score <= 70 with circuit OPEN, got %v", score)
	}
	if data["circuit_breaker"] != "OPEN" {
		t.Errorf("expected circuit_breaker=OPEN, got %v", data["circuit_breaker"])
	}

	issues := data["issues"].([]any)
	foundCB := false
	for _, iss := range issues {
		if s, ok := iss.(string); ok && s == "circuit breaker OPEN: too many failures" {
			foundCB = true
		}
	}
	if !foundCB {
		t.Errorf("expected circuit breaker issue in issues list, got %v", issues)
	}
}

func TestRepoHealth_CircuitBreakerHalfOpenPenalty(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	writeRepoCircuit(t, repoPath, model.CircuitBreakerState{State: "HALF_OPEN"})
	writeRepoStatus(t, repoPath, model.LoopStatus{Timestamp: time.Now()})

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	score := data["health_score"].(float64)
	if score > 90 {
		t.Errorf("expected score <= 90 with circuit HALF_OPEN, got %v", score)
	}
}

func TestRepoHealth_BudgetExceededPenalty(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	writeRepoCircuit(t, repoPath, model.CircuitBreakerState{State: "CLOSED"})
	writeRepoStatus(t, repoPath, model.LoopStatus{
		Timestamp:    time.Now(),
		BudgetStatus: "exceeded",
	})

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	score := data["health_score"].(float64)
	if score > 80 {
		t.Errorf("expected score <= 80 with budget exceeded, got %v", score)
	}

	issues := data["issues"].([]any)
	foundBudget := false
	for _, iss := range issues {
		if s, ok := iss.(string); ok && s == "budget exceeded" {
			foundBudget = true
		}
	}
	if !foundBudget {
		t.Errorf("expected 'budget exceeded' in issues, got %v", issues)
	}
}

func TestRepoHealth_StaleStatusPenalty(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	writeRepoCircuit(t, repoPath, model.CircuitBreakerState{State: "CLOSED"})
	writeRepoStatus(t, repoPath, model.LoopStatus{
		Timestamp: time.Now().Add(-2 * time.Hour),
	})

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	score := data["health_score"].(float64)
	if score > 85 {
		t.Errorf("expected score <= 85 with stale status, got %v", score)
	}
}

func TestRepoHealth_ErroredSessionsPenalty(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	writeRepoCircuit(t, repoPath, model.CircuitBreakerState{State: "CLOSED"})
	writeRepoStatus(t, repoPath, model.LoopStatus{Timestamp: time.Now()})

	// Add 3 errored sessions (-5 each = -15).
	for i := 0; i < 3; i++ {
		srv.SessMgr.AddSessionForTesting(&session.Session{
			ID:       "sess-err-health-" + string(rune('a'+i)),
			Provider: session.ProviderClaude,
			RepoPath: repoPath,
			RepoName: "test-repo",
			Status:   session.StatusErrored,
			SpentUSD: 0.10,
		})
	}

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	score := data["health_score"].(float64)
	if score > 85 {
		t.Errorf("expected score <= 85 with 3 errored sessions, got %v", score)
	}
	erroredCount := data["errored_sessions"].(float64)
	if erroredCount < 3 {
		t.Errorf("expected errored_sessions >= 3, got %v", erroredCount)
	}
}

func TestRepoHealth_CombinedPenaltiesStackCorrectly(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")

	// Circuit OPEN (-30), budget exceeded (-20), stale (-15) = max 35.
	writeRepoCircuit(t, repoPath, model.CircuitBreakerState{
		State:  "OPEN",
		Reason: "failures",
	})
	writeRepoStatus(t, repoPath, model.LoopStatus{
		Timestamp:    time.Now().Add(-2 * time.Hour),
		BudgetStatus: "exceeded",
	})

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	score := data["health_score"].(float64)
	// 100 - 30 - 20 - 15 = 35 max.
	if score > 35 {
		t.Errorf("expected score <= 35 with combined penalties, got %v", score)
	}

	issues := data["issues"].([]any)
	if len(issues) < 3 {
		t.Errorf("expected at least 3 issues, got %d: %v", len(issues), issues)
	}
}

func TestRepoHealth_ScoreFloorAtZero(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")

	// Stack all possible penalties to exceed 100.
	writeRepoCircuit(t, repoPath, model.CircuitBreakerState{
		State:  "OPEN",
		Reason: "failures",
	})
	writeRepoStatus(t, repoPath, model.LoopStatus{
		Timestamp:    time.Now().Add(-2 * time.Hour),
		BudgetStatus: "exceeded",
	})
	// Add many errored sessions.
	for i := 0; i < 20; i++ {
		srv.SessMgr.AddSessionForTesting(&session.Session{
			ID:       "sess-floor-" + string(rune('A'+i)),
			Provider: session.ProviderClaude,
			RepoPath: repoPath,
			RepoName: "test-repo",
			Status:   session.StatusErrored,
		})
	}

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	score := data["health_score"].(float64)
	if score < 0 {
		t.Errorf("health_score should never be negative, got %v", score)
	}
}

func TestRepoHealth_ActiveSessionsAndSpend(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	writeRepoCircuit(t, repoPath, model.CircuitBreakerState{State: "CLOSED"})
	writeRepoStatus(t, repoPath, model.LoopStatus{Timestamp: time.Now()})

	srv.SessMgr.AddSessionForTesting(&session.Session{
		ID:       "sess-active-h1",
		Provider: session.ProviderClaude,
		RepoPath: repoPath,
		RepoName: "test-repo",
		Status:   session.StatusRunning,
		SpentUSD: 2.00,
	})
	srv.SessMgr.AddSessionForTesting(&session.Session{
		ID:       "sess-active-h2",
		Provider: session.ProviderGemini,
		RepoPath: repoPath,
		RepoName: "test-repo",
		Status:   session.StatusRunning,
		SpentUSD: 1.00,
	})

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["active_sessions"].(float64) < 2 {
		t.Errorf("expected active_sessions >= 2, got %v", data["active_sessions"])
	}
	if data["total_spend_usd"].(float64) < 3.0 {
		t.Errorf("expected total_spend_usd >= 3.0, got %v", data["total_spend_usd"])
	}
}

func TestRepoHealth_ScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}
	assertErrorCode(t, "handleRepoHealth", result, "SCAN_FAILED")
}

func TestRepoHealth_EmptyArraysNotNull(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")

	// Write a CLAUDE.md so the health check can inspect it.
	_ = os.WriteFile(filepath.Join(repoPath, "CLAUDE.md"), []byte("# Test\nSome instructions.\n"), 0644)

	// Write healthy state — no issues should be generated.
	writeRepoCircuit(t, repoPath, model.CircuitBreakerState{State: "CLOSED"})
	writeRepoStatus(t, repoPath, model.LoopStatus{
		Timestamp: time.Now(),
		LoopCount: 5,
		Status:    "running",
	})

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRepoHealth returned error: %s", getResultText(result))
	}

	raw := getResultText(result)

	// Verify "issues" is [] not null in the raw JSON.
	if strings.Contains(raw, `"issues":null`) {
		t.Error("issues field marshaled as null instead of []")
	}
	if !strings.Contains(raw, `"issues":[]`) {
		t.Errorf("expected issues:[] in JSON output, got: %s", raw)
	}

	// Verify "claudemd_findings" is [] not null in the raw JSON.
	if strings.Contains(raw, `"claudemd_findings":null`) {
		t.Error("claudemd_findings field marshaled as null instead of []")
	}
	if !strings.Contains(raw, `"claudemd_findings":[]`) {
		t.Errorf("expected claudemd_findings:[] in JSON output, got: %s", raw)
	}
}

func TestRepoOptimize_EmptyArraysNotNull(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	// Create a bare repo inside the scan path so ValidatePath accepts it.
	emptyRepo := filepath.Join(root, "empty-optimize-repo")
	if err := os.MkdirAll(emptyRepo, 0755); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleRepoOptimize(context.Background(), makeRequest(map[string]any{
		"path": emptyRepo,
	}))
	if err != nil {
		t.Fatalf("handleRepoOptimize: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleRepoOptimize returned error: %s", getResultText(result))
	}

	raw := getResultText(result)

	// Optimizations should be [] not null.
	if strings.Contains(raw, `"optimizations":null`) {
		t.Error("optimizations field marshaled as null instead of []")
	}
	if !strings.Contains(raw, `"optimizations":[]`) {
		t.Errorf("expected optimizations:[] in JSON output, got: %s", raw)
	}

	// Issues should also be a non-null array (there will be issues for missing files).
	if strings.Contains(raw, `"issues":null`) {
		t.Error("issues field marshaled as null instead of []")
	}
}

// --- HealthWeights / computeHealthScore tests ---

func TestDefaultHealthWeights_AllOne(t *testing.T) {
	w := DefaultHealthWeights()
	if w.CircuitBreakerOpen != 1.0 || w.Staleness != 1.0 || w.BudgetExceeded != 1.0 ||
		w.ErroredSession != 1.0 || w.ConfigParseError != 1.0 || w.MissingDirectory != 1.0 ||
		w.StaleLockFile != 1.0 || w.ClaudeMDWarnings != 1.0 || w.CircuitBreakerHalfOpen != 1.0 {
		t.Error("DefaultHealthWeights should have all fields at 1.0")
	}
}

func TestComputeHealthScore_PerfectHealth(t *testing.T) {
	params := healthParams{cbState: "CLOSED"}
	score, issues := computeHealthScore(params, DefaultHealthWeights())
	if score != 100 {
		t.Errorf("score = %d, want 100", score)
	}
	if len(issues) != 0 {
		t.Errorf("issues = %v, want none", issues)
	}
}

func TestComputeHealthScore_CircuitBreakerOpen(t *testing.T) {
	params := healthParams{cbState: "OPEN", cbReason: "failures"}
	score, issues := computeHealthScore(params, DefaultHealthWeights())
	if score != 70 {
		t.Errorf("score = %d, want 70 (100-30)", score)
	}
	if len(issues) != 1 {
		t.Errorf("issues len = %d, want 1", len(issues))
	}
}

func TestComputeHealthScore_CustomWeights(t *testing.T) {
	params := healthParams{cbState: "OPEN", cbReason: "test"}
	w := DefaultHealthWeights()
	w.CircuitBreakerOpen = 2.0 // double penalty
	score, _ := computeHealthScore(params, w)
	if score != 40 {
		t.Errorf("score = %d, want 40 (100 - 30*2)", score)
	}
}

func TestComputeHealthScore_ZeroWeightDisablesCheck(t *testing.T) {
	params := healthParams{
		cbState:        "OPEN",
		cbReason:       "test",
		budgetExceeded: true,
	}
	w := DefaultHealthWeights()
	w.CircuitBreakerOpen = 0 // disable CB check
	score, issues := computeHealthScore(params, w)
	// Only budget penalty should apply: 100 - 20 = 80
	if score != 80 {
		t.Errorf("score = %d, want 80 (CB disabled, budget -20)", score)
	}
	// Should still report both issues.
	if len(issues) != 2 {
		t.Errorf("issues len = %d, want 2", len(issues))
	}
}

func TestComputeHealthScore_FloorAtZero(t *testing.T) {
	params := healthParams{
		cbState:         "OPEN",
		cbReason:        "test",
		budgetExceeded:  true,
		erroredSessions: 20,
		staleMinutes:    120,
	}
	w := DefaultHealthWeights()
	w.CircuitBreakerOpen = 5.0 // 30*5 = 150 penalty alone
	score, _ := computeHealthScore(params, w)
	if score != 0 {
		t.Errorf("score = %d, want 0 (floor)", score)
	}
}

func TestComputeHealthScore_AllPenalties(t *testing.T) {
	params := healthParams{
		cbState:          "OPEN",
		cbReason:         "test",
		staleMinutes:     120,
		budgetExceeded:   true,
		erroredSessions:  2,
		configParseError: "bad",
		missingDirs:      []string{".ralph"},
		staleLockMinutes: 120,
		claudeMDWarnings: 5,
	}
	// 100 - 30 - 15 - 20 - 10 - 5 - 5 - 10 - 10 = -5 -> 0
	score, issues := computeHealthScore(params, DefaultHealthWeights())
	if score > 5 {
		t.Errorf("score = %d, expected near 0 with all penalties", score)
	}
	if len(issues) < 7 {
		t.Errorf("issues len = %d, expected >= 7", len(issues))
	}
}

func TestWeightedPenalty(t *testing.T) {
	if got := weightedPenalty(30, 1.0); got != 30 {
		t.Errorf("weightedPenalty(30, 1.0) = %d, want 30", got)
	}
	if got := weightedPenalty(30, 0.5); got != 15 {
		t.Errorf("weightedPenalty(30, 0.5) = %d, want 15", got)
	}
	if got := weightedPenalty(30, 0); got != 0 {
		t.Errorf("weightedPenalty(30, 0) = %d, want 0", got)
	}
	if got := weightedPenalty(10, 2.0); got != 20 {
		t.Errorf("weightedPenalty(10, 2.0) = %d, want 20", got)
	}
}

func TestRepoHealth_LoopRunningField(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	writeRepoCircuit(t, repoPath, model.CircuitBreakerState{State: "CLOSED"})
	writeRepoStatus(t, repoPath, model.LoopStatus{Timestamp: time.Now()})

	// Mark as running.
	srv.ProcMgr.AddProcForTesting(repoPath, false)

	result, err := srv.handleRepoHealth(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRepoHealth: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["loop_running"] != true {
		t.Errorf("expected loop_running=true when process is managed, got %v", data["loop_running"])
	}
}
