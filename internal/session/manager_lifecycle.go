package session

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// Launch starts a new Claude Code session via claude -p.
func (m *Manager) Launch(ctx context.Context, opts LaunchOptions) (*Session, error) {
	if opts.Provider == "" {
		opts.Provider = ProviderClaude
	}
	if opts.Model == "" {
		opts.Model = ProviderDefaults(opts.Provider)
	}

	// Level 2+ auto-optimization: consult FeedbackAnalyzer for provider/budget
	m.mu.RLock()
	optimizer := m.optimizer
	m.mu.RUnlock()
	if optimizer != nil {
		var changed bool
		opts, changed = optimizer.OptimizedLaunchOptions(opts)
		if changed {
			slog.Info("auto-optimizer adjusted launch options", "provider", opts.Provider, "model", opts.Model)
		}
	}

	s, err := launch(ctx, opts, m.bus)
	if err != nil {
		return nil, err
	}

	// Set persistence and feedback callbacks so runner can persist and learn on completion
	s.onComplete = func(sess *Session) {
		m.persistOrWarn(sess, "on session complete")
		// Feed session results back into the self-improvement loop
		if optimizer != nil {
			optimizer.IngestSessionJournal(sess)
			optimizer.HandleSessionComplete(ctx, sess)
		}
		// Transition team status when lead session completes
		m.updateTeamOnSessionEnd(sess)
	}

	m.mu.Lock()
	m.sessions[s.ID] = s
	m.mu.Unlock()

	// Persist initial state to disk
	m.persistOrWarn(s, "after session start")

	if m.bus != nil {
		m.bus.PublishCtx(ctx, events.Event{
			Type:      events.SessionStarted,
			SessionID: s.ID,
			RepoPath:  s.RepoPath,
			RepoName:  filepath.Base(s.RepoPath),
			Provider:  string(s.Provider),
			Data:      map[string]any{"model": s.Model, "prompt_len": len(opts.Prompt)},
		})
	}

	return s, nil
}

// Resume resumes a previous session by its provider session ID.
func (m *Manager) Resume(ctx context.Context, repoPath string, provider Provider, sessionID, prompt string) (*Session, error) {
	if provider == "" {
		provider = ProviderClaude
	}
	opts := LaunchOptions{
		Provider: provider,
		RepoPath: repoPath,
		Prompt:   prompt,
		Resume:   sessionID,
	}
	return m.Launch(ctx, opts)
}

// Stop gracefully stops a running session.
func (m *Manager) Stop(id string) error {
	m.mu.RLock()
	s, ok := m.sessions[id]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("session %s: %w", id, ErrSessionNotFound)
	}

	s.mu.Lock()

	if s.Status != StatusRunning && s.Status != StatusLaunching {
		s.mu.Unlock()
		return fmt.Errorf("session %s (status: %s): %w", id, s.Status, ErrSessionNotRunning)
	}

	s.Status = StatusStopped

	// Cancel context first
	if s.cancel != nil {
		s.cancel()
	}

	// Capture cmd, bus, and doneCh before releasing the lock — killWithEscalation
	// may block for up to 5 seconds waiting for graceful exit.
	cmd := s.cmd
	bus := s.bus
	doneCh := s.doneCh
	s.mu.Unlock()

	// Kill with escalation (SIGTERM -> wait -> SIGKILL) outside the lock.
	killTimeout := m.effectiveKillTimeout()
	if cmd != nil && cmd.Process != nil {
		escalated := killWithEscalation(cmd, killTimeout, doneCh)
		if escalated && bus != nil {
			bus.Publish(events.Event{
				Type:      events.SessionStopped,
				SessionID: s.ID,
				RepoPath:  s.RepoPath,
				Data:      map[string]any{"escalated_to_sigkill": true},
			})
		}
	}

	// Persist stopped state (synchronous; s.mu is released above).
	// Best-effort: stop succeeds even if persistence fails.
	m.persistOrWarn(s, "after stop")

	return nil
}

// StopAll stops all running sessions.
func (m *Manager) StopAll() {
	m.mu.RLock()
	ids := make([]string, 0, len(m.sessions))
	for id, s := range m.sessions {
		s.mu.Lock()
		if s.Status == StatusRunning || s.Status == StatusLaunching {
			ids = append(ids, id)
		}
		s.mu.Unlock()
	}
	m.mu.RUnlock()

	for _, id := range ids {
		if err := m.Stop(id); err != nil {
			slog.Warn("failed to stop session during StopAll", "session", id, "error", err)
		}
	}
}

// MigrateSession stops a running session and relaunches it on a different provider.
// The new session inherits the original prompt, remaining budget, max turns, and team.
// Returns the new session on success; the old session is stopped regardless.
func (m *Manager) MigrateSession(ctx context.Context, sessionID string, targetProvider Provider) (*Session, error) {
	m.mu.RLock()
	s, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %s: %w", sessionID, ErrSessionNotFound)
	}

	s.mu.Lock()
	if s.Status != StatusRunning && s.Status != StatusLaunching {
		s.mu.Unlock()
		return nil, fmt.Errorf("session %s (status: %s): %w", sessionID, s.Status, ErrSessionNotRunning)
	}
	if s.Provider == targetProvider {
		s.mu.Unlock()
		return nil, fmt.Errorf("session %s on %s: %w", sessionID, targetProvider, ErrAlreadyOnProvider)
	}
	// Capture state before stopping.
	remaining := s.BudgetUSD - s.SpentUSD
	if remaining < 0 {
		remaining = 0
	}
	opts := LaunchOptions{
		Provider:     targetProvider,
		RepoPath:     s.RepoPath,
		Prompt:       s.Prompt,
		Model:        ProviderDefaults(targetProvider),
		MaxBudgetUSD: remaining,
		MaxTurns:     s.MaxTurns,
		TeamName:     s.TeamName,
	}
	s.mu.Unlock()

	if err := m.Stop(sessionID); err != nil {
		return nil, fmt.Errorf("stop source session: %w", err)
	}

	newSession, err := m.Launch(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("launch on %s: %w", targetProvider, err)
	}
	return newSession, nil
}
