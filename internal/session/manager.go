package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// DefaultStateDir is the shared directory for session state persistence.
const DefaultStateDir = "~/.ralphglasses/sessions"

// Manager tracks all active Claude Code sessions and teams.
type Manager struct {
	mu            sync.Mutex
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
	m.mu.Lock()
	activePIDs := make(map[int]bool)
	for _, s := range m.sessions {
		if s.Pid > 0 {
			activePIDs[s.Pid] = true
		}
	}
	ralphDir := filepath.Dir(m.stateDir)
	m.mu.Unlock()

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

// SetAutoOptimizer attaches the self-improvement engine (Level 2+).
// When set, Launch will consult FeedbackAnalyzer for provider and budget
// suggestions, and session completion will feed back into profiles.
func (m *Manager) SetAutoOptimizer(opt *AutoOptimizer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.optimizer = opt
}

// SetReflexionStore attaches the reflexion loop subsystem.
func (m *Manager) SetReflexionStore(rs *ReflexionStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reflexion = rs
}

// SetEpisodicMemory attaches the episodic memory subsystem.
func (m *Manager) SetEpisodicMemory(em *EpisodicMemory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.episodic = em
}

// SetCascadeRouter attaches the cascade routing subsystem.
// If bandit hooks are already configured, they are forwarded to the new router.
func (m *Manager) SetCascadeRouter(cr *CascadeRouter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cascade = cr
	if cr != nil && m.banditSelect != nil {
		cr.SetBanditHooks(m.banditSelect, m.banditUpdate)
	}
}

// SetCurriculumSorter attaches the curriculum learning subsystem.
func (m *Manager) SetCurriculumSorter(cs *CurriculumSorter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.curriculum = cs
}

// HasReflexion returns true if a ReflexionStore is already attached.
func (m *Manager) HasReflexion() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reflexion != nil
}

// HasEpisodicMemory returns true if an EpisodicMemory is already attached.
func (m *Manager) HasEpisodicMemory() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.episodic != nil
}

// GetEpisodicMemory returns the attached EpisodicMemory, or nil.
func (m *Manager) GetEpisodicMemory() *EpisodicMemory {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.episodic
}

// HasCascadeRouter returns true if a CascadeRouter is already attached.
func (m *Manager) HasCascadeRouter() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cascade != nil
}

// HasCurriculumSorter returns true if a CurriculumSorter is already attached.
func (m *Manager) HasCurriculumSorter() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.curriculum != nil
}

// SetBanditHooks attaches bandit-based provider selection functions to the manager
// and forwards them to the cascade router if one is attached.
func (m *Manager) SetBanditHooks(selectFn func() (string, string), updateFn func(string, float64)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.banditSelect = selectFn
	m.banditUpdate = updateFn
	if m.cascade != nil {
		m.cascade.SetBanditHooks(selectFn, updateFn)
	}
}

// HasBandit returns true if bandit hooks have been configured.
func (m *Manager) HasBandit() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.banditSelect != nil
}

// GetCascadeRouter returns the attached CascadeRouter, or nil.
func (m *Manager) GetCascadeRouter() *CascadeRouter {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cascade
}

// SetBlackboard attaches the shared blackboard subsystem.
func (m *Manager) SetBlackboard(bb *Blackboard) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blackboard = bb
}

// HasBlackboard returns true if a Blackboard is already attached.
func (m *Manager) HasBlackboard() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.blackboard != nil
}

// GetBlackboard returns the attached Blackboard, or nil.
func (m *Manager) GetBlackboard() *Blackboard {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.blackboard
}

// SetCostPredictor attaches the cost prediction subsystem.
func (m *Manager) SetCostPredictor(cp *CostPredictor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.costPredictor = cp
}

// HasCostPredictor returns true if a CostPredictor is already attached.
func (m *Manager) HasCostPredictor() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.costPredictor != nil
}

// GetCostPredictor returns the attached CostPredictor, or nil.
func (m *Manager) GetCostPredictor() *CostPredictor {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.costPredictor
}

// GetReflexionStore returns the attached ReflexionStore, or nil.
func (m *Manager) GetReflexionStore() *ReflexionStore {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.reflexion
}

// SetHooksForTesting overrides session launch/wait behavior. Intended for tests.
func (m *Manager) SetHooksForTesting(
	launch func(context.Context, LaunchOptions) (*Session, error),
	wait func(context.Context, *Session) error,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.launchSession = launch
	m.waitSession = wait
}

// SetHealthCheckForTesting overrides the provider health check function.
// Intended for tests that need to control health check results.
func (m *Manager) SetHealthCheckForTesting(fn func(Provider) ProviderHealth) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthCheck = fn
}

// checkHealth returns the health of a provider, using the injectable function
// if set, otherwise falling back to CheckProviderHealth.
func (m *Manager) checkHealth(p Provider) ProviderHealth {
	m.mu.Lock()
	fn := m.healthCheck
	m.mu.Unlock()
	if fn != nil {
		return fn(p)
	}
	return CheckProviderHealth(p)
}

// AddSessionForTesting inserts a pre-built session into the manager. Intended for tests.
func (m *Manager) AddSessionForTesting(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
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
	m.mu.Lock()
	optimizer := m.optimizer
	m.mu.Unlock()
	if optimizer != nil {
		opts, _ = optimizer.OptimizedLaunchOptions(opts)
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
		m.bus.Publish(events.Event{
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
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	return s, ok
}

// List returns all sessions, optionally filtered by repo path.
func (m *Manager) List(repoPath string) []*Session {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*Session
	for _, s := range m.sessions {
		if repoPath != "" && s.RepoPath != repoPath {
			continue
		}
		result = append(result, s)
	}
	return result
}

// killWithEscalation sends SIGTERM, waits up to timeout, then sends SIGKILL if still alive.
// Returns true if SIGKILL was needed.
//
// The done channel should be closed when the process has exited (typically by the
// runner goroutine that calls cmd.Wait()). If done is nil, killWithEscalation
// spawns its own Wait() goroutine internally.
func killWithEscalation(cmd *exec.Cmd, timeout time.Duration, done <-chan struct{}) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}

	// If no external done channel, create one by calling Wait() ourselves.
	if done == nil {
		ch := make(chan struct{})
		go func() {
			_ = cmd.Wait()
			close(ch)
		}()
		done = ch
	}

	// Send SIGTERM to process group.
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
	} else {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
	}

	// Wait for the process to exit or timeout.
	select {
	case <-done:
		return false
	case <-time.After(timeout):
		// Escalate to SIGKILL.
		if pgid > 0 {
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
		} else {
			_ = cmd.Process.Kill()
		}
		return true
	}
}

// Stop gracefully stops a running session.
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	s, ok := m.sessions[id]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}

	s.mu.Lock()

	if s.Status != StatusRunning && s.Status != StatusLaunching {
		s.mu.Unlock()
		return fmt.Errorf("session %s is not running (status: %s)", id, s.Status)
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
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id, s := range m.sessions {
		s.mu.Lock()
		if s.Status == StatusRunning || s.Status == StatusLaunching {
			ids = append(ids, id)
		}
		s.mu.Unlock()
	}
	m.mu.Unlock()

	for _, id := range ids {
		_ = m.Stop(id)
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
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []*Session
	for _, s := range m.sessions {
		if filepath.Base(s.RepoPath) == repoName {
			result = append(result, s)
		}
	}
	return result
}

// LaunchTeam creates an agent team by launching a lead session with team env vars.
func (m *Manager) LaunchTeam(ctx context.Context, config TeamConfig) (*TeamStatus, error) {
	if config.Name == "" {
		return nil, fmt.Errorf("team name required")
	}
	if config.RepoPath == "" {
		return nil, fmt.Errorf("repo path required")
	}
	if len(config.Tasks) == 0 {
		return nil, fmt.Errorf("at least one task required")
	}

	// Build a lead prompt that instructs the lead to use agent teams
	var taskList string
	for i, t := range config.Tasks {
		taskList += fmt.Sprintf("%d. %s\n", i+1, t)
	}

	workerProvider := config.WorkerProvider
	if workerProvider == "" {
		workerProvider = config.Provider
	}
	if workerProvider == "" {
		workerProvider = ProviderClaude
	}

	leadPrompt := fmt.Sprintf(
		`You are a team lead coordinating work on this project.

## Tasks to delegate

%s
## MCP Tools available

- ralphglasses_session_launch — Launch a worker session (required: repo, prompt; optional: provider, model, max_budget_usd, agent, system_prompt)
- ralphglasses_session_status — Check a worker's progress (required: session_id)
- ralphglasses_session_list — List all sessions (optional: repo, provider, status filters)
- ralphglasses_session_stop — Stop a stuck/completed worker (required: session_id)

## Provider capabilities

| Parameter       | claude (all) | gemini         | codex          |
|-----------------|-------------|----------------|----------------|
| prompt          | yes         | yes            | yes            |
| model           | yes         | yes            | yes            |
| resume          | yes         | yes            | no             |
| system_prompt   | yes         | no (ignored)   | no (ignored)   |
| max_budget_usd  | yes         | no (ignored)   | no (ignored)   |
| agent           | yes         | no (ignored)   | no (ignored)   |
| allowed_tools   | yes         | no (ignored)   | no (ignored)   |

## Workflow

1. Launch worker sessions with ralphglasses_session_launch (provider=%q)
2. Poll status with ralphglasses_session_status every 30-60 seconds
3. Stop stuck workers with ralphglasses_session_stop if no progress
4. Verify completed work by reading output from session_status
5. Report final status summarizing all task outcomes

Default worker provider: %s.
Provider strengths: claude (complex architecture), gemini (fast bulk generation), codex (focused refactoring).`,
		taskList, workerProvider, workerProvider,
	)

	// Enhance team lead prompt for its target provider
	var leadEnhance enhanceResult
	if m.Enhancer != nil {
		leadProvider := config.Provider
		if leadProvider == "" {
			leadProvider = ProviderClaude
		}
		leadEnhance = m.enhanceForProvider(ctx, leadPrompt, leadProvider)
		leadPrompt = leadEnhance.prompt
	} else {
		leadEnhance = enhanceResult{prompt: leadPrompt, source: "none", preScore: 0}
	}

	opts := LaunchOptions{
		Provider:     config.Provider,
		RepoPath:     config.RepoPath,
		Prompt:       leadPrompt,
		Model:        config.Model,
		MaxBudgetUSD: config.MaxBudgetUSD,
		Agent:        config.LeadAgent,
		TeamName:     config.Name,
		AllowedTools: teamLeadAllowedTools(),
	}

	s, err := m.Launch(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("launch team lead: %w", err)
	}
	s.EnhancementSource = leadEnhance.source
	s.EnhancementPreScore = leadEnhance.preScore

	tasks := make([]TeamTask, len(config.Tasks))
	for i, desc := range config.Tasks {
		tasks[i] = TeamTask{
			Description: desc,
			Status:      "pending",
		}
	}

	team := &TeamStatus{
		Name:     config.Name,
		RepoPath: config.RepoPath,
		LeadID:   s.ID,
		Status:   StatusRunning,
		Tasks:    tasks,
	}

	m.mu.Lock()
	m.teams[config.Name] = team
	m.mu.Unlock()

	if m.bus != nil {
		m.bus.Publish(events.Event{
			Type:      events.TeamCreated,
			SessionID: s.ID,
			RepoPath:  config.RepoPath,
			RepoName:  filepath.Base(config.RepoPath),
			Provider:  string(config.Provider),
			Data:      map[string]any{"team": config.Name, "tasks": len(config.Tasks)},
		})
	}

	return team, nil
}

// teamLeadAllowedTools returns the MCP tools a team lead session needs
// to autonomously launch and monitor worker sessions.
func teamLeadAllowedTools() []string {
	return []string{
		"mcp__ralphglasses__ralphglasses_session_launch",
		"mcp__ralphglasses__ralphglasses_session_status",
		"mcp__ralphglasses__ralphglasses_session_list",
		"mcp__ralphglasses__ralphglasses_session_stop",
		"mcp__ralphglasses__ralphglasses_session_output",
	}
}

// DelegateTask appends a task to a team under the manager mutex.
// Returns the updated task count.
func (m *Manager) DelegateTask(teamName string, task TeamTask) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	team, ok := m.teams[teamName]
	if !ok {
		return 0, fmt.Errorf("team not found: %s", teamName)
	}
	team.Tasks = append(team.Tasks, task)
	return len(team.Tasks), nil
}

// GetTeam returns team status by name.
// It also correlates task statuses from worker sessions.
func (m *Manager) GetTeam(name string) (*TeamStatus, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	team, ok := m.teams[name]
	if !ok {
		return nil, false
	}

	// Update team status based on lead session
	if s, sOk := m.sessions[team.LeadID]; sOk {
		s.mu.Lock()
		team.Status = s.Status
		s.mu.Unlock()
	}

	// Correlate task statuses from worker sessions.
	// Workers launched by the lead have TeamName set and their prompt
	// contains the task description as a substring.
	m.correlateTaskStatuses(team)

	return team, true
}

// correlateTaskStatuses updates task statuses by matching worker sessions.
// Must be called with m.mu held.
func (m *Manager) correlateTaskStatuses(team *TeamStatus) {
	// Collect worker sessions for this team (excluding the lead)
	var workers []*Session
	for _, s := range m.sessions {
		if s.TeamName == team.Name && s.ID != team.LeadID {
			workers = append(workers, s)
		}
	}

	for i := range team.Tasks {
		task := &team.Tasks[i]
		if task.Status == "completed" || task.Status == "errored" {
			continue // terminal states don't change
		}
		for _, w := range workers {
			if !strings.Contains(w.Prompt, task.Description) {
				continue
			}
			w.mu.Lock()
			ws := w.Status
			w.mu.Unlock()
			switch ws {
			case StatusRunning, StatusLaunching:
				task.Status = "in-progress"
			case StatusCompleted:
				task.Status = "completed"
			case StatusErrored:
				task.Status = "errored"
			case StatusStopped:
				task.Status = "errored"
			}
			break // first match wins
		}
	}
}

// updateTeamOnSessionEnd checks if the completed session is a team lead
// and transitions the team status accordingly.
func (m *Manager) updateTeamOnSessionEnd(sess *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, team := range m.teams {
		if team.LeadID != sess.ID {
			continue
		}
		sess.mu.Lock()
		team.Status = sess.Status
		sess.mu.Unlock()

		// Mark pending tasks as cancelled if lead exited without delegating
		if team.Status == StatusCompleted || team.Status == StatusErrored || team.Status == StatusStopped {
			for i := range team.Tasks {
				if team.Tasks[i].Status == "pending" {
					team.Tasks[i].Status = "cancelled"
				}
			}
		}
		break
	}
}

// ListTeams returns all teams.
func (m *Manager) ListTeams() []*TeamStatus {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]*TeamStatus, 0, len(m.teams))
	for _, t := range m.teams {
		result = append(result, t)
	}
	return result
}

// GetWorkflowRun returns a workflow run by ID.
func (m *Manager) GetWorkflowRun(id string) (*WorkflowRun, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
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
			return true, fmt.Errorf("session %s errored", s.ID)
		case StatusStopped:
			if exitReason != "" {
				return true, errors.New(exitReason)
			}
			return true, fmt.Errorf("session %s stopped", s.ID)
		}
		return false, nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return fmt.Errorf("waitForSession: timed out after %s waiting for session %s", timeout, s.ID)
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
			return fmt.Errorf("session %s process exited unexpectedly with status %s", s.ID, status)
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
	m.mu.Lock()
	s, ok := m.sessions[sessionID]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}

	s.mu.Lock()
	if s.Status != StatusRunning && s.Status != StatusLaunching {
		s.mu.Unlock()
		return nil, fmt.Errorf("session %s is not running (status: %s)", sessionID, s.Status)
	}
	if s.Provider == targetProvider {
		s.mu.Unlock()
		return nil, fmt.Errorf("session %s is already on provider %s", sessionID, targetProvider)
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
			_ = os.Remove(filepath.Join(m.stateDir, id+".json"))
		}
	}
}

// HITLSnapshot returns the current HITL score over a 24h window.
// Returns nil if no AutoOptimizer is configured.
func (m *Manager) HITLSnapshot() *HITLSnapshot {
	m.mu.Lock()
	opt := m.optimizer
	m.mu.Unlock()
	if opt == nil || opt.hitl == nil {
		return nil
	}
	snap := opt.hitl.CurrentScore(24 * time.Hour)
	return &snap
}

// FeedbackProfiles returns all prompt profiles from the feedback analyzer.
// Returns nil if no AutoOptimizer is configured.
func (m *Manager) FeedbackProfiles() []PromptProfile {
	m.mu.Lock()
	opt := m.optimizer
	m.mu.Unlock()
	if opt == nil || opt.feedback == nil {
		return nil
	}
	return opt.feedback.AllPromptProfiles()
}

// ProviderProfiles returns all provider profiles from the feedback analyzer.
// Returns nil if no AutoOptimizer is configured.
func (m *Manager) ProviderProfiles() []ProviderProfile {
	m.mu.Lock()
	opt := m.optimizer
	m.mu.Unlock()
	if opt == nil || opt.feedback == nil {
		return nil
	}
	return opt.feedback.AllProviderProfiles()
}

// RecentDecisions returns the last n autonomous decisions.
// Returns nil if no AutoOptimizer is configured.
func (m *Manager) RecentDecisions(n int) []AutonomousDecision {
	m.mu.Lock()
	opt := m.optimizer
	m.mu.Unlock()
	if opt == nil || opt.decisions == nil {
		return nil
	}
	return opt.decisions.Recent(n)
}

// GetAutonomyLevel returns the current autonomy level.
// Returns LevelObserve if no AutoOptimizer is configured.
func (m *Manager) GetAutonomyLevel() AutonomyLevel {
	m.mu.Lock()
	opt := m.optimizer
	m.mu.Unlock()
	if opt == nil || opt.decisions == nil {
		return LevelObserve
	}
	return opt.decisions.Level()
}
