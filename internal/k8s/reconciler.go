package k8s

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// SessionRegistry tracks active RalphSession resources and their last-known
// state. The reconciliation loop uses it to detect drift between desired and
// observed state without re-listing the entire API on every tick.
type SessionRegistry struct {
	mu       sync.RWMutex
	sessions map[string]*RalphSession // key: "namespace/name"
}

// NewSessionRegistry creates an empty session registry.
func NewSessionRegistry() *SessionRegistry {
	return &SessionRegistry{
		sessions: make(map[string]*RalphSession),
	}
}

func registryKey(namespace, name string) string {
	return namespace + "/" + name
}

// Get returns the cached session, or nil if not tracked.
func (r *SessionRegistry) Get(namespace, name string) *RalphSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sessions[registryKey(namespace, name)]
}

// Set stores or updates a session in the registry.
func (r *SessionRegistry) Set(session *RalphSession) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sessions[registryKey(session.Namespace, session.Name)] = session
}

// Delete removes a session from the registry.
func (r *SessionRegistry) Delete(namespace, name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.sessions, registryKey(namespace, name))
}

// List returns a snapshot of all tracked sessions.
func (r *SessionRegistry) List() []*RalphSession {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*RalphSession, 0, len(r.sessions))
	for _, s := range r.sessions {
		out = append(out, s)
	}
	return out
}

// Len returns the number of tracked sessions.
func (r *SessionRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.sessions)
}

// ActiveCount returns the number of sessions in Running or Launching phase.
func (r *SessionRegistry) ActiveCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, s := range r.sessions {
		switch s.Status.Phase {
		case "Running", "Launching":
			count++
		}
	}
	return count
}

// ReconcileLoop wraps a Reconciler with a continuous polling loop that
// periodically lists all RalphSession resources, updates the SessionRegistry,
// and reconciles each session. It provides Start/Stop lifecycle management
// suitable for running as a long-lived goroutine in an operator binary.
type ReconcileLoop struct {
	reconciler *Reconciler
	client     Client
	registry   *SessionRegistry
	interval   time.Duration
	namespace  string // empty string means all namespaces
	logger     *slog.Logger

	mu      sync.RWMutex
	running bool
	cancel  context.CancelFunc

	// stats tracks loop-level metrics for observability.
	stats LoopStats
}

// LoopStats holds counters for the reconciliation loop.
type LoopStats struct {
	mu             sync.Mutex
	TotalPasses    int64     `json:"totalPasses"`
	TotalErrors    int64     `json:"totalErrors"`
	LastPassTime   time.Time `json:"lastPassTime,omitempty"`
	LastErrorTime  time.Time `json:"lastErrorTime,omitempty"`
	LastError      string    `json:"lastError,omitempty"`
	SessionsSynced int64     `json:"sessionsSynced"`
}

// ReconcileLoopOption configures the ReconcileLoop.
type ReconcileLoopOption func(*ReconcileLoop)

// WithNamespace restricts the reconciliation loop to a single namespace.
func WithNamespace(ns string) ReconcileLoopOption {
	return func(rl *ReconcileLoop) {
		rl.namespace = ns
	}
}

// WithLogger sets a custom logger for the reconciliation loop.
func WithLogger(logger *slog.Logger) ReconcileLoopOption {
	return func(rl *ReconcileLoop) {
		rl.logger = logger
	}
}

// NewReconcileLoop creates a reconciliation loop that polls at the given interval.
// The client is used for listing sessions; the reconciler handles individual
// session reconciliation. The registry is updated on every pass to reflect
// the current cluster state.
func NewReconcileLoop(client Client, registry *SessionRegistry, interval time.Duration, opts ...ReconcileLoopOption) *ReconcileLoop {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if registry == nil {
		registry = NewSessionRegistry()
	}

	rl := &ReconcileLoop{
		reconciler: NewReconciler(client, nil),
		client:     client,
		registry:   registry,
		interval:   interval,
	}

	for _, opt := range opts {
		opt(rl)
	}

	if rl.logger == nil {
		rl.logger = slog.Default()
	}
	// Ensure the reconciler uses the same logger.
	rl.reconciler = NewReconciler(client, rl.logger)

	return rl
}

// Start begins the reconciliation loop in a blocking fashion. It returns when
// the context is cancelled or Stop is called. The first reconciliation pass
// runs immediately, then repeats at the configured interval.
func (rl *ReconcileLoop) Start(ctx context.Context) error {
	rl.mu.Lock()
	if rl.running {
		rl.mu.Unlock()
		return fmt.Errorf("reconcile loop already running")
	}
	ctx, cancel := context.WithCancel(ctx)
	rl.cancel = cancel
	rl.running = true
	rl.mu.Unlock()

	defer func() {
		rl.mu.Lock()
		rl.running = false
		rl.cancel = nil
		rl.mu.Unlock()
	}()

	rl.logger.Info("reconcile loop started",
		"interval", rl.interval,
		"namespace", rl.namespace,
	)

	// Run the first pass immediately.
	if err := rl.Reconcile(ctx); err != nil {
		rl.logger.Error("initial reconciliation pass failed", "error", err)
	}

	ticker := time.NewTicker(rl.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			rl.logger.Info("reconcile loop stopping", "reason", ctx.Err())
			return ctx.Err()
		case <-ticker.C:
			if err := rl.Reconcile(ctx); err != nil {
				rl.logger.Error("reconciliation pass failed", "error", err)
			}
		}
	}
}

// Reconcile performs a single full reconciliation pass: lists all sessions,
// updates the registry, and reconciles each one. This method is safe to call
// independently of Start for manual or test-driven reconciliation.
func (rl *ReconcileLoop) Reconcile(ctx context.Context) error {
	log := rl.logger.With("component", "reconcile-loop")

	sessionList, err := rl.client.ListSessions(ctx, rl.namespace)
	if err != nil {
		rl.recordError(err)
		return fmt.Errorf("list sessions: %w", err)
	}

	log.Debug("reconciliation pass starting",
		"sessionCount", len(sessionList.Items),
	)

	// Track which sessions we see in this pass for stale-entry cleanup.
	seen := make(map[string]bool, len(sessionList.Items))

	var reconcileErrors []error

	for i := range sessionList.Items {
		session := &sessionList.Items[i]
		key := registryKey(session.Namespace, session.Name)
		seen[key] = true

		// Update the registry with the latest state from the API.
		rl.registry.Set(session)

		// Reconcile this session.
		result, err := rl.reconciler.Reconcile(ctx, ReconcileRequest{
			Namespace: session.Namespace,
			Name:      session.Name,
		})
		if err != nil {
			log.Error("failed to reconcile session",
				"namespace", session.Namespace,
				"name", session.Name,
				"error", err,
			)
			reconcileErrors = append(reconcileErrors, fmt.Errorf("%s/%s: %w", session.Namespace, session.Name, err))
			continue
		}

		if result.Requeue {
			log.Debug("session requires requeue",
				"namespace", session.Namespace,
				"name", session.Name,
				"requeueAfter", result.RequeueAfter,
			)
		}

		rl.stats.mu.Lock()
		rl.stats.SessionsSynced++
		rl.stats.mu.Unlock()
	}

	// Remove stale entries from the registry (sessions that no longer exist
	// in the API but are still tracked).
	for _, s := range rl.registry.List() {
		key := registryKey(s.Namespace, s.Name)
		if !seen[key] {
			log.Info("removing stale session from registry",
				"namespace", s.Namespace,
				"name", s.Name,
			)
			rl.registry.Delete(s.Namespace, s.Name)
		}
	}

	rl.stats.mu.Lock()
	rl.stats.TotalPasses++
	rl.stats.LastPassTime = time.Now()
	rl.stats.mu.Unlock()

	if len(reconcileErrors) > 0 {
		rl.recordError(reconcileErrors[0])
		return fmt.Errorf("reconciliation pass had %d errors; first: %w", len(reconcileErrors), reconcileErrors[0])
	}

	log.Debug("reconciliation pass complete",
		"sessionCount", len(sessionList.Items),
		"activeCount", rl.registry.ActiveCount(),
	)

	return nil
}

// Stop cancels the reconciliation loop. It is safe to call multiple times.
func (rl *ReconcileLoop) Stop() error {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if !rl.running || rl.cancel == nil {
		return nil
	}

	rl.logger.Info("stopping reconcile loop")
	rl.cancel()
	return nil
}

// Running returns whether the loop is currently active.
func (rl *ReconcileLoop) Running() bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.running
}

// Registry returns the session registry used by this loop.
func (rl *ReconcileLoop) Registry() *SessionRegistry {
	return rl.registry
}

// Stats returns a snapshot of the loop's operational statistics.
func (rl *ReconcileLoop) Stats() LoopStats {
	rl.stats.mu.Lock()
	defer rl.stats.mu.Unlock()
	return LoopStats{
		TotalPasses:    rl.stats.TotalPasses,
		TotalErrors:    rl.stats.TotalErrors,
		LastPassTime:   rl.stats.LastPassTime,
		LastErrorTime:  rl.stats.LastErrorTime,
		LastError:      rl.stats.LastError,
		SessionsSynced: rl.stats.SessionsSynced,
	}
}

// recordError updates the error statistics.
func (rl *ReconcileLoop) recordError(err error) {
	rl.stats.mu.Lock()
	defer rl.stats.mu.Unlock()
	rl.stats.TotalErrors++
	rl.stats.LastErrorTime = time.Now()
	rl.stats.LastError = err.Error()
}
