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
	RepoPath           string        // Target repository path.
	Resume             bool          // Resume from last checkpoint.

	// ResourceCheckInterval controls how often resource health is sampled.
	// Defaults to 60s when zero.
	ResourceCheckInterval time.Duration
}

// Stats summarizes a marathon run.
type Stats struct {
	CyclesCompleted int           `json:"cycles_completed"`
	TotalSpentUSD   float64       `json:"total_spent_usd"`
	Duration        time.Duration `json:"duration"`
	SessionsRun     int           `json:"sessions_run"`
}

// Marathon orchestrates continuous improvement cycles with budget and duration
// constraints, periodic checkpoints, and resource monitoring.
type Marathon struct {
	cfg  Config
	mgr  *session.Manager
	bus  *events.Bus
	sup  *session.Supervisor

	mu        sync.Mutex
	startedAt time.Time
	stats     Stats
}

// New creates a Marathon with the given configuration.
func New(cfg Config, mgr *session.Manager, bus *events.Bus) *Marathon {
	if cfg.ResourceCheckInterval == 0 {
		cfg.ResourceCheckInterval = 60 * time.Second
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
	m.mu.Unlock()
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
