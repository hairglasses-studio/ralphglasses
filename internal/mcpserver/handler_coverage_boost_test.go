package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- InitSelfImprovement / WireAutoOptimizer coverage ---

func TestInitSelfImprovement(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()

	srv.HITLTracker = nil
	srv.DecisionLog = nil
	srv.FeedbackAnalyzer = nil
	srv.AutoOptimizer = nil

	srv.InitSelfImprovement(stateDir, 1)

	if srv.HITLTracker == nil {
		t.Fatal("expected HITLTracker to be initialized")
	}
	if srv.DecisionLog == nil {
		t.Fatal("expected DecisionLog to be initialized")
	}
	if srv.FeedbackAnalyzer == nil {
		t.Fatal("expected FeedbackAnalyzer to be initialized")
	}
	if srv.AutoOptimizer == nil {
		t.Fatal("expected AutoOptimizer to be initialized")
	}
}

func TestInitSelfImprovement_Idempotent(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()

	srv.InitSelfImprovement(stateDir, 0)
	tracker := srv.HITLTracker
	srv.InitSelfImprovement(stateDir, 0)

	if srv.HITLTracker != tracker {
		t.Error("expected second call to preserve existing HITLTracker")
	}
}

func TestWireAutoOptimizer_NilSafety(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.AutoOptimizer = nil

	// Should not panic with nil optimizer
	srv.WireAutoOptimizer(nil)
	srv.WireAutoOptimizer(srv.SessMgr)
}

func TestWireAutoOptimizer_WithState(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	srv.WireAutoOptimizer(srv.SessMgr)

	if srv.AutoOptimizer == nil {
		t.Fatal("expected AutoOptimizer to be wired")
	}
}

// --- HITL and autonomy with initialized state ---

func TestHandleHITLScore_Initialized(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	result, err := srv.handleHITLScore(context.Background(), makeRequest(map[string]any{
		"hours": float64(24),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
}

func TestHandleHITLHistory_Initialized(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	result, err := srv.handleHITLHistory(context.Background(), makeRequest(map[string]any{
		"hours": float64(48),
		"limit": float64(10),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "events") {
		t.Errorf("expected events field, got: %s", text)
	}
}

// --- Autonomy level with initialized state ---

func TestHandleAutonomyLevel_GetCurrent(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	result, err := srv.handleAutonomyLevel(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "level") {
		t.Errorf("expected level in response, got: %s", text)
	}
}

func TestHandleAutonomyLevel_SetByNumber(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 0)

	result, err := srv.handleAutonomyLevel(context.Background(), makeRequest(map[string]any{
		"level": "2",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"level":2`) {
		t.Errorf("expected level 2 after set, got: %s", text)
	}
}

func TestHandleAutonomyLevel_SetByName(t *testing.T) {
	t.Parallel()

	names := []struct {
		name  string
		level int
	}{
		{"observe", 0},
		{"auto-recover", 1},
		{"auto-optimize", 2},
		{"full-autonomy", 3},
	}
	for _, tt := range names {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv, _ := setupTestServer(t)
			stateDir := t.TempDir()
			srv.InitSelfImprovement(stateDir, 0)

			result, err := srv.handleAutonomyLevel(context.Background(), makeRequest(map[string]any{
				"level": tt.name,
			}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.IsError {
				t.Fatalf("unexpected error: %s", getResultText(result))
			}
		})
	}
}

func TestHandleAutonomyLevel_InvalidLevel(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 0)

	result, err := srv.handleAutonomyLevel(context.Background(), makeRequest(map[string]any{
		"level": "invalid-level",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid level")
	}
	assertErrorCode(t, "handleAutonomyLevel", result, "INVALID_PARAMS")
}

func TestHandleAutonomyDecisions_Initialized(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	result, err := srv.handleAutonomyDecisions(context.Background(), makeRequest(map[string]any{
		"limit": float64(5),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "decisions") {
		t.Errorf("expected decisions field, got: %s", text)
	}
}

func TestHandleAutonomyOverride_Initialized(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	result, err := srv.handleAutonomyOverride(context.Background(), makeRequest(map[string]any{
		"decision_id": "test-dec-1",
		"details":     "user override for testing",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "overridden") {
		t.Errorf("expected overridden status, got: %s", text)
	}
}

func TestHandleAutonomyOverride_MissingDecisionID(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	result, err := srv.handleAutonomyOverride(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing decision_id")
	}
	assertErrorCode(t, "handleAutonomyOverride", result, "INVALID_PARAMS")
}

func TestHandleAutonomyOverride_DefaultDetails(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	// No details provided, should use default
	result, err := srv.handleAutonomyOverride(context.Background(), makeRequest(map[string]any{
		"decision_id": "test-dec-2",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
}

// --- Feedback and provider recommend ---

func TestHandleFeedbackProfiles_Initialized(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	// All profiles
	result, err := srv.handleFeedbackProfiles(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "prompt_profiles") {
		t.Errorf("expected prompt_profiles, got: %s", text)
	}
	if !strings.Contains(text, "provider_profiles") {
		t.Errorf("expected provider_profiles, got: %s", text)
	}
}

func TestHandleFeedbackProfiles_PromptOnly(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	result, err := srv.handleFeedbackProfiles(context.Background(), makeRequest(map[string]any{
		"type": "prompt",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "prompt_profiles") {
		t.Errorf("expected prompt_profiles, got: %s", text)
	}
}

func TestHandleFeedbackProfiles_ProviderOnly(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	result, err := srv.handleFeedbackProfiles(context.Background(), makeRequest(map[string]any{
		"type": "provider",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "provider_profiles") {
		t.Errorf("expected provider_profiles, got: %s", text)
	}
}

func TestHandleProviderRecommend_Initialized(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	result, err := srv.handleProviderRecommend(context.Background(), makeRequest(map[string]any{
		"task": "implement a parser for JSON",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
}

func TestHandleProviderRecommend_MissingTask_Boost(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	stateDir := t.TempDir()
	srv.InitSelfImprovement(stateDir, 1)

	result, err := srv.handleProviderRecommend(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing task")
	}
	assertErrorCode(t, "handleProviderRecommend", result, "INVALID_PARAMS")
}

// --- Bandit status and confidence calibration ---

func TestHandleBanditStatus_NilSessMgr(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.SessMgr = nil

	result, err := srv.handleBanditStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nil session manager")
	}
}

func TestHandleBanditStatus_NoCascadeRouter_Boost(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Default SessMgr has no cascade router
	result, err := srv.handleBanditStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected not_configured status, got: %s", text)
	}
}

func TestHandleConfidenceCalibration_NilSessMgr(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.SessMgr = nil

	result, err := srv.handleConfidenceCalibration(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nil session manager")
	}
}

func TestHandleConfidenceCalibration_NoCascadeRouter_Boost(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleConfidenceCalibration(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected not_configured status, got: %s", text)
	}
}

// --- resolveRepoPath coverage ---

func TestResolveRepoPath_InvalidRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	_, err := srv.resolveRepoPath("../evil")
	if err == nil {
		t.Fatal("expected error for invalid repo name")
	}
	if !strings.Contains(err.Error(), "invalid repo name") {
		t.Errorf("expected 'invalid repo name' error, got: %v", err)
	}
}

func TestResolveRepoPath_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	_, err := srv.resolveRepoPath("nonexistent-repo")
	if err == nil {
		t.Fatal("expected error for nonexistent repo")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestResolveRepoPath_ValidRepo(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	// Scan to find repos
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	path, err := srv.resolveRepoPath("test-repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(root, "test-repo")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
}

func TestResolveRepoPath_EmptyUsesFirstRepo(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	// Scan to find repos
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	path, err := srv.resolveRepoPath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := filepath.Join(root, "test-repo")
	if path != expected {
		t.Errorf("path = %q, want %q", path, expected)
	}
}

func TestResolveRepoPath_EmptyNoRepos(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	srv := NewServer(root)

	path, err := srv.resolveRepoPath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Falls back to ScanPath
	if path != root {
		t.Errorf("path = %q, want %q", path, root)
	}
}

func TestResolveRepoPath_EmptyScanPathEmpty(t *testing.T) {
	t.Parallel()
	srv := &Server{ScanPath: ""}

	_, err := srv.resolveRepoPath("")
	if err == nil {
		t.Fatal("expected error for empty scan path with no repos")
	}
	if !strings.Contains(err.Error(), "no repo available") {
		t.Errorf("expected 'no repo available' error, got: %v", err)
	}
}

func TestResolveRepoPath_ScanFails(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	_, err := srv.resolveRepoPath("some-repo")
	if err == nil {
		t.Fatal("expected error when scan fails")
	}
	if !strings.Contains(err.Error(), "scan failed") {
		t.Errorf("expected 'scan failed' error, got: %v", err)
	}
}

// --- Session errors: additional paths ---

func TestHandleSessionErrors_LimitBelowOne(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusErrored
		s.Error = "something bad"
	})

	// limit < 1 should be clamped to 50
	result, err := srv.handleSessionErrors(context.Background(), makeRequest(map[string]any{
		"limit": float64(0),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "something bad") {
		t.Errorf("expected error message, got: %s", text)
	}
}

// --- Cost forecast (handler_fleet_h.go) ---

func TestHandleCostForecast_NilPredictor(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.CostPredictor = nil

	result, err := srv.handleCostForecast(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected not_configured status, got: %s", text)
	}
}

// --- A2A offers (handler_fleet_h.go) ---

func TestHandleA2AOffers_NilA2A_Boost(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.A2A = nil

	result, err := srv.handleA2AOffers(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "not_configured") {
		t.Errorf("expected not_configured status, got: %s", text)
	}
}

// --- Scratchpad: missing name ---

func TestHandleScratchpadRead_MissingName(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	result, err := srv.handleScratchpadRead(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}
}

func TestHandleScratchpadAppend_MissingName(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	result, err := srv.handleScratchpadAppend(context.Background(), makeRequest(map[string]any{
		"content": "some text",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}
}

func TestHandleScratchpadAppend_MissingContent(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	result, err := srv.handleScratchpadAppend(context.Background(), makeRequest(map[string]any{
		"name": "test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing content")
	}
}

func TestHandleScratchpadResolve_MissingParams(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	// Missing name
	result, err := srv.handleScratchpadResolve(context.Background(), makeRequest(map[string]any{
		"item_number": float64(1),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}

	// Missing item_number
	result, err = srv.handleScratchpadResolve(context.Background(), makeRequest(map[string]any{
		"name": "test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing item_number")
	}
}

func TestHandleScratchpadResolve_NonexistentScratchpad(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	result, err := srv.handleScratchpadResolve(context.Background(), makeRequest(map[string]any{
		"name":        "nonexistent",
		"item_number": float64(1),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return empty result for nonexistent scratchpad
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
}

func TestHandleScratchpadResolve_ItemNotFound(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.WriteFile(filepath.Join(ralphDir, "test_scratchpad.md"), []byte("1. Only item\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleScratchpadResolve(context.Background(), makeRequest(map[string]any{
		"name":        "test",
		"item_number": float64(99),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for item not found")
	}
}

func TestHandleScratchpadResolve_AlreadyResolved(t *testing.T) {
	t.Parallel()
	srv, root := scratchpadServer(t)
	ralphDir := filepath.Join(root, ".ralph")
	if err := os.WriteFile(filepath.Join(ralphDir, "test_scratchpad.md"), []byte("1. Already done -- RESOLVED\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleScratchpadResolve(context.Background(), makeRequest(map[string]any{
		"name":        "test",
		"item_number": float64(1),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "already resolved") {
		t.Errorf("expected already resolved message, got: %s", text)
	}
}

func TestHandleScratchpadDelete_NonexistentScratchpad(t *testing.T) {
	t.Parallel()
	srv, _ := scratchpadServer(t)

	result, err := srv.handleScratchpadDelete(context.Background(), makeRequest(map[string]any{
		"scratchpad": "nonexistent",
		"finding_id": "1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent scratchpad")
	}
}
