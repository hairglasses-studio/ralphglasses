package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestHandleTeamCreate(t *testing.T) {
	t.Parallel()

	t.Run("missing repo", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)
		result, err := srv.handleTeamCreate(context.Background(), makeRequest(map[string]any{
			"name":  "my-team",
			"tasks": "task1\ntask2",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing repo")
		}
		text := getResultText(result)
		if !strings.Contains(text, "INVALID_PARAMS") {
			t.Errorf("expected INVALID_PARAMS code, got: %s", text)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)
		result, err := srv.handleTeamCreate(context.Background(), makeRequest(map[string]any{
			"repo":  "test-repo",
			"tasks": "task1",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing name")
		}
		text := getResultText(result)
		if !strings.Contains(text, "INVALID_PARAMS") {
			t.Errorf("expected INVALID_PARAMS code, got: %s", text)
		}
	})

	t.Run("missing tasks", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)
		result, err := srv.handleTeamCreate(context.Background(), makeRequest(map[string]any{
			"repo": "test-repo",
			"name": "my-team",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing tasks")
		}
		text := getResultText(result)
		if !strings.Contains(text, "INVALID_PARAMS") {
			t.Errorf("expected INVALID_PARAMS code, got: %s", text)
		}
	})

	t.Run("valid creation", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)
		// Scan first so repos are populated
		_, err := srv.handleScan(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		result, err := srv.handleTeamCreate(context.Background(), makeRequest(map[string]any{
			"repo":  "test-repo",
			"name":  "my-team",
			"tasks": "implement feature A\nwrite tests for feature A",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected success, got error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, "my-team") {
			t.Errorf("expected team name in result, got: %s", text)
		}
	})
}

func TestHandleTeamCreate_DryRun(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	// Scan first so repos are populated
	_, err := srv.handleScan(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	result, err := srv.handleTeamCreate(context.Background(), makeRequest(map[string]any{
		"repo":           "test-repo",
		"name":           "dry-team",
		"tasks":          "implement feature A\nwrite tests for feature A",
		"provider":       "claude",
		"max_budget_usd": 5.0,
		"dry_run":        true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getResultText(result))
	}

	text := getResultText(result)

	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("failed to parse dry_run result as JSON: %v\nraw: %s", err, text)
	}

	// Verify dry_run flag is set
	if dryRun, ok := parsed["dry_run"].(bool); !ok || !dryRun {
		t.Errorf("expected dry_run=true, got: %v", parsed["dry_run"])
	}

	// Verify team name
	if name, _ := parsed["name"].(string); name != "dry-team" {
		t.Errorf("expected name=dry-team, got: %s", name)
	}

	// Verify tasks were parsed
	tasks, ok := parsed["tasks"].([]any)
	if !ok {
		t.Fatalf("expected tasks array, got: %T", parsed["tasks"])
	}
	if len(tasks) != 2 {
		t.Errorf("expected 2 tasks, got: %d", len(tasks))
	}

	// Verify task_count
	if count, _ := parsed["task_count"].(float64); count != 2 {
		t.Errorf("expected task_count=2, got: %v", count)
	}

	// Verify no team was actually created by checking team status returns not found
	statusResult, err := srv.handleTeamStatus(context.Background(), makeRequest(map[string]any{
		"name": "dry-team",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !statusResult.IsError {
		t.Fatal("expected team NOT to be created during dry_run, but team was found")
	}
	statusText := getResultText(statusResult)
	if !strings.Contains(statusText, "TEAM_NOT_FOUND") {
		t.Errorf("expected TEAM_NOT_FOUND, got: %s", statusText)
	}
}

func TestHandleTeamCreate_DryRunDefaults(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	_, err := srv.handleScan(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// Call dry_run without specifying optional fields to verify effective defaults.
	result, err := srv.handleTeamCreate(context.Background(), makeRequest(map[string]any{
		"repo":    "test-repo",
		"name":    "default-team",
		"tasks":   "task one\ntask two",
		"dry_run": true,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getResultText(result))
	}

	text := getResultText(result)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		t.Fatalf("failed to parse dry_run result: %v\nraw: %s", err, text)
	}

	// provider should default to "claude", not empty
	if prov, _ := parsed["provider"].(string); prov != "claude" {
		t.Errorf("provider = %q, want %q", prov, "claude")
	}
	// worker_provider should default to provider value
	if wp, _ := parsed["worker_provider"].(string); wp != "claude" {
		t.Errorf("worker_provider = %q, want %q", wp, "claude")
	}
	// model should show effective default, not empty
	if m, _ := parsed["model"].(string); m == "" {
		t.Error("model should not be empty in dry_run preview")
	}
	// lead_agent should show effective default
	if la, _ := parsed["lead_agent"].(string); la == "" {
		t.Error("lead_agent should not be empty in dry_run preview")
	}
	// max_budget_usd should show effective default, not 0
	if budget, _ := parsed["max_budget_usd"].(float64); budget <= 0 {
		t.Errorf("max_budget_usd = %f, want > 0", budget)
	}
}

func TestHandleTeamDelegate(t *testing.T) {
	t.Parallel()

	t.Run("missing name", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)
		result, err := srv.handleTeamDelegate(context.Background(), makeRequest(map[string]any{
			"task": "do something",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing name")
		}
		text := getResultText(result)
		if !strings.Contains(text, "INVALID_PARAMS") {
			t.Errorf("expected INVALID_PARAMS code, got: %s", text)
		}
	})

	t.Run("missing task", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)
		result, err := srv.handleTeamDelegate(context.Background(), makeRequest(map[string]any{
			"name": "nonexistent-team",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing task")
		}
		text := getResultText(result)
		if !strings.Contains(text, "INVALID_PARAMS") {
			t.Errorf("expected INVALID_PARAMS code, got: %s", text)
		}
	})

	t.Run("non-existent team", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)
		result, err := srv.handleTeamDelegate(context.Background(), makeRequest(map[string]any{
			"name": "ghost-team",
			"task": "do something",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for non-existent team")
		}
		text := getResultText(result)
		if !strings.Contains(text, "TEAM_NOT_FOUND") {
			t.Errorf("expected TEAM_NOT_FOUND code, got: %s", text)
		}
	})
}

func TestHandleTeamStatus(t *testing.T) {
	t.Parallel()

	t.Run("missing name", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)
		result, err := srv.handleTeamStatus(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error result for missing name")
		}
		text := getResultText(result)
		if !strings.Contains(text, "INVALID_PARAMS") {
			t.Errorf("expected INVALID_PARAMS code, got: %s", text)
		}
	})

	t.Run("non-existent team", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)
		result, err := srv.handleTeamStatus(context.Background(), makeRequest(map[string]any{
			"name": "no-such-team",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for non-existent team")
		}
		text := getResultText(result)
		if !strings.Contains(text, "TEAM_NOT_FOUND") {
			t.Errorf("expected TEAM_NOT_FOUND code, got: %s", text)
		}
	})

	t.Run("existing team", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)
		// Scan first so repos are populated
		_, err := srv.handleScan(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("scan failed: %v", err)
		}

		// Create a team first
		_, err = srv.handleTeamCreate(context.Background(), makeRequest(map[string]any{
			"repo":  "test-repo",
			"name":  "status-team",
			"tasks": "task one\ntask two",
		}))
		if err != nil {
			t.Fatalf("create team failed: %v", err)
		}

		// Now check its status
		result, err := srv.handleTeamStatus(context.Background(), makeRequest(map[string]any{
			"name": "status-team",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected success, got error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, "status-team") {
			t.Errorf("expected team name in status result, got: %s", text)
		}
	})
}
