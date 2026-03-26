package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- handleFleetSubmit ---

func TestHandleFleetSubmit_NilFleet(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.FleetCoordinator = nil
	srv.FleetClient = nil

	result, err := srv.handleFleetSubmit(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"prompt": "do something",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when fleet is not active")
	}
	text := getResultText(result)
	if !strings.Contains(text, "NOT_RUNNING") {
		t.Errorf("expected NOT_RUNNING error code, got: %s", text)
	}
}

func TestHandleFleetSubmit_MissingParams(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Need a coordinator so we get past the nil check
	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	// Missing repo
	result, err := srv.handleFleetSubmit(context.Background(), makeRequest(map[string]any{
		"prompt": "do something",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when repo is missing")
	}
	text := getResultText(result)
	if !strings.Contains(text, "INVALID_PARAMS") {
		t.Errorf("expected INVALID_PARAMS error code, got: %s", text)
	}

	// Missing prompt
	result, err = srv.handleFleetSubmit(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when prompt is missing")
	}
	text = getResultText(result)
	if !strings.Contains(text, "INVALID_PARAMS") {
		t.Errorf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleFleetSubmit_ValidCoordinator(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	result, err := srv.handleFleetSubmit(context.Background(), makeRequest(map[string]any{
		"repo":     "test-repo",
		"prompt":   "implement feature X",
		"provider": "claude",
		"budget":   float64(10),
		"priority": float64(3),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "work_item_id") {
		t.Errorf("expected work_item_id in result, got: %s", text)
	}
	if !strings.Contains(text, "pending") {
		t.Errorf("expected pending status in result, got: %s", text)
	}
	if !strings.Contains(text, "local_coordinator") {
		t.Errorf("expected local_coordinator queue in result, got: %s", text)
	}
}

// --- handleFleetStatus ---

func TestHandleFleetStatus_Basic(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	// Scan first so repos are populated
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "repos") {
		t.Errorf("expected repos in output, got: %s", text)
	}
	if !strings.Contains(text, "summary") {
		t.Errorf("expected summary in output, got: %s", text)
	}
}

func TestHandleFleetStatus_SummaryOnly(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	// Scan first so repos are populated
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleFleetStatus(context.Background(), makeRequest(map[string]any{
		"summary_only": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "repos") {
		t.Errorf("expected repos in summary output, got: %s", text)
	}
	if !strings.Contains(text, "total_sessions") {
		t.Errorf("expected total_sessions in summary output, got: %s", text)
	}
	if !strings.Contains(text, "total_spend_usd") {
		t.Errorf("expected total_spend_usd in summary output, got: %s", text)
	}
	if !strings.Contains(text, "running_sessions") {
		t.Errorf("expected running_sessions in summary output, got: %s", text)
	}
	if !strings.Contains(text, "repo_sessions") {
		t.Errorf("expected repo_sessions in summary output, got: %s", text)
	}
	// Summary should NOT contain full-dump fields like "sessions" array or "alerts"
	if strings.Contains(text, "\"sessions\"") {
		t.Errorf("summary_only should not contain full sessions array, got: %s", text)
	}
}

// --- handleFleetWorkers ---

func TestHandleFleetWorkers_NilFleet(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.FleetCoordinator = nil
	srv.FleetClient = nil

	result, err := srv.handleFleetWorkers(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when fleet is not active")
	}
	text := getResultText(result)
	if !strings.Contains(text, "NOT_RUNNING") {
		t.Errorf("expected NOT_RUNNING error code, got: %s", text)
	}
}

func TestHandleFleetWorkers_WithCoordinator(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	result, err := srv.handleFleetWorkers(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "workers") {
		t.Errorf("expected workers in result, got: %s", text)
	}
	if !strings.Contains(text, "total") {
		t.Errorf("expected total in result, got: %s", text)
	}
}

// --- handleFleetBudget ---

func TestHandleFleetBudget_NilFleet(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.FleetCoordinator = nil
	srv.FleetClient = nil

	result, err := srv.handleFleetBudget(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when fleet is not active")
	}
	text := getResultText(result)
	if !strings.Contains(text, "NOT_RUNNING") {
		t.Errorf("expected NOT_RUNNING error code, got: %s", text)
	}
}

func TestHandleFleetBudget_WithCoordinator(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	result, err := srv.handleFleetBudget(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "budget_usd") {
		t.Errorf("expected budget_usd in result, got: %s", text)
	}
	if !strings.Contains(text, "spent_usd") {
		t.Errorf("expected spent_usd in result, got: %s", text)
	}
	if !strings.Contains(text, "remaining") {
		t.Errorf("expected remaining in result, got: %s", text)
	}
}

func TestHandleFleetBudget_SetLimit(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	result, err := srv.handleFleetBudget(context.Background(), makeRequest(map[string]any{
		"limit": float64(50),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "50") {
		t.Errorf("expected budget of 50 in result, got: %s", text)
	}
}

// --- handleFleetAnalytics ---

func TestHandleFleetAnalytics_NoSessions(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleFleetAnalytics(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "total_sessions") {
		t.Errorf("expected total_sessions in output, got: %s", text)
	}
}

func TestHandleFleetAnalytics_WithRepoFilter(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Launch a session first to have data
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleFleetAnalytics(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	// With a nonexistent repo filter, total_sessions should be 0
	if !strings.Contains(text, "total_sessions") {
		t.Errorf("expected total_sessions in output, got: %s", text)
	}
}

func TestHandleFleetAnalytics_WithProviderFilter(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleFleetAnalytics(context.Background(), makeRequest(map[string]any{
		"provider": string(session.ProviderClaude),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "total_sessions") {
		t.Errorf("expected total_sessions in output, got: %s", text)
	}
}
