package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
)

// ---------------------------------------------------------------------------
// handleSupervisorStatus (0%)
// ---------------------------------------------------------------------------

func TestHandleSupervisorStatus_NilManager(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.SessMgr = nil

	result, err := srv.handleSupervisorStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "no manager") {
		t.Errorf("expected 'no manager' in result, got: %s", text)
	}
}

func TestHandleSupervisorStatus_NoSupervisor(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Manager exists but no supervisor is running.
	result, err := srv.handleSupervisorStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "supervisor not active") {
		t.Errorf("expected 'supervisor not active' in result, got: %s", text)
	}
	if !strings.Contains(text, `"running":false`) && !strings.Contains(text, `"running": false`) {
		t.Errorf("expected running=false in result, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// handleWorktreeCreate (0%)
// ---------------------------------------------------------------------------

func TestHandleWorktreeCreate_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleWorktreeCreate(context.Background(), makeRequest(map[string]any{
		"name": "test-wt",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo name required") {
		t.Errorf("expected 'repo name required', got: %s", text)
	}
}

func TestHandleWorktreeCreate_MissingName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleWorktreeCreate(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}
	text := getResultText(result)
	if !strings.Contains(text, "worktree name required") {
		t.Errorf("expected 'worktree name required', got: %s", text)
	}
}

func TestHandleWorktreeCreate_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleWorktreeCreate(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
		"name": "test-wt",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "not found") {
		t.Errorf("expected 'not found' in error, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// ConcurrencyMiddleware (0%)
// ---------------------------------------------------------------------------

func TestConcurrencyMiddleware_ZeroLimit(t *testing.T) {
	t.Parallel()
	// limit=0 should be a passthrough (no-op middleware).
	mw := ConcurrencyMiddleware(0)
	called := false
	handler := mw(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return textResult("ok"), nil
	})

	result, err := handler(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler should have been called")
	}
	if getResultText(result) != "ok" {
		t.Errorf("result = %q, want ok", getResultText(result))
	}
}

func TestConcurrencyMiddleware_PositiveLimit(t *testing.T) {
	t.Parallel()
	mw := ConcurrencyMiddleware(5)
	called := false
	handler := mw(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return textResult("ok"), nil
	})

	result, err := handler(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("handler should have been called")
	}
	if getResultText(result) != "ok" {
		t.Errorf("result = %q, want ok", getResultText(result))
	}
}

func TestConcurrencyMiddleware_CancelledContext(t *testing.T) {
	t.Parallel()
	mw := ConcurrencyMiddleware(1)

	// Occupy the single slot.
	occupied := make(chan struct{})
	release := make(chan struct{})
	handler := mw(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		close(occupied)
		<-release
		return textResult("ok"), nil
	})

	go func() {
		_, _ = handler(context.Background(), makeRequest(nil))
	}()
	<-occupied

	// Second call with cancelled context should get rejected.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	secondHandler := mw(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return textResult("should not reach"), nil
	})
	result, err := secondHandler(ctx, makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for cancelled context with full semaphore")
	}
	close(release)
}

// ---------------------------------------------------------------------------
// handleCostForecast with initialized predictor (40%)
// ---------------------------------------------------------------------------

func TestHandleCostForecast_WithPredictor(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.CostPredictor = fleet.NewCostPredictor(2.0)

	// Record some data so Forecast has something to work with.
	now := time.Now()
	srv.CostPredictor.Record(fleet.CostSample{Timestamp: now.Add(-3 * time.Minute), CostUSD: 1.0, Provider: "claude"})
	srv.CostPredictor.Record(fleet.CostSample{Timestamp: now.Add(-2 * time.Minute), CostUSD: 1.5, Provider: "claude"})
	srv.CostPredictor.Record(fleet.CostSample{Timestamp: now.Add(-1 * time.Minute), CostUSD: 2.0, Provider: "claude"})

	result, err := srv.handleCostForecast(context.Background(), makeRequest(map[string]any{
		"budget_remaining": float64(100),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	text := getResultText(result)
	if text == "" {
		t.Error("expected non-empty forecast result")
	}
}

func TestHandleCostForecast_WithPredictor_ZeroBudget(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.CostPredictor = fleet.NewCostPredictor(2.0)

	result, err := srv.handleCostForecast(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
}

// ---------------------------------------------------------------------------
// handleCostEstimate (67.6%) — improve coverage of loop mode + invalid mode
// ---------------------------------------------------------------------------

func TestHandleCostEstimate_LoopMode(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider":   "claude",
		"mode":       "loop",
		"iterations": float64(5),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if data["mode"] != "loop" {
		t.Errorf("mode = %v, want loop", data["mode"])
	}
}

func TestHandleCostEstimate_InvalidMode(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider": "claude",
		"mode":     "invalid",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid mode")
	}
}

func TestHandleCostEstimate_InvalidProvider(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider": "invalid-provider",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid provider")
	}
}

func TestHandleCostEstimate_MissingProvider(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing provider")
	}
}

func TestHandleCostEstimate_WithRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCostEstimate(context.Background(), makeRequest(map[string]any{
		"provider": "gemini",
		"repo":     "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
}

// ---------------------------------------------------------------------------
// normalizeMetricName helper (helpers.go)
// ---------------------------------------------------------------------------

func TestNormalizeMetricName_Known(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input, want string
	}{
		{"cost", "total_cost_usd"},
		{"latency", "total_latency_ms"},
		{"difficulty", "difficulty_score"},
	}
	for _, tc := range tests {
		if got := normalizeMetricName(tc.input); got != tc.want {
			t.Errorf("normalizeMetricName(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestNormalizeMetricName_Unknown(t *testing.T) {
	t.Parallel()
	if got := normalizeMetricName("unknown_metric"); got != "unknown_metric" {
		t.Errorf("normalizeMetricName(unknown_metric) = %q, want unknown_metric", got)
	}
}

// ---------------------------------------------------------------------------
// cascadeConfigFromRepo (0%)
// ---------------------------------------------------------------------------

func TestCascadeConfigFromRepo_NoRalphrc(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := cascadeConfigFromRepo(context.Background(), dir, "")
	// Should return DefaultCascadeConfig when no .ralphrc exists.
	if !cfg.Enabled {
		t.Error("expected cascade enabled from default config")
	}
	if cfg.CheapProvider == "" {
		t.Error("expected non-empty cheap provider from default config")
	}
}

func TestCascadeConfigFromRepo_WithRalphrc(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_ = srv
	repoPath := filepath.Join(root, "test-repo")

	cfg := cascadeConfigFromRepo(context.Background(), repoPath, "")
	// The test server creates a .ralphrc with CASCADE_ENABLED=true.
	if !cfg.Enabled {
		t.Error("expected cascade enabled")
	}
}

func TestCascadeConfigFromRepo_EmptyPath(t *testing.T) {
	t.Parallel()
	cfg := cascadeConfigFromRepo(context.Background(), "", "")
	// Should fall back to default.
	if !cfg.Enabled {
		t.Error("expected cascade enabled from default config")
	}
}
