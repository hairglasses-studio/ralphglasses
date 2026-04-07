package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
)

// pidDir returns the path to the PID files directory, derived from stateDir.
func (m *Manager) pidDir() string {
	if m.stateDir == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(m.stateDir), "pids")
}

// InitPIDFiles cleans up PID files for dead processes in the PID directory.
func (m *Manager) InitPIDFiles() {
	dir := m.pidDir()
	if dir == "" {
		return
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}
	cleaned, err := process.CleanupOrphans(dir)
	if err != nil {
		slog.Warn("failed to clean up PID files", "dir", dir, "error", err)
		return
	}
	if cleaned > 0 {
		slog.Info("cleaned up orphan PID files", "count", cleaned, "dir", dir)
	}
}

// Launch starts a new provider session via the configured CLI.
func (m *Manager) Launch(ctx context.Context, opts LaunchOptions) (*Session, error) {
	opts.TenantID = NormalizeTenantID(opts.TenantID)
	if opts.Provider == "" {
		opts.Provider = DefaultPrimaryProvider()
	}
	if opts.Model == "" {
		opts.Model = ProviderDefaults(opts.Provider)
	}
	// Apply default budget from .ralphrc (RALPH_SESSION_BUDGET) when none specified.
	if opts.MaxBudgetUSD <= 0 && m.DefaultBudgetUSD > 0 {
		opts.MaxBudgetUSD = m.DefaultBudgetUSD
	}
	// Hard floor: if no budget is set from any source, cap at $5 to prevent uncapped spending.
	if opts.MaxBudgetUSD <= 0 && m.DefaultBudgetUSD <= 0 {
		opts.MaxBudgetUSD = 5.0
	}

	// Fleet budget gate: reject launch if spending would exceed fleet cap.
	if m.FleetPool != nil {
		estimatedCost := opts.MaxBudgetUSD
		if estimatedCost <= 0 {
			estimatedCost = DefaultEstimatedSessionCost
		}
		if !m.FleetPool.CanSpend(estimatedCost) {
			sum := m.FleetPool.GetSummary()
			return nil, fmt.Errorf("fleet budget cap exceeded: spent $%.2f of $%.2f cap",
				sum.TotalSpentUSD, sum.BudgetCapUSD)
		}
	}

	// Hourly spend circuit breaker gate: reject launch if rolling hourly rate exceeds threshold.
	if m.spendMonitor != nil && m.spendMonitor.Tripped() {
		return nil, fmt.Errorf("hourly spend limit exceeded (%.2f/hr, threshold $%.2f)",
			m.spendMonitor.HourlyRate(), m.spendMonitor.threshold)
	}

	// Level 2+ auto-optimization: consult FeedbackAnalyzer for provider/budget
	m.configMu.RLock()
	optimizer := m.optimizer
	m.configMu.RUnlock()
	if optimizer != nil {
		var changed bool
		opts, changed = optimizer.OptimizedLaunchOptions(opts)
		if changed {
			slog.Info("auto-optimizer adjusted launch options", "provider", opts.Provider, "model", opts.Model)
		}
	}

	tenant, err := m.resolveTenant(ctx, opts.TenantID)
	if err != nil {
		return nil, fmt.Errorf("resolve tenant %s: %w", opts.TenantID, err)
	}
	if !tenant.AllowsRepoPath(opts.RepoPath) {
		return nil, fmt.Errorf("tenant %s cannot access repo path %s", tenant.ID, opts.RepoPath)
	}

	var s *Session
	m.configMu.RLock()
	hook := m.launchSession
	m.configMu.RUnlock()
	if hook != nil {
		s, err = hook(ctx, opts)
	} else {
		s, err = launch(ctx, opts, m.bus)
	}
	if err != nil {
		return nil, err
	}
	s.TenantID = opts.TenantID

	// Set persistence and feedback callbacks so runner can persist and learn on completion
	s.onComplete = func(sess *Session) {
		m.persistOrWarn(sess, "on session complete")
		// FINDING-237: Write observation data so fleet_analytics has data in
		// standalone mode (no fleet coordinator). This mirrors what
		// emitLoopObservation does for loop iterations, but covers direct
		// session launches.
		emitSessionObservation(sess)
		// Feed session results back into the self-improvement loop
		if optimizer != nil {
			optimizer.IngestSessionJournal(sess)
			optimizer.HandleSessionComplete(ctx, sess)
		}
		// Transition team status when lead session completes
		m.updateTeamOnSessionEnd(sess)
	}

	m.sessionsMu.Lock()
	m.sessions[s.ID] = s
	m.sessionsMu.Unlock()
	m.updateStatusCache(s.ID, s.Status) // Phase 10.5.1: seed hot-read status cache

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
	return m.ResumeWithTenant(ctx, DefaultTenantID, repoPath, provider, sessionID, prompt)
}

// ResumeWithTenant resumes a previous session within the specified tenant.
func (m *Manager) ResumeWithTenant(ctx context.Context, tenantID, repoPath string, provider Provider, sessionID, prompt string) (*Session, error) {
	if provider == "" {
		provider = DefaultPrimaryProvider()
	}
	opts := LaunchOptions{
		TenantID: tenantID,
		Provider: provider,
		RepoPath: repoPath,
		Prompt:   prompt,
		Resume:   sessionID,
	}
	return m.Launch(ctx, opts)
}

// Stop gracefully stops a running session.
func (m *Manager) Stop(id string) error {
	m.sessionsMu.RLock()
	s, ok := m.sessions[id]
	m.sessionsMu.RUnlock()

	if !ok {
		return fmt.Errorf("session %s: %w", id, ErrSessionNotFound)
	}

	s.mu.Lock()

	if s.Status != StatusRunning && s.Status != StatusLaunching {
		s.mu.Unlock()
		return fmt.Errorf("session %s (status: %s): %w", id, s.Status, ErrSessionNotRunning)
	}

	s.Status = StatusStopped
	m.updateStatusCache(s.ID, StatusStopped) // Phase 10.5.1: update hot-read status cache

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

	// Write active state for starship integration (fire-and-forget).
	_ = WriteActiveState(s)

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

// StopAll stops all running sessions and the supervisor if active.
func (m *Manager) StopAll() {
	m.stopAllFiltered("")
}

// StopAllForTenant stops all running sessions belonging to one tenant.
func (m *Manager) StopAllForTenant(tenantID string) {
	m.stopAllFiltered(NormalizeTenantID(tenantID))
}

func (m *Manager) stopAllFiltered(tenantID string) {
	// Stop the supervisor first so it doesn't relaunch sessions we're about to kill.
	m.configMu.Lock()
	m.stopSupervisor()
	m.configMu.Unlock()

	m.sessionsMu.RLock()
	ids := make([]string, 0, len(m.sessions))
	for id, s := range m.sessions {
		if tenantID != "" && NormalizeTenantID(s.TenantID) != tenantID {
			continue
		}
		s.mu.Lock()
		if s.Status == StatusRunning || s.Status == StatusLaunching {
			ids = append(ids, id)
		}
		s.mu.Unlock()
	}
	m.sessionsMu.RUnlock()

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
	m.sessionsMu.RLock()
	s, ok := m.sessions[sessionID]
	m.sessionsMu.RUnlock()
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
		TenantID:     s.TenantID,
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

func (m *Manager) waitForSession(ctx context.Context, s *Session) error {
	if m.waitSession != nil {
		return m.waitSession(ctx, s)
	}

	// Capture doneCh under lock. A nil channel blocks forever in select,
	// which effectively disables that case for test sessions without a process.
	s.Lock()
	doneCh := s.doneCh
	s.Unlock()

	timeout := m.effectiveSessionTimeout()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	// checkTerminal reads session status under lock and returns (done, error).
	checkTerminal := func() (bool, error) {
		s.Lock()
		status := s.Status
		errMsg := s.Error
		exitReason := s.ExitReason
		s.Unlock()

		switch status {
		case StatusCompleted:
			return true, nil
		case StatusErrored:
			if errMsg != "" {
				return true, errors.New(errMsg)
			}
			if exitReason != "" {
				return true, errors.New(exitReason)
			}
			return true, fmt.Errorf("session %s: %w", s.ID, ErrSessionErrored)
		case StatusStopped:
			if exitReason != "" {
				return true, errors.New(exitReason)
			}
			return true, fmt.Errorf("session %s: %w", s.ID, ErrSessionStopped)
		}
		return false, nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("session %s after %s: %w", s.ID, timeout, ErrWaitTimeout)
		case <-doneCh:
			// Process exited. Give the runner goroutine a moment to set status.
			time.Sleep(50 * time.Millisecond)
			if done, err := checkTerminal(); done {
				return err
			}
			// Runner didn't set a terminal status — the process exited unexpectedly.
			s.Lock()
			status := s.Status
			s.Unlock()
			return fmt.Errorf("session %s (status: %s): %w", s.ID, status, ErrUnexpectedExit)
		case <-ticker.C:
			if done, err := checkTerminal(); done {
				return err
			}
		}
	}
}

// DefaultErrorRetention is the default duration errored sessions remain queryable.
const DefaultErrorRetention = 5 * time.Minute

// effectiveErrorRetention returns the errored session retention window, defaulting to 5 minutes.
func (m *Manager) effectiveErrorRetention() time.Duration {
	if m.ErrorRetention <= 0 {
		return DefaultErrorRetention
	}
	return m.ErrorRetention
}

// effectiveSessionTimeout returns the session wait timeout, defaulting to 10 minutes.
func (m *Manager) effectiveSessionTimeout() time.Duration {
	if m.SessionTimeout <= 0 {
		return 10 * time.Minute
	}
	return m.SessionTimeout
}

// effectiveKillTimeout returns the SIGTERM→SIGKILL escalation timeout,
// defaulting to DefaultSessionKillTimeout (15s). Increased from 5s to give
// self-improvement loops adequate time to checkpoint state before SIGKILL
// (addresses the "signal: killed" pattern — 6 occurrences).
func (m *Manager) effectiveKillTimeout() time.Duration {
	if m.KillTimeout <= 0 {
		return DefaultSessionKillTimeout
	}
	return m.KillTimeout
}

// DefaultMinSessionDuration is the default minimum age before a session can be
// stopped by automated reapers. Sessions younger than this are protected.
const DefaultMinSessionDuration = 30 * time.Second

// effectiveMinSessionDuration returns the minimum session age before reaping, defaulting to 30s.
func (m *Manager) effectiveMinSessionDuration() time.Duration {
	if m.MinSessionDuration <= 0 {
		return DefaultMinSessionDuration
	}
	return m.MinSessionDuration
}

// IsReapable returns true if the session is old enough to be stopped by a reaper.
// Sessions younger than MinSessionDuration are protected from automated cleanup
// to prevent the "killed in <1s" problem (FINDING-160).
func (m *Manager) IsReapable(s *Session) bool {
	s.mu.Lock()
	launched := s.LaunchedAt
	s.mu.Unlock()
	return time.Since(launched) >= m.effectiveMinSessionDuration()
}

// persistOrWarn persists session state and logs a warning on failure.
func (m *Manager) persistOrWarn(s *Session, context string) {
	if err := m.PersistSession(s); err != nil {
		slog.Warn("persist session failed",
			"session_id", s.ID,
			"context", context,
			"err", err,
		)
	}
}

// PersistSession writes session state to the shared state directory and,
// if a Store is configured, also saves to the store.
// Safe to call from any goroutine; acquires the session lock.
func (m *Manager) PersistSession(s *Session) error {
	s.TenantID = NormalizeTenantID(s.TenantID)
	// Write to Store if configured.
	if m.store != nil {
		s.mu.Lock()
		// SaveSession reads exported fields; lock protects concurrent mutation.
		err := m.store.SaveSession(context.Background(), s)
		s.mu.Unlock()
		if err != nil {
			slog.Warn("store save failed, falling back to JSON", "session_id", s.ID, "err", err)
		}
	}

	// Legacy JSON file persistence (still active until full migration).
	if m.stateDir == "" {
		return nil
	}
	dir := m.sessionStateDirForTenant(s.TenantID)
	if dir == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("persist session: mkdir: %w", err)
	}

	s.mu.Lock()
	data, err := json.Marshal(s)
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("persist session: marshal: %w", err)
	}

	path := m.sessionStatePath(s.TenantID, s.ID)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("persist session: write %s: %w", path, err)
	}
	if legacyPath := m.legacySessionStatePath(s.ID); legacyPath != "" && legacyPath != path {
		_ = os.Remove(legacyPath)
	}
	return nil
}

// RehydrateFromStore loads persisted sessions from the SQLite store into the
// in-memory map so they survive process restarts. Sessions that were running
// or launching when the previous process died are marked as interrupted since
// their OS process no longer exists. Sessions already in the in-memory map
// (launched by the current process) are not overwritten.
// Returns nil if no store is configured (no-op).
func (m *Manager) RehydrateFromStore() error {
	if m.store == nil {
		return nil
	}

	// Load all non-terminal sessions, plus recently-terminal ones for visibility.
	// We use an empty ListOpts to get everything, then filter client-side.
	all, err := m.store.ListSessions(context.Background(), ListOpts{})
	if err != nil {
		return fmt.Errorf("rehydrate: list sessions from store: %w", err)
	}

	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()

	rehydrated := 0
	interrupted := 0
	for _, sess := range all {
		// Skip sessions already in the in-memory map (current process owns them).
		if _, exists := m.sessions[sess.ID]; exists {
			continue
		}

		// Sessions that were running or launching when the old process died
		// cannot still be alive — mark them as interrupted.
		sess.TenantID = NormalizeTenantID(sess.TenantID)
		if sess.Status == StatusRunning || sess.Status == StatusLaunching {
			sess.Status = StatusInterrupted
			now := time.Now()
			sess.EndedAt = &now
			sess.LastActivity = now
			sess.ExitReason = "interrupted: process not found after restart"
			sess.Pid = 0 // PID is stale
			interrupted++

			// Persist the updated status back to the store.
			if err := m.store.SaveSession(context.Background(), sess); err != nil {
				slog.Warn("rehydrate: failed to persist interrupted status",
					"session_id", sess.ID, "err", err)
			}

			// Write active state for starship integration (fire-and-forget).
			_ = WriteActiveState(sess)
		}

		m.sessions[sess.ID] = sess
		m.updateStatusCache(sess.ID, sess.Status) // Phase 10.5.1: seed status cache
		rehydrated++
	}

	if rehydrated > 0 {
		slog.Info("rehydrated sessions from store",
			"total", rehydrated,
			"interrupted", interrupted,
		)
	}
	return nil
}

// LoadExternalSessions reads session JSON files from the shared state directory
// and merges any unknown sessions into the manager. This allows the TUI to
// discover sessions launched by the MCP server (a separate process).
func (m *Manager) LoadExternalSessions() {
	if m.stateDir == "" {
		return
	}
	files := m.discoverSessionStateFiles()

	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()

	for _, file := range files {
		id := file.ID

		// If we already own this session (launched in-process), update the file
		// but don't overwrite in-memory state.
		if existing, ok := m.sessions[id]; ok {
			// Re-persist in-process sessions so disk stays current (best-effort).
			// Done synchronously to avoid goroutine leaks in tests.
			m.persistOrWarn(existing, "re-persist on load")
			continue
		}

		data, err := os.ReadFile(file.Path)
		if err != nil {
			continue
		}

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		s.TenantID = NormalizeTenantID(s.TenantID)
		if file.Legacy {
			s.TenantID = DefaultTenantID
		}

		// Skip sessions older than 24h
		cutoff := time.Now().Add(-24 * time.Hour)
		if !s.LaunchedAt.IsZero() && s.LaunchedAt.Before(cutoff) {
			continue
		}

		m.sessions[id] = &s
		m.updateStatusCache(id, s.Status) // Phase 10.5.1: seed status cache
		if file.Legacy {
			m.persistOrWarn(&s, "migrate legacy session state")
		}
	}

	// Clean up terminal sessions from memory and disk.
	// Errored sessions use a shorter retention window (default 5m) so they
	// remain queryable by session_status and rc_read for debugging.
	// Completed/stopped sessions use the 24h cutoff.
	now := time.Now()
	completedCutoff := now.Add(-24 * time.Hour)
	errorRetention := m.effectiveErrorRetention()
	erroredCutoff := now.Add(-errorRetention)

	for id, s := range m.sessions {
		s.mu.Lock()
		ended := s.EndedAt
		status := s.Status
		s.mu.Unlock()

		if !status.IsTerminal() {
			continue
		}
		if ended == nil {
			continue
		}

		var shouldRemove bool
		if status == StatusErrored {
			shouldRemove = ended.Before(erroredCutoff)
		} else {
			shouldRemove = ended.Before(completedCutoff)
		}

		if shouldRemove {
			delete(m.sessions, id)
			m.evictStatusCache(id) // Phase 10.5.1: evict hot-read cache entry
			for _, candidate := range []string{
				m.sessionStatePath(s.TenantID, id),
				m.legacySessionStatePath(id),
			} {
				if candidate == "" {
					continue
				}
				if err := os.Remove(candidate); err != nil && !os.IsNotExist(err) {
					slog.Warn("failed to remove session state file", "session", id, "error", err, "path", candidate)
				}
			}
		}
	}
}
