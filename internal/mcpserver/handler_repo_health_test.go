package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
