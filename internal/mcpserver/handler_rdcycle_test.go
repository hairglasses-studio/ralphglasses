package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// handleFindingToTask
// ---------------------------------------------------------------------------

func TestHandleFindingToTask_HappyPath(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	scratchpad := `### FINDING-1
Severity: HIGH
Description: test finding with enough words to push it past the low threshold

### FINDING-2
Severity: LOW
Description: minor issue
`
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", "tool_improvement_scratchpad.md"), []byte(scratchpad), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleFindingToTask(context.Background(), makeRequest(map[string]any{
		"finding_id":     "FINDING-1",
		"scratchpad_name": "tool_improvement",
		"repo":           "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleFindingToTask returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["finding_id"] != "FINDING-1" {
		t.Errorf("finding_id = %v, want FINDING-1", data["finding_id"])
	}
	if data["status"] != "ready" {
		t.Errorf("status = %v, want ready", data["status"])
	}
	if _, ok := data["difficulty_score"]; !ok {
		t.Error("expected difficulty_score in response")
	}
	if _, ok := data["provider_hint"]; !ok {
		t.Error("expected provider_hint in response")
	}
	if _, ok := data["estimated_cost"]; !ok {
		t.Error("expected estimated_cost in response")
	}
}

func TestHandleFindingToTask_MissingFindingID(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleFindingToTask(context.Background(), makeRequest(map[string]any{
		"scratchpad_name": "tool_improvement",
		"repo":           "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing finding_id")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Errorf("expected INVALID_PARAMS, got: %s", text)
	}
}

func TestHandleFindingToTask_MissingScratchpadName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleFindingToTask(context.Background(), makeRequest(map[string]any{
		"finding_id": "FINDING-1",
		"repo":       "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing scratchpad_name")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Errorf("expected INVALID_PARAMS, got: %s", text)
	}
}

func TestHandleFindingToTask_FindingNotFound(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	scratchpad := `### FINDING-1
Severity: HIGH
Description: test finding
`
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", "tool_improvement_scratchpad.md"), []byte(scratchpad), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleFindingToTask(context.Background(), makeRequest(map[string]any{
		"finding_id":     "FINDING-999",
		"scratchpad_name": "tool_improvement",
		"repo":           "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent finding")
	}
	text := getResultText(result)
	if !strings.Contains(text, "not found") {
		t.Errorf("expected 'not found' in error, got: %s", text)
	}
}

func TestHandleFindingToTask_ScratchpadNotExist(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleFindingToTask(context.Background(), makeRequest(map[string]any{
		"finding_id":     "FINDING-1",
		"scratchpad_name": "nonexistent",
		"repo":           "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing scratchpad file")
	}
	text := getResultText(result)
	if !strings.Contains(text, "cannot read scratchpad") {
		t.Errorf("expected 'cannot read scratchpad' in error, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// handleCycleBaseline
// ---------------------------------------------------------------------------

func TestHandleCycleBaseline_HappyPath(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Write a simple Go file so go build/test have something to work with.
	repoPath := filepath.Join(root, "test-repo")
	if err := os.WriteFile(filepath.Join(repoPath, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleCycleBaseline(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleCycleBaseline returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["status"] != "captured" {
		t.Errorf("status = %v, want captured", data["status"])
	}
	if data["baseline_id"] == nil || data["baseline_id"] == "" {
		t.Error("expected baseline_id in response")
	}
	if _, ok := data["metrics"]; !ok {
		t.Error("expected metrics in response")
	}
	if _, ok := data["path"]; !ok {
		t.Error("expected path in response")
	}

	// Verify file was written.
	pathStr, _ := data["path"].(string)
	if _, err := os.Stat(pathStr); err != nil {
		t.Errorf("baseline file not found at %s: %v", pathStr, err)
	}
}

func TestHandleCycleBaseline_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCycleBaseline(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Errorf("expected INVALID_PARAMS, got: %s", text)
	}
}

func TestHandleCycleBaseline_CustomMetrics(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	if err := os.WriteFile(filepath.Join(repoPath, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleCycleBaseline(context.Background(), makeRequest(map[string]any{
		"repo":    "test-repo",
		"metrics": "build_ok,vet_clean",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleCycleBaseline returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	metrics, ok := data["metrics"].(map[string]any)
	if !ok {
		t.Fatal("expected metrics to be a map")
	}
	if _, ok := metrics["build_ok"]; !ok {
		t.Error("expected build_ok in metrics")
	}
	if _, ok := metrics["vet_clean"]; !ok {
		t.Error("expected vet_clean in metrics")
	}
}

// ---------------------------------------------------------------------------
// handleCyclePlan
// ---------------------------------------------------------------------------

func TestHandleCyclePlan_HappyPath(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	// Write scratchpad with items.
	scratchpad := `# Tool Improvement Scratchpad
- Fix broken test helper
- Add missing coverage for parser
- Refactor config loader
`
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", "tool_improvement_scratchpad.md"), []byte(scratchpad), 0644); err != nil {
		t.Fatal(err)
	}

	// Write improvement patterns.
	patterns := []map[string]any{
		{"pattern": "test", "frequency": 5, "recurrence": 3},
		{"pattern": "config", "frequency": 2, "recurrence": 1},
	}
	pdata, _ := json.Marshal(patterns)
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", "improvement_patterns.json"), pdata, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleCyclePlan(context.Background(), makeRequest(map[string]any{
		"repo":      "test-repo",
		"max_tasks": float64(5),
		"budget":    float64(2.0),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleCyclePlan returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["status"] != "planned" {
		t.Errorf("status = %v, want planned", data["status"])
	}
	if _, ok := data["plan_id"]; !ok {
		t.Error("expected plan_id in response")
	}
	tasks, ok := data["tasks"].([]any)
	if !ok {
		t.Fatal("expected tasks to be an array")
	}
	if len(tasks) == 0 {
		t.Error("expected at least one task in plan")
	}
}

func TestHandleCyclePlan_NoScratchpads(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCyclePlan(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleCyclePlan returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	tasks, _ := data["tasks"].([]any)
	if tasks == nil {
		// nil tasks is acceptable when no scratchpads
		return
	}
	// Empty array is also fine
}

func TestHandleCyclePlan_DefaultParams(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCyclePlan(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleCyclePlan returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	constraints, ok := data["constraints"].(map[string]any)
	if !ok {
		t.Fatal("expected constraints in response")
	}
	if constraints["max_tasks"] != float64(10) {
		t.Errorf("default max_tasks = %v, want 10", constraints["max_tasks"])
	}
	if constraints["budget_usd"] != float64(5) {
		t.Errorf("default budget_usd = %v, want 5", constraints["budget_usd"])
	}
}

// ---------------------------------------------------------------------------
// handleCycleMerge
// ---------------------------------------------------------------------------

func TestHandleCycleMerge_HappyPath(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Create two temp "worktree" directories with git repos and changed files.
	wt1 := t.TempDir()
	wt2 := t.TempDir()

	// Write initial files so initGitRepo has something to commit.
	if err := os.WriteFile(filepath.Join(wt1, "file_a.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt2, "file_b.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Init git repos in both worktrees.
	initGitRepo(t, wt1)
	initGitRepo(t, wt2)

	// Modify files after initial commit so git diff HEAD shows changes.
	if err := os.WriteFile(filepath.Join(wt1, "file_a.go"), []byte("package main\n// changed\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt2, "file_b.go"), []byte("package main\n// changed\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleCycleMerge(context.Background(), makeRequest(map[string]any{
		"worktree_paths": wt1 + "," + wt2,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleCycleMerge returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["status"] != "completed" {
		t.Errorf("status = %v, want completed", data["status"])
	}
	if data["worktree_count"] != float64(2) {
		t.Errorf("worktree_count = %v, want 2", data["worktree_count"])
	}
}

func TestHandleCycleMerge_MissingWorktreePaths(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCycleMerge(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing worktree_paths")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Errorf("expected INVALID_PARAMS, got: %s", text)
	}
}

func TestHandleCycleMerge_NonexistentPath(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCycleMerge(context.Background(), makeRequest(map[string]any{
		"worktree_paths": "/nonexistent/path/12345",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent worktree path")
	}
	text := getResultText(result)
	if !strings.Contains(text, "does not exist") {
		t.Errorf("expected 'does not exist' in error, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// handleCycleSchedule
// ---------------------------------------------------------------------------

func TestHandleCycleSchedule_HappyPath(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleSchedule(context.Background(), makeRequest(map[string]any{
		"cron_expr": "0 */6 * * *",
		"repo":      "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleCycleSchedule returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["status"] != "created" {
		t.Errorf("status = %v, want created", data["status"])
	}
	if data["cron_expr"] != "0 */6 * * *" {
		t.Errorf("cron_expr = %v, want '0 */6 * * *'", data["cron_expr"])
	}
	if _, ok := data["schedule_id"]; !ok {
		t.Error("expected schedule_id in response")
	}
	nextRuns, ok := data["next_runs"].([]any)
	if !ok || len(nextRuns) == 0 {
		t.Error("expected next_runs array with entries")
	}

	// Verify file was written.
	pathStr, _ := data["path"].(string)
	if pathStr != "" {
		if _, err := os.Stat(pathStr); err != nil {
			t.Errorf("schedule file not found at %s: %v", pathStr, err)
		}
	}
}

func TestHandleCycleSchedule_MissingCronExpr(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleCycleSchedule(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing cron_expr")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Errorf("expected INVALID_PARAMS, got: %s", text)
	}
}

func TestHandleCycleSchedule_InvalidCronExpr(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleSchedule(context.Background(), makeRequest(map[string]any{
		"cron_expr": "invalid",
		"repo":      "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid cron expression")
	}
	text := getResultText(result)
	if !strings.Contains(text, "5 fields") {
		t.Errorf("expected '5 fields' in error, got: %s", text)
	}
}

func TestHandleCycleSchedule_InvalidCronField(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleCycleSchedule(context.Background(), makeRequest(map[string]any{
		"cron_expr": "0 abc * * *",
		"repo":      "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid cron field")
	}
	text := getResultText(result)
	if !strings.Contains(text, "invalid cron field") {
		t.Errorf("expected 'invalid cron field' in error, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// handleLoopReplay
// ---------------------------------------------------------------------------

func TestHandleLoopReplay_HappyPath(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	loopRunsDir := filepath.Join(repoPath, ".ralph", "loop_runs")
	if err := os.MkdirAll(loopRunsDir, 0755); err != nil {
		t.Fatal(err)
	}

	loopRun := map[string]any{
		"id":       "test-loop-1",
		"model":    "sonnet",
		"provider": "claude",
		"budget":   5.0,
		"prompt":   "improve tests",
		"iterations": []any{
			map[string]any{
				"index": 0,
				"config": map[string]any{
					"model": "sonnet",
				},
				"error": "timeout after 30s",
			},
			map[string]any{
				"index": 1,
				"config": map[string]any{
					"model": "opus",
				},
			},
		},
	}
	data, _ := json.Marshal(loopRun)
	if err := os.WriteFile(filepath.Join(loopRunsDir, "test-loop-1.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleLoopReplay(context.Background(), makeRequest(map[string]any{
		"loop_id":   "test-loop-1",
		"iteration": float64(0),
		"repo":      "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleLoopReplay returned error: %s", getResultText(result))
	}

	var respData map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &respData); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if respData["status"] != "ready_to_replay" {
		t.Errorf("status = %v, want ready_to_replay", respData["status"])
	}
	if respData["loop_id"] != "test-loop-1" {
		t.Errorf("loop_id = %v, want test-loop-1", respData["loop_id"])
	}
	if respData["original_error"] != "timeout after 30s" {
		t.Errorf("original_error = %v, want 'timeout after 30s'", respData["original_error"])
	}
	newConfig, ok := respData["new_config"].(map[string]any)
	if !ok {
		t.Fatal("expected new_config map")
	}
	if newConfig["model"] != "sonnet" {
		t.Errorf("new_config.model = %v, want sonnet", newConfig["model"])
	}
}

func TestHandleLoopReplay_MissingLoopID(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopReplay(context.Background(), makeRequest(map[string]any{
		"iteration": float64(0),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing loop_id")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Errorf("expected INVALID_PARAMS, got: %s", text)
	}
}

func TestHandleLoopReplay_MissingIteration(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleLoopReplay(context.Background(), makeRequest(map[string]any{
		"loop_id": "test-loop-1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing iteration")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Errorf("expected INVALID_PARAMS, got: %s", text)
	}
}

func TestHandleLoopReplay_IterationOutOfRange(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	loopRunsDir := filepath.Join(repoPath, ".ralph", "loop_runs")
	if err := os.MkdirAll(loopRunsDir, 0755); err != nil {
		t.Fatal(err)
	}

	loopRun := map[string]any{
		"id": "test-loop-2",
		"iterations": []any{
			map[string]any{"index": 0},
		},
	}
	data, _ := json.Marshal(loopRun)
	if err := os.WriteFile(filepath.Join(loopRunsDir, "test-loop-2.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleLoopReplay(context.Background(), makeRequest(map[string]any{
		"loop_id":   "test-loop-2",
		"iteration": float64(5),
		"repo":      "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for out-of-range iteration")
	}
	text := getResultText(result)
	if !strings.Contains(text, "out of range") {
		t.Errorf("expected 'out of range' in error, got: %s", text)
	}
}

func TestHandleLoopReplay_WithOverrides(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	loopRunsDir := filepath.Join(repoPath, ".ralph", "loop_runs")
	if err := os.MkdirAll(loopRunsDir, 0755); err != nil {
		t.Fatal(err)
	}

	loopRun := map[string]any{
		"id": "test-loop-3",
		"iterations": []any{
			map[string]any{
				"index":  0,
				"config": map[string]any{"model": "sonnet"},
			},
		},
	}
	data, _ := json.Marshal(loopRun)
	if err := os.WriteFile(filepath.Join(loopRunsDir, "test-loop-3.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	overrides, _ := json.Marshal(map[string]any{"model": "opus", "budget": 10})
	result, err := srv.handleLoopReplay(context.Background(), makeRequest(map[string]any{
		"loop_id":   "test-loop-3",
		"iteration": float64(0),
		"repo":      "test-repo",
		"overrides": string(overrides),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleLoopReplay returned error: %s", getResultText(result))
	}

	var respData map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &respData); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	newConfig, ok := respData["new_config"].(map[string]any)
	if !ok {
		t.Fatal("expected new_config map")
	}
	if newConfig["model"] != "opus" {
		t.Errorf("overridden model = %v, want opus", newConfig["model"])
	}
}

// ---------------------------------------------------------------------------
// handleBudgetForecast
// ---------------------------------------------------------------------------

func TestHandleBudgetForecast_HappyPath(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	costObs := []map[string]any{
		{"loop_id": "loop-a", "cost": 0.10},
		{"loop_id": "loop-a", "cost": 0.15},
		{"loop_id": "loop-a", "cost": 0.12},
		{"loop_id": "loop-b", "cost": 0.50}, // different loop, should be ignored
	}
	data, _ := json.Marshal(costObs)
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", "cost_observations.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleBudgetForecast(context.Background(), makeRequest(map[string]any{
		"loop_id":    "loop-a",
		"iterations": float64(10),
		"repo":       "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleBudgetForecast returned error: %s", getResultText(result))
	}

	var respData map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &respData); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if respData["loop_id"] != "loop-a" {
		t.Errorf("loop_id = %v, want loop-a", respData["loop_id"])
	}
	if respData["iterations_analyzed"] != float64(3) {
		t.Errorf("iterations_analyzed = %v, want 3", respData["iterations_analyzed"])
	}
	if respData["iterations_requested"] != float64(10) {
		t.Errorf("iterations_requested = %v, want 10", respData["iterations_requested"])
	}
	if _, ok := respData["estimated_cost_p50"]; !ok {
		t.Error("expected estimated_cost_p50")
	}
	if _, ok := respData["estimated_cost_p95"]; !ok {
		t.Error("expected estimated_cost_p95")
	}
	if _, ok := respData["confidence_pct"]; !ok {
		t.Error("expected confidence_pct")
	}
}

func TestHandleBudgetForecast_MissingLoopID(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleBudgetForecast(context.Background(), makeRequest(map[string]any{
		"iterations": float64(10),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing loop_id")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Errorf("expected INVALID_PARAMS, got: %s", text)
	}
}

func TestHandleBudgetForecast_NoMatchingObservations(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	costObs := []map[string]any{
		{"loop_id": "other-loop", "cost": 0.10},
	}
	data, _ := json.Marshal(costObs)
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", "cost_observations.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleBudgetForecast(context.Background(), makeRequest(map[string]any{
		"loop_id":    "nonexistent-loop",
		"iterations": float64(10),
		"repo":       "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for no matching observations")
	}
	text := getResultText(result)
	if !strings.Contains(text, "no cost observations") {
		t.Errorf("expected 'no cost observations' in error, got: %s", text)
	}
}

func TestHandleBudgetForecast_CostUSDField(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	costObs := []map[string]any{
		{"loop_id": "loop-c", "cost_usd": 0.20},
		{"loop_id": "loop-c", "cost_usd": 0.30},
	}
	data, _ := json.Marshal(costObs)
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", "cost_observations.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleBudgetForecast(context.Background(), makeRequest(map[string]any{
		"loop_id":    "loop-c",
		"iterations": float64(5),
		"repo":       "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleBudgetForecast returned error: %s", getResultText(result))
	}

	var respData map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &respData); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if respData["iterations_analyzed"] != float64(2) {
		t.Errorf("iterations_analyzed = %v, want 2", respData["iterations_analyzed"])
	}
}

// ---------------------------------------------------------------------------
// handleDiffReview
// ---------------------------------------------------------------------------

func TestHandleDiffReview_HappyPath(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")

	// Add a Go file and commit it, then modify it for a diff.
	if err := os.WriteFile(filepath.Join(repoPath, "handler.go"), []byte("package main\n\nfunc handler() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", "handler.go")
	runGit(t, repoPath, "commit", "-m", "add handler")

	// Modify file and commit again to create a diff.
	if err := os.WriteFile(filepath.Join(repoPath, "handler.go"), []byte("package main\n\n// TODO: fix this\nfunc handler() {\n\tprintln(\"hello\")\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", "handler.go")
	runGit(t, repoPath, "commit", "-m", "modify handler")

	result, err := srv.handleDiffReview(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"ref":  "HEAD",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleDiffReview returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["status"] != "reviewed" {
		t.Errorf("status = %v, want reviewed", data["status"])
	}
	filesReviewed, _ := data["files_reviewed"].(float64)
	if filesReviewed < 1 {
		t.Errorf("files_reviewed = %v, want >= 1", filesReviewed)
	}

	// Should have a TODO issue and a missing_tests warning.
	issues, ok := data["issues"].([]any)
	if !ok {
		t.Fatal("expected issues array")
	}

	var foundTodo bool
	for _, iss := range issues {
		issMap, ok := iss.(map[string]any)
		if !ok {
			continue
		}
		if issMap["check"] == "todos" {
			foundTodo = true
		}
	}
	if !foundTodo {
		t.Error("expected a todos issue for the TODO comment")
	}
}

func TestHandleDiffReview_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleDiffReview(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Errorf("expected INVALID_PARAMS, got: %s", text)
	}
}

func TestHandleDiffReview_CleanDiff(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")

	// Create two identical commits so HEAD~1..HEAD has no diff.
	if err := os.WriteFile(filepath.Join(repoPath, "nodiff.txt"), []byte("same\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", "nodiff.txt")
	runGit(t, repoPath, "commit", "-m", "add nodiff")

	// Commit an empty change (amend with same content = no diff possible,
	// so instead commit a new empty file).
	if err := os.WriteFile(filepath.Join(repoPath, "empty.txt"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", "empty.txt")
	runGit(t, repoPath, "commit", "-m", "add empty")

	result, err := srv.handleDiffReview(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"ref":  "HEAD",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleDiffReview returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Should have reviewed at least the new empty.txt file.
	if data["status"] != "reviewed" && data["status"] != "clean" {
		t.Errorf("status = %v, want reviewed or clean", data["status"])
	}
}

func TestHandleDiffReview_MissingTestsWarning(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")

	// Add a .go file (not test) and commit.
	if err := os.WriteFile(filepath.Join(repoPath, "logic.go"), []byte("package main\nfunc logic() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", "logic.go")
	runGit(t, repoPath, "commit", "-m", "add logic")

	// Modify without adding test.
	if err := os.WriteFile(filepath.Join(repoPath, "logic.go"), []byte("package main\nfunc logic() { println(\"v2\") }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", "logic.go")
	runGit(t, repoPath, "commit", "-m", "update logic")

	result, err := srv.handleDiffReview(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
		"ref":  "HEAD",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleDiffReview returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	issues, ok := data["issues"].([]any)
	if !ok {
		t.Fatal("expected issues array")
	}

	var foundMissingTests bool
	for _, iss := range issues {
		issMap, _ := iss.(map[string]any)
		if issMap["check"] == "missing_tests" {
			foundMissingTests = true
		}
	}
	if !foundMissingTests {
		t.Error("expected missing_tests warning when .go file changed without _test.go")
	}
}

// ---------------------------------------------------------------------------
// handleFindingReason
// ---------------------------------------------------------------------------

func TestHandleFindingReason_HappyPath(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	// Write scratchpad with multiple findings that map to same category.
	scratchpad := `# Findings

## FINDING-1 Test assertion failure
The test assertion fails due to missing mock setup.

## FINDING-2 Test coverage gap
Coverage is below threshold for the parser module.

## FINDING-3 Test timeout
Test times out due to slow network mock.

## FINDING-4 Error handling bug
Panic on nil pointer in config loader.
`
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", "analysis_scratchpad.md"), []byte(scratchpad), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleFindingReason(context.Background(), makeRequest(map[string]any{
		"name": "analysis",
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleFindingReason returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["status"] != "analyzed" {
		t.Errorf("status = %v, want analyzed", data["status"])
	}
	totalFindings, _ := data["total_findings"].(float64)
	if totalFindings < 1 {
		t.Errorf("total_findings = %v, want >= 1", totalFindings)
	}
	if data["scratchpad"] != "analysis" {
		t.Errorf("scratchpad = %v, want analysis", data["scratchpad"])
	}
}

func TestHandleFindingReason_MissingName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleFindingReason(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing name")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Errorf("expected INVALID_PARAMS, got: %s", text)
	}
}

func TestHandleFindingReason_ScratchpadNotExist(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleFindingReason(context.Background(), makeRequest(map[string]any{
		"name": "nonexistent",
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent scratchpad")
	}
	text := getResultText(result)
	if !strings.Contains(text, "cannot read scratchpad") {
		t.Errorf("expected 'cannot read scratchpad' in error, got: %s", text)
	}
}

func TestHandleFindingReason_RootCauseDetection(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	// 3+ test-related findings should trigger a root cause.
	scratchpad := `# Findings

## FINDING-1 Test failure alpha
The test for alpha module fails.

## FINDING-2 Test failure beta
The test for beta module fails.

## FINDING-3 Test failure gamma
The test for gamma module assertion is wrong.
`
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", "qa_scratchpad.md"), []byte(scratchpad), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleFindingReason(context.Background(), makeRequest(map[string]any{
		"name": "qa",
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleFindingReason returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	rootCauses, ok := data["root_causes"].([]any)
	if !ok || len(rootCauses) == 0 {
		t.Error("expected at least one root cause for 3+ test-related findings")
	}
}

// ---------------------------------------------------------------------------
// handleObservationCorrelate
// ---------------------------------------------------------------------------

func TestHandleObservationCorrelate_HappyPath(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	logsDir := filepath.Join(repoPath, ".ralph", "logs")

	// Write observations with recent timestamps.
	now := time.Now().UTC()
	var lines []string
	for i := 0; i < 3; i++ {
		obs := map[string]any{
			"id":         fmt.Sprintf("obs-%d", i),
			"timestamp":  now.Add(-time.Duration(i) * time.Minute).Format(time.RFC3339),
			"session_id": fmt.Sprintf("sess-%d", i),
			"loop_id":    "loop-x",
			"status":     "completed",
		}
		data, _ := json.Marshal(obs)
		lines = append(lines, string(data))
	}
	if err := os.WriteFile(filepath.Join(logsDir, "loop_observations.jsonl"), []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleObservationCorrelate(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"hours": float64(24),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleObservationCorrelate returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if data["status"] != "correlated" {
		t.Errorf("status = %v, want correlated", data["status"])
	}
	totalObs, _ := data["total_observations"].(float64)
	if totalObs != 3 {
		t.Errorf("total_observations = %v, want 3", totalObs)
	}
	if _, ok := data["correlations"]; !ok {
		t.Error("expected correlations in response")
	}
}

func TestHandleObservationCorrelate_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleObservationCorrelate(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, string(ErrInvalidParams)) {
		t.Errorf("expected INVALID_PARAMS, got: %s", text)
	}
}

func TestHandleObservationCorrelate_NoObservations(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleObservationCorrelate(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"hours": float64(1),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleObservationCorrelate returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	totalObs, _ := data["total_observations"].(float64)
	if totalObs != 0 {
		t.Errorf("total_observations = %v, want 0", totalObs)
	}
}

func TestHandleObservationCorrelate_WithCorrelations(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	repoPath := filepath.Join(root, "test-repo")
	logsDir := filepath.Join(repoPath, ".ralph", "logs")

	// Make a commit with a known timestamp, then create an observation close to it.
	if err := os.WriteFile(filepath.Join(repoPath, "corr.txt"), []byte("correlation test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", "corr.txt")
	runGit(t, repoPath, "commit", "-m", "correlation test commit")

	// Write observation timestamped now (close to the commit).
	now := time.Now().UTC()
	obs := map[string]any{
		"id":         "obs-corr",
		"timestamp":  now.Format(time.RFC3339),
		"session_id": "sess-corr",
		"status":     "completed",
	}
	obsData, _ := json.Marshal(obs)
	if err := os.WriteFile(filepath.Join(logsDir, "loop_observations.jsonl"), obsData, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := srv.handleObservationCorrelate(context.Background(), makeRequest(map[string]any{
		"repo":  "test-repo",
		"hours": float64(1),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleObservationCorrelate returned error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	correlations, ok := data["correlations"].([]any)
	if !ok {
		t.Fatal("expected correlations array")
	}
	if len(correlations) == 0 {
		t.Error("expected at least one correlation between commit and observation")
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestSliceContains(t *testing.T) {
	t.Parallel()
	if !sliceContains([]string{"a", "b", "c"}, "b") {
		t.Error("expected true for existing element")
	}
	if sliceContains([]string{"a", "b", "c"}, "d") {
		t.Error("expected false for missing element")
	}
	if sliceContains(nil, "a") {
		t.Error("expected false for nil slice")
	}
}

func TestTruncate(t *testing.T) {
	t.Parallel()
	if truncate("short", 100) != "short" {
		t.Error("should not truncate short strings")
	}
	result := truncate("this is a long string", 10)
	if !strings.HasSuffix(result, "...[truncated]") {
		t.Errorf("expected truncated suffix, got: %s", result)
	}
}

func TestPercentileFloat(t *testing.T) {
	t.Parallel()
	if percentileFloat(nil, 50) != 0 {
		t.Error("nil slice should return 0")
	}
	if percentileFloat([]float64{5.0}, 50) != 5.0 {
		t.Error("single element should return that element")
	}
	sorted := []float64{1, 2, 3, 4, 5}
	p50 := percentileFloat(sorted, 50)
	if p50 != 3.0 {
		t.Errorf("p50 of [1,2,3,4,5] = %v, want 3.0", p50)
	}
}

func TestCronFieldMatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		field string
		value int
		want  bool
	}{
		{"*", 5, true},
		{"*/6", 0, true},
		{"*/6", 6, true},
		{"*/6", 3, false},
		{"5", 5, true},
		{"5", 3, false},
	}
	for _, tt := range tests {
		got := cronFieldMatch(tt.field, tt.value)
		if got != tt.want {
			t.Errorf("cronFieldMatch(%q, %d) = %v, want %v", tt.field, tt.value, got, tt.want)
		}
	}
}

func TestComputeNextCronRuns(t *testing.T) {
	t.Parallel()
	fields := []string{"0", "*/6", "*", "*", "*"}
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	runs := computeNextCronRuns(fields, from, 3)
	if len(runs) != 3 {
		t.Fatalf("expected 3 runs, got %d", len(runs))
	}
	// First run should be at 06:00 since from=00:00 and */6 matches 0,6,12,18.
	// Actually from is truncated to minute and +1 minute, so 00:01.
	// Next match for minute=0 and hour=*/6: 06:00.
	if runs[0].Hour() != 6 || runs[0].Minute() != 0 {
		t.Errorf("first run = %v, expected 06:00", runs[0])
	}
}

// ---------------------------------------------------------------------------
// Security: Vuln 1 — scratchpad_name path traversal
// ---------------------------------------------------------------------------

func TestHandleFindingToTask_PathTraversal(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	cases := []struct {
		name           string
		scratchpadName string
	}{
		{"dot-dot-etc-passwd", "../../etc/passwd"},
		{"dot-dot-secrets", "../secrets"},
		{"nested-slash", "foo/bar"},
		{"absolute-path", "/absolute/path"},
		{"backslash-traversal", "..\\..\\windows\\system32"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := srv.handleFindingToTask(context.Background(), makeRequest(map[string]any{
				"finding_id":      "FINDING-1",
				"scratchpad_name": tc.scratchpadName,
				"repo":            "test-repo",
			}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected error for traversal input %q", tc.scratchpadName)
			}
			text := getResultText(result)
			if !strings.Contains(text, string(ErrInvalidParams)) {
				t.Errorf("expected INVALID_PARAMS for %q, got: %s", tc.scratchpadName, text)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Security: Vuln 2 — worktree_paths path traversal
// ---------------------------------------------------------------------------

func TestHandleCycleMerge_PathTraversal(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	cases := []struct {
		name          string
		worktreePaths string
	}{
		{"dot-dot-outside", "../../outside"},
		{"null-byte", "/tmp/safe\x00/etc/shadow"},
		{"shell-metachar-semicolon", "/tmp/foo;rm -rf /"},
		{"shell-metachar-pipe", "/tmp/foo|cat /etc/passwd"},
		{"shell-metachar-backtick", "/tmp/`whoami`"},
		{"absolute-outside-scanroot", "/etc/passwd"},
		{"absolute-tmp-escape", "/tmp/malicious-repo"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := srv.handleCycleMerge(context.Background(), makeRequest(map[string]any{
				"worktree_paths": tc.worktreePaths,
			}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected error for traversal input %q", tc.worktreePaths)
			}
			text := getResultText(result)
			if !strings.Contains(text, string(ErrInvalidParams)) {
				t.Errorf("expected INVALID_PARAMS for %q, got: %s", tc.worktreePaths, text)
			}
		})
	}
}

func TestHandleLoopReplay_PathTraversal(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	cases := []struct {
		name   string
		loopID string
	}{
		{"dot-dot", "../../../etc/passwd"},
		{"slash", "foo/bar"},
		{"backslash", "foo\\bar"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := srv.handleLoopReplay(context.Background(), makeRequest(map[string]any{
				"loop_id":   tc.loopID,
				"iteration": 0,
			}))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatalf("expected error for loopID %q", tc.loopID)
			}
			text := getResultText(result)
			if !strings.Contains(text, string(ErrInvalidParams)) {
				t.Errorf("expected INVALID_PARAMS for loopID %q, got: %s", tc.loopID, text)
			}
		})
	}
}
