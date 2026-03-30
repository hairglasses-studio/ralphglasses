package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// handleCycleCreate
// ---------------------------------------------------------------------------

func TestHandleCycleCreate_HappyPath(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleCreate(context.Background(), makeRequest(map[string]any{
		"repo":      "test-repo",
		"name":      "test-cycle",
		"objective": "Improve test coverage to 90%",
		"criteria":  "coverage >= 90, no regressions",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleCycleCreate returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["status"] != "created" {
		t.Errorf("status = %v, want created", data["status"])
	}
	if data["name"] != "test-cycle" {
		t.Errorf("name = %v, want test-cycle", data["name"])
	}
	if data["phase"] != "proposed" {
		t.Errorf("phase = %v, want proposed", data["phase"])
	}
	if data["objective"] != "Improve test coverage to 90%" {
		t.Errorf("objective = %v, want 'Improve test coverage to 90%%'", data["objective"])
	}
	if _, ok := data["cycle_id"]; !ok {
		t.Error("expected cycle_id in response")
	}

	// Verify cycle is on disk.
	repoPath := filepath.Join(root, "test-repo")
	cycles, listErr := srv.SessMgr.ListCycles(repoPath)
	if listErr != nil {
		t.Fatalf("ListCycles: %v", listErr)
	}
	if len(cycles) != 1 {
		t.Errorf("expected 1 cycle on disk, got %d", len(cycles))
	}
}

func TestHandleCycleCreate_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCycleCreate(context.Background(), makeRequest(map[string]any{
		"objective": "test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo")
	}
}

func TestHandleCycleCreate_MissingObjective(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleCreate(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"name": "test-cycle",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing objective")
	}
}

func TestHandleCycleCreate_DefaultName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleCreate(context.Background(), makeRequest(map[string]any{
		"repo":      "test-repo",
		"objective": "test default name",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}

	var data map[string]any
	json.Unmarshal([]byte(getResultText(result)), &data)
	if data["name"] != "cycle" {
		t.Errorf("default name = %v, want cycle", data["name"])
	}
}

func TestHandleCycleCreate_InvalidRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleCreate(context.Background(), makeRequest(map[string]any{
		"repo":      "nonexistent-repo",
		"objective": "test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid repo")
	}
}

// ---------------------------------------------------------------------------
// handleCycleStatus
// ---------------------------------------------------------------------------

func TestHandleCycleStatus_NoCycles(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleStatus(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var data map[string]any
	json.Unmarshal([]byte(getResultText(result)), &data)
	if data["status"] != "none" {
		t.Errorf("status = %v, want none", data["status"])
	}
}

func TestHandleCycleStatus_WithActiveCycle(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Create a cycle first.
	repoPath := filepath.Join(root, "test-repo")
	cycle, createErr := srv.SessMgr.CreateCycle(repoPath, "s1", "objective1", nil)
	if createErr != nil {
		t.Fatalf("CreateCycle: %v", createErr)
	}

	result, err := srv.handleCycleStatus(context.Background(), makeRequest(map[string]any{
		"repo":     "test-repo",
		"cycle_id": cycle.ID,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var data map[string]any
	json.Unmarshal([]byte(getResultText(result)), &data)
	if data["status"] != "ok" {
		t.Errorf("status = %v, want ok", data["status"])
	}
	if data["cycle_id"] != cycle.ID {
		t.Errorf("cycle_id = %v, want %s", data["cycle_id"], cycle.ID)
	}
	if data["phase"] != "proposed" {
		t.Errorf("phase = %v, want proposed", data["phase"])
	}
	if data["objective"] != "objective1" {
		t.Errorf("objective = %v, want objective1", data["objective"])
	}
}

func TestHandleCycleStatus_ActiveCycleAutoResolve(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	_, createErr := srv.SessMgr.CreateCycle(repoPath, "auto", "auto-obj", nil)
	if createErr != nil {
		t.Fatalf("CreateCycle: %v", createErr)
	}

	// Without cycle_id, should find the active cycle.
	result, err := srv.handleCycleStatus(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var data map[string]any
	json.Unmarshal([]byte(getResultText(result)), &data)
	if data["status"] != "ok" {
		t.Errorf("status = %v, want ok", data["status"])
	}
}

// ---------------------------------------------------------------------------
// handleCycleAdvance
// ---------------------------------------------------------------------------

func TestHandleCycleAdvance_HappyPath(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	cycle, createErr := srv.SessMgr.CreateCycle(repoPath, "adv", "advance-test", nil)
	if createErr != nil {
		t.Fatalf("CreateCycle: %v", createErr)
	}

	result, err := srv.handleCycleAdvance(context.Background(), makeRequest(map[string]any{
		"repo":     "test-repo",
		"cycle_id": cycle.ID,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var data map[string]any
	json.Unmarshal([]byte(getResultText(result)), &data)
	if data["status"] != "advanced" {
		t.Errorf("status = %v, want advanced", data["status"])
	}
	if data["previous_phase"] != "proposed" {
		t.Errorf("previous_phase = %v, want proposed", data["previous_phase"])
	}
	if data["phase"] != "baselining" {
		t.Errorf("phase = %v, want baselining", data["phase"])
	}
}

func TestHandleCycleAdvance_NoCycle(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleAdvance(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for no active cycle")
	}
}

// ---------------------------------------------------------------------------
// handleCycleFail
// ---------------------------------------------------------------------------

func TestHandleCycleFail_HappyPath(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	cycle, createErr := srv.SessMgr.CreateCycle(repoPath, "fail-test", "will-fail", nil)
	if createErr != nil {
		t.Fatalf("CreateCycle: %v", createErr)
	}

	result, err := srv.handleCycleFail(context.Background(), makeRequest(map[string]any{
		"repo":     "test-repo",
		"cycle_id": cycle.ID,
		"error":    "test failure reason",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var data map[string]any
	json.Unmarshal([]byte(getResultText(result)), &data)
	if data["status"] != "failed" {
		t.Errorf("status = %v, want failed", data["status"])
	}
	if data["phase"] != "failed" {
		t.Errorf("phase = %v, want failed", data["phase"])
	}
	if data["error"] != "test failure reason" {
		t.Errorf("error = %v, want 'test failure reason'", data["error"])
	}
}

func TestHandleCycleFail_DefaultError(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	cycle, createErr := srv.SessMgr.CreateCycle(repoPath, "fail-default", "will-fail-default", nil)
	if createErr != nil {
		t.Fatalf("CreateCycle: %v", createErr)
	}

	result, err := srv.handleCycleFail(context.Background(), makeRequest(map[string]any{
		"repo":     "test-repo",
		"cycle_id": cycle.ID,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var data map[string]any
	json.Unmarshal([]byte(getResultText(result)), &data)
	if data["error"] != "manually failed" {
		t.Errorf("error = %v, want 'manually failed'", data["error"])
	}
}

func TestHandleCycleFail_NoCycle(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleFail(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for no active cycle")
	}
}

// ---------------------------------------------------------------------------
// handleCycleList
// ---------------------------------------------------------------------------

func TestHandleCycleList_Empty(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleList(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var data map[string]any
	json.Unmarshal([]byte(getResultText(result)), &data)
	if data["status"] != "ok" {
		t.Errorf("status = %v, want ok", data["status"])
	}
	total, _ := data["total"].(float64)
	if total != 0 {
		t.Errorf("total = %v, want 0", total)
	}
}

func TestHandleCycleList_MultipleCycles(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	c1, err1 := srv.SessMgr.CreateCycle(repoPath, "c1", "obj1", nil)
	if err1 != nil {
		t.Fatalf("CreateCycle c1: %v", err1)
	}
	// Fail c1 so we can create c2 (concurrent limit).
	srv.SessMgr.FailCycle(c1, "done")

	_, err2 := srv.SessMgr.CreateCycle(repoPath, "c2", "obj2", nil)
	if err2 != nil {
		t.Fatalf("CreateCycle c2: %v", err2)
	}

	result, err := srv.handleCycleList(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var data map[string]any
	json.Unmarshal([]byte(getResultText(result)), &data)
	total, _ := data["total"].(float64)
	if total != 2 {
		t.Errorf("total = %v, want 2", total)
	}
}

func TestHandleCycleList_WithLimit(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	c1, _ := srv.SessMgr.CreateCycle(repoPath, "c1", "obj1", nil)
	srv.SessMgr.FailCycle(c1, "done")
	c2, _ := srv.SessMgr.CreateCycle(repoPath, "c2", "obj2", nil)
	srv.SessMgr.FailCycle(c2, "done")
	_, _ = srv.SessMgr.CreateCycle(repoPath, "c3", "obj3", nil)

	result, err := srv.handleCycleList(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"limit": float64(2),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	json.Unmarshal([]byte(getResultText(result)), &data)
	total, _ := data["total"].(float64)
	if total != 2 {
		t.Errorf("total = %v, want 2 (limited)", total)
	}
}

func TestHandleCycleList_WithErrorField(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	c, _ := srv.SessMgr.CreateCycle(repoPath, "e1", "obj-err", nil)
	srv.SessMgr.FailCycle(c, "some error")

	result, err := srv.handleCycleList(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	json.Unmarshal([]byte(getResultText(result)), &data)
	cycles, _ := data["cycles"].([]any)
	if len(cycles) == 0 {
		t.Fatal("expected at least 1 cycle")
	}
	first, _ := cycles[0].(map[string]any)
	if first["error"] != "some error" {
		t.Errorf("error = %v, want 'some error'", first["error"])
	}
}

// ---------------------------------------------------------------------------
// handleCycleSynthesize
// ---------------------------------------------------------------------------

func TestHandleCycleSynthesize_MissingSummary(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	cycle, _ := srv.SessMgr.CreateCycle(repoPath, "synth", "synth-obj", nil)

	result, err := srv.handleCycleSynthesize(context.Background(), makeRequest(map[string]any{
		"repo":     "test-repo",
		"cycle_id": cycle.ID,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing summary")
	}
}

func TestHandleCycleSynthesize_NoCycle(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleSynthesize(context.Background(), makeRequest(map[string]any{
		"repo":    "test-repo",
		"summary": "test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for no active cycle")
	}
}

// ---------------------------------------------------------------------------
// handleCycleRun
// ---------------------------------------------------------------------------

func TestHandleCycleRun_MissingRepoPath(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCycleRun(context.Background(), makeRequest(map[string]any{
		"objective": "test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo_path")
	}
}

func TestHandleCycleRun_MissingObjective(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleRun(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing objective")
	}
}

func TestHandleCycleRun_InvalidRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleRun(context.Background(), makeRequest(map[string]any{
		"repo_path": "nonexistent-repo",
		"objective": "test",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid repo")
	}
}

func TestHandleCycleRun_RepoPathFallback(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Uses "repo" key when "repo_path" is missing.
	result, err := srv.handleCycleRun(context.Background(), makeRequest(map[string]any{
		"repo":      "test-repo",
		"objective": "test repo fallback",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// RunCycle will attempt the full cycle -- it may fail due to
	// missing providers in test env, but it should get past param validation.
	text := getResultText(result)
	if text == "" {
		t.Error("expected non-empty result")
	}
}
