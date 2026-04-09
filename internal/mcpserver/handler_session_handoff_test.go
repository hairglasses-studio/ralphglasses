package mcpserver

import (
	"context"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleSessionHandoff_DefaultsEmptyProviderToCodex(t *testing.T) {
	t.Parallel()

	srv, root := setupTestServer(t)
	repoPath := root + "/test-repo"

	sourceID := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = ""
		s.Model = ""
		s.BudgetUSD = 10
		s.SpentUSD = 1.25
		s.TeamName = "test-team"
	})

	var captured session.LaunchOptions
	srv.SessMgr.SetHooksForTesting(
		func(_ context.Context, opts session.LaunchOptions) (*session.Session, error) {
			captured = opts
			return &session.Session{
				ID:         "handoff-target",
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   "test-repo",
				Status:     session.StatusRunning,
				LaunchedAt: time.Now(),
			}, nil
		},
		func(_ context.Context, _ *session.Session) error { return nil },
	)

	result, err := srv.handleSessionHandoff(context.Background(), makeRequest(map[string]any{
		"source_session_id": sourceID,
		"include_context":   false,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getResultText(result))
	}
	if captured.Provider != session.ProviderCodex {
		t.Fatalf("captured provider = %q, want %q", captured.Provider, session.ProviderCodex)
	}
	if captured.Model != session.ProviderDefaults(session.ProviderCodex) {
		t.Fatalf("captured model = %q, want %q", captured.Model, session.ProviderDefaults(session.ProviderCodex))
	}
}

func TestHandleSessionHandoff_ReroutesClaudeAfterCacheAnomalies(t *testing.T) {
	t.Parallel()

	srv, root := setupTestServer(t)
	repoPath := root + "/test-repo"

	for i := 0; i < claudeCacheRerouteThreshold; i++ {
		injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.Provider = session.ProviderClaude
			s.Resumed = true
			s.CacheWriteTokens = 1500
			s.CacheReadTokens = 0
			s.LastActivity = time.Now()
		})
	}

	sourceID := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = session.ProviderClaude
		s.Model = "sonnet"
		s.BudgetUSD = 12
		s.SpentUSD = 2
	})

	var captured session.LaunchOptions
	srv.SessMgr.SetHooksForTesting(
		func(_ context.Context, opts session.LaunchOptions) (*session.Session, error) {
			captured = opts
			return &session.Session{
				ID:         "handoff-target",
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   "test-repo",
				Status:     session.StatusRunning,
				LaunchedAt: time.Now(),
			}, nil
		},
		func(_ context.Context, _ *session.Session) error { return nil },
	)

	result, err := srv.handleSessionHandoff(context.Background(), makeRequest(map[string]any{
		"source_session_id": sourceID,
		"include_context":   false,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getResultText(result))
	}
	if captured.Provider != session.ProviderCodex {
		t.Fatalf("captured provider = %q, want %q", captured.Provider, session.ProviderCodex)
	}
}

func TestHandleSessionHandoff_AllowsAntigravityTarget(t *testing.T) {
	t.Parallel()

	srv, root := setupTestServer(t)
	repoPath := root + "/test-repo"

	sourceID := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = session.ProviderCodex
		s.BudgetUSD = 10
		s.SpentUSD = 1
	})

	var captured session.LaunchOptions
	srv.SessMgr.SetHooksForTesting(
		func(_ context.Context, opts session.LaunchOptions) (*session.Session, error) {
			captured = opts
			return &session.Session{
				ID:         "handoff-target",
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   "test-repo",
				Status:     session.StatusRunning,
				LaunchedAt: time.Now(),
			}, nil
		},
		func(_ context.Context, _ *session.Session) error { return nil },
	)

	result, err := srv.handleSessionHandoff(context.Background(), makeRequest(map[string]any{
		"source_session_id": sourceID,
		"target_provider":   "antigravity",
		"include_context":   false,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", getResultText(result))
	}
	if captured.Provider != session.ProviderAntigravity {
		t.Fatalf("captured provider = %q, want %q", captured.Provider, session.ProviderAntigravity)
	}
}
