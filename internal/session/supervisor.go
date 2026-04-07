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
	mgr          *Manager
	decisions    *DecisionLog
	monitor      *HealthMonitor
	chainer      *CycleChainer
	optimizer    *AutoOptimizer
	stallHandler *SupervisorStallHandler
	gates        *SupervisorGates
	planner      *SprintPlanner
	budget       *BudgetEnvelope

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
	wg      sync.WaitGroup

	RepoPath        string
	TickInterval    time.Duration // default 60s
	MaxConcurrent   int           // default 1
	CooldownBetween time.Duration // default 5m

	// Termination conditions (0 = unlimited).
	MaxCycles       int
	MaxTotalCostUSD float64
	MaxDuration     time.Duration

	bus                 *events.Bus
	lastCycleLaunch     time.Time
	cyclesLaunched      int
	tickCount           int
	startedAt           time.Time
	consecutiveFailures int

	// Passive background research daemon (ticks on its own internal schedule).
	researchDaemon *ResearchDaemon
	automation     *SubscriptionAutomationController

	// Crash recovery: detects dead Claude Code sessions and orchestrates resume.
	crashRecovery       *CrashRecoveryOrchestrator
	crashCheckWindow    time.Duration // default 4h
	crashCheckThreshold int           // default 2
	lastCrashCheck      time.Time
	crashCheckInterval  time.Duration // default 5m — don't check every tick
}

// SupervisorState is persisted to .ralph/supervisor_state.json.
type SupervisorState struct {
	Running              bool                      `json:"running"`
	BudgetSpentUSD       float64                   `json:"budget_spent_usd,omitempty"`
	RepoPath             string                    `json:"repo_path"`
	LastCycleLaunch      time.Time                 `json:"last_cycle_launch"`
	TickCount            int                       `json:"tick_count"`
	StartedAt            time.Time                 `json:"started_at"`
	Automation           *AutomationStatusSnapshot `json:"automation,omitempty"`
	ResearchDaemonActive bool                      `json:"research_daemon_active,omitempty"`
	CrashRecoveryActive  bool                      `json:"crash_recovery_active,omitempty"`
	CrashRecoveryPolicy  *CrashRecoveryPolicy      `json:"crash_recovery_policy,omitempty"`
}

// NewSupervisor creates a Supervisor with sensible defaults.
func NewSupervisor(mgr *Manager, repoPath string) *Supervisor {
	return &Supervisor{
		mgr:             mgr,
		decisions:       NewDecisionLog("", LevelAutoOptimize),
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

// SetStallHandler sets the stall detection handler.
func (s *Supervisor) SetStallHandler(h *SupervisorStallHandler) {
	s.mu.Lock()
	s.stallHandler = h
	s.mu.Unlock()
}

// SetGates sets the acceptance gates for post-cycle validation.
func (s *Supervisor) SetGates(g *SupervisorGates) { s.mu.Lock(); s.gates = g; s.mu.Unlock() }

// SetSprintPlanner sets the sprint planner for automatic roadmap-driven sprint planning.
func (s *Supervisor) SetSprintPlanner(sp *SprintPlanner) { s.mu.Lock(); s.planner = sp; s.mu.Unlock() }

// SetBudget sets the budget envelope for real-time cost tracking.
func (s *Supervisor) SetBudget(b *BudgetEnvelope) { s.mu.Lock(); s.budget = b; s.mu.Unlock() }

// SetResearchDaemon attaches the passive research daemon to the supervisor tick loop.
func (s *Supervisor) SetResearchDaemon(rd *ResearchDaemon) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.researchDaemon = rd
}

// SetSubscriptionAutomation attaches the subscription-window automation controller.
func (s *Supervisor) SetSubscriptionAutomation(ctrl *SubscriptionAutomationController) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.automation = ctrl
}

// SetCrashRecovery attaches a crash recovery orchestrator to the supervisor tick loop.
func (s *Supervisor) SetCrashRecovery(cr *CrashRecoveryOrchestrator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.crashRecovery = cr
	if s.crashCheckWindow == 0 {
		s.crashCheckWindow = 4 * time.Hour
	}
	if s.crashCheckThreshold == 0 {
		s.crashCheckThreshold = 2
	}
	if s.crashCheckInterval == 0 {
		s.crashCheckInterval = 5 * time.Minute
	}
}

// checkForClaudeCrash runs crash detection if enough time has passed since last check.
func (s *Supervisor) checkForClaudeCrash(ctx context.Context) {
	s.mu.Lock()
	cr := s.crashRecovery
	window := s.crashCheckWindow
	threshold := s.crashCheckThreshold
	interval := s.crashCheckInterval
	lastCheck := s.lastCrashCheck
	s.mu.Unlock()

	if cr == nil {
		return
	}
	if time.Since(lastCheck) < interval {
		return
	}

	plan, err := cr.DetectCrash(ctx, window, threshold)
	if err != nil {
		slog.Warn("supervisor: crash detection failed", "error", err)
		return
	}

	s.mu.Lock()
	s.lastCrashCheck = time.Now()
	s.mu.Unlock()

	if plan.DeadCount >= threshold {
		slog.Info("supervisor: Claude Code crash detected",
			"dead", plan.DeadCount,
			"alive", plan.AliveCount,
			"severity", plan.Severity,
			"recoverable", len(plan.SessionsToResume),
		)

		if s.bus != nil {
			s.bus.Publish(events.Event{
				Type:      events.SessionRecovered,
				Timestamp: time.Now(),
				Data: map[string]any{
					"action":   "crash_detected",
					"dead":     plan.DeadCount,
					"severity": plan.Severity,
				},
			})
		}

		// Auto-execute recovery if policy allows.
		policy := cr.Policy()
		if !policy.ShouldAutoExecute(plan.Severity) {
			slog.Info("supervisor: crash recovery policy does not allow auto-execute",
				"severity", plan.Severity, "policy_enabled", policy.Enabled)
			return
		}

		// Check cooldown.
		if policy.CooldownAfterRecovery > 0 && time.Since(cr.LastRecovery()) < policy.CooldownAfterRecovery {
			slog.Info("supervisor: crash recovery cooldown active")
			return
		}

		// Propose decision via DecisionLog.
		s.mu.Lock()
		dl := s.decisions
		s.mu.Unlock()

		if dl != nil {
			decision := AutonomousDecision{
				ID:            fmt.Sprintf("rec-%d", time.Now().UnixNano()),
				Category:      DecisionRestart,
				RequiredLevel: LevelAutoRecover,
				Rationale:     fmt.Sprintf("crash detected: %d dead sessions, severity=%s", plan.DeadCount, plan.Severity),
				Action:        fmt.Sprintf("auto-recover %d sessions", len(plan.SessionsToResume)),
			}
			if !dl.Propose(decision) {
				slog.Info("supervisor: crash recovery decision not approved by autonomy level")
				return
			}

			// Create budget envelope and execute.
			budget := NewRecoveryBudgetEnvelope(policy.MaxAutoRecoveryCost, policy.PerSessionBudget)
			cr.SetBudget(budget)

			maxConcurrent := policy.MaxConcurrent
			if maxConcurrent <= 0 {
				maxConcurrent = 1
			}

			s.wg.Go(func() {
				err := cr.ExecuteRecovery(ctx, plan, maxConcurrent)

				outcome := DecisionOutcome{
					EvaluatedAt: time.Now(),
					Success:     err == nil,
				}
				if err != nil {
					outcome.Details = err.Error()
					if cr.FailedRecoveries() >= policy.EscalationThreshold {
						slog.Error("supervisor: crash recovery escalation threshold reached",
							"failures", cr.FailedRecoveries(),
							"threshold", policy.EscalationThreshold)
						s.publishEvent(events.EmergencyStop, map[string]any{
							"source": "crash_recovery",
							"reason": "escalation_threshold",
						})
					}
				} else {
					outcome.Details = fmt.Sprintf("recovered %d sessions, spent $%.2f",
						len(plan.SessionsToResume), budget.SpentUSD)
				}
				dl.RecordOutcome(decision.ID, outcome)
			})
		}
	}
}

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
	GateEnabled.Store(true)

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
	s.wg.Wait()
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
	budget := s.budget
	st := SupervisorState{
		Running:              s.running,
		RepoPath:             s.RepoPath,
		LastCycleLaunch:      s.lastCycleLaunch,
		TickCount:            s.tickCount,
		StartedAt:            s.startedAt,
		ResearchDaemonActive: s.researchDaemon != nil,
		CrashRecoveryActive:  s.crashRecovery != nil,
	}
	automation := s.automation
	crashRecovery := s.crashRecovery
	s.mu.Unlock()
	if budget != nil {
		st.BudgetSpentUSD = budget.Spent()
	}
	if automation != nil {
		snapshot := automation.Status()
		st.Automation = &snapshot
	}
	if crashRecovery != nil {
		policy := crashRecovery.Policy()
		st.CrashRecoveryPolicy = &policy
	}
	return st
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

	// Check for Claude Code session crashes (rate-limited internally).
	s.checkForClaudeCrash(ctx)

	// Check for stalled sessions before health evaluation.
	s.mu.Lock()
	stallHandler := s.stallHandler
	s.mu.Unlock()
	if stallHandler != nil && s.mgr != nil {
		if killed := stallHandler.CheckAndHandle(ctx, s.mgr, s.bus, s.RepoPath); len(killed) > 0 {
			slog.Info("supervisor: killed stalled sessions", "count", len(killed), "ids", killed)
		}
	}

	s.mu.Lock()
	monitor, chainer, planner := s.monitor, s.chainer, s.planner
	mgr, repoPath := s.mgr, s.RepoPath
	s.mu.Unlock()

	if signals := monitor.Evaluate(s.RepoPath); len(signals) > 0 {
		for _, sig := range signals {
			s.executeDecision(ctx, sig)
		}
		s.publishEvent(events.AutoOptimized, map[string]any{
			"source": "supervisor", "signal_count": len(signals),
		})
	}

	var chainedCycle *CycleRun
	if chainer != nil {
		if nextCycle, err := chainer.CheckAndChain(ctx, s.RepoPath); err != nil {
			slog.Warn("supervisor: chain check failed", "error", err)
		} else if nextCycle != nil && mgr != nil {
			chainedCycle = nextCycle
			s.wg.Go(func() {
				if _, err := mgr.RunCycle(ctx, nextCycle.RepoPath, nextCycle.Name, nextCycle.Objective, nextCycle.SuccessCriteria, 3); err != nil {
					slog.Warn("supervisor: chained cycle failed", "error", err)
				}
			})
		}
	}

	// If chainer did not produce a cycle, try sprint planner.
	if planner != nil && chainedCycle == nil && mgr != nil {
		if planned := planner.PlanNextSprint(repoPath); planned != nil {
			slog.Info("supervisor: sprint planner produced cycle",
				"name", planned.Name, "tasks", len(planned.Tasks))
			s.wg.Go(func() {
				if _, err := mgr.RunCycle(ctx, planned.RepoPath, planned.Name, planned.Objective, planned.SuccessCriteria, len(planned.Tasks)); err != nil {
					slog.Warn("supervisor: planned sprint failed", "error", err)
				}
			})
			s.mu.Lock()
			s.lastCycleLaunch = time.Now()
			s.cyclesLaunched++
			s.mu.Unlock()
			s.publishEvent(events.LoopStarted, map[string]any{
				"source": "sprint_planner", "cycle": planned.Name, "objective": planned.Objective,
			})
		}
	}

	// Research daemon — runs on its own internal schedule (every Nth tick).
	s.mu.Lock()
	rd := s.researchDaemon
	s.mu.Unlock()
	if rd != nil {
		rd.Tick(ctx)
	}

	s.mu.Lock()
	automation := s.automation
	s.mu.Unlock()
	if automation != nil {
		automation.Tick(ctx)
	}

	s.mu.Lock()
	s.tickCount++
	s.mu.Unlock()
	s.persistState()

	// Cross-subsystem feedback loop (runs every 10 ticks).
	s.RunFeedbackLoop()
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
	if s.budget != nil && !s.budget.CanSpend(0) {
		return fmt.Sprintf("budget exhausted ($%.2f spent of $%.2f)", s.budget.Spent(), s.budget.TotalBudgetUSD)
	}
	if s.budget == nil && s.MaxTotalCostUSD > 0 {
		// Fallback: file-polling cost check when no budget envelope is set.
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
	budget := s.budget
	s.mu.Unlock()

	if !isFirst && elapsed < cooldown {
		slog.Debug("supervisor: cycle launch skipped (cooldown)", "elapsed", elapsed)
		return
	}
	if budget != nil && !budget.CanSpend(budget.PerCycleCap()) {
		slog.Info("supervisor: cycle launch skipped (budget)", "remaining", budget.Remaining())
		return
	}
	objective := "Explore improvements from ROADMAP.md"
	if signal.Metric == "completion_rate" {
		objective = "Investigate and fix recent failures"
	}
	name := fmt.Sprintf("auto-%d", time.Now().Unix())
	if mgr != nil {
		s.wg.Go(func() {
			_, err := mgr.RunCycle(ctx, repoPath, name, objective, []string{"Tests pass", "No regressions"}, 3)
			outcome := DecisionOutcome{
				EvaluatedAt: time.Now(),
				Success:     err == nil,
			}
			if err != nil {
				outcome.Details = err.Error()
				s.mu.Lock()
				s.consecutiveFailures++
				failures := s.consecutiveFailures
				s.mu.Unlock()
				slog.Error("supervisor: RunCycle failed", "error", err, "consecutive_failures", failures)
				if failures >= 3 {
					slog.Error("supervisor: 3 consecutive RunCycle failures — consider demoting autonomy level",
						"consecutive_failures", failures)
				}
			} else {
				// Reset consecutive failure counter on success.
				s.mu.Lock()
				s.consecutiveFailures = 0
				gates := s.gates
				s.mu.Unlock()
				if gates != nil {
					findings, passed := gates.Evaluate(ctx, repoPath)
					if !passed {
						outcome.Details = fmt.Sprintf("cycle completed but %d gate(s) failed", len(findings))
						slog.Warn("supervisor: gate failures", "count", len(findings))
						s.recordGateFindings(repoPath, findings)
					} else {
						outcome.Details = "cycle completed, all gates passed"
					}
				} else {
					outcome.Details = "cycle completed"
				}
				outcome.Success = true
			}
			if dl != nil && decisionID != "" {
				dl.RecordOutcome(decisionID, outcome)
			}
		})
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
		_ = os.WriteFile(covPath, fmt.Appendf(nil, "%.1f\n", pct), 0644)
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
	budget := s.budget
	repoPath := s.RepoPath
	s.mu.Unlock()
	if budget != nil {
		state.BudgetSpentUSD = budget.Spent()
	}
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

// ResumeFromState reads .ralph/supervisor_state.json and restores state.
func (s *Supervisor) ResumeFromState() error {
	statePath := filepath.Join(s.RepoPath, ".ralph", "supervisor_state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}
	var state SupervisorState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("parse state: %w", err)
	}
	s.mu.Lock()
	s.tickCount = state.TickCount
	s.lastCycleLaunch = state.LastCycleLaunch
	// Don't restore startedAt — this is a new run.
	s.mu.Unlock()
	slog.Info("supervisor: resumed state", "tick_count", state.TickCount, "last_launch", state.LastCycleLaunch)
	return nil
}

// recordGateFindings writes gate findings to .ralph/gate_findings.json.
func (s *Supervisor) recordGateFindings(repoPath string, findings []CycleFinding) {
	dir := filepath.Join(repoPath, ".ralph")
	_ = os.MkdirAll(dir, 0755)
	data, err := json.MarshalIndent(findings, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "gate_findings.json"), data, 0644)
}
