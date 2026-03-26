package fleet

import (
	"context"
	"encoding/json"
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

func (c *Coordinator) handleRegister(w http.ResponseWriter, r *http.Request) {
	var payload RegisterPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	workerID := fmt.Sprintf("%s-%d", payload.Hostname, payload.Port)

	c.mu.Lock()
	c.workers[workerID] = &WorkerInfo{
		ID:            workerID,
		Hostname:      payload.Hostname,
		TailscaleIP:   payload.TailscaleIP,
		Port:          payload.Port,
		Status:        WorkerOnline,
		Providers:     payload.Providers,
		Repos:         payload.Repos,
		MaxSessions:   payload.MaxSessions,
		RegisteredAt:  time.Now(),
		LastHeartbeat: time.Now(),
		Version:       payload.Version,
	}
	c.mu.Unlock()

	// Initialize health tracking for the new worker
	c.health.RecordHeartbeat(workerID)

	if c.bus != nil {
		c.bus.Publish(events.Event{
			Type: "fleet.worker_registered",
			Data: map[string]any{"worker_id": workerID, "hostname": payload.Hostname},
		})
	}

	writeJSON(w, map[string]any{"worker_id": workerID, "status": "registered"})
}

func (c *Coordinator) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var payload HeartbeatPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c.mu.Lock()
	worker, ok := c.workers[payload.WorkerID]
	if ok {
		worker.Status = WorkerOnline
		worker.LastHeartbeat = time.Now()
		worker.ActiveSessions = payload.ActiveSessions
		worker.SpentUSD = payload.SpentUSD
		worker.Repos = payload.Repos
		worker.Providers = payload.Providers
	}
	c.mu.Unlock()

	if !ok {
		http.Error(w, "unknown worker; re-register", http.StatusNotFound)
		return
	}

	// Record heartbeat in health tracker
	c.health.RecordHeartbeat(payload.WorkerID)

	writeJSON(w, map[string]string{"status": "ok"})
}

func (c *Coordinator) handleWorkPoll(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		WorkerID string `json:"worker_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	c.mu.RLock()
	worker, ok := c.workers[payload.WorkerID]
	c.mu.RUnlock()
	if !ok {
		http.Error(w, "unknown worker", http.StatusNotFound)
		return
	}

	// Check capacity
	if worker.ActiveSessions >= worker.MaxSessions {
		writeJSON(w, WorkPollResponse{})
		return
	}

	item := c.assignWork(payload.WorkerID, worker)
	writeJSON(w, WorkPollResponse{Item: item})
}

func (c *Coordinator) handleWorkComplete(w http.ResponseWriter, r *http.Request) {
	var payload WorkCompletePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	item, ok := c.queue.Get(payload.WorkItemID)
	if !ok {
		http.Error(w, "work item not found", http.StatusNotFound)
		return
	}

	now := time.Now()
	item.CompletedAt = &now
	item.Result = payload.Result
	item.Error = payload.Error

	if payload.Status == WorkCompleted {
		item.Status = WorkCompleted
		c.retries.RecordSuccess(item.ID)
		if payload.Result != nil {
			c.mu.Lock()
			c.budget.SpentUSD += payload.Result.SpentUSD
			if item.MaxBudgetUSD > 0 {
				c.budget.ReservedUSD -= item.MaxBudgetUSD
				if c.budget.ReservedUSD < 0 {
					c.budget.ReservedUSD = 0
				}
			}
			c.budget.LastUpdated = now
			c.mu.Unlock()

			// Track per-worker spend
			if item.AssignedTo != "" {
				c.budgetMgr.RecordCost(item.AssignedTo, payload.Result.SpentUSD)
			}
		}
	} else {
		retryable, delay := c.retries.RecordFailure(item.ID)
		// Check retry using both legacy counter and retry tracker
		if retryable && item.RetryCount < item.MaxRetries {
			item.RetryCount++
			item.Status = WorkPending
			item.AssignedTo = ""
			item.AssignedAt = nil
			item.Error = payload.Error
			item.RetryAfter = timePtr(time.Now().Add(delay))
		} else {
			item.Status = WorkFailed
		}
		// Release reserved budget on failure
		c.mu.Lock()
		if item.MaxBudgetUSD > 0 {
			c.budget.ReservedUSD -= item.MaxBudgetUSD
			if c.budget.ReservedUSD < 0 {
				c.budget.ReservedUSD = 0
			}
		}
		c.budget.LastUpdated = now
		c.mu.Unlock()
	}

	c.queue.Update(item)

	// Move permanently failed items to the dead letter queue
	if item.Status == WorkFailed {
		c.queue.MoveToDLQ(item.ID)
	}

	if c.bus != nil {
		c.bus.Publish(events.Event{
			Type:      events.EventType("fleet.work_" + string(item.Status)),
			SessionID: item.SessionID,
			RepoName:  item.RepoName,
			Data: map[string]any{
				"work_item_id": item.ID,
				"worker":       item.AssignedTo,
			},
		})
	}

	writeJSON(w, map[string]string{"status": "ok"})
}

func (c *Coordinator) handleWorkSubmit(w http.ResponseWriter, r *http.Request) {
	var item WorkItem
	if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Budget gate
	c.mu.RLock()
	avail := c.budget.AvailableBudget()
	c.mu.RUnlock()

	if item.MaxBudgetUSD > 0 && item.MaxBudgetUSD > avail {
		http.Error(w, fmt.Sprintf("insufficient budget: need $%.2f, available $%.2f", item.MaxBudgetUSD, avail), http.StatusPaymentRequired)
		return
	}

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

	c.queue.Push(&item)

	// Reserve budget
	if item.MaxBudgetUSD > 0 {
		c.mu.Lock()
		c.budget.ReservedUSD += item.MaxBudgetUSD
		c.budget.LastUpdated = time.Now()
		c.mu.Unlock()
	}

	if c.bus != nil {
		c.bus.Publish(events.Event{
			Type:     "fleet.work_submitted",
			RepoName: item.RepoName,
			Data:     map[string]any{"work_item_id": item.ID, "priority": item.Priority},
		})
	}

	writeJSON(w, map[string]any{"work_item_id": item.ID, "status": "pending"})
}

func (c *Coordinator) handleEventBatch(w http.ResponseWriter, r *http.Request) {
	var batch EventBatch
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if c.bus != nil {
		for _, fe := range batch.Events {
			c.bus.Publish(events.Event{
				Type:      events.EventType(fe.Type),
				Timestamp: fe.Timestamp,
				RepoName:  fe.RepoName,
				SessionID: fe.SessionID,
				Provider:  fe.Provider,
				Data: mergeData(fe.Data, map[string]any{
					"node_id": fe.NodeID,
					"remote":  true,
				}),
			})
		}
	}

	writeJSON(w, map[string]any{"accepted": len(batch.Events)})
}

func (c *Coordinator) handleEventStream(w http.ResponseWriter, r *http.Request) {
	if c.bus == nil {
		http.Error(w, "no event bus", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	subID := "sse-" + generateID()
	ch := c.bus.Subscribe(subID)
	defer c.bus.Unsubscribe(subID)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (c *Coordinator) handleStatus(w http.ResponseWriter, r *http.Request) {
	status := NodeStatus{
		NodeID:    c.nodeID,
		Role:      "coordinator",
		Hostname:  c.hostname,
		Uptime:    time.Since(c.startedAt).Seconds(),
		Version:   c.version,
		StartedAt: c.startedAt,
	}

	if c.sessMgr != nil {
		sessions := c.sessMgr.List("")
		status.Sessions = len(sessions)
		for _, s := range sessions {
			s.Lock()
			status.SpentUSD += s.SpentUSD
			s.Unlock()
		}
	}

	writeJSON(w, status)
}

func (c *Coordinator) handleFleetState(w http.ResponseWriter, r *http.Request) {
	c.mu.RLock()
	workers := make([]WorkerInfo, 0, len(c.workers))
	for _, w := range c.workers {
		workers = append(workers, *w)
	}
	budget := c.budget
	c.mu.RUnlock()

	counts := c.queue.Counts()

	state := FleetState{
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

	writeJSON(w, state)
}

func (c *Coordinator) handleSessions(w http.ResponseWriter, r *http.Request) {
	if c.sessMgr == nil {
		writeJSON(w, []any{})
		return
	}
	writeJSON(w, c.sessMgr.List(""))
}

// buildCandidates constructs WorkerCandidate entries from all registered workers,
// incorporating health state and per-worker budget remaining.
func (c *Coordinator) buildCandidates() []WorkerCandidate {
	c.mu.RLock()
	defer c.mu.RUnlock()

	candidates := make([]WorkerCandidate, 0, len(c.workers))
	for _, w := range c.workers {
		// Skip paused workers from routing candidates
		if w.Status == WorkerPaused {
			continue
		}
		candidates = append(candidates, WorkerCandidate{
			ID:              w.ID,
			ActiveTasks:     w.ActiveSessions,
			HealthState:     c.health.GetState(w.ID),
			BudgetRemaining: c.budgetMgr.GetBudget(w.ID).Remaining(),
		})
	}
	return candidates
}

// assignWork finds the best pending work item for a worker and assigns it.
// Uses the router to validate the polling worker is a good candidate, then
// scores work items for best fit.
func (c *Coordinator) assignWork(workerID string, worker *WorkerInfo) *WorkItem {
	// Ensure all registered workers have health records (handles workers added
	// programmatically without going through the register handler).
	c.mu.RLock()
	for id, w := range c.workers {
		state := c.health.GetState(id)
		if (state == HealthUnhealthy || state == HealthUnknown) && w.Status != WorkerDisconnected {
			c.health.RecordHeartbeat(id)
		}
	}
	c.mu.RUnlock()

	// Use router to check if this worker should receive work
	candidates := c.buildCandidates()
	preferredWorker, err := c.router.SelectWorker(candidates)
	if err != nil {
		// No healthy workers available at all
		return nil
	}

	// Boost score if the router prefers this worker
	routerBoost := 0
	if preferredWorker == workerID {
		routerBoost = 15
	}

	// Skip if this worker is paused
	c.mu.RLock()
	if w, ok := c.workers[workerID]; ok && w.Status == WorkerPaused {
		c.mu.RUnlock()
		return nil
	}
	c.mu.RUnlock()

	// Skip if this worker's health is down
	if c.health.GetState(workerID) == HealthUnhealthy {
		return nil
	}

	c.mu.RLock()
	avail := c.budget.AvailableBudget()
	c.mu.RUnlock()

	repoSet := make(map[string]bool, len(worker.Repos))
	for _, r := range worker.Repos {
		repoSet[r] = true
	}
	providerSet := make(map[session.Provider]bool, len(worker.Providers))
	for _, p := range worker.Providers {
		providerSet[p] = true
	}

	item := c.queue.AssignBest(func(item *WorkItem) int {
		// Retry delay gate — skip items still in backoff
		if item.RetryAfter != nil && time.Now().Before(*item.RetryAfter) {
			return -1
		}

		// Budget gate
		if item.MaxBudgetUSD > 0 && item.MaxBudgetUSD > avail {
			return -1 // skip
		}

		// Per-worker budget gate
		if item.MaxBudgetUSD > 0 && item.MaxBudgetUSD > c.budgetMgr.GetBudget(workerID).Remaining() {
			return -1
		}

		score := item.Priority*100 + routerBoost

		// Provider match
		if item.Provider != "" && providerSet[item.Provider] {
			score += 10
		}
		if item.Constraints.RequireProvider != "" && !providerSet[item.Constraints.RequireProvider] {
			return -1
		}

		// Repo locality
		if repoSet[item.RepoName] {
			score += 5
		}
		if item.Constraints.RequireLocal && !repoSet[item.RepoName] {
			return -1
		}

		// Node preference
		if item.Constraints.NodePreference == workerID {
			score += 20
		}

		return score
	}, workerID)

	return item
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

// ResumeWorker sets a paused worker's status back to online.
func (c *Coordinator) ResumeWorker(workerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	w, ok := c.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %s not found", workerID)
	}
	if w.Status != WorkerPaused {
		return fmt.Errorf("worker %s is not paused (status: %s)", workerID, w.Status)
	}
	w.Status = WorkerOnline
	return nil
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

// helper functions

func timePtr(t time.Time) *time.Time {
	return &t
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func mergeData(base, extra map[string]any) map[string]any {
	merged := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range extra {
		merged[k] = v
	}
	return merged
}

func generateID() string {
	// Reuse the uuid dependency already in go.mod
	return fmt.Sprintf("fl-%d", time.Now().UnixNano())
}
