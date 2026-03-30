// Package marathon provides a Go orchestrator for continuous improvement cycles
// with budget limits, duration limits, checkpointing, and resource monitoring.
package marathon

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/resource"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Config holds marathon orchestration parameters.
type Config struct {
	BudgetUSD          float64       // Maximum budget in USD.
	Duration           time.Duration // Maximum run duration.
	CheckpointInterval time.Duration // How often to save checkpoints.
	SessionCount       int           // Number of parallel sessions to maintain.
	RepoPath           string        // Target repository path.
	Resume             bool          // Resume from last checkpoint.

	// ResourceCheckInterval controls how often resource health is sampled.
	// Defaults to 60s when zero.
	ResourceCheckInterval time.Duration
}

// Validate returns an error if the Config has invalid or missing fields.
func (c Config) Validate() error {
	if c.Duration <= 0 {
		return fmt.Errorf("duration must be positive, got %s", c.Duration)
	}
	if c.CheckpointInterval <= 0 {
		return fmt.Errorf("checkpoint interval must be positive, got %s", c.CheckpointInterval)
	}
	if c.BudgetUSD < 0 {
		return fmt.Errorf("budget must be non-negative, got %f", c.BudgetUSD)
	}
	if c.SessionCount < 0 {
		return fmt.Errorf("session count must be non-negative, got %d", c.SessionCount)
	}
	if c.RepoPath == "" {
		return fmt.Errorf("repo path must be set")
	}
	return nil
}

// Stats summarizes a marathon run.
type Stats struct {
	CyclesCompleted int           `json:"cycles_completed"`
	TotalSpentUSD   float64       `json:"total_spent_usd"`
	Duration        time.Duration `json:"duration"`
	SessionsRun     int           `json:"sessions_run"`
}

// MarathonStatus provides a full snapshot of a running marathon's state,
// including active session count, elapsed time, and spend.
type MarathonStatus struct {
	Running        bool          `json:"running"`
	SessionsActive int           `json:"sessions_active"`
	Elapsed        time.Duration `json:"elapsed"`
	SpentUSD       float64       `json:"spent_usd"`
	BudgetUSD      float64       `json:"budget_usd"`
	CyclesCompleted int          `json:"cycles_completed"`
	SessionCount   int           `json:"session_count"`
}

// ErrBudgetExceeded is returned when the marathon's spend exceeds its budget.
var ErrBudgetExceeded = fmt.Errorf("marathon: budget limit exceeded")

// Marathon orchestrates continuous improvement cycles with budget and duration
// constraints, periodic checkpoints, and resource monitoring.
type Marathon struct {
	cfg    Config
	mgr    *session.Manager
	bus    *events.Bus
	sup    *session.Supervisor
	cancel context.CancelFunc // set during Run, used for budget-triggered shutdown

	mu        sync.Mutex
	startedAt time.Time
	stats     Stats
}

// New creates a Marathon with the given configuration.
func New(cfg Config, mgr *session.Manager, bus *events.Bus) *Marathon {
	if cfg.ResourceCheckInterval == 0 {
		cfg.ResourceCheckInterval = 60 * time.Second
	}
	if cfg.SessionCount <= 0 {
		cfg.SessionCount = 1
	}
	return &Marathon{
		cfg: cfg,
		mgr: mgr,
		bus: bus,
	}
}

// Run executes the marathon loop. It blocks until the budget or duration limit
// is reached, the context is cancelled, or an unrecoverable error occurs.
// It returns summary stats on completion.
func (m *Marathon) Run(ctx context.Context) (*Stats, error) {
	ctx, cancel := context.WithTimeout(ctx, m.cfg.Duration)
	defer cancel()

	m.mu.Lock()
	m.startedAt = time.Now()
	m.cancel = cancel
	m.mu.Unlock()

	// Create and configure supervisor.
	sup := session.NewSupervisor(m.mgr, m.cfg.RepoPath)
	sup.MaxTotalCostUSD = m.cfg.BudgetUSD
	sup.MaxDuration = m.cfg.Duration
	sup.SetBus(m.bus)
	sup.SetMonitor(session.NewHealthMonitor(session.DefaultHealthThresholds()))
	sup.SetChainer(session.NewCycleChainer())

	m.mu.Lock()
	m.sup = sup
	m.mu.Unlock()

	// Resume from previous state if requested.
	if m.cfg.Resume {
		if err := sup.ResumeFromState(); err != nil {
			slog.Warn("marathon: resume failed, starting fresh", "error", err)
		} else {
			// Load latest checkpoint to restore stats.
			cpDir := checkpointDir(m.cfg.RepoPath)
			if cp, err := LoadLatestCheckpoint(cpDir); err == nil {
				m.mu.Lock()
				m.stats.CyclesCompleted = cp.CyclesCompleted
				m.stats.TotalSpentUSD = cp.SpentUSD
				m.mu.Unlock()
			}
			slog.Info("marathon: resumed from previous state")
		}
	}

	// Start supervisor.
	if err := sup.Start(ctx); err != nil {
		return nil, fmt.Errorf("supervisor start: %w", err)
	}
	defer sup.Stop()

	slog.Info("marathon: supervisor started",
		"budget", m.cfg.BudgetUSD,
		"duration", m.cfg.Duration,
		"checkpoint_interval", m.cfg.CheckpointInterval,
	)

	// Subscribe to cost events for stats tracking.
	costCh := m.bus.SubscribeFiltered("marathon-cost", events.CostUpdate)
	defer m.bus.Unsubscribe("marathon-cost")

	// Subscribe to session events for session count tracking.
	sessCh := m.bus.SubscribeFiltered("marathon-sess", events.SessionStarted)
	defer m.bus.Unsubscribe("marathon-sess")

	// Checkpoint ticker.
	cpTicker := time.NewTicker(m.cfg.CheckpointInterval)
	defer cpTicker.Stop()

	// Resource monitoring ticker.
	resTicker := time.NewTicker(m.cfg.ResourceCheckInterval)
	defer resTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return m.finalize(), nil

		case <-cpTicker.C:
			m.saveCheckpoint()

		case <-resTicker.C:
			m.checkResources()

		case evt, ok := <-costCh:
			if !ok {
				continue
			}
			m.handleCostEvent(evt)

		case _, ok := <-sessCh:
			if !ok {
				continue
			}
			m.mu.Lock()
			m.stats.SessionsRun++
			m.mu.Unlock()
		}
	}
}

// Supervisor returns the underlying supervisor, or nil if Run has not been called.
func (m *Marathon) Supervisor() *session.Supervisor {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sup
}

// Status returns a full snapshot of the marathon's current state.
func (m *Marathon) Status() MarathonStatus {
	m.mu.Lock()
	s := m.stats
	started := m.startedAt
	sup := m.sup
	m.mu.Unlock()

	var elapsed time.Duration
	running := false
	sessActive := 0

	if !started.IsZero() {
		elapsed = time.Since(started)
		running = true
	}

	if sup != nil {
		st := sup.Status()
		running = st.Running
		s.CyclesCompleted += st.TickCount
	}

	// Count active sessions from the manager.
	if m.mgr != nil {
		for _, sess := range m.mgr.List(m.cfg.RepoPath) {
			if sess.Status == session.StatusRunning {
				sessActive++
			}
		}
	}

	return MarathonStatus{
		Running:         running,
		SessionsActive:  sessActive,
		Elapsed:         elapsed,
		SpentUSD:        s.TotalSpentUSD,
		BudgetUSD:       m.cfg.BudgetUSD,
		CyclesCompleted: s.CyclesCompleted,
		SessionCount:    m.cfg.SessionCount,
	}
}

// Checkpoint triggers a manual checkpoint save and returns any error.
func (m *Marathon) Checkpoint() error {
	m.mu.Lock()
	stats := m.stats
	m.mu.Unlock()

	supState := session.SupervisorState{}
	if sup := m.Supervisor(); sup != nil {
		supState = sup.Status()
	}

	cp := &Checkpoint{
		Timestamp:       time.Now(),
		CyclesCompleted: stats.CyclesCompleted + supState.TickCount,
		SpentUSD:        stats.TotalSpentUSD,
		SupervisorState: supState,
	}

	cpDir := checkpointDir(m.cfg.RepoPath)
	if err := SaveCheckpoint(cpDir, cp); err != nil {
		return fmt.Errorf("manual checkpoint: %w", err)
	}
	slog.Info("marathon: manual checkpoint saved", "time", cp.Timestamp.Format(time.RFC3339))
	return nil
}

// Stats returns a snapshot of current marathon statistics.
func (m *Marathon) CurrentStats() Stats {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.stats
	if !m.startedAt.IsZero() {
		s.Duration = time.Since(m.startedAt)
	}
	return s
}

func (m *Marathon) handleCostEvent(evt events.Event) {
	if evt.Data == nil {
		return
	}
	spentRaw, ok := evt.Data["spent_usd"]
	if !ok {
		return
	}
	spent, ok := spentRaw.(float64)
	if !ok {
		return
	}
	m.mu.Lock()
	if spent > m.stats.TotalSpentUSD {
		m.stats.TotalSpentUSD = spent
	}
	exceeded := m.cfg.BudgetUSD > 0 && m.stats.TotalSpentUSD >= m.cfg.BudgetUSD
	cancelFn := m.cancel
	m.mu.Unlock()

	if exceeded {
		slog.Warn("marathon: budget limit reached",
			"spent", spent, "budget", m.cfg.BudgetUSD)
		if cancelFn != nil {
			cancelFn()
		}
	}
}

func (m *Marathon) saveCheckpoint() {
	m.mu.Lock()
	stats := m.stats
	m.mu.Unlock()

	supState := session.SupervisorState{}
	if m.sup != nil {
		supState = m.sup.Status()
	}

	cp := &Checkpoint{
		Timestamp:       time.Now(),
		CyclesCompleted: stats.CyclesCompleted + supState.TickCount,
		SpentUSD:        stats.TotalSpentUSD,
		SupervisorState: supState,
	}

	cpDir := checkpointDir(m.cfg.RepoPath)
	if err := SaveCheckpoint(cpDir, cp); err != nil {
		slog.Warn("marathon: checkpoint save failed", "error", err)
	} else {
		slog.Info("marathon: checkpoint saved", "time", cp.Timestamp.Format(time.RFC3339))
	}
}

func (m *Marathon) checkResources() {
	status := resource.Check(m.cfg.RepoPath)
	if !status.IsHealthy() {
		for _, w := range status.Warnings {
			slog.Warn("marathon: resource warning", "warning", w)
		}
		if m.bus != nil {
			m.bus.Publish(events.Event{
				Type:     events.SessionError,
				RepoPath: m.cfg.RepoPath,
				Data:     map[string]any{"source": "marathon", "warnings": status.Warnings},
			})
		}
	}
}

func (m *Marathon) finalize() *Stats {
	// Save a final checkpoint.
	m.saveCheckpoint()

	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.startedAt.IsZero() {
		m.stats.Duration = time.Since(m.startedAt)
	}

	// Pull cycle count from supervisor if available.
	if m.sup != nil {
		st := m.sup.Status()
		m.stats.CyclesCompleted += st.TickCount
		if st.BudgetSpentUSD > m.stats.TotalSpentUSD {
			m.stats.TotalSpentUSD = st.BudgetSpentUSD
		}
	}

	slog.Info("marathon: finished",
		"cycles", m.stats.CyclesCompleted,
		"spent_usd", m.stats.TotalSpentUSD,
		"duration", m.stats.Duration,
		"sessions", m.stats.SessionsRun,
	)

	result := m.stats
	return &result
}

// checkpointDir returns the checkpoint storage directory for a repo.
func checkpointDir(repoPath string) string {
	return repoPath + "/.ralph/marathon/checkpoints"
}
