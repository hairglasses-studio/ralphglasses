package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Supervisor monitors health metrics, proposes decisions via DecisionLog,
// and executes them. It runs when autonomy level >= 2.
type Supervisor struct {
	mgr       *Manager
	decisions *DecisionLog
	monitor   *HealthMonitor
	chainer   *CycleChainer

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}

	RepoPath        string
	TickInterval    time.Duration // default 60s
	MaxConcurrent   int           // default 1
	CooldownBetween time.Duration // default 5m

	lastCycleLaunch time.Time
	tickCount       int
	startedAt       time.Time
}

// SupervisorState is persisted to .ralph/supervisor_state.json.
type SupervisorState struct {
	Running         bool      `json:"running"`
	RepoPath        string    `json:"repo_path"`
	LastCycleLaunch time.Time `json:"last_cycle_launch"`
	TickCount       int       `json:"tick_count"`
	StartedAt       time.Time `json:"started_at"`
}

// NewSupervisor creates a Supervisor with sensible defaults.
func NewSupervisor(mgr *Manager, repoPath string) *Supervisor {
	return &Supervisor{
		mgr:             mgr,
		decisions:        NewDecisionLog("", LevelAutoOptimize),
		RepoPath:        repoPath,
		TickInterval:    60 * time.Second,
		MaxConcurrent:   1,
		CooldownBetween: 5 * time.Minute,
	}
}

// SetDecisionLog replaces the decision log.
func (s *Supervisor) SetDecisionLog(dl *DecisionLog) { s.mu.Lock(); s.decisions = dl; s.mu.Unlock() }

// SetMonitor sets the health monitor.
func (s *Supervisor) SetMonitor(m *HealthMonitor) { s.mu.Lock(); s.monitor = m; s.mu.Unlock() }

// SetChainer sets the cycle chainer.
func (s *Supervisor) SetChainer(c *CycleChainer) { s.mu.Lock(); s.chainer = c; s.mu.Unlock() }

// Start launches the supervisor goroutine. Idempotent.
func (s *Supervisor) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	if s.RepoPath == "" {
		return fmt.Errorf("supervisor: RepoPath is required")
	}
	childCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.running = true
	s.startedAt = time.Now()
	s.done = make(chan struct{})
	go s.run(childCtx)
	return nil
}

// Stop cancels the supervisor and waits up to 5s.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	cancel, done := s.cancel, s.done
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			slog.Warn("supervisor: stop timed out")
		}
	}
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}

// Running returns whether the supervisor is active.
func (s *Supervisor) Running() bool { s.mu.Lock(); defer s.mu.Unlock(); return s.running }

// TickCount returns completed ticks.
func (s *Supervisor) TickCount() int { s.mu.Lock(); defer s.mu.Unlock(); return s.tickCount }

// Status returns a state snapshot.
func (s *Supervisor) Status() SupervisorState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return SupervisorState{
		Running: s.running, RepoPath: s.RepoPath,
		LastCycleLaunch: s.lastCycleLaunch, TickCount: s.tickCount, StartedAt: s.startedAt,
	}
}

func (s *Supervisor) run(ctx context.Context) {
	ticker := time.NewTicker(s.TickInterval)
	defer ticker.Stop()
	defer close(s.done)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

func (s *Supervisor) tick(ctx context.Context) {
	s.mu.Lock()
	monitor, chainer := s.monitor, s.chainer
	s.mu.Unlock()

	if signals := monitor.Evaluate(s.RepoPath); len(signals) > 0 {
		for _, sig := range signals {
			s.executeDecision(ctx, sig)
		}
	}
	if chainer != nil {
		if nextCycle, err := chainer.CheckAndChain(ctx, s.RepoPath); err != nil {
			slog.Warn("supervisor: chain check failed", "error", err)
		} else if nextCycle != nil && s.mgr != nil {
			go func() {
				if _, err := s.mgr.RunCycle(ctx, nextCycle.RepoPath, nextCycle.Name, nextCycle.Objective, nextCycle.SuccessCriteria, 3); err != nil {
					slog.Warn("supervisor: chained cycle failed", "error", err)
				}
			}()
		}
	}
	s.mu.Lock()
	s.tickCount++
	s.mu.Unlock()
	s.persistState()
}

func (s *Supervisor) executeDecision(ctx context.Context, signal HealthSignal) {
	decision := AutonomousDecision{
		Category:      signal.Category,
		RequiredLevel: LevelAutoOptimize,
		Rationale:     signal.Rationale,
		Action:        signal.SuggestedAction,
		Inputs: map[string]any{
			"metric": signal.Metric, "value": signal.Value, "threshold": signal.Threshold,
		},
	}
	s.mu.Lock()
	dl := s.decisions
	s.mu.Unlock()
	if dl == nil || !dl.Propose(decision) {
		return
	}
	switch signal.Category {
	case DecisionLaunch:
		s.launchCycle(ctx, signal)
	case DecisionBudgetAdjust:
		slog.Info("supervisor: budget adjustment deferred", "metric", signal.Metric)
	case DecisionSelfTest:
		slog.Info("supervisor: self-test requested (not yet wired)")
	case DecisionReflexion:
		slog.Info("supervisor: consolidation requested (not yet wired)")
	}
}

func (s *Supervisor) launchCycle(ctx context.Context, signal HealthSignal) {
	s.mu.Lock()
	elapsed := time.Since(s.lastCycleLaunch)
	cooldown, mgr, repoPath := s.CooldownBetween, s.mgr, s.RepoPath
	isFirst := s.lastCycleLaunch.IsZero()
	s.mu.Unlock()

	if !isFirst && elapsed < cooldown {
		slog.Debug("supervisor: cycle launch skipped (cooldown)", "elapsed", elapsed)
		return
	}
	objective := "Explore improvements from ROADMAP.md"
	if signal.Metric == "completion_rate" {
		objective = "Investigate and fix recent failures"
	}
	name := fmt.Sprintf("auto-%d", time.Now().Unix())
	if mgr != nil {
		go func() {
			if _, err := mgr.RunCycle(ctx, repoPath, name, objective, []string{"Tests pass", "No regressions"}, 3); err != nil {
				slog.Warn("supervisor: RunCycle failed", "error", err)
			}
		}()
	}
	s.mu.Lock()
	s.lastCycleLaunch = time.Now()
	s.mu.Unlock()
}

func (s *Supervisor) persistState() {
	s.mu.Lock()
	state := SupervisorState{
		Running: s.running, RepoPath: s.RepoPath,
		LastCycleLaunch: s.lastCycleLaunch, TickCount: s.tickCount, StartedAt: s.startedAt,
	}
	repoPath := s.RepoPath
	s.mu.Unlock()
	if repoPath == "" {
		return
	}
	dir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "supervisor_state.json"), data, 0644)
}
