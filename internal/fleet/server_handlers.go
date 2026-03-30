package fleet

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

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
		// Preserve manually-set statuses (paused, draining)
		if worker.Status != WorkerPaused && worker.Status != WorkerDraining {
			worker.Status = WorkerOnline
		}
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

	if err := c.queue.PushValidated(&item); err != nil {
		http.Error(w, fmt.Sprintf("invalid work item: %v", err), http.StatusBadRequest)
		return
	}

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

// HealthCheckResponse is returned by GET /healthz.
type HealthCheckResponse struct {
	Status  string            `json:"status"`
	Checks  map[string]string `json:"checks"`
	Uptime  float64           `json:"uptime_seconds,omitempty"`
}

func (c *Coordinator) handleHealthz(w http.ResponseWriter, r *http.Request) {
	checks := make(map[string]string)
	healthy := true

	// Check event bus responsiveness
	if c.bus != nil {
		// Publish a no-op ping event and verify the bus accepts it without error.
		err := c.bus.PublishCtx(r.Context(), events.Event{
			Type:      events.EventType("health.ping"),
			Timestamp: time.Now(),
		})
		if err != nil {
			checks["event_bus"] = "error"
			checks["event_bus_error"] = err.Error()
			healthy = false
		} else {
			checks["event_bus"] = "ok"
		}
	} else {
		checks["event_bus"] = "not_configured"
	}

	// Check work queue accessibility
	func() {
		defer func() {
			if rv := recover(); rv != nil {
				checks["queue"] = "error"
				checks["queue_error"] = fmt.Sprintf("panic: %v", rv)
				healthy = false
			}
		}()
		c.queue.Counts()
		checks["queue"] = "ok"
	}()

	resp := HealthCheckResponse{
		Checks: checks,
		Uptime: time.Since(c.startedAt).Seconds(),
	}

	if healthy {
		resp.Status = "healthy"
		w.WriteHeader(http.StatusOK)
	} else {
		resp.Status = "degraded"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	writeJSON(w, resp)
}

func (c *Coordinator) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	card := BuildAgentCard(c)
	writeJSON(w, card)
}

// handleA2ATaskStatus is kept as a deprecated alias for backward compatibility.
// New code should use handleA2ATaskGet in a2a_handler.go.
// This stub is retained only in case external code references it directly.


