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

// --- handleFleetDLQ ---

func TestHandleFleetDLQ(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      map[string]any
		noCoord   bool
		wantErr   bool
		errCode   string
		checkText func(t *testing.T, text string)
	}{
		{
			name:    "nil coordinator",
			args:    map[string]any{},
			noCoord: true,
			wantErr: true,
			errCode: "NOT_RUNNING",
		},
		{
			name: "list empty DLQ (default action)",
			args: map[string]any{},
			checkText: func(t *testing.T, text string) {
				if !strings.Contains(text, "\"count\":0") {
					t.Errorf("expected count:0 for empty DLQ, got: %s", text)
				}
				if !strings.Contains(text, "\"items\"") {
					t.Errorf("expected items field, got: %s", text)
				}
			},
		},
		{
			name: "list action explicit",
			args: map[string]any{"action": "list"},
			checkText: func(t *testing.T, text string) {
				if !strings.Contains(text, "\"count\":0") {
					t.Errorf("expected count:0, got: %s", text)
				}
			},
		},
		{
			name: "depth action",
			args: map[string]any{"action": "depth"},
			checkText: func(t *testing.T, text string) {
				if !strings.Contains(text, "dlq_depth") {
					t.Errorf("expected dlq_depth field, got: %s", text)
				}
			},
		},
		{
			name: "purge empty DLQ",
			args: map[string]any{"action": "purge"},
			checkText: func(t *testing.T, text string) {
				if !strings.Contains(text, "purged") {
					t.Errorf("expected purged status, got: %s", text)
				}
				if !strings.Contains(text, "\"count\":0") {
					t.Errorf("expected count:0 after purging empty DLQ, got: %s", text)
				}
			},
		},
		{
			name:    "retry without item_id",
			args:    map[string]any{"action": "retry"},
			wantErr: true,
			errCode: "INVALID_PARAMS",
		},
		{
			name:    "retry with nonexistent item_id",
			args:    map[string]any{"action": "retry", "item_id": "nonexistent-item"},
			wantErr: true,
			errCode: "INTERNAL_ERROR",
		},
		{
			name:    "unknown action",
			args:    map[string]any{"action": "invalid_action"},
			wantErr: true,
			errCode: "INVALID_PARAMS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, _ := setupTestServer(t)
			if tt.noCoord {
				srv.FleetCoordinator = nil
			} else {
				srv.FleetCoordinator = fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
			}

			result, err := srv.handleFleetDLQ(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			text := getResultText(result)
			if tt.wantErr {
				if !result.IsError {
					t.Fatalf("expected error result, got: %s", text)
				}
				if tt.errCode != "" && !strings.Contains(text, tt.errCode) {
					t.Errorf("expected %s error code, got: %s", tt.errCode, text)
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", text)
			}
			if tt.checkText != nil {
				tt.checkText(t, text)
			}
		})
	}
}

// --- handleFleetWorkers actions ---

func TestHandleFleetWorkers_Actions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    map[string]any
		noCoord bool
		wantErr bool
		errCode string
	}{
		{
			name:    "action without coordinator (client-only)",
			args:    map[string]any{"action": "pause", "worker_id": "w1"},
			noCoord: true,
			wantErr: true,
			errCode: "NOT_RUNNING",
		},
		{
			name:    "action missing worker_id",
			args:    map[string]any{"action": "pause"},
			wantErr: true,
			errCode: "INVALID_PARAMS",
		},
		{
			name:    "unknown action",
			args:    map[string]any{"action": "reboot", "worker_id": "w1"},
			wantErr: true,
			errCode: "INVALID_PARAMS",
		},
		{
			name:    "pause non-existent worker",
			args:    map[string]any{"action": "pause", "worker_id": "no-such-worker"},
			wantErr: true,
			errCode: "INTERNAL_ERROR",
		},
		{
			name:    "resume non-existent worker",
			args:    map[string]any{"action": "resume", "worker_id": "no-such-worker"},
			wantErr: true,
			errCode: "INTERNAL_ERROR",
		},
		{
			name:    "drain non-existent worker",
			args:    map[string]any{"action": "drain", "worker_id": "no-such-worker"},
			wantErr: true,
			errCode: "INTERNAL_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, _ := setupTestServer(t)
			if tt.noCoord {
				srv.FleetCoordinator = nil
				srv.FleetClient = nil
			} else {
				srv.FleetCoordinator = fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
			}

			result, err := srv.handleFleetWorkers(context.Background(), makeRequest(tt.args))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			text := getResultText(result)
			if tt.wantErr {
				if !result.IsError {
					t.Fatalf("expected error result, got: %s", text)
				}
				if tt.errCode != "" && !strings.Contains(text, tt.errCode) {
					t.Errorf("expected %s error code, got: %s", tt.errCode, text)
				}
				return
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", text)
			}
		})
	}
}
