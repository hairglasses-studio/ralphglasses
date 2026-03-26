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

// writeFleetStatus and writeFleetCircuit write state to disk so RefreshRepo picks it up.
func writeFleetStatus(t *testing.T, repoPath string, status model.LoopStatus) {
	t.Helper()
	data, _ := json.Marshal(status)
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", "status.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func writeFleetCircuit(t *testing.T, repoPath string, cb model.CircuitBreakerState) {
	t.Helper()
	data, _ := json.Marshal(cb)
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", ".circuit_breaker_state"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func writeFleetConfig(t *testing.T, repoPath string, values map[string]string) {
	t.Helper()
	content := ""
	for k, v := range values {
		content += k + "=" + v + "\n"
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".ralphrc"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestFleetStatus_AutoScansWhenNoRepos(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Don't scan first -- handleFleetStatus should auto-scan.
	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleFleetStatus returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	summary := data["summary"].(map[string]any)
	if summary["total_repos"].(float64) < 1 {
		t.Errorf("expected at least 1 repo after auto-scan, got %v", summary["total_repos"])
	}
}

func TestFleetStatus_SummaryOnlyWithSessions(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Add a session so we can verify spend/counts appear in summary.
	srv.SessMgr.AddSessionForTesting(&session.Session{
		ID:       "sess-summary-1",
		Provider: session.ProviderClaude,
		RepoPath: "/tmp/test-repo",
		RepoName: "test-repo",
		Status:   session.StatusRunning,
		SpentUSD: 1.25,
	})

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(map[string]any{
		"summary_only": true,
	}))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleFleetStatus returned error: %s", getResultText(result))
	}

	var data map[string]any
	text := getResultText(result)
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	for _, key := range []string{"repos", "total_sessions", "running_sessions", "total_spend_usd"} {
		if _, ok := data[key]; !ok {
			t.Errorf("expected %q key in summary_only result", key)
		}
	}

	if data["running_sessions"].(float64) < 1 {
		t.Errorf("expected at least 1 running session, got %v", data["running_sessions"])
	}
	if data["total_spend_usd"].(float64) < 1.0 {
		t.Errorf("expected total_spend_usd >= 1.0, got %v", data["total_spend_usd"])
	}

	// summary_only should NOT contain full detail keys like "alerts".
	if _, ok := data["alerts"]; ok {
		t.Error("summary_only should not contain 'alerts' key")
	}
}

func TestFleetStatus_PopulatedSessionsDetail(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	srv.SessMgr.AddSessionForTesting(&session.Session{
		ID:        "sess-running-1",
		Provider:  session.ProviderClaude,
		RepoPath:  srv.Repos[0].Path,
		RepoName:  "test-repo",
		Status:    session.StatusRunning,
		Model:     "sonnet",
		SpentUSD:  2.50,
		TurnCount: 10,
	})
	srv.SessMgr.AddSessionForTesting(&session.Session{
		ID:        "sess-errored-1",
		Provider:  session.ProviderClaude,
		RepoPath:  srv.Repos[0].Path,
		RepoName:  "test-repo",
		Status:    session.StatusErrored,
		Error:     "out of tokens",
		SpentUSD:  0.50,
		TurnCount: 3,
	})
	srv.SessMgr.AddSessionForTesting(&session.Session{
		ID:       "sess-completed-1",
		Provider: session.ProviderClaude,
		RepoPath: srv.Repos[0].Path,
		RepoName: "test-repo",
		Status:   session.StatusCompleted,
		SpentUSD: 1.00,
	})

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleFleetStatus returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	summary := data["summary"].(map[string]any)
	if summary["total_sessions"].(float64) < 3 {
		t.Errorf("expected at least 3 total_sessions, got %v", summary["total_sessions"])
	}
	if summary["running_sessions"].(float64) < 1 {
		t.Errorf("expected at least 1 running_sessions, got %v", summary["running_sessions"])
	}

	sessions := data["sessions"].([]any)
	if len(sessions) < 3 {
		t.Errorf("expected at least 3 sessions in detail, got %d", len(sessions))
	}

	repos := data["repos"].([]any)
	if len(repos) < 1 {
		t.Fatal("expected at least 1 repo in result")
	}
	repo0 := repos[0].(map[string]any)
	if repo0["sessions_total"].(float64) < 3 {
		t.Errorf("expected sessions_total >= 3, got %v", repo0["sessions_total"])
	}
}

func TestFleetStatus_AlertCircuitBreakerOpen(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	writeFleetCircuit(t, repoPath, model.CircuitBreakerState{
		State:  "OPEN",
		Reason: "3 consecutive failures",
	})

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleFleetStatus returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	summary := data["summary"].(map[string]any)
	if summary["open_circuits"].(float64) < 1 {
		t.Errorf("expected open_circuits >= 1, got %v", summary["open_circuits"])
	}

	alerts := data["alerts"].([]any)
	foundCritical := false
	for _, a := range alerts {
		alert := a.(map[string]any)
		if alert["severity"] == "critical" && strings.Contains(alert["message"].(string), "Circuit breaker OPEN") {
			foundCritical = true
			break
		}
	}
	if !foundCritical {
		t.Errorf("expected critical circuit breaker alert, alerts: %v", alerts)
	}
}

func TestFleetStatus_AlertStaleLoop(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	writeFleetStatus(t, repoPath, model.LoopStatus{
		Timestamp: time.Now().Add(-2 * time.Hour),
		LoopCount: 5,
		Status:    "running",
	})
	srv.ProcMgr.AddProcForTesting(repoPath, false)

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	alerts := data["alerts"].([]any)
	foundStale := false
	for _, a := range alerts {
		alert := a.(map[string]any)
		if alert["severity"] == "warning" && strings.Contains(alert["message"].(string), "Loop stale") {
			foundStale = true
			break
		}
	}
	if !foundStale {
		t.Errorf("expected stale loop warning alert, alerts: %v", alerts)
	}
}

func TestFleetStatus_AlertBudgetNearLimit(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	writeFleetConfig(t, repoPath, map[string]string{
		"RALPH_SESSION_BUDGET": "10.00",
		"MODEL":                "sonnet",
	})
	writeFleetStatus(t, repoPath, model.LoopStatus{
		Timestamp:       time.Now(),
		SessionSpendUSD: 9.50,
	})

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	alerts := data["alerts"].([]any)
	foundBudget := false
	for _, a := range alerts {
		alert := a.(map[string]any)
		if alert["severity"] == "warning" && strings.Contains(alert["message"].(string), "Budget at") {
			foundBudget = true
			break
		}
	}
	if !foundBudget {
		t.Errorf("expected budget warning alert, alerts: %v", alerts)
	}
}

func TestFleetStatus_AlertNoProgressStreak(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	writeFleetCircuit(t, repoPath, model.CircuitBreakerState{
		State:                 "CLOSED",
		ConsecutiveNoProgress: 5,
	})

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	alerts := data["alerts"].([]any)
	foundNoProgress := false
	for _, a := range alerts {
		alert := a.(map[string]any)
		if alert["severity"] == "warning" && strings.Contains(alert["message"].(string), "No-progress streak") {
			foundNoProgress = true
			break
		}
	}
	if !foundNoProgress {
		t.Errorf("expected no-progress streak warning, alerts: %v", alerts)
	}
}

func TestFleetStatus_AlertErroredSession(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	srv.SessMgr.AddSessionForTesting(&session.Session{
		ID:       "sess-err-alert",
		Provider: session.ProviderClaude,
		RepoPath: srv.Repos[0].Path,
		RepoName: "test-repo",
		Status:   session.StatusErrored,
		Error:    "timeout exceeded",
	})

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	alerts := data["alerts"].([]any)
	foundSessionErr := false
	for _, a := range alerts {
		alert := a.(map[string]any)
		if alert["severity"] == "info" && strings.Contains(alert["message"].(string), "errored") {
			foundSessionErr = true
			break
		}
	}
	if !foundSessionErr {
		t.Errorf("expected errored session info alert, alerts: %v", alerts)
	}
}

func TestFleetStatus_AlertLoopPaused(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	srv.ProcMgr.AddProcForTesting(repoPath, true)

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	alerts := data["alerts"].([]any)
	foundPaused := false
	for _, a := range alerts {
		alert := a.(map[string]any)
		if alert["severity"] == "info" && strings.Contains(alert["message"].(string), "Loop paused") {
			foundPaused = true
			break
		}
	}
	if !foundPaused {
		t.Errorf("expected paused loop info alert, alerts: %v", alerts)
	}
}

func TestFleetStatus_ProviderBreakdown(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	srv.SessMgr.AddSessionForTesting(&session.Session{
		ID:       "sess-prov-1",
		Provider: session.ProviderClaude,
		RepoPath: srv.Repos[0].Path,
		RepoName: "test-repo",
		Status:   session.StatusRunning,
		SpentUSD: 3.00,
	})
	srv.SessMgr.AddSessionForTesting(&session.Session{
		ID:       "sess-prov-2",
		Provider: session.ProviderGemini,
		RepoPath: srv.Repos[0].Path,
		RepoName: "test-repo",
		Status:   session.StatusCompleted,
		SpentUSD: 0.50,
	})

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetStatus: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	summary := data["summary"].(map[string]any)
	providers := summary["providers"].(map[string]any)
	if len(providers) < 2 {
		t.Errorf("expected at least 2 providers in breakdown, got %d", len(providers))
	}

	claudeStats := providers["claude"].(map[string]any)
	if claudeStats["sessions"].(float64) < 1 {
		t.Errorf("expected claude sessions >= 1, got %v", claudeStats["sessions"])
	}
	if claudeStats["spend_usd"].(float64) < 3.0 {
		t.Errorf("expected claude spend >= 3.0, got %v", claudeStats["spend_usd"])
	}
}
