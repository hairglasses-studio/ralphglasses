package fleet

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tracing"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

const (
	DefaultPort         = 9473
	HeartbeatInterval   = 30 * time.Second
	StaleThreshold      = 90 * time.Second
	DisconnectThreshold = 5 * time.Minute
	ClaimTimeout        = 5 * time.Minute
)

// Coordinator manages the fleet from a single node.
type Coordinator struct {
	mu       sync.RWMutex
	nodeID   string
	hostname string
	port     int
	version  string

	workers map[string]*WorkerInfo // keyed by worker ID
	queue   *WorkQueue
	budget  GlobalBudget
	bus     *events.Bus
	sessMgr *session.Manager

	// Subsystems wired in Phase B1
	health  *HealthTracker
	budgetMgr *BudgetManager
	router  Router
	retries *RetryTracker

	startedAt time.Time
	server    *http.Server
}

// NewCoordinator creates a coordinator node.
func NewCoordinator(nodeID, hostname string, port int, version string, bus *events.Bus, sessMgr *session.Manager) *Coordinator {
	return &Coordinator{
		nodeID:    nodeID,
		hostname:  hostname,
		port:      port,
		version:   version,
		workers:   make(map[string]*WorkerInfo),
		queue:     NewWorkQueue(),
		budget:    GlobalBudget{LimitUSD: 500},
		bus:       bus,
		sessMgr:   sessMgr,
		health:    NewHealthTracker(DefaultHealthConfig()),
		budgetMgr: NewBudgetManager(10.0),
		router:    &LeastLoadedRouter{},
		retries:   NewRetryTracker(DefaultRetryPolicy()),
		startedAt: time.Now(),
	}
}

// SetBudgetLimit sets the fleet-wide budget ceiling.
func (c *Coordinator) SetBudgetLimit(limit float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.budget.LimitUSD = limit
}

// Start begins the HTTP server and maintenance goroutines.
func (c *Coordinator) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/register", c.handleRegister)
	mux.HandleFunc("POST /api/v1/heartbeat", c.handleHeartbeat)
	mux.HandleFunc("POST /api/v1/work/poll", c.handleWorkPoll)
	mux.HandleFunc("POST /api/v1/work/complete", c.handleWorkComplete)
	mux.HandleFunc("POST /api/v1/work/submit", c.handleWorkSubmit)
	mux.HandleFunc("POST /api/v1/events/batch", c.handleEventBatch)
	mux.HandleFunc("GET /api/v1/events", c.handleEventStream)
	mux.HandleFunc("GET /api/v1/status", c.handleStatus)
	mux.HandleFunc("GET /api/v1/fleet", c.handleFleetState)
	mux.HandleFunc("GET /api/v1/sessions", c.handleSessions)
	mux.HandleFunc("GET /healthz", c.handleHealthz)

	// A2A AgentCard discovery endpoint
	mux.HandleFunc("GET /.well-known/agent.json", c.handleAgentCard)
	mux.HandleFunc("GET /api/v1/a2a/task/{taskID}", c.handleA2ATaskStatus)

	// Prometheus metrics endpoint
	if promRec, ok := tracing.Get().(*tracing.PrometheusRecorder); ok {
		mux.HandleFunc("GET /metrics", promRec.Handler())
	}

	c.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", c.port),
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}

	// Maintenance: expire stale workers and reclaim timed-out work
	go c.maintenanceLoop(ctx)

	util.Debug.Debugf("coordinator starting on :%d", c.port)
	if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully shuts down the coordinator.
func (c *Coordinator) Stop(ctx context.Context) error {
	if c.server != nil {
		return c.server.Shutdown(ctx)
	}
	return nil
}

// maintenanceLoop periodically checks for stale workers and timed-out assignments.
func (c *Coordinator) maintenanceLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.expireWorkers()
			c.reclaimTimedOut()
		}
	}
}

func (c *Coordinator) expireWorkers() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for _, w := range c.workers {
		// Preserve manually-set statuses (paused, draining)
		if w.Status == WorkerPaused || w.Status == WorkerDraining {
			continue
		}
		since := now.Sub(w.LastHeartbeat)
		switch {
		case since > DisconnectThreshold:
			w.Status = WorkerDisconnected
		case since > StaleThreshold:
			w.Status = WorkerStale
		default:
			w.Status = WorkerOnline
		}
	}
}

func (c *Coordinator) reclaimTimedOut() {
	c.queue.ReclaimTimedOut(ClaimTimeout)
}

// SubmitWork adds a work item to the queue (for internal use).
func (c *Coordinator) SubmitWork(item *WorkItem) error {
	if item.ID == "" {
		item.ID = generateID()
	}
	if item.Status == "" {
		item.Status = WorkPending
	}
	item.SubmittedAt = time.Now()
	if item.MaxRetries == 0 {
		item.MaxRetries = 2
	}

	c.mu.RLock()
	avail := c.budget.AvailableBudget()
	c.mu.RUnlock()

	if item.MaxBudgetUSD > 0 && item.MaxBudgetUSD > avail {
		return fmt.Errorf("insufficient budget: need $%.2f, available $%.2f", item.MaxBudgetUSD, avail)
	}

	c.queue.Push(item)

	if item.MaxBudgetUSD > 0 {
		c.mu.Lock()
		c.budget.ReservedUSD += item.MaxBudgetUSD
		c.budget.LastUpdated = time.Now()
		c.mu.Unlock()
	}

	return nil
}

// GetFleetState returns a snapshot of the fleet state.
func (c *Coordinator) GetFleetState() FleetState {
	c.mu.RLock()
	workers := make([]WorkerInfo, 0, len(c.workers))
	for _, w := range c.workers {
		workers = append(workers, *w)
	}
	budget := c.budget
	c.mu.RUnlock()

	counts := c.queue.Counts()

	return FleetState{
		Workers:       workers,
		QueueDepth:    counts[WorkPending],
		ActiveWork:    counts[WorkAssigned] + counts[WorkRunning],
		CompletedWork: counts[WorkCompleted],
		FailedWork:    counts[WorkFailed],
		DLQDepth:      c.queue.DLQDepth(),
		TotalSpentUSD: budget.SpentUSD,
		BudgetUSD:     budget.LimitUSD,
		UpdatedAt:     time.Now(),
	}
}

// DeregisterWorker removes a worker from the fleet and reclaims its assigned work.
func (c *Coordinator) DeregisterWorker(workerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok := c.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %s not found", workerID)
	}

	// Reclaim assigned work items back to pending
	for _, item := range c.queue.All() {
		if item.AssignedTo == workerID && item.Status == WorkAssigned {
			item.Status = WorkPending
			item.AssignedTo = ""
			item.AssignedAt = nil
			c.queue.Update(item)
		}
	}

	delete(c.workers, workerID)

	// Clean up health tracking
	if c.health != nil {
		c.health.Remove(workerID)
	}

	return nil
}

// PauseWorker sets a worker's status to paused, preventing work assignment.
func (c *Coordinator) PauseWorker(workerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	w, ok := c.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %s not found", workerID)
	}
	w.Status = WorkerPaused
	return nil
}

// ResumeWorker sets a paused or draining worker's status back to online.
func (c *Coordinator) ResumeWorker(workerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	w, ok := c.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %s not found", workerID)
	}
	if w.Status != WorkerPaused && w.Status != WorkerDraining {
		return fmt.Errorf("worker %s is not paused or draining (status: %s)", workerID, w.Status)
	}
	w.Status = WorkerOnline
	return nil
}

// DrainWorker sets a worker's status to draining: no new work will be assigned,
// but existing active work continues to completion. Returns immediately (non-blocking).
func (c *Coordinator) DrainWorker(workerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	w, ok := c.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %s not found", workerID)
	}
	w.Status = WorkerDraining
	return nil
}

// IsWorkerDrained returns true when a worker is in draining status and has no
// active (assigned or running) work items remaining.
func (c *Coordinator) IsWorkerDrained(workerID string) bool {
	c.mu.RLock()
	w, ok := c.workers[workerID]
	c.mu.RUnlock()

	if !ok || w.Status != WorkerDraining {
		return false
	}

	for _, item := range c.queue.All() {
		if item.AssignedTo == workerID && (item.Status == WorkAssigned || item.Status == WorkRunning) {
			return false
		}
	}
	return true
}

// ListDLQ returns all items in the dead letter queue.
func (c *Coordinator) ListDLQ() []*WorkItem {
	return c.queue.ListDLQ()
}

// RetryFromDLQ moves an item from the dead letter queue back to the main queue.
func (c *Coordinator) RetryFromDLQ(itemID string) error {
	return c.queue.RetryFromDLQ(itemID)
}

// PurgeDLQ removes all items from the dead letter queue.
func (c *Coordinator) PurgeDLQ() int {
	return c.queue.PurgeDLQ()
}

// DLQDepth returns the number of items in the dead letter queue.
func (c *Coordinator) DLQDepth() int {
	return c.queue.DLQDepth()
}
