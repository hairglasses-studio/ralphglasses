package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// Supervisor monitors health metrics, proposes decisions via DecisionLog,
// and executes them. It runs when autonomy level >= 2.
type Supervisor struct {
	mgr       *Manager
	decisions *DecisionLog
	monitor   *HealthMonitor
	chainer   *CycleChainer
	optimizer *AutoOptimizer

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}

	RepoPath        string
	TickInterval    time.Duration // default 60s
	MaxConcurrent   int           // default 1
	CooldownBetween time.Duration // default 5m

	// Termination conditions (0 = unlimited).
	MaxCycles       int
	MaxTotalCostUSD float64
	MaxDuration     time.Duration

	bus             *events.Bus
	lastCycleLaunch time.Time
	cyclesLaunched  int
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

// SetOptimizer sets the auto-optimizer for note generation and application.
func (s *Supervisor) SetOptimizer(o *AutoOptimizer) { s.mu.Lock(); s.optimizer = o; s.mu.Unlock() }

// SetBus sets the event bus for publishing supervisor events.
func (s *Supervisor) SetBus(b *events.Bus) { s.mu.Lock(); s.bus = b; s.mu.Unlock() }

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

	// Enable the E2E test gate at L2+ so auto-optimizer changes are validated.
	GateEnabled = true

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
	if reason := s.shouldTerminate(); reason != "" {
		slog.Info("supervisor: termination condition met", "reason", reason)
		s.publishEvent(events.LoopStopped, map[string]any{
			"source": "supervisor", "reason": reason,
		})
		if s.cancel != nil {
			s.cancel()
		}
		return
	}

	s.mu.Lock()
	monitor, chainer := s.monitor, s.chainer
	s.mu.Unlock()

	if signals := monitor.Evaluate(s.RepoPath); len(signals) > 0 {
		for _, sig := range signals {
			s.executeDecision(ctx, sig)
		}
		s.publishEvent(events.AutoOptimized, map[string]any{
			"source": "supervisor", "signal_count": len(signals),
		})
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

// shouldTerminate checks if any termination condition is met.
func (s *Supervisor) shouldTerminate() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.MaxCycles > 0 && s.cyclesLaunched >= s.MaxCycles {
		return fmt.Sprintf("max_cycles reached (%d)", s.MaxCycles)
	}
	if s.MaxDuration > 0 && !s.startedAt.IsZero() && time.Since(s.startedAt) >= s.MaxDuration {
		return fmt.Sprintf("max_duration elapsed (%s)", s.MaxDuration)
	}
	if s.MaxTotalCostUSD > 0 {
		obsPath := filepath.Join(s.RepoPath, ".ralph", "cost_observations.json")
		if obs, err := LoadObservations(obsPath, s.startedAt); err == nil {
			var total float64
			for _, o := range obs {
				total += o.TotalCostUSD
			}
			if total >= s.MaxTotalCostUSD {
				return fmt.Sprintf("budget exhausted ($%.2f >= $%.2f)", total, s.MaxTotalCostUSD)
			}
		}
	}
	return ""
}

// publishEvent sends an event to the bus if one is configured.
func (s *Supervisor) publishEvent(typ events.EventType, data map[string]any) {
	s.mu.Lock()
	bus, repoPath := s.bus, s.RepoPath
	s.mu.Unlock()
	if bus == nil {
		return
	}
	bus.Publish(events.Event{
		Type:     typ,
		RepoPath: repoPath,
		Data:     data,
	})
}

func (s *Supervisor) executeDecision(ctx context.Context, signal HealthSignal) {
	decision := AutonomousDecision{
		ID:            fmt.Sprintf("dec-%d", time.Now().UnixNano()),
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
		s.launchCycle(ctx, signal, decision.ID)
	case DecisionBudgetAdjust:
		slog.Info("supervisor: budget adjustment advisory",
			"metric", signal.Metric, "value", signal.Value, "threshold", signal.Threshold)
		s.publishEvent(events.AutoOptimized, map[string]any{
			"source": "supervisor", "action": "budget_advisory",
			"metric": signal.Metric, "value": signal.Value,
		})
	case DecisionSelfTest:
		s.runSelfTest(ctx)
	case DecisionReflexion:
		s.runConsolidation()
	}
}

func (s *Supervisor) launchCycle(ctx context.Context, signal HealthSignal, decisionID string) {
	s.mu.Lock()
	elapsed := time.Since(s.lastCycleLaunch)
	cooldown, mgr, repoPath := s.CooldownBetween, s.mgr, s.RepoPath
	dl := s.decisions
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
			_, err := mgr.RunCycle(ctx, repoPath, name, objective, []string{"Tests pass", "No regressions"}, 3)
			outcome := DecisionOutcome{
				EvaluatedAt: time.Now(),
				Success:     err == nil,
			}
			if err != nil {
				outcome.Details = err.Error()
				slog.Warn("supervisor: RunCycle failed", "error", err)
			} else {
				outcome.Details = "cycle completed"
			}
			if dl != nil && decisionID != "" {
				dl.RecordOutcome(decisionID, outcome)
			}
		}()
	}
	s.mu.Lock()
	s.lastCycleLaunch = time.Now()
	s.cyclesLaunched++
	s.mu.Unlock()
	s.publishEvent(events.LoopStarted, map[string]any{
		"source": "supervisor", "cycle": name, "objective": objective,
	})
}

// runSelfTest runs go test with recursion guard and captures coverage.
func (s *Supervisor) runSelfTest(ctx context.Context) {
	if err := RecursionGuard(); err != nil {
		slog.Warn("supervisor: self-test blocked by recursion guard", "error", err)
		return
	}
	testCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()

	coverOut := filepath.Join(s.RepoPath, ".ralph", "coverage.out")
	cmd := exec.CommandContext(testCtx, "go", "test", "./...", "-count=1",
		"-coverprofile="+coverOut)
	cmd.Dir = s.RepoPath
	cmd.Env = SetSelfTestEnv(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("supervisor: self-test failed", "error", err, "output", string(out))
	} else {
		slog.Info("supervisor: self-test passed")
	}

	// Write coverage percentage so HealthMonitor can read it.
	if pct, parseErr := ParseCoveragePercent(coverOut); parseErr == nil {
		covPath := filepath.Join(s.RepoPath, ".ralph", "coverage.txt")
		_ = os.WriteFile(covPath, []byte(fmt.Sprintf("%.1f\n", pct)), 0644)
		slog.Info("supervisor: coverage captured", "percent", pct)
	}
}

// runConsolidation consolidates journal patterns, generates improvement notes,
// and auto-applies eligible ones — closing the reflexion feedback loop.
func (s *Supervisor) runConsolidation() {
	if err := ConsolidatePatterns(s.RepoPath); err != nil {
		slog.Warn("supervisor: consolidation failed", "error", err)
		return
	}
	slog.Info("supervisor: patterns consolidated")

	s.mu.Lock()
	opt := s.optimizer
	s.mu.Unlock()
	if opt == nil {
		return
	}

	// Load consolidated patterns and generate improvement notes.
	patternsPath := filepath.Join(s.RepoPath, ".ralph", "improvement_patterns.json")
	data, err := os.ReadFile(patternsPath)
	if err != nil {
		slog.Debug("supervisor: read patterns for notes", "error", err)
		return
	}
	var patterns ConsolidatedPatterns
	if err := json.Unmarshal(data, &patterns); err != nil {
		slog.Warn("supervisor: parse patterns", "error", err)
		return
	}

	notes := opt.GenerateNotes(&patterns)
	for _, note := range notes {
		if err := WriteImprovementNote(s.RepoPath, note); err != nil {
			slog.Warn("supervisor: write note", "error", err, "note", note.Title)
		}
	}
	if len(notes) > 0 {
		slog.Info("supervisor: improvement notes generated", "count", len(notes))
	}

	// Auto-apply eligible pending notes.
	applied, rejected, err := opt.ApplyPendingNotes(s.RepoPath)
	if err != nil {
		slog.Warn("supervisor: apply notes failed", "error", err)
	} else if applied > 0 || rejected > 0 {
		slog.Info("supervisor: notes applied", "applied", applied, "rejected", rejected)
		s.publishEvent(events.AutoOptimized, map[string]any{
			"source": "supervisor", "action": "apply_notes",
			"applied": applied, "rejected": rejected,
		})
	}
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
