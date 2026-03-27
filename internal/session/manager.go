package session

import (
	"context"
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
	noopDetector  *NoOpDetector          // WS2-noop: consecutive no-op iteration detection
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
		noopDetector: NewNoOpDetector(2),
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
		noopDetector: NewNoOpDetector(2),
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

