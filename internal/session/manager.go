package session

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hairglasses-studio/mcpkit/worktree"
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/fleet/pool"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

// DefaultStateDir is the shared directory for session state persistence.
const DefaultStateDir = "~/.ralphglasses/sessions"

// Manager tracks all active provider sessions and teams.
type Manager struct {
	sessionsMu  sync.RWMutex
	sessions    map[string]*Session // keyed by session ID
	statusCache sync.Map            // hot-read path: session ID -> SessionStatus (Phase 10.5.1)

	workersMu              sync.RWMutex
	teams                  map[string]*TeamStatus  // keyed by team name
	workflowRuns           map[string]*WorkflowRun // keyed by workflow run ID
	loops                  map[string]*LoopRun     // keyed by loop run ID
	totalPrunedThisSession int                     // counter for pruned runs this session

	teamBackend     StructuredTeamBackend      // structured team execution backend (fleet)
	teamControllers map[string]*teamController // active team controllers keyed by team name

	configMu                 sync.RWMutex
	compactionFailuresMu     sync.Mutex
	bus                      *events.Bus
	stateDir                 string                  // directory for persisted session JSON files
	optimizer                *AutoOptimizer          // Level 2+ self-improvement engine
	reflexion                *ReflexionStore         // WS1: failure reflection extraction
	episodic                 *EpisodicMemory         // WS2: successful trajectory memory
	cascade                  *CascadeRouter          // WS3: cheap-then-expensive provider routing
	curriculum               *CurriculumSorter       // WS5: task difficulty scoring and ordering
	banditSelect             func() (string, string) // bandit-based provider selection hook
	banditUpdate             func(string, float64)   // bandit reward recording hook
	blackboard               *Blackboard             // Phase H: shared inter-subsystem state
	costPredictor            *CostPredictor          // Phase H: task cost prediction
	noopDetector             *NoOpDetector           // WS2-noop: consecutive no-op iteration detection
	budgetEnforcer           *BudgetEnforcer         // WS5: secondary budget enforcement for loops
	depthEstimator           *DepthEstimator         // Phase 10.5.5: adaptive iteration depth
	resumeCompactionFailures map[string]int
	researchGateway          ResearchGateway // docs-backed research integration for supervisor ticks
	crashRecovery            *CrashRecoveryOrchestrator
	store                    Store // pluggable session persistence (default: MemoryStore)
	launchSession            func(context.Context, LaunchOptions) (*Session, error)
	waitSession              func(context.Context, *Session) error
	healthCheck              func(Provider) ProviderHealth // injectable health check (default: CheckProviderHealth)
	supervisor               *Supervisor                   // autonomous R&D supervisor, runs at level >= 2

	SessionTimeout     time.Duration          // timeout for waitForSession; 0 uses default (10m)
	KillTimeout        time.Duration          // SIGTERM→SIGKILL escalation timeout; 0 uses default (5s)
	ErrorRetention     time.Duration          // how long errored sessions remain queryable; 0 uses default (5m)
	MinSessionDuration time.Duration          // sessions younger than this are protected from reaper; 0 uses default (30s)
	Enhancer           *enhancer.HybridEngine // optional prompt enhancement for loop integration
	promptEvolution    *PromptEvolution       // tournament-based prompt variant selection
	FleetPool          *pool.State            // fleet-wide budget pooling and metrics aggregation
	worktreePool       *worktree.WorktreePool // Phase 10.5.8: reusable worktree pool
	automation         map[string]*SubscriptionAutomationController

	spendMonitor  *SpendRateMonitor        // hourly spend circuit breaker (nil = disabled)
	promptRouter  PromptRouter             // Prompt DJ quality-aware routing (nil = disabled)
	errorContexts map[string]*ErrorContext // per-session error context for LLM injection

	DefaultBudgetUSD float64 // from RALPH_SESSION_BUDGET; applied when Launch opts has no budget

	// WS-7: Loop engine hygiene — auto-prune and journal consolidation config.
	PruneRetention    time.Duration // max age for stale loop runs; 0 uses default (7 days)
	PruneMaxRuns      int           // unused currently; reserved for max run cap
	JournalMaxEntries int           // auto-consolidation threshold; 0 uses default (100)
}

// DefaultEstimatedSessionCost is the conservative per-launch cost estimate
// used by the CanSpend gate when no specific estimate is available.
const DefaultEstimatedSessionCost = 0.50

// NewManager creates a new session manager.
func NewManager() *Manager {
	stateDir := expandHome(DefaultStateDir)
	return &Manager{
		sessions:                 make(map[string]*Session),
		teams:                    make(map[string]*TeamStatus),
		workflowRuns:             make(map[string]*WorkflowRun),
		loops:                    make(map[string]*LoopRun),
		stateDir:                 stateDir,
		noopDetector:             NewNoOpDetector(2),
		budgetEnforcer:           NewBudgetEnforcer(),
		cascade:                  NewCascadeRouter(DefaultCascadeConfig(), nil, nil, stateDir),
		FleetPool:                pool.NewState(0), // 0 = unlimited by default
		automation:               make(map[string]*SubscriptionAutomationController),
		resumeCompactionFailures: make(map[string]int),
	}
}

// NewManagerWithBus creates a session manager wired to an event bus.
func NewManagerWithBus(bus *events.Bus) *Manager {
	stateDir := expandHome(DefaultStateDir)
	return &Manager{
		sessions:                 make(map[string]*Session),
		teams:                    make(map[string]*TeamStatus),
		workflowRuns:             make(map[string]*WorkflowRun),
		loops:                    make(map[string]*LoopRun),
		bus:                      bus,
		stateDir:                 stateDir,
		noopDetector:             NewNoOpDetector(2),
		budgetEnforcer:           NewBudgetEnforcer(),
		cascade:                  NewCascadeRouter(DefaultCascadeConfig(), nil, nil, stateDir),
		FleetPool:                pool.NewState(0),
		automation:               make(map[string]*SubscriptionAutomationController),
		resumeCompactionFailures: make(map[string]int),
	}
}

// NewManagerWithStore creates a session manager backed by the given Store.
// The in-memory sessions map is still used for active (in-process) sessions;
// the Store provides durable persistence.
func NewManagerWithStore(store Store, bus *events.Bus) *Manager {
	stateDir := expandHome(DefaultStateDir)
	return &Manager{
		sessions:                 make(map[string]*Session),
		teams:                    make(map[string]*TeamStatus),
		workflowRuns:             make(map[string]*WorkflowRun),
		loops:                    make(map[string]*LoopRun),
		bus:                      bus,
		store:                    store,
		stateDir:                 stateDir,
		noopDetector:             NewNoOpDetector(2),
		cascade:                  NewCascadeRouter(DefaultCascadeConfig(), nil, nil, stateDir),
		FleetPool:                pool.NewState(0),
		automation:               make(map[string]*SubscriptionAutomationController),
		resumeCompactionFailures: make(map[string]int),
	}
}

// Store returns the session store, or nil if none is configured.
func (m *Manager) Store() Store {
	return m.store
}

// SetStore sets the session store. Intended for late initialization or tests.
func (m *Manager) SetStore(store Store) {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	m.store = store
}

// Init performs one-time startup work after the Manager is constructed.
// It sweeps for orphaned processes from previous runs and logs them without
// killing them — the user should decide what to do.
// It also auto-prunes stale loop runs (WS-7: FINDING-267).
func (m *Manager) Init() {
	m.sessionsMu.RLock()
	activePIDs := make(map[int]bool)
	for _, s := range m.sessions {
		if s.Pid > 0 {
			activePIDs[s.Pid] = true
		}
	}
	m.sessionsMu.RUnlock()

	m.configMu.RLock()
	ralphDir := filepath.Dir(m.stateDir)
	m.configMu.RUnlock()

	orphans := SweepOrphans(ralphDir, activePIDs)
	if len(orphans) > 0 {
		slog.Warn("found orphaned processes from previous run", "count", len(orphans))
	}

	// QW-9: Restore persisted autonomy level on startup when the optimizer is available.
	m.RestoreAutonomyLevel()

	// Rehydrate persisted sessions from SQLite store so they survive restarts.
	if err := m.RehydrateFromStore(); err != nil {
		slog.Warn("failed to rehydrate sessions from store", "error", err)
	}

	// WS-7: Auto-prune stale loop runs on startup (non-blocking).
	go m.autoPruneLoopRuns()
}

// SetStateDir overrides the persistence directory. Intended for tests and
// alternate embedding environments that want to isolate on-disk state.
func (m *Manager) SetStateDir(dir string) {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	m.stateDir = dir
}

// ApplyConfig reads relevant settings from a .ralphrc config and applies them
// to the Manager. Handles KILL_ESCALATION_TIMEOUT, PRUNE_RETENTION_DAYS,
// and JOURNAL_MAX_ENTRIES.
func (m *Manager) ApplyConfig(cfg *model.RalphConfig) {
	if cfg == nil {
		return
	}
	if raw := cfg.Get("KILL_ESCALATION_TIMEOUT", ""); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 1 && v <= 60 {
			m.KillTimeout = time.Duration(v) * time.Second
		}
	}
	// WS-7: Configurable prune retention (days). Default 7 if unset.
	if raw := cfg.Get("PRUNE_RETENTION_DAYS", ""); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 1 && v <= 365 {
			m.PruneRetention = time.Duration(v) * 24 * time.Hour
		}
	}
	// WS-7: Configurable journal max entries before auto-consolidation. Default 100.
	if raw := cfg.Get("JOURNAL_MAX_ENTRIES", ""); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 10 && v <= 10000 {
			m.JournalMaxEntries = v
		}
	}
	// Default session budget from .ralphrc (2.5.3).
	if raw := cfg.Get("RALPH_SESSION_BUDGET", ""); raw != "" {
		if f, err := strconv.ParseFloat(raw, 64); err == nil && f > 0 {
			m.DefaultBudgetUSD = f
		}
	}
	// Fleet budget cap from config (0 = unlimited).
	if raw := cfg.Get("FLEET_BUDGET_CAP_USD", ""); raw != "" {
		if f, err := strconv.ParseFloat(raw, 64); err == nil && f >= 0 {
			if m.FleetPool != nil {
				m.FleetPool.SetBudgetCap(f)
				slog.Info("fleet budget cap configured", "cap_usd", f)
			}
		}
	}

	// Hourly spend circuit breaker.
	if raw := cfg.Get("HOURLY_SPEND_THRESHOLD_USD", ""); raw != "" {
		if f, err := strconv.ParseFloat(raw, 64); err == nil && f > 0 {
			m.spendMonitor = NewSpendRateMonitor(f)
			if m.bus != nil {
				m.spendMonitor.SubscribeToBus(context.Background(), m.bus)
			}
			slog.Info("hourly spend breaker configured", "threshold_usd", f)
		}
	}

	// QW-2: Cascade routing is enabled by default in constructors.
	// Allow explicit config to disable it, or re-configure with custom values.
	cascadeVal := strings.ToLower(strings.TrimSpace(cfg.Get("CASCADE_ENABLED", "true")))
	if cascadeVal == "false" || cascadeVal == "0" || cascadeVal == "no" {
		// Explicitly disabled — remove the default cascade router.
		m.SetCascadeRouter(nil)
		slog.Info("cascade routing explicitly disabled via config")
	} else if !m.HasCascadeRouter() {
		// Not yet configured (shouldn't happen with new constructors, but defensive).
		cascadeCfg := DefaultCascadeConfig()
		cr := NewCascadeRouter(cascadeCfg, nil, nil, m.stateDir)
		m.SetCascadeRouter(cr)
		slog.Info("cascade routing enabled by default")
	}
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

// autoPruneLoopRuns prunes stale loop runs (pending/failed) older than the
// configured retention period. Safe to call from a goroutine.
func (m *Manager) autoPruneLoopRuns() {
	loopDir := m.LoopStateDir()
	if loopDir == "" {
		return
	}

	retention := m.PruneRetention
	if retention == 0 {
		retention = 7 * 24 * time.Hour // default: 7 days
	}

	pruned, err := PruneLoopRuns(loopDir, retention, []string{"pending", "failed"}, false)
	if err != nil {
		slog.Warn("auto-prune loop runs failed", "error", err)
		return
	}
	if pruned > 0 {
		slog.Info("auto-pruned stale loop runs on startup", "pruned", pruned, "retention", retention.String())
		m.workersMu.Lock()
		m.totalPrunedThisSession += pruned
		m.workersMu.Unlock()
	}
}

// TotalPrunedThisSession returns how many loop runs have been pruned since
// the Manager was created (WS-7 metric).
func (m *Manager) TotalPrunedThisSession() int {
	m.workersMu.RLock()
	defer m.workersMu.RUnlock()
	return m.totalPrunedThisSession
}

// WorktreePool returns the worktree pool, or nil if none is configured.
func (m *Manager) WorktreePool() *worktree.WorktreePool {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	return m.worktreePool
}

// SetWorktreePool sets the worktree pool for reuse across sessions.
func (m *Manager) SetWorktreePool(pool *worktree.WorktreePool) {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	m.worktreePool = pool
}

// ConsecutiveNoOps returns the current consecutive no-op iteration count
// for the given loop ID. Returns 0 if no detector is configured.
func (m *Manager) ConsecutiveNoOps(loopID string) int {
	if m.noopDetector == nil {
		return 0
	}
	return m.noopDetector.ConsecutiveCount(loopID)
}

// SetAutonomyLevel changes the autonomy level and starts/stops the supervisor.
func (m *Manager) SetAutonomyLevel(level AutonomyLevel, repoPath string) {
	m.configMu.Lock()
	if m.optimizer != nil && m.optimizer.decisions != nil {
		m.optimizer.decisions.SetLevel(level)
	}

	// Start or stop supervisor based on level.
	if level >= LevelAutoOptimize {
		m.startSupervisor(repoPath)
	} else {
		m.stopSupervisor()
	}
	m.configMu.Unlock()
}

// startSupervisor creates and starts the autonomous R&D supervisor.
// Must be called with m.configMu held.
func (m *Manager) startSupervisor(repoPath string) {
	if m.supervisor != nil && m.supervisor.Running() {
		return // already running
	}
	m.supervisor = NewSupervisor(m, repoPath)
	m.supervisor.monitor = NewHealthMonitor(DefaultHealthThresholds())
	m.supervisor.chainer = NewCycleChainer()
	m.supervisor.bus = m.bus
	m.supervisor.SetSubscriptionAutomation(m.ensureSubscriptionAutomationLocked(repoPath))
	if m.optimizer != nil {
		m.supervisor.decisions = m.optimizer.decisions
		m.supervisor.optimizer = m.optimizer
	}
	if m.researchGateway != nil {
		rd := NewResearchDaemon(m.researchGateway, DefaultResearchDaemonConfig())
		rd.SetBus(m.bus)
		if m.optimizer != nil && m.optimizer.decisions != nil {
			rd.SetDecisionLog(m.optimizer.decisions)
		}
		if m.cascade != nil {
			rd.SetRouter(m.cascade)
		}
		m.supervisor.SetResearchDaemon(rd)
	}
	if m.crashRecovery != nil {
		m.supervisor.SetCrashRecovery(m.crashRecovery)
	}
	ctx := context.Background()
	m.supervisor.Start(ctx)
}

// stopSupervisor stops the supervisor if running.
// Must be called with m.configMu held.
func (m *Manager) stopSupervisor() {
	if m.supervisor != nil {
		m.supervisor.Stop()
		m.supervisor = nil
	}
}

// SupervisorStatus returns the current supervisor state, or nil if not running.
func (m *Manager) SupervisorStatus() *SupervisorState {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	if m.supervisor == nil {
		return nil
	}
	state := m.supervisor.Status()
	return &state
}

func (m *Manager) ensureSubscriptionAutomationLocked(repoPath string) *SubscriptionAutomationController {
	if repoPath == "" {
		return nil
	}
	if m.automation == nil {
		m.automation = make(map[string]*SubscriptionAutomationController)
	}
	if ctrl, ok := m.automation[repoPath]; ok {
		return ctrl
	}
	ctrl := NewSubscriptionAutomationController(m, repoPath)
	m.automation[repoPath] = ctrl
	return ctrl
}

func (m *Manager) EnsureSubscriptionAutomation(repoPath string) *SubscriptionAutomationController {
	m.configMu.Lock()
	defer m.configMu.Unlock()
	return m.ensureSubscriptionAutomationLocked(repoPath)
}

func (m *Manager) SubscriptionAutomationStatus(repoPath string) *AutomationStatusSnapshot {
	if repoPath == "" {
		m.configMu.RLock()
		if m.supervisor != nil {
			repoPath = m.supervisor.RepoPath
		}
		m.configMu.RUnlock()
	}
	if repoPath == "" {
		return nil
	}
	ctrl := m.EnsureSubscriptionAutomation(repoPath)
	if ctrl == nil {
		return nil
	}
	snapshot := ctrl.Status()
	return &snapshot
}

// RunWorkflow validates and starts a workflow asynchronously.
func (m *Manager) RunWorkflow(ctx context.Context, repoPath string, wf WorkflowDef) (*WorkflowRun, error) {
	if err := ValidateWorkflow(wf); err != nil {
		return nil, err
	}

	run := newWorkflowRun(repoPath, wf)

	m.workersMu.Lock()
	m.workflowRuns[run.ID] = run
	m.workersMu.Unlock()

	go m.executeWorkflow(detachContext(ctx), run, repoPath, wf)
	return run, nil
}

func (m *Manager) launchWorkflowSession(ctx context.Context, opts LaunchOptions) (*Session, error) {
	if m.launchSession != nil {
		return m.launchSession(ctx, opts)
	}
	return m.Launch(ctx, opts)
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
		provider = DefaultPrimaryProvider()
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

// RefreshFleetState updates the fleet pool state from current session data.
// Intended to be called periodically from the TUI tick or a manager goroutine.
func (m *Manager) RefreshFleetState() {
	if m.FleetPool == nil {
		return
	}
	snapshots := m.SnapshotSessions()
	m.FleetPool.Update(snapshots)
}

// SnapshotSessions creates read-only snapshots of all sessions for fleet aggregation.
// Each session is locked briefly to copy fields, then unlocked.
func (m *Manager) SnapshotSessions() []pool.SessionSnapshot {
	m.sessionsMu.RLock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessionsMu.RUnlock()

	snaps := make([]pool.SessionSnapshot, 0, len(sessions))
	for _, s := range sessions {
		s.Lock()
		snap := pool.SessionSnapshot{
			ID:        s.ID,
			Provider:  string(s.Provider),
			Status:    string(s.Status),
			SpentUSD:  s.SpentUSD,
			BudgetUSD: s.BudgetUSD,
			RepoPath:  s.RepoPath,
			StartedAt: s.LaunchedAt,
		}
		s.Unlock()
		snaps = append(snaps, snap)
	}
	return snaps
}
