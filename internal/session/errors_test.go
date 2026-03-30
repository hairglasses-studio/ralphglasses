package session

import (
	"errors"
	"testing"
)

func TestSessionErrors_Is(t *testing.T) {
	m := NewManager()

	// --- ErrSessionNotFound ---
	t.Run("Stop_SessionNotFound", func(t *testing.T) {
		err := m.Stop("nonexistent-id")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("expected errors.Is(err, ErrSessionNotFound), got: %v", err)
		}
	})

	// --- ErrSessionNotRunning (Stop on a completed session) ---
	t.Run("Stop_SessionNotRunning", func(t *testing.T) {
		s := &Session{
			ID:     "completed-1",
			Status: StatusCompleted,
		}
		m.AddSessionForTesting(s)
		err := m.Stop("completed-1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrSessionNotRunning) {
			t.Errorf("expected errors.Is(err, ErrSessionNotRunning), got: %v", err)
		}
	})

	// --- ErrTeamNameRequired ---
	t.Run("LaunchTeam_TeamNameRequired", func(t *testing.T) {
		_, err := m.LaunchTeam(t.Context(), TeamConfig{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrTeamNameRequired) {
			t.Errorf("expected errors.Is(err, ErrTeamNameRequired), got: %v", err)
		}
	})

	// --- ErrRepoPathRequired ---
	t.Run("LaunchTeam_RepoPathRequired", func(t *testing.T) {
		_, err := m.LaunchTeam(t.Context(), TeamConfig{Name: "test-team"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrRepoPathRequired) {
			t.Errorf("expected errors.Is(err, ErrRepoPathRequired), got: %v", err)
		}
	})

	// --- ErrNoTasks ---
	t.Run("LaunchTeam_NoTasks", func(t *testing.T) {
		_, err := m.LaunchTeam(t.Context(), TeamConfig{
			Name:     "test-team",
			RepoPath: "/tmp/repo",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrNoTasks) {
			t.Errorf("expected errors.Is(err, ErrNoTasks), got: %v", err)
		}
	})

	// --- ErrTeamNotFound ---
	t.Run("DelegateTask_TeamNotFound", func(t *testing.T) {
		_, err := m.DelegateTask("nonexistent-team", TeamTask{Description: "task"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrTeamNotFound) {
			t.Errorf("expected errors.Is(err, ErrTeamNotFound), got: %v", err)
		}
	})

	// --- ErrAlreadyOnProvider (MigrateSession) ---
	t.Run("MigrateSession_NotFound", func(t *testing.T) {
		_, err := m.MigrateSession(t.Context(), "no-such-session", ProviderGemini)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrSessionNotFound) {
			t.Errorf("expected errors.Is(err, ErrSessionNotFound), got: %v", err)
		}
	})

	t.Run("MigrateSession_NotRunning", func(t *testing.T) {
		s := &Session{
			ID:       "stopped-migrate",
			Status:   StatusStopped,
			Provider: ProviderClaude,
		}
		m.AddSessionForTesting(s)
		_, err := m.MigrateSession(t.Context(), "stopped-migrate", ProviderGemini)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrSessionNotRunning) {
			t.Errorf("expected errors.Is(err, ErrSessionNotRunning), got: %v", err)
		}
	})

	t.Run("MigrateSession_AlreadyOnProvider", func(t *testing.T) {
		s := &Session{
			ID:       "running-migrate",
			Status:   StatusRunning,
			Provider: ProviderClaude,
		}
		m.AddSessionForTesting(s)
		_, err := m.MigrateSession(t.Context(), "running-migrate", ProviderClaude)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrAlreadyOnProvider) {
			t.Errorf("expected errors.Is(err, ErrAlreadyOnProvider), got: %v", err)
		}
	})

	// --- ErrRepoPathRequired from launch ---
	t.Run("Launch_RepoPathRequired", func(t *testing.T) {
		_, err := m.Launch(t.Context(), LaunchOptions{Prompt: "test"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrRepoPathRequired) {
			t.Errorf("expected errors.Is(err, ErrRepoPathRequired), got: %v", err)
		}
	})

	// --- ErrRepoNotExist from launch ---
	t.Run("Launch_RepoNotExist", func(t *testing.T) {
		_, err := m.Launch(t.Context(), LaunchOptions{
			RepoPath: "/nonexistent/repo/path",
			Prompt:   "test",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrRepoNotExist) {
			t.Errorf("expected errors.Is(err, ErrRepoNotExist), got: %v", err)
		}
	})

	// --- ErrRepoNotGit from launch ---
	t.Run("Launch_RepoNotGit", func(t *testing.T) {
		dir := t.TempDir() // exists but no .git
		_, err := m.Launch(t.Context(), LaunchOptions{
			RepoPath: dir,
			Prompt:   "test",
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrRepoNotGit) {
			t.Errorf("expected errors.Is(err, ErrRepoNotGit), got: %v", err)
		}
	})
}
