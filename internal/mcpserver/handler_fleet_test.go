package mcpserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if result.IsError {
		t.Fatal("expected graceful degradation (non-error), got IsError=true")
	}
	text := getResultText(result)
	if !strings.Contains(text, `"status":"not_configured"`) {
		t.Errorf("expected status not_configured, got: %s", text)
	}
	if !strings.Contains(text, `"fleet_mode":false`) {
		t.Errorf("expected fleet_mode:false, got: %s", text)
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
		"budget_usd":   float64(10),
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
	if result.IsError {
		t.Fatal("expected graceful degradation (non-error), got IsError=true")
	}
	text := getResultText(result)
	if !strings.Contains(text, `"status":"not_configured"`) {
		t.Errorf("expected status not_configured, got: %s", text)
	}
	if !strings.Contains(text, `"fleet_mode":false`) {
		t.Errorf("expected fleet_mode:false, got: %s", text)
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
	if result.IsError {
		t.Fatal("expected graceful degradation (non-error), got IsError=true")
	}
	text := getResultText(result)
	if !strings.Contains(text, `"status":"not_configured"`) {
		t.Errorf("expected status not_configured, got: %s", text)
	}
	if !strings.Contains(text, `"fleet_mode":false`) {
		t.Errorf("expected fleet_mode:false, got: %s", text)
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
			wantErr: false,
			checkText: func(t *testing.T, text string) {
				if !strings.Contains(text, `"status":"not_configured"`) {
					t.Errorf("expected status not_configured, got: %s", text)
				}
				if !strings.Contains(text, `"fleet_mode":false`) {
					t.Errorf("expected fleet_mode:false, got: %s", text)
				}
			},
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
			name:    "action without coordinator (client-only) graceful degrade",
			args:    map[string]any{"action": "pause", "worker_id": "w1"},
			noCoord: true,
			wantErr: false,
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

// --- handleHITLScore ---

func TestHandleHITLScore_NilTracker(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.HITLTracker = nil

	result, err := srv.handleHITLScore(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleHITLScore", result, "NOT_RUNNING")
}

// --- handleHITLHistory ---

func TestHandleHITLHistory_NilTracker(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.HITLTracker = nil

	result, err := srv.handleHITLHistory(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleHITLHistory", result, "NOT_RUNNING")
}

// --- handleAutonomyLevel ---

func TestHandleAutonomyLevel_NilDecisionLog(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.DecisionLog = nil

	result, err := srv.handleAutonomyLevel(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleAutonomyLevel", result, "NOT_RUNNING")
}

// --- handleAutonomyDecisions ---

func TestHandleAutonomyDecisions_NilDecisionLog(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.DecisionLog = nil

	result, err := srv.handleAutonomyDecisions(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleAutonomyDecisions", result, "NOT_RUNNING")
}

// --- handleAutonomyOverride ---

func TestHandleAutonomyOverride_NilDecisionLog(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.DecisionLog = nil

	result, err := srv.handleAutonomyOverride(context.Background(), makeRequest(map[string]any{
		"decision_id": "test-decision",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleAutonomyOverride", result, "NOT_RUNNING")
}

// --- handleFeedbackProfiles ---

func TestHandleFeedbackProfiles_NilAnalyzer(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.FeedbackAnalyzer = nil

	result, err := srv.handleFeedbackProfiles(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleFeedbackProfiles", result, "NOT_RUNNING")
}

func TestHandleFeedbackProfiles_EmptyAutoSeeds(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	stateDir := filepath.Join(root, "state")
	srv.InitSelfImprovement(stateDir, 3)

	// Write observations for the test repo.
	repoPath := filepath.Join(root, "test-repo")
	obsPath := session.ObservationPath(repoPath)
	for _, obs := range []session.LoopObservation{
		{WorkerProvider: "claude", TaskType: "bug_fix", TotalCostUSD: 1.0, VerifyPassed: true, WorkerTokensOut: 5},
		{WorkerProvider: "claude", TaskType: "feature", TotalCostUSD: 2.0, VerifyPassed: false, WorkerTokensOut: 10},
	} {
		if err := session.WriteObservation(obsPath, obs); err != nil {
			t.Fatalf("write observation: %v", err)
		}
	}

	// Trigger scan so repos are discovered.
	if err := srv.scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	result, err := srv.handleFeedbackProfiles(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "bug_fix") {
		t.Errorf("expected seeded bug_fix profile, got: %s", text)
	}
	if !strings.Contains(text, `"seeded":true`) {
		t.Errorf("expected seeded=true, got: %s", text)
	}
}

func TestHandleFeedbackProfiles_SeedAction(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	stateDir := filepath.Join(root, "state")
	srv.InitSelfImprovement(stateDir, 3)

	// Write observations.
	repoPath := filepath.Join(root, "test-repo")
	obsPath := session.ObservationPath(repoPath)
	if err := session.WriteObservation(obsPath, session.LoopObservation{
		WorkerProvider: "gemini", TaskType: "test", TotalCostUSD: 0.5, VerifyPassed: true,
	}); err != nil {
		t.Fatalf("write observation: %v", err)
	}

	if err := srv.scan(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	result, err := srv.handleFeedbackProfiles(context.Background(), makeRequest(map[string]any{
		"action": "seed",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "gemini") {
		t.Errorf("expected gemini in seeded profiles, got: %s", text)
	}
}

// --- handleProviderRecommend ---

func TestHandleProviderRecommend_NilOptimizer(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.AutoOptimizer = nil

	result, err := srv.handleProviderRecommend(context.Background(), makeRequest(map[string]any{
		"task": "test task",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleProviderRecommend", result, "NOT_RUNNING")
}

func TestHandleProviderRecommend_MissingTask(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.AutoOptimizer = nil

	result, err := srv.handleProviderRecommend(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// nil AutoOptimizer returns before param check
	assertErrorCode(t, "handleProviderRecommend", result, "NOT_RUNNING")
}

func TestHandleProviderRecommend_ColdStartBootstrap(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	// Set up self-improvement infrastructure.
	stateDir := filepath.Join(root, ".session-state")
	srv.InitSelfImprovement(stateDir, 2)

	// Wire a cascade router so SelectTier is available.
	cfg := session.DefaultCascadeConfig()
	cr := session.NewCascadeRouter(cfg, nil, nil, stateDir)
	srv.SessMgr.SetCascadeRouter(cr)

	// No feedback data seeded — cold-start condition.
	// Simple task (lint/format) should route to Gemini, not Claude.
	result, err := srv.handleProviderRecommend(context.Background(), makeRequest(map[string]any{
		"task": "lint the project files",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)

	// Cold-start should use heuristic data source.
	if !strings.Contains(text, `"data_source":"heuristic"`) {
		t.Errorf("expected data_source=heuristic, got: %s", text)
	}
	// Lint tasks are complexity 1, should pick gemini (cheapest).
	if !strings.Contains(text, `"provider":"gemini"`) {
		t.Errorf("expected provider=gemini for lint task, got: %s", text)
	}
	// Confidence should be low during cold start.
	if !strings.Contains(text, `"confidence":"low"`) {
		t.Errorf("expected confidence=low, got: %s", text)
	}

	// Complex task (architecture/planning) should pick Claude.
	result2, err := srv.handleProviderRecommend(context.Background(), makeRequest(map[string]any{
		"task": "plan the system architecture for the new module",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text2 := getResultText(result2)
	if !strings.Contains(text2, `"provider":"claude"`) {
		t.Errorf("expected provider=claude for architecture task, got: %s", text2)
	}
	if !strings.Contains(text2, `"data_source":"heuristic"`) {
		t.Errorf("expected data_source=heuristic for architecture task, got: %s", text2)
	}
}

func TestHandleProviderRecommend_WithFeedbackData(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	stateDir := filepath.Join(root, ".session-state")
	srv.InitSelfImprovement(stateDir, 2)

	cfg := session.DefaultCascadeConfig()
	cr := session.NewCascadeRouter(cfg, nil, nil, stateDir)
	srv.SessMgr.SetCascadeRouter(cr)

	// Seed sufficient multi-provider data to exit cold start.
	entries := make([]session.JournalEntry, 0, 10)
	for i := 0; i < 5; i++ {
		entries = append(entries, session.JournalEntry{
			Provider:  "gemini",
			TaskFocus: "lint code",
			SpentUSD:  0.01,
			TurnCount: 3,
		})
	}
	for i := 0; i < 5; i++ {
		entries = append(entries, session.JournalEntry{
			Provider:  "claude",
			TaskFocus: "lint code",
			SpentUSD:  0.10,
			TurnCount: 3,
		})
	}
	srv.FeedbackAnalyzer.Ingest(entries)

	result, err := srv.handleProviderRecommend(context.Background(), makeRequest(map[string]any{
		"task": "lint the project",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)

	// With sufficient multi-provider data, should use feedback_data.
	if !strings.Contains(text, `"data_source":"feedback_data"`) {
		t.Errorf("expected data_source=feedback_data, got: %s", text)
	}
}

// --- handleFleetDLQ ---

// --- handleFleetAnalytics with injected sessions ---

func TestHandleFleetAnalytics_WithSessions(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = session.ProviderClaude
		s.SpentUSD = 2.5
		s.TurnCount = 10
		s.Status = session.StatusRunning
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = session.ProviderGemini
		s.SpentUSD = 1.0
		s.TurnCount = 20
		s.Status = session.StatusStopped
	})

	result, err := srv.handleFleetAnalytics(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"total_sessions":2`) {
		t.Errorf("expected 2 total sessions, got: %s", text)
	}
	if !strings.Contains(text, `"claude"`) {
		t.Errorf("expected claude provider stats, got: %s", text)
	}
	if !strings.Contains(text, `"gemini"`) {
		t.Errorf("expected gemini provider stats, got: %s", text)
	}
	if !strings.Contains(text, "avg_cost_per_turn") {
		t.Errorf("expected avg_cost_per_turn, got: %s", text)
	}
}

func TestHandleFleetAnalytics_FilterByProvider(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = session.ProviderClaude
		s.SpentUSD = 2.5
		s.TurnCount = 10
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = session.ProviderGemini
		s.SpentUSD = 1.0
		s.TurnCount = 5
	})

	result, err := srv.handleFleetAnalytics(context.Background(), makeRequest(map[string]any{
		"provider": "claude",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, `"claude"`) {
		t.Errorf("expected claude stats, got: %s", text)
	}
	// gemini sessions should be filtered out, but total_sessions still shows all
	if strings.Contains(text, `"gemini"`) {
		t.Errorf("should not contain gemini stats after filter, got: %s", text)
	}
}

func TestHandleFleetAnalytics_FilterByRepo(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.RepoName = "test-repo"
		s.SpentUSD = 3.0
		s.TurnCount = 15
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.RepoName = "other-repo"
		s.SpentUSD = 1.0
		s.TurnCount = 5
	})

	result, err := srv.handleFleetAnalytics(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "test-repo") {
		t.Errorf("expected test-repo in repos, got: %s", text)
	}
}

// --- FINDING-237: observation-store fallback ---

func TestHandleFleetAnalytics_ObservationFallback(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	// Ensure FleetAnalytics is nil (standalone mode).
	srv.FleetAnalytics = nil

	// Write observation data to the test repo.
	obsPath := session.ObservationPath(repoPath)
	obs := []session.LoopObservation{
		{
			Timestamp:       time.Now().Add(-10 * time.Minute),
			LoopID:          "loop-1",
			RepoName:        "test-repo",
			PlannerProvider: "claude",
			WorkerProvider:  "claude",
			TotalLatencyMs:  500,
			TotalCostUSD:    0.05,
			PlannerCostUSD:  0.03,
			WorkerCostUSD:   0.02,
			Status:          "idle",
		},
		{
			Timestamp:       time.Now().Add(-5 * time.Minute),
			LoopID:          "loop-1",
			RepoName:        "test-repo",
			PlannerProvider: "gemini",
			WorkerProvider:  "gemini",
			TotalLatencyMs:  300,
			TotalCostUSD:    0.02,
			PlannerCostUSD:  0.01,
			WorkerCostUSD:   0.01,
			Status:          "idle",
		},
		{
			Timestamp:      time.Now().Add(-2 * time.Minute),
			LoopID:         "loop-1",
			RepoName:       "test-repo",
			TotalCostUSD:   0,
			TotalLatencyMs: 100,
			Status:         "failed",
			Error:          "test error",
		},
	}
	for _, o := range obs {
		if err := session.WriteObservation(obsPath, o); err != nil {
			t.Fatalf("write observation: %v", err)
		}
	}

	// Trigger a scan so the server knows about the repo.
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleFleetAnalytics(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)

	// Must include data_source=observation_store.
	if !strings.Contains(text, `"data_source":"observation_store"`) {
		t.Errorf("expected data_source observation_store, got: %s", text)
	}
	// Must include observation_count.
	if !strings.Contains(text, `"observation_count":3`) {
		t.Errorf("expected observation_count:3, got: %s", text)
	}
	// Must include metrics block with completions and failures.
	if !strings.Contains(text, `"completions":2`) {
		t.Errorf("expected 2 completions, got: %s", text)
	}
	if !strings.Contains(text, `"failures":1`) {
		t.Errorf("expected 1 failure, got: %s", text)
	}
	// Must have latency data.
	if !strings.Contains(text, "latency_p50_ms") {
		t.Errorf("expected latency_p50_ms in output, got: %s", text)
	}
	// Must have cost_per_provider.
	if !strings.Contains(text, "cost_per_provider") {
		t.Errorf("expected cost_per_provider in output, got: %s", text)
	}
}

func TestHandleFleetAnalytics_FleetCoordinatorDataSource(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Set up FleetAnalytics (simulating fleet mode).
	srv.FleetAnalytics = fleet.NewFleetAnalytics(1000, 24*time.Hour)
	srv.FleetAnalytics.RecordCompletion("worker-1", "claude", 500*time.Millisecond, 0.10)

	result, err := srv.handleFleetAnalytics(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, `"data_source":"fleet_coordinator"`) {
		t.Errorf("expected data_source fleet_coordinator, got: %s", text)
	}
	if !strings.Contains(text, `"completions":1`) {
		t.Errorf("expected 1 completion, got: %s", text)
	}
}

func TestHandleFleetDLQ_NilCoordinator(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.FleetCoordinator = nil

	result, err := srv.handleFleetDLQ(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected graceful degradation (non-error), got IsError=true")
	}
	text := getResultText(result)
	if !strings.Contains(text, `"status":"not_configured"`) {
		t.Errorf("expected status not_configured, got: %s", text)
	}
	if !strings.Contains(text, `"fleet_mode":false`) {
		t.Errorf("expected fleet_mode:false, got: %s", text)
	}
}

func TestHandleFleetDLQ_ListAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	result, err := srv.handleFleetDLQ(context.Background(), makeRequest(map[string]any{
		"action": "list",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "items") {
		t.Errorf("expected items in result, got: %s", text)
	}
	if !strings.Contains(text, "count") {
		t.Errorf("expected count in result, got: %s", text)
	}
}

func TestHandleFleetDLQ_DefaultAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	// No action specified defaults to "list"
	result, err := srv.handleFleetDLQ(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "items") {
		t.Errorf("expected items in result, got: %s", text)
	}
}

func TestHandleFleetDLQ_DepthAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	result, err := srv.handleFleetDLQ(context.Background(), makeRequest(map[string]any{
		"action": "depth",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "dlq_depth") {
		t.Errorf("expected dlq_depth in result, got: %s", text)
	}
}

func TestHandleFleetDLQ_PurgeAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	result, err := srv.handleFleetDLQ(context.Background(), makeRequest(map[string]any{
		"action": "purge",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "purged") {
		t.Errorf("expected purged in result, got: %s", text)
	}
}

func TestHandleFleetDLQ_RetryMissingItemID(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	result, err := srv.handleFleetDLQ(context.Background(), makeRequest(map[string]any{
		"action": "retry",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleFleetDLQ", result, "INVALID_PARAMS")
}

func TestHandleFleetDLQ_UnknownAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	result, err := srv.handleFleetDLQ(context.Background(), makeRequest(map[string]any{
		"action": "invalid_action",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleFleetDLQ", result, "INVALID_PARAMS")
}

// --- handleFleetWorkers action paths ---

func TestHandleFleetWorkers_ActionMissingWorkerID(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	result, err := srv.handleFleetWorkers(context.Background(), makeRequest(map[string]any{
		"action": "pause",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleFleetWorkers", result, "INVALID_PARAMS")
}

func TestHandleFleetWorkers_UnknownAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	coord := fleet.NewCoordinator("test-node", "localhost", 0, "test", nil, nil)
	srv.FleetCoordinator = coord

	result, err := srv.handleFleetWorkers(context.Background(), makeRequest(map[string]any{
		"action":    "unknown_action",
		"worker_id": "w1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertErrorCode(t, "handleFleetWorkers", result, "INVALID_PARAMS")
}

func TestHandleFleetWorkers_ActionRequiresCoordinator(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.FleetCoordinator = nil
	srv.FleetClient = nil

	result, err := srv.handleFleetWorkers(context.Background(), makeRequest(map[string]any{
		"action":    "pause",
		"worker_id": "w1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected graceful degradation (non-error), got IsError=true")
	}
	text := getResultText(result)
	if !strings.Contains(text, `"status":"not_configured"`) {
		t.Errorf("expected status not_configured, got: %s", text)
	}
	if !strings.Contains(text, `"fleet_mode":false`) {
		t.Errorf("expected fleet_mode:false, got: %s", text)
	}
}

// --- handleFleetStatus pagination ---

func TestHandleFleetStatus_Pagination(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Test with limit=1
	result, err := srv.handleFleetStatus(context.Background(), makeRequest(map[string]any{
		"limit": float64(1),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "has_more") {
		t.Errorf("expected has_more in output, got: %s", text)
	}
	if !strings.Contains(text, "total_count") {
		t.Errorf("expected total_count in output, got: %s", text)
	}
}

func TestHandleFleetStatus_PaginationOffset(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Test with large offset to get empty repos
	result, err := srv.handleFleetStatus(context.Background(), makeRequest(map[string]any{
		"offset": float64(9999),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	// Should have empty repos array but still have summary
	if !strings.Contains(text, "summary") {
		t.Errorf("expected summary in output, got: %s", text)
	}
	if !strings.Contains(text, `"has_more":false`) {
		t.Errorf("expected has_more=false with large offset, got: %s", text)
	}
}

func TestHandleFleetStatus_RepoFilter(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.RepoName = "test-repo"
		s.Provider = session.ProviderClaude
		s.SpentUSD = 2.5
		s.Status = session.StatusRunning
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.RepoName = "other-repo"
		s.Provider = session.ProviderGemini
		s.SpentUSD = 1.0
		s.Status = session.StatusStopped
	})

	// Full mode with repo filter
	result, err := srv.handleFleetStatus(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	// Sessions for other-repo should be filtered
	if strings.Contains(text, "other-repo") {
		t.Errorf("expected other-repo to be filtered out, got: %s", text)
	}
}

func TestHandleFleetStatus_RepoFilterSummaryOnly(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.RepoName = "test-repo"
		s.Provider = session.ProviderClaude
		s.SpentUSD = 2.5
		s.Status = session.StatusRunning
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.RepoName = "other-repo"
		s.Provider = session.ProviderGemini
		s.SpentUSD = 1.0
		s.Status = session.StatusStopped
	})

	// Summary with repo filter
	result, err := srv.handleFleetStatus(context.Background(), makeRequest(map[string]any{
		"summary_only": true,
		"repo":         "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "total_sessions") {
		t.Errorf("expected total_sessions, got: %s", text)
	}
	if !strings.Contains(text, "total_spend_usd") {
		t.Errorf("expected total_spend_usd, got: %s", text)
	}
}

func TestHandleFleetStatus_SummaryOnlySizeConstraint(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
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
	// Summary-only should be compact (<5KB)
	if len(text) > 5*1024 {
		t.Errorf("summary_only output should be <5KB, got %d bytes", len(text))
	}
}
