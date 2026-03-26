package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// DefaultStateDir is the shared directory for session state persistence.
const DefaultStateDir = "~/.ralphglasses/sessions"

// Manager tracks all active Claude Code sessions and teams.
type Manager struct {
	mu            sync.RWMutex
	sessions      map[string]*Session     // keyed by session ID
	teams         map[string]*TeamStatus  // keyed by team name
	workflowRuns  map[string]*WorkflowRun // keyed by workflow run ID
	loops         map[string]*LoopRun     // keyed by loop run ID
	bus           *events.Bus
	stateDir      string // directory for persisted session JSON files
	optimizer     *AutoOptimizer          // Level 2+ self-improvement engine
	reflexion     *ReflexionStore         // WS1: failure reflection extraction
	episodic      *EpisodicMemory         // WS2: successful trajectory memory
	cascade       *CascadeRouter          // WS3: cheap-then-expensive provider routing
	curriculum    *CurriculumSorter       // WS5: task difficulty scoring and ordering
	banditSelect func() (string, string) // bandit-based provider selection hook
	banditUpdate func(string, float64)   // bandit reward recording hook
	blackboard   *Blackboard             // Phase H: shared inter-subsystem state
	costPredictor *CostPredictor         // Phase H: task cost prediction
	launchSession  func(context.Context, LaunchOptions) (*Session, error)
	waitSession    func(context.Context, *Session) error
	healthCheck    func(Provider) ProviderHealth // injectable health check (default: CheckProviderHealth)
	SessionTimeout time.Duration                 // timeout for waitForSession; 0 uses default (10m)
	KillTimeout    time.Duration                 // SIGTERM→SIGKILL escalation timeout; 0 uses default (5s)
	Enhancer       *enhancer.HybridEngine        // optional prompt enhancement for loop integration
}

// NewManager creates a new session manager.
func NewManager() *Manager {
	return &Manager{
		sessions:     make(map[string]*Session),
		teams:        make(map[string]*TeamStatus),
		workflowRuns: make(map[string]*WorkflowRun),
		loops:        make(map[string]*LoopRun),
		stateDir:     expandHome(DefaultStateDir),
	}
}

// NewManagerWithBus creates a session manager wired to an event bus.
func NewManagerWithBus(bus *events.Bus) *Manager {
	return &Manager{
		sessions:     make(map[string]*Session),
		teams:        make(map[string]*TeamStatus),
		workflowRuns: make(map[string]*WorkflowRun),
		loops:        make(map[string]*LoopRun),
		bus:          bus,
		stateDir:     expandHome(DefaultStateDir),
	}
}

// Init performs one-time startup work after the Manager is constructed.
// It sweeps for orphaned processes from previous runs and logs them without
// killing them — the user should decide what to do.
func (m *Manager) Init() {
	m.mu.RLock()
	activePIDs := make(map[int]bool)
	for _, s := range m.sessions {
		if s.Pid > 0 {
			activePIDs[s.Pid] = true
		}
	}
	ralphDir := filepath.Dir(m.stateDir)
	m.mu.RUnlock()

	orphans := SweepOrphans(ralphDir, activePIDs)
	if len(orphans) > 0 {
		slog.Warn("found orphaned processes from previous run", "count", len(orphans))
	}
}

// SetStateDir overrides the persistence directory. Intended for tests and
// alternate embedding environments that want to isolate on-disk state.
func (m *Manager) SetStateDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stateDir = dir
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

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

// Get returns a session by ID.
func (m *Manager) Get(id string) (*Session, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[id]
	return s, ok
}

// List returns all sessions, optionally filtered by repo path.
func (m *Manager) List(repoPath string) []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Session
	for _, s := range m.sessions {
		if repoPath != "" && s.RepoPath != repoPath {
			continue
		}
		result = append(result, s)
	}
	return result
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

// IsRunning checks if any session is running for the given repo path.
func (m *Manager) IsRunning(repoPath string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, s := range m.sessions {
		if s.RepoPath == repoPath {
			s.mu.Lock()
			running := s.Status == StatusRunning || s.Status == StatusLaunching
			s.mu.Unlock()
			if running {
				return true
			}
		}
	}
	return false
}

// FindByRepo returns all sessions for a given repo name.
func (m *Manager) FindByRepo(repoName string) []*Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*Session
	for _, s := range m.sessions {
		if filepath.Base(s.RepoPath) == repoName {
			result = append(result, s)
		}
	}
	return result
}


// GetWorkflowRun returns a workflow run by ID.
func (m *Manager) GetWorkflowRun(id string) (*WorkflowRun, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	run, ok := m.workflowRuns[id]
	return run, ok
}

// RunWorkflow validates and starts a workflow asynchronously.
func (m *Manager) RunWorkflow(ctx context.Context, repoPath string, wf WorkflowDef) (*WorkflowRun, error) {
	if err := ValidateWorkflow(wf); err != nil {
		return nil, err
	}

	run := newWorkflowRun(repoPath, wf)

	m.mu.Lock()
	m.workflowRuns[run.ID] = run
	m.mu.Unlock()

	go m.executeWorkflow(detachContext(ctx), run, repoPath, wf)
	return run, nil
}

func (m *Manager) launchWorkflowSession(ctx context.Context, opts LaunchOptions) (*Session, error) {
	if m.launchSession != nil {
		return m.launchSession(ctx, opts)
	}
	return m.Launch(ctx, opts)
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

func (m *Manager) executeWorkflow(ctx context.Context, run *WorkflowRun, repoPath string, wf WorkflowDef) {
	run.setStatus("running")

	remaining := make([]WorkflowStep, len(wf.Steps))
	copy(remaining, wf.Steps)
	completed := make(map[string]bool, len(wf.Steps))
	terminal := make(map[string]string, len(wf.Steps))
	runFailed := false

	for len(remaining) > 0 {
		var ready []WorkflowStep
		var pending []WorkflowStep

		for _, step := range remaining {
			blocked := false
			depsReady := true
			for _, dep := range step.DependsOn {
				if status := terminal[dep]; status == "failed" || status == "blocked" {
					blocked = true
					break
				}
				if !completed[dep] {
					depsReady = false
				}
			}
			if blocked {
				run.updateStep(step.Name, "blocked", func(result *WorkflowStepResult) {
					result.Error = "blocked by failed dependency"
					now := time.Now()
					result.EndedAt = &now
				})
				terminal[step.Name] = "blocked"
				runFailed = true
				continue
			}
			if depsReady {
				ready = append(ready, step)
				continue
			}
			pending = append(pending, step)
		}

		if len(ready) == 0 {
			run.setStatus("failed")
			return
		}

		for i := 0; i < len(ready); {
			if ready[i].Parallel {
				j := i
				for j < len(ready) && ready[j].Parallel {
					j++
				}
				outcomes := m.runWorkflowParallelGroup(ctx, run, repoPath, ready[i:j])
				for _, outcome := range outcomes {
					terminal[outcome.Name] = outcome.Status
					if outcome.Status == "completed" {
						completed[outcome.Name] = true
					} else {
						runFailed = true
					}
				}
				i = j
				continue
			}
			outcome := m.runWorkflowStep(ctx, run, repoPath, ready[i])
			terminal[outcome.Name] = outcome.Status
			if outcome.Status == "completed" {
				completed[outcome.Name] = true
			} else {
				runFailed = true
			}
			i++
		}

		remaining = pending
	}

	if runFailed {
		run.setStatus("failed")
		return
	}
	run.setStatus("completed")
}

type workflowStepOutcome struct {
	Name   string
	Status string
}

func (m *Manager) runWorkflowParallelGroup(ctx context.Context, run *WorkflowRun, repoPath string, steps []WorkflowStep) []workflowStepOutcome {
	var wg sync.WaitGroup
	outcomes := make(chan workflowStepOutcome, len(steps))

	for _, step := range steps {
		wg.Add(1)
		go func(step WorkflowStep) {
			defer wg.Done()
			outcomes <- m.runWorkflowStep(ctx, run, repoPath, step)
		}(step)
	}
	wg.Wait()
	close(outcomes)

	var result []workflowStepOutcome
	for outcome := range outcomes {
		result = append(result, outcome)
	}
	return result
}

func (m *Manager) runWorkflowStep(ctx context.Context, run *WorkflowRun, repoPath string, step WorkflowStep) workflowStepOutcome {
	provider := Provider(step.Provider)
	if provider == "" {
		provider = ProviderClaude
	}

	started := time.Now()
	run.updateStep(step.Name, "running", func(result *WorkflowStepResult) {
		result.Provider = provider
		result.StartedAt = &started
	})

	// Enhance workflow step prompt for its target provider
	prompt := step.Prompt
	var stepEnhance enhanceResult
	if m.Enhancer != nil {
		stepEnhance = m.enhanceForProvider(ctx, prompt, provider)
		prompt = stepEnhance.prompt
	} else {
		stepEnhance = enhanceResult{prompt: prompt, source: "none", preScore: 0}
	}

	opts := LaunchOptions{
		Provider: provider,
		RepoPath: repoPath,
		Prompt:   prompt,
		Model:    step.Model,
		Agent:    step.Agent,
	}

	sess, err := m.launchWorkflowSession(ctx, opts)
	if err != nil {
		run.updateStep(step.Name, "failed", func(result *WorkflowStepResult) {
			result.Provider = provider
			result.Error = err.Error()
			now := time.Now()
			result.EndedAt = &now
		})
		return workflowStepOutcome{Name: step.Name, Status: "failed"}
	}

	sess.EnhancementSource = stepEnhance.source
	sess.EnhancementPreScore = stepEnhance.preScore

	run.updateStep(step.Name, "running", func(result *WorkflowStepResult) {
		result.SessionID = sess.ID
		result.Provider = sess.Provider
	})

	if err := m.waitForSession(ctx, sess); err != nil {
		run.updateStep(step.Name, "failed", func(result *WorkflowStepResult) {
			result.SessionID = sess.ID
			result.Provider = sess.Provider
			result.Error = err.Error()
			now := time.Now()
			result.EndedAt = &now
		})
		return workflowStepOutcome{Name: step.Name, Status: "failed"}
	}

	run.updateStep(step.Name, "completed", func(result *WorkflowStepResult) {
		result.SessionID = sess.ID
		result.Provider = sess.Provider
		now := time.Now()
		result.EndedAt = &now
	})
	return workflowStepOutcome{Name: step.Name, Status: "completed"}
}

// effectiveSessionTimeout returns the session wait timeout, defaulting to 10 minutes.
func (m *Manager) effectiveSessionTimeout() time.Duration {
	if m.SessionTimeout <= 0 {
		return 10 * time.Minute
	}
	return m.SessionTimeout
}

// effectiveKillTimeout returns the SIGTERM→SIGKILL escalation timeout, defaulting to 5 seconds.
func (m *Manager) effectiveKillTimeout() time.Duration {
	if m.KillTimeout <= 0 {
		return 5 * time.Second
	}
	return m.KillTimeout
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

// PersistSession writes session state to the shared state directory.
// Safe to call from any goroutine; acquires the session lock.
func (m *Manager) PersistSession(s *Session) error {
	if m.stateDir == "" {
		return nil
	}
	if err := os.MkdirAll(m.stateDir, 0755); err != nil {
		return fmt.Errorf("persist session: mkdir: %w", err)
	}

	s.mu.Lock()
	data, err := json.Marshal(s)
	s.mu.Unlock()
	if err != nil {
		return fmt.Errorf("persist session: marshal: %w", err)
	}

	path := filepath.Join(m.stateDir, s.ID+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("persist session: write %s: %w", path, err)
	}
	return nil
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

// LoadExternalSessions reads session JSON files from the shared state directory
// and merges any unknown sessions into the manager. This allows the TUI to
// discover sessions launched by the MCP server (a separate process).
func (m *Manager) LoadExternalSessions() {
	if m.stateDir == "" {
		return
	}
	entries, err := os.ReadDir(m.stateDir)
	if err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		id := strings.TrimSuffix(entry.Name(), ".json")

		// If we already own this session (launched in-process), update the file
		// but don't overwrite in-memory state.
		if existing, ok := m.sessions[id]; ok {
			// Re-persist in-process sessions so disk stays current (best-effort).
			go func(s *Session) {
			m.persistOrWarn(s, "re-persist on load")
		}(existing)
			continue
		}

		data, err := os.ReadFile(filepath.Join(m.stateDir, entry.Name()))
		if err != nil {
			continue
		}

		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}

		// Skip sessions older than 24h
		cutoff := time.Now().Add(-24 * time.Hour)
		if !s.LaunchedAt.IsZero() && s.LaunchedAt.Before(cutoff) {
			continue
		}

		m.sessions[id] = &s
	}

	// Clean up completed sessions older than 24h from disk
	cutoff := time.Now().Add(-24 * time.Hour)
	for id, s := range m.sessions {
		s.mu.Lock()
		ended := s.EndedAt
		status := s.Status
		s.mu.Unlock()

		isTerminal := status == StatusCompleted || status == StatusErrored || status == StatusStopped
		if isTerminal && ended != nil && ended.Before(cutoff) {
			delete(m.sessions, id)
			if err := os.Remove(filepath.Join(m.stateDir, id+".json")); err != nil && !os.IsNotExist(err) {
				slog.Warn("failed to remove session state file", "session", id, "error", err)
			}
		}
	}
}

