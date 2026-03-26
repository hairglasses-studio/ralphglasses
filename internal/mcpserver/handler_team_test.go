package mcpserver

import (
	"context"
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
