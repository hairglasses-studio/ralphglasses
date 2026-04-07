package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func newTestCoordinator() *Coordinator {
	bus := events.NewBus(100)
	return NewCoordinator("test-coord", "localhost", 0, "test", bus, session.NewManager())
}

func TestCoordinator_RegisterAndHeartbeat(t *testing.T) {
	coord := newTestCoordinator()

	// Register a worker
	payload := `{"hostname":"worker1","tailscale_ip":"100.1.2.3","port":9473,"providers":["claude"],"repos":["test-repo"],"max_sessions":4}`
	req := httptest.NewRequest("POST", "/api/v1/register", strings.NewReader(payload))
	w := httptest.NewRecorder()
	coord.handleRegister(w, req)

	if w.Code != 200 {
		t.Fatalf("register: got %d, want 200", w.Code)
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	workerID, _ := resp["worker_id"].(string)
	if workerID == "" {
		t.Fatal("expected worker_id in response")
	}

	// Heartbeat
	hb := HeartbeatPayload{
		WorkerID:       workerID,
		ActiveSessions: 1,
		SpentUSD:       2.50,
		AvailableSlots: 3,
		Providers:      []session.Provider{session.ProviderClaude},
	}
	hbData, _ := json.Marshal(hb)
	req2 := httptest.NewRequest("POST", "/api/v1/heartbeat", strings.NewReader(string(hbData)))
	w2 := httptest.NewRecorder()
	coord.handleHeartbeat(w2, req2)

	if w2.Code != 200 {
		t.Fatalf("heartbeat: got %d, want 200", w2.Code)
	}

	// Verify worker state
	coord.mu.RLock()
	worker := coord.workers[workerID]
	coord.mu.RUnlock()

	if worker.ActiveSessions != 1 {
		t.Errorf("active sessions: got %d, want 1", worker.ActiveSessions)
	}
	if worker.SpentUSD != 2.50 {
		t.Errorf("spent: got $%.2f, want $2.50", worker.SpentUSD)
	}
}

func TestCoordinator_SubmitAndPollWork(t *testing.T) {
	coord := newTestCoordinator()

	// Register worker
	coord.mu.Lock()
	coord.workers["w1"] = &WorkerInfo{
		ID:            "w1",
		Status:        WorkerOnline,
		Providers:     []session.Provider{session.ProviderClaude},
		Repos:         []string{"test-repo"},
		MaxSessions:   4,
		LastHeartbeat: time.Now(),
	}
	coord.mu.Unlock()

	// Submit work
	submitPayload := `{"repo_name":"test-repo","prompt":"fix lint","max_budget_usd":5,"priority":5}`
	req := httptest.NewRequest("POST", "/api/v1/work/submit", strings.NewReader(submitPayload))
	w := httptest.NewRecorder()
	coord.handleWorkSubmit(w, req)

	if w.Code != 200 {
		t.Fatalf("submit: got %d, body: %s", w.Code, w.Body.String())
	}

	var submitResp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &submitResp)
	workID, _ := submitResp["work_item_id"].(string)
	if workID == "" {
		t.Fatal("expected work_item_id")
	}

	// Poll for work
	pollPayload := `{"worker_id":"w1"}`
	req2 := httptest.NewRequest("POST", "/api/v1/work/poll", strings.NewReader(pollPayload))
	w2 := httptest.NewRecorder()
	coord.handleWorkPoll(w2, req2)

	if w2.Code != 200 {
		t.Fatalf("poll: got %d", w2.Code)
	}

	var pollResp WorkPollResponse
	_ = json.Unmarshal(w2.Body.Bytes(), &pollResp)
	if pollResp.Item == nil {
		t.Fatal("expected work item from poll")
	}
	if pollResp.Item.ID != workID {
		t.Errorf("got work %q, want %q", pollResp.Item.ID, workID)
	}
	if pollResp.Item.Status != WorkAssigned {
		t.Errorf("got status %q, want assigned", pollResp.Item.Status)
	}
}

func TestCoordinator_BudgetGate(t *testing.T) {
	coord := newTestCoordinator()
	coord.SetBudgetLimit(10)

	// Submit work that exceeds budget
	submitPayload := `{"repo_name":"test","prompt":"big task","max_budget_usd":15}`
	req := httptest.NewRequest("POST", "/api/v1/work/submit", strings.NewReader(submitPayload))
	w := httptest.NewRecorder()
	coord.handleWorkSubmit(w, req)

	if w.Code != http.StatusPaymentRequired {
		t.Errorf("expected 402, got %d", w.Code)
	}
}

func TestCoordinator_WorkComplete(t *testing.T) {
	coord := newTestCoordinator()

	// Add a work item directly
	item := &WorkItem{
		ID:           "w1",
		Status:       WorkAssigned,
		MaxBudgetUSD: 5,
		AssignedTo:   "worker1",
	}
	coord.queue.Push(item)
	coord.mu.Lock()
	coord.budget.ReservedUSD = 5
	coord.mu.Unlock()

	// Complete it
	completePayload := `{"work_item_id":"w1","status":"completed","result":{"session_id":"s1","spent_usd":2.50,"turn_count":10,"duration_seconds":120}}`
	req := httptest.NewRequest("POST", "/api/v1/work/complete", strings.NewReader(completePayload))
	w := httptest.NewRecorder()
	coord.handleWorkComplete(w, req)

	if w.Code != 200 {
		t.Fatalf("complete: got %d", w.Code)
	}

	// Check budget updated
	coord.mu.RLock()
	spent := coord.budget.SpentUSD
	reserved := coord.budget.ReservedUSD
	coord.mu.RUnlock()

	if spent != 2.50 {
		t.Errorf("spent: got $%.2f, want $2.50", spent)
	}
	if reserved != 0 {
		t.Errorf("reserved: got $%.2f, want $0", reserved)
	}
}

func TestCoordinator_HandleWorkStart(t *testing.T) {
	coord := newTestCoordinator()
	item := &WorkItem{
		ID:         "start-1",
		Status:     WorkAssigned,
		AssignedTo: "worker-1",
		Source:     WorkSourceStructuredCodexTeam,
	}
	coord.queue.Push(item)

	payload, _ := json.Marshal(WorkStartPayload{
		WorkItemID:     item.ID,
		SessionID:      "sess-1",
		WorkerNodeID:   "worker-1",
		WorktreePath:   "/tmp/worktree-start-1",
		WorktreeBranch: "codex/start-1",
		HeadSHA:        "abc123",
		MergeBaseSHA:   "def456",
	})
	req := httptest.NewRequest("POST", "/api/v1/work/start", strings.NewReader(string(payload)))
	w := httptest.NewRecorder()
	coord.handleWorkStart(w, req)

	if w.Code != 200 {
		t.Fatalf("start: got %d, want 200", w.Code)
	}

	got, ok := coord.queue.Get(item.ID)
	if !ok {
		t.Fatal("expected work item after start")
	}
	if got.Status != WorkRunning {
		t.Fatalf("status after start = %q, want %q", got.Status, WorkRunning)
	}
	if got.StartedAt == nil {
		t.Fatal("expected StartedAt to be set")
	}
	if got.SessionID != "sess-1" {
		t.Fatalf("session id = %q, want sess-1", got.SessionID)
	}
	if got.Result == nil {
		t.Fatal("expected work result metadata")
	}
	if got.Result.WorkerNodeID != "worker-1" {
		t.Fatalf("worker node id = %q, want worker-1", got.Result.WorkerNodeID)
	}
	if got.Result.WorktreeBranch != "codex/start-1" {
		t.Fatalf("worktree branch = %q, want codex/start-1", got.Result.WorktreeBranch)
	}
	if got.Result.HeadSHA != "abc123" {
		t.Fatalf("head sha = %q, want abc123", got.Result.HeadSHA)
	}
}

func TestCoordinator_HandleWorkStatus_IncludesDLQ(t *testing.T) {
	coord := newTestCoordinator()
	item := &WorkItem{
		ID:          "dlq-1",
		Status:      WorkFailed,
		SubmittedAt: time.Now(),
		Result:      &WorkResult{TaskStatus: session.TeamTaskFailed},
	}
	coord.queue.Push(item)
	if ok := coord.queue.MoveToDLQ(item.ID); !ok {
		t.Fatal("expected item to move to DLQ")
	}

	req := httptest.NewRequest("GET", "/api/v1/work/dlq-1", nil)
	req.SetPathValue("workID", item.ID)
	w := httptest.NewRecorder()
	coord.handleWorkStatus(w, req)

	if w.Code != 200 {
		t.Fatalf("status: got %d, want 200", w.Code)
	}

	var got WorkItem
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal work status: %v", err)
	}
	if got.ID != item.ID {
		t.Fatalf("work id = %q, want %q", got.ID, item.ID)
	}
	if got.Status != WorkFailed {
		t.Fatalf("status = %q, want %q", got.Status, WorkFailed)
	}
}

func TestCoordinator_HandleWorkCancel_ReleasesBudget(t *testing.T) {
	coord := newTestCoordinator()
	now := time.Now()
	item := &WorkItem{
		ID:           "cancel-1",
		Status:       WorkRunning,
		Source:       WorkSourceStructuredCodexTeam,
		AssignedTo:   "worker-1",
		AssignedAt:   &now,
		MaxBudgetUSD: 5,
		Result:       &WorkResult{WorkerNodeID: "worker-1"},
	}
	coord.queue.Push(item)
	coord.mu.Lock()
	coord.budget.ReservedUSD = 5
	coord.mu.Unlock()

	req := httptest.NewRequest("POST", "/api/v1/work/cancel-1/cancel", nil)
	req.SetPathValue("workID", item.ID)
	w := httptest.NewRecorder()
	coord.handleWorkCancel(w, req)

	if w.Code != 200 {
		t.Fatalf("cancel: got %d, want 200", w.Code)
	}

	got, ok := coord.WorkItem(item.ID)
	if !ok {
		t.Fatal("expected cancelled work item")
	}
	if got.Status != WorkFailed {
		t.Fatalf("status after cancel = %q, want %q", got.Status, WorkFailed)
	}
	if got.Error != "cancelled" {
		t.Fatalf("error after cancel = %q, want cancelled", got.Error)
	}
	if got.CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set")
	}
	if got.AssignedTo != "" {
		t.Fatalf("assigned worker after cancel = %q, want empty", got.AssignedTo)
	}
	if got.Result == nil || got.Result.TaskStatus != session.TeamTaskCancelled {
		t.Fatalf("task status after cancel = %v, want %q", got.Result, session.TeamTaskCancelled)
	}

	coord.mu.RLock()
	reserved := coord.budget.ReservedUSD
	coord.mu.RUnlock()
	if reserved != 0 {
		t.Fatalf("reserved budget after cancel = %.2f, want 0", reserved)
	}
}

func TestCoordinator_Status(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()
	coord.handleStatus(w, req)

	if w.Code != 200 {
		t.Fatalf("status: got %d", w.Code)
	}

	var status NodeStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	if status.Role != "coordinator" {
		t.Errorf("role: got %q, want coordinator", status.Role)
	}
}

func TestCoordinator_ExpireWorkers(t *testing.T) {
	coord := newTestCoordinator()

	coord.mu.Lock()
	coord.workers["stale"] = &WorkerInfo{
		ID:            "stale",
		Status:        WorkerOnline,
		LastHeartbeat: time.Now().Add(-2 * time.Minute),
	}
	coord.workers["disconnected"] = &WorkerInfo{
		ID:            "disconnected",
		Status:        WorkerOnline,
		LastHeartbeat: time.Now().Add(-10 * time.Minute),
	}
	coord.workers["fresh"] = &WorkerInfo{
		ID:            "fresh",
		Status:        WorkerOnline,
		LastHeartbeat: time.Now(),
	}
	coord.mu.Unlock()

	coord.expireWorkers()

	coord.mu.RLock()
	defer coord.mu.RUnlock()

	if coord.workers["stale"].Status != WorkerStale {
		t.Errorf("stale worker: got %q, want stale", coord.workers["stale"].Status)
	}
	if coord.workers["disconnected"].Status != WorkerDisconnected {
		t.Errorf("disconnected worker: got %q, want disconnected", coord.workers["disconnected"].Status)
	}
	if coord.workers["fresh"].Status != WorkerOnline {
		t.Errorf("fresh worker: got %q, want online", coord.workers["fresh"].Status)
	}
}

func TestCoordinator_EventBatch(t *testing.T) {
	coord := newTestCoordinator()

	batch := EventBatch{
		WorkerID: "w1",
		Events: []FleetEvent{
			{NodeID: "w1", Type: "session.started", Timestamp: time.Now(), RepoName: "test"},
			{NodeID: "w1", Type: "cost.update", Timestamp: time.Now(), SessionID: "s1"},
		},
	}
	data, _ := json.Marshal(batch)
	req := httptest.NewRequest("POST", "/api/v1/events/batch", strings.NewReader(string(data)))
	w := httptest.NewRecorder()
	coord.handleEventBatch(w, req)

	if w.Code != 200 {
		t.Fatalf("event batch: got %d", w.Code)
	}

	// Verify events were published to bus
	events := coord.bus.History("", 10)
	if len(events) < 2 {
		t.Errorf("expected at least 2 events in bus, got %d", len(events))
	}
}

func TestCoordinator_FleetState(t *testing.T) {
	coord := newTestCoordinator()
	coord.SetBudgetLimit(200)

	coord.queue.Push(&WorkItem{ID: "1", Status: WorkPending})
	coord.queue.Push(&WorkItem{ID: "2", Status: WorkRunning})
	coord.queue.Push(&WorkItem{ID: "3", Status: WorkCompleted})

	state := coord.GetFleetState()
	if state.QueueDepth != 1 {
		t.Errorf("queue depth: got %d, want 1", state.QueueDepth)
	}
	if state.CompletedWork != 1 {
		t.Errorf("completed: got %d, want 1", state.CompletedWork)
	}
	if state.BudgetUSD != 200 {
		t.Errorf("budget: got $%.2f, want $200", state.BudgetUSD)
	}
}

func TestCoordinator_SubmitWork_Internal(t *testing.T) {
	coord := newTestCoordinator()

	err := coord.SubmitWork(&WorkItem{
		RepoName: "test",
		Prompt:   "fix tests",
		Priority: 5,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	counts := coord.queue.Counts()
	if counts[WorkPending] != 1 {
		t.Errorf("pending: got %d, want 1", counts[WorkPending])
	}
}

func TestCoordinator_DeregisterWorker(t *testing.T) {
	coord := newTestCoordinator()

	// Register worker
	coord.mu.Lock()
	coord.workers["w1"] = &WorkerInfo{
		ID:            "w1",
		Status:        WorkerOnline,
		Providers:     []session.Provider{session.ProviderClaude},
		Repos:         []string{"test-repo"},
		MaxSessions:   4,
		LastHeartbeat: time.Now(),
	}
	coord.mu.Unlock()
	coord.health.RecordHeartbeat("w1")

	// Submit and assign work
	err := coord.SubmitWork(&WorkItem{
		RepoName: "test-repo",
		Prompt:   "fix lint",
		Priority: 5,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Poll to assign work to w1
	coord.mu.RLock()
	w := coord.workers["w1"]
	coord.mu.RUnlock()
	item := coord.assignWork("w1", w)
	if item == nil {
		t.Fatal("expected work to be assigned")
	}
	if item.Status != WorkAssigned || item.AssignedTo != "w1" {
		t.Fatalf("work not assigned to w1: status=%s, assigned=%s", item.Status, item.AssignedTo)
	}

	// Deregister
	if err := coord.DeregisterWorker("w1"); err != nil {
		t.Fatalf("deregister: %v", err)
	}

	// Worker should be gone
	coord.mu.RLock()
	_, exists := coord.workers["w1"]
	coord.mu.RUnlock()
	if exists {
		t.Error("worker w1 should have been removed")
	}

	// Work should be reclaimed to pending
	reclaimed, ok := coord.queue.Get(item.ID)
	if !ok {
		t.Fatal("work item should still exist in queue")
	}
	if reclaimed.Status != WorkPending {
		t.Errorf("reclaimed work status: got %q, want pending", reclaimed.Status)
	}
	if reclaimed.AssignedTo != "" {
		t.Errorf("reclaimed work assignedTo: got %q, want empty", reclaimed.AssignedTo)
	}

	// Health tracking should be cleaned up
	if state := coord.health.GetState("w1"); state != HealthUnknown {
		t.Errorf("health state: got %q, want unknown", state)
	}

	// Deregister non-existent should error
	if err := coord.DeregisterWorker("w1"); err == nil {
		t.Error("expected error deregistering non-existent worker")
	}
}

func TestCoordinator_PauseResumeWorker(t *testing.T) {
	coord := newTestCoordinator()

	// Register worker
	coord.mu.Lock()
	coord.workers["w1"] = &WorkerInfo{
		ID:            "w1",
		Status:        WorkerOnline,
		Providers:     []session.Provider{session.ProviderClaude},
		Repos:         []string{"test-repo"},
		MaxSessions:   4,
		LastHeartbeat: time.Now(),
	}
	coord.mu.Unlock()
	coord.health.RecordHeartbeat("w1")

	// Submit work
	err := coord.SubmitWork(&WorkItem{
		RepoName: "test-repo",
		Prompt:   "fix lint",
		Priority: 5,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Pause worker
	if err := coord.PauseWorker("w1"); err != nil {
		t.Fatalf("pause: %v", err)
	}

	coord.mu.RLock()
	if coord.workers["w1"].Status != WorkerPaused {
		t.Errorf("status after pause: got %q, want paused", coord.workers["w1"].Status)
	}
	coord.mu.RUnlock()

	// Paused worker should not get work
	coord.mu.RLock()
	w := coord.workers["w1"]
	coord.mu.RUnlock()
	item := coord.assignWork("w1", w)
	if item != nil {
		t.Error("paused worker should not receive work")
	}

	// Resume worker
	if err := coord.ResumeWorker("w1"); err != nil {
		t.Fatalf("resume: %v", err)
	}

	coord.mu.RLock()
	if coord.workers["w1"].Status != WorkerOnline {
		t.Errorf("status after resume: got %q, want online", coord.workers["w1"].Status)
	}
	coord.mu.RUnlock()

	// Resumed worker should get work
	coord.mu.RLock()
	w = coord.workers["w1"]
	coord.mu.RUnlock()
	item = coord.assignWork("w1", w)
	if item == nil {
		t.Error("resumed worker should receive work")
	}

	// Pause non-existent should error
	if err := coord.PauseWorker("no-such"); err == nil {
		t.Error("expected error pausing non-existent worker")
	}

	// Resume non-paused should error
	if err := coord.ResumeWorker("w1"); err == nil {
		t.Error("expected error resuming non-paused worker")
	}
}

func TestCoordinator_RetryDelay(t *testing.T) {
	coord := newTestCoordinator()

	// Register worker
	coord.mu.Lock()
	coord.workers["w1"] = &WorkerInfo{
		ID:            "w1",
		Status:        WorkerOnline,
		Providers:     []session.Provider{session.ProviderClaude},
		Repos:         []string{"test-repo"},
		MaxSessions:   4,
		LastHeartbeat: time.Now(),
	}
	coord.mu.Unlock()
	coord.health.RecordHeartbeat("w1")

	// Submit work
	item := &WorkItem{
		ID:       "retry-test",
		RepoName: "test-repo",
		Prompt:   "fix lint",
		Priority: 5,
	}
	err := coord.SubmitWork(item)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Assign to worker
	coord.mu.RLock()
	w := coord.workers["w1"]
	coord.mu.RUnlock()
	assigned := coord.assignWork("w1", w)
	if assigned == nil {
		t.Fatal("expected work to be assigned")
	}

	// Fail the work — triggers retry with delay
	completePayload := `{"work_item_id":"retry-test","status":"failed","error":"test failure"}`
	req := httptest.NewRequest("POST", "/api/v1/work/complete", strings.NewReader(completePayload))
	rec := httptest.NewRecorder()
	coord.handleWorkComplete(rec, req)

	if rec.Code != 200 {
		t.Fatalf("complete: got %d", rec.Code)
	}

	// Item should be back to pending with RetryAfter set
	retried, ok := coord.queue.Get("retry-test")
	if !ok {
		t.Fatal("work item should still exist")
	}
	if retried.Status != WorkPending {
		t.Errorf("status: got %q, want pending", retried.Status)
	}
	if retried.RetryAfter == nil {
		t.Fatal("RetryAfter should be set after failure")
	}
	if retried.RetryAfter.Before(time.Now()) {
		t.Error("RetryAfter should be in the future")
	}

	// Try to assign — should be skipped due to retry delay
	assigned2 := coord.assignWork("w1", w)
	if assigned2 != nil {
		t.Error("work item in retry backoff should not be assigned")
	}

	// Manually clear the retry delay to simulate time passing
	retried.RetryAfter = new(time.Now().Add(-1 * time.Second))
	coord.queue.Update(retried)

	// Now it should be assignable
	assigned3 := coord.assignWork("w1", w)
	if assigned3 == nil {
		t.Error("work item past retry delay should be assigned")
	}
}

func TestHealthz_Healthy(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	coord.handleHealthz(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("healthz: got %d, want 200; body: %s", w.Code, w.Body.String())
	}

	var resp HealthCheckResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Status != "healthy" {
		t.Errorf("status: got %q, want healthy", resp.Status)
	}
	if resp.Checks["event_bus"] != "ok" {
		t.Errorf("event_bus check: got %q, want ok", resp.Checks["event_bus"])
	}
	if resp.Checks["queue"] != "ok" {
		t.Errorf("queue check: got %q, want ok", resp.Checks["queue"])
	}
	if resp.Uptime <= 0 {
		t.Errorf("uptime should be positive, got %f", resp.Uptime)
	}
}

func TestHealthz_Degraded(t *testing.T) {
	// Create coordinator without an event bus to simulate a degraded state
	coord := NewCoordinator("test-coord", "localhost", 0, "test", nil, nil)

	// Verify event_bus reports not_configured (still healthy — bus is optional)
	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	coord.handleHealthz(w, req)

	var resp HealthCheckResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Checks["event_bus"] != "not_configured" {
		t.Errorf("event_bus check: got %q, want not_configured", resp.Checks["event_bus"])
	}

	// Now test with a cancelled context to make the bus check fail
	coordWithBus := newTestCoordinator()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req2 := httptest.NewRequest("GET", "/healthz", nil)
	req2 = req2.WithContext(ctx)
	w2 := httptest.NewRecorder()
	coordWithBus.handleHealthz(w2, req2)

	if w2.Code != http.StatusServiceUnavailable {
		t.Fatalf("healthz degraded: got %d, want 503; body: %s", w2.Code, w2.Body.String())
	}

	var resp2 HealthCheckResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp2.Status != "degraded" {
		t.Errorf("status: got %q, want degraded", resp2.Status)
	}
	if resp2.Checks["event_bus"] != "error" {
		t.Errorf("event_bus check: got %q, want error", resp2.Checks["event_bus"])
	}
	if resp2.Checks["event_bus_error"] == "" {
		t.Error("expected event_bus_error to be set")
	}
}

func TestCoordinator_StartStop(t *testing.T) {
	bus := events.NewBus(100)
	coord := NewCoordinator("test", "localhost", 0, "test", bus, session.NewManager())

	ctx, cancel := context.WithCancel(context.Background())

	// Start in background with ephemeral port
	errCh := make(chan error, 1)
	go func() {
		errCh <- coord.Start(ctx)
	}()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Stop
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer shutCancel()
	_ = coord.Stop(shutCtx)
}

func TestDrainWorker_StopsNewAssignments(t *testing.T) {
	coord := newTestCoordinator()

	// Register worker
	coord.mu.Lock()
	coord.workers["w1"] = &WorkerInfo{
		ID:            "w1",
		Status:        WorkerOnline,
		Providers:     []session.Provider{session.ProviderClaude},
		Repos:         []string{"test-repo"},
		MaxSessions:   4,
		LastHeartbeat: time.Now(),
	}
	coord.mu.Unlock()
	coord.health.RecordHeartbeat("w1")

	// Submit work
	err := coord.SubmitWork(&WorkItem{
		RepoName: "test-repo",
		Prompt:   "fix lint",
		Priority: 5,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	// Drain the worker
	if err := coord.DrainWorker("w1"); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Verify status is draining
	coord.mu.RLock()
	if coord.workers["w1"].Status != WorkerDraining {
		t.Errorf("status after drain: got %q, want draining", coord.workers["w1"].Status)
	}
	coord.mu.RUnlock()

	// Draining worker should not receive new work
	coord.mu.RLock()
	w := coord.workers["w1"]
	coord.mu.RUnlock()
	item := coord.assignWork("w1", w)
	if item != nil {
		t.Error("draining worker should not receive new work")
	}

	// Drain non-existent worker should error
	if err := coord.DrainWorker("no-such"); err == nil {
		t.Error("expected error draining non-existent worker")
	}
}

func TestDrainWorker_ActiveWorkContinues(t *testing.T) {
	coord := newTestCoordinator()

	// Register worker
	coord.mu.Lock()
	coord.workers["w1"] = &WorkerInfo{
		ID:            "w1",
		Status:        WorkerOnline,
		Providers:     []session.Provider{session.ProviderClaude},
		Repos:         []string{"test-repo"},
		MaxSessions:   4,
		LastHeartbeat: time.Now(),
	}
	coord.mu.Unlock()
	coord.health.RecordHeartbeat("w1")

	// Submit and assign work
	err := coord.SubmitWork(&WorkItem{
		RepoName: "test-repo",
		Prompt:   "fix lint",
		Priority: 5,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	coord.mu.RLock()
	w := coord.workers["w1"]
	coord.mu.RUnlock()
	item := coord.assignWork("w1", w)
	if item == nil {
		t.Fatal("expected work to be assigned")
	}

	// Drain the worker while work is active
	if err := coord.DrainWorker("w1"); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Active work item should still be assigned (not reclaimed)
	active, ok := coord.queue.Get(item.ID)
	if !ok {
		t.Fatal("work item should still exist")
	}
	if active.Status != WorkAssigned {
		t.Errorf("active work status: got %q, want assigned", active.Status)
	}
	if active.AssignedTo != "w1" {
		t.Errorf("active work assigned to: got %q, want w1", active.AssignedTo)
	}

	// Worker should not be drained yet (still has active work)
	if coord.IsWorkerDrained("w1") {
		t.Error("worker should not be drained while work is active")
	}
}

func TestIsWorkerDrained_NoActiveWork(t *testing.T) {
	coord := newTestCoordinator()

	// Register worker
	coord.mu.Lock()
	coord.workers["w1"] = &WorkerInfo{
		ID:            "w1",
		Status:        WorkerOnline,
		Providers:     []session.Provider{session.ProviderClaude},
		Repos:         []string{"test-repo"},
		MaxSessions:   4,
		LastHeartbeat: time.Now(),
	}
	coord.mu.Unlock()
	coord.health.RecordHeartbeat("w1")

	// Submit and assign work
	err := coord.SubmitWork(&WorkItem{
		RepoName: "test-repo",
		Prompt:   "fix lint",
		Priority: 5,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	coord.mu.RLock()
	w := coord.workers["w1"]
	coord.mu.RUnlock()
	item := coord.assignWork("w1", w)
	if item == nil {
		t.Fatal("expected work to be assigned")
	}

	// Drain the worker
	if err := coord.DrainWorker("w1"); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Not drained yet — work still active
	if coord.IsWorkerDrained("w1") {
		t.Error("should not be drained with active work")
	}

	// Complete the work
	completePayload := `{"work_item_id":"` + item.ID + `","status":"completed","result":{"session_id":"s1","spent_usd":1.00,"turn_count":5,"duration_seconds":60}}`
	req := httptest.NewRequest("POST", "/api/v1/work/complete", strings.NewReader(completePayload))
	rec := httptest.NewRecorder()
	coord.handleWorkComplete(rec, req)
	if rec.Code != 200 {
		t.Fatalf("complete: got %d", rec.Code)
	}

	// Now worker should be drained
	if !coord.IsWorkerDrained("w1") {
		t.Error("worker should be drained after all work completed")
	}

	// IsWorkerDrained for non-existent worker returns false
	if coord.IsWorkerDrained("no-such") {
		t.Error("non-existent worker should not be considered drained")
	}

	// IsWorkerDrained for non-draining worker returns false
	coord.mu.Lock()
	coord.workers["w2"] = &WorkerInfo{
		ID:     "w2",
		Status: WorkerOnline,
	}
	coord.mu.Unlock()
	if coord.IsWorkerDrained("w2") {
		t.Error("online worker should not be considered drained")
	}

	// Draining worker can be resumed
	if err := coord.ResumeWorker("w1"); err != nil {
		t.Fatalf("resume draining worker: %v", err)
	}
	coord.mu.RLock()
	if coord.workers["w1"].Status != WorkerOnline {
		t.Errorf("status after resume: got %q, want online", coord.workers["w1"].Status)
	}
	coord.mu.RUnlock()
}

func TestCoordinator_HandleFleetState(t *testing.T) {
	coord := newTestCoordinator()
	coord.SetBudgetLimit(200)

	// Register a worker
	coord.mu.Lock()
	coord.workers["w1"] = &WorkerInfo{
		ID:       "w1",
		Status:   WorkerOnline,
		Hostname: "host1",
	}
	coord.mu.Unlock()

	coord.queue.Push(&WorkItem{ID: "1", Status: WorkPending})
	coord.queue.Push(&WorkItem{ID: "2", Status: WorkRunning})

	req := httptest.NewRequest("GET", "/api/v1/fleet", nil)
	w := httptest.NewRecorder()
	coord.handleFleetState(w, req)

	if w.Code != 200 {
		t.Fatalf("fleet state: got %d, want 200", w.Code)
	}

	var state FleetState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(state.Workers) != 1 {
		t.Errorf("workers: got %d, want 1", len(state.Workers))
	}
	if state.QueueDepth != 1 {
		t.Errorf("queue depth: got %d, want 1", state.QueueDepth)
	}
	if state.BudgetUSD != 200 {
		t.Errorf("budget: got $%.2f, want $200", state.BudgetUSD)
	}
}

func TestCoordinator_HandleSessions(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("GET", "/api/v1/sessions", nil)
	w := httptest.NewRecorder()
	coord.handleSessions(w, req)

	if w.Code != 200 {
		t.Fatalf("sessions: got %d, want 200", w.Code)
	}
}

func TestCoordinator_HandleSessionsNilManager(t *testing.T) {
	coord := NewCoordinator("test", "localhost", 0, "test", nil, nil)

	req := httptest.NewRequest("GET", "/api/v1/sessions", nil)
	w := httptest.NewRecorder()
	coord.handleSessions(w, req)

	if w.Code != 200 {
		t.Fatalf("sessions nil mgr: got %d, want 200", w.Code)
	}
}

func TestCoordinator_HandleA2ATaskStatus(t *testing.T) {
	coord := newTestCoordinator()

	// Submit a work item
	item := &WorkItem{
		ID:       "task-123",
		Status:   WorkAssigned,
		RepoName: "test-repo",
		Prompt:   "fix tests",
		Type:     WorkTypeSession,
	}
	coord.queue.Push(item)

	// Test found
	req := httptest.NewRequest("GET", "/api/v1/a2a/task/task-123", nil)
	req.SetPathValue("taskID", "task-123")
	w := httptest.NewRecorder()
	coord.handleA2ATaskGet(w, req)

	if w.Code != 200 {
		t.Fatalf("a2a task status: got %d, want 200", w.Code)
	}

	var a2aResp A2ATaskResponse
	if err := json.Unmarshal(w.Body.Bytes(), &a2aResp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if a2aResp.ID != "task-123" {
		t.Errorf("task ID: got %q, want task-123", a2aResp.ID)
	}

	// Test not found
	req2 := httptest.NewRequest("GET", "/api/v1/a2a/task/no-such", nil)
	req2.SetPathValue("taskID", "no-such")
	w2 := httptest.NewRecorder()
	coord.handleA2ATaskGet(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("a2a task not found: got %d, want 404", w2.Code)
	}

	// Test missing taskID
	req3 := httptest.NewRequest("GET", "/api/v1/a2a/task/", nil)
	w3 := httptest.NewRecorder()
	coord.handleA2ATaskGet(w3, req3)

	if w3.Code != http.StatusBadRequest {
		t.Errorf("a2a task missing id: got %d, want 400", w3.Code)
	}
}

func TestCoordinator_HandleA2ATaskStatus_WithCompletedAt(t *testing.T) {
	coord := newTestCoordinator()

	now := time.Now()
	item := &WorkItem{
		ID:          "task-done",
		Status:      WorkCompleted,
		CompletedAt: &now,
	}
	coord.queue.Push(item)

	req := httptest.NewRequest("GET", "/api/v1/a2a/task/task-done", nil)
	req.SetPathValue("taskID", "task-done")
	w := httptest.NewRecorder()
	coord.handleA2ATaskGet(w, req)

	if w.Code != 200 {
		t.Fatalf("a2a completed task: got %d", w.Code)
	}
}

func TestCoordinator_HandleA2ATaskStatus_WithAssignedAt(t *testing.T) {
	coord := newTestCoordinator()

	now := time.Now()
	item := &WorkItem{
		ID:         "task-assigned",
		Status:     WorkAssigned,
		AssignedTo: "w1",
		AssignedAt: &now,
	}
	coord.queue.Push(item)

	req := httptest.NewRequest("GET", "/api/v1/a2a/task/task-assigned", nil)
	req.SetPathValue("taskID", "task-assigned")
	w := httptest.NewRecorder()
	coord.handleA2ATaskGet(w, req)

	if w.Code != 200 {
		t.Fatalf("a2a assigned task: got %d", w.Code)
	}

	var a2aResp A2ATaskResponse
	_ = json.Unmarshal(w.Body.Bytes(), &a2aResp)
	if a2aResp.ID != "task-assigned" {
		t.Errorf("task ID: got %q, want task-assigned", a2aResp.ID)
	}
	// Assigned items map to queued in A2A terms.
	if a2aResp.Status != TaskStateQueued {
		t.Errorf("status: got %q, want %q", a2aResp.Status, TaskStateQueued)
	}
}

func TestCoordinator_DLQOperations(t *testing.T) {
	coord := newTestCoordinator()

	// Initially empty DLQ
	if depth := coord.DLQDepth(); depth != 0 {
		t.Errorf("initial DLQ depth: got %d, want 0", depth)
	}
	if items := coord.ListDLQ(); len(items) != 0 {
		t.Errorf("initial DLQ list: got %d items, want 0", len(items))
	}

	// Add an item and move it to DLQ
	item := &WorkItem{
		ID:         "dlq-test",
		Status:     WorkFailed,
		MaxRetries: 0,
	}
	coord.queue.Push(item)
	coord.queue.MoveToDLQ("dlq-test")

	if depth := coord.DLQDepth(); depth != 1 {
		t.Errorf("DLQ depth after move: got %d, want 1", depth)
	}

	items := coord.ListDLQ()
	if len(items) != 1 {
		t.Fatalf("DLQ list: got %d items, want 1", len(items))
	}
	if items[0].ID != "dlq-test" {
		t.Errorf("DLQ item ID: got %q, want dlq-test", items[0].ID)
	}

	// Retry from DLQ
	if err := coord.RetryFromDLQ("dlq-test"); err != nil {
		t.Fatalf("retry from DLQ: %v", err)
	}
	if depth := coord.DLQDepth(); depth != 0 {
		t.Errorf("DLQ depth after retry: got %d, want 0", depth)
	}

	// Retry non-existent
	if err := coord.RetryFromDLQ("no-such"); err == nil {
		t.Error("expected error retrying non-existent DLQ item")
	}
}

func TestCoordinator_PurgeDLQ(t *testing.T) {
	coord := newTestCoordinator()

	// Add items to DLQ
	for i := range 3 {
		item := &WorkItem{
			ID:     fmt.Sprintf("dlq-%d", i),
			Status: WorkFailed,
		}
		coord.queue.Push(item)
		coord.queue.MoveToDLQ(item.ID)
	}

	if depth := coord.DLQDepth(); depth != 3 {
		t.Fatalf("DLQ depth before purge: got %d, want 3", depth)
	}

	purged := coord.PurgeDLQ()
	if purged != 3 {
		t.Errorf("purged count: got %d, want 3", purged)
	}
	if depth := coord.DLQDepth(); depth != 0 {
		t.Errorf("DLQ depth after purge: got %d, want 0", depth)
	}
}

func TestCoordinator_ReclaimTimedOut(t *testing.T) {
	coord := newTestCoordinator()

	// Add a work item that's been assigned for a long time
	assignedAt := time.Now().Add(-10 * time.Minute)
	item := &WorkItem{
		ID:         "stale-work",
		Status:     WorkAssigned,
		AssignedTo: "w1",
		AssignedAt: &assignedAt,
	}
	coord.queue.Push(item)

	// Reclaim timed-out work
	coord.reclaimTimedOut()

	reclaimed, ok := coord.queue.Get("stale-work")
	if !ok {
		t.Fatal("work item should still exist")
	}
	if reclaimed.Status != WorkPending {
		t.Errorf("status: got %q, want pending", reclaimed.Status)
	}
	if reclaimed.AssignedTo != "" {
		t.Errorf("assigned_to: got %q, want empty", reclaimed.AssignedTo)
	}
}

func TestCoordinator_WorkPollUnknownWorker(t *testing.T) {
	coord := newTestCoordinator()

	pollPayload := `{"worker_id":"no-such"}`
	req := httptest.NewRequest("POST", "/api/v1/work/poll", strings.NewReader(pollPayload))
	w := httptest.NewRecorder()
	coord.handleWorkPoll(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("poll unknown worker: got %d, want 404", w.Code)
	}
}

func TestCoordinator_WorkPollAtCapacity(t *testing.T) {
	coord := newTestCoordinator()

	// Register worker at capacity
	coord.mu.Lock()
	coord.workers["w1"] = &WorkerInfo{
		ID:             "w1",
		Status:         WorkerOnline,
		MaxSessions:    2,
		ActiveSessions: 2,
		LastHeartbeat:  time.Now(),
	}
	coord.mu.Unlock()

	pollPayload := `{"worker_id":"w1"}`
	req := httptest.NewRequest("POST", "/api/v1/work/poll", strings.NewReader(pollPayload))
	w := httptest.NewRecorder()
	coord.handleWorkPoll(w, req)

	if w.Code != 200 {
		t.Fatalf("poll at capacity: got %d, want 200", w.Code)
	}

	var resp WorkPollResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Item != nil {
		t.Error("worker at capacity should not receive work")
	}
}

func TestCoordinator_WorkPollBadPayload(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("POST", "/api/v1/work/poll", strings.NewReader("invalid json"))
	w := httptest.NewRecorder()
	coord.handleWorkPoll(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("poll bad payload: got %d, want 400", w.Code)
	}
}

func TestCoordinator_RegisterBadPayload(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("POST", "/api/v1/register", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	coord.handleRegister(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("register bad payload: got %d, want 400", w.Code)
	}
}

func TestCoordinator_HeartbeatBadPayload(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("POST", "/api/v1/heartbeat", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	coord.handleHeartbeat(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("heartbeat bad payload: got %d, want 400", w.Code)
	}
}

func TestCoordinator_HeartbeatUnknownWorker(t *testing.T) {
	coord := newTestCoordinator()

	hb := HeartbeatPayload{WorkerID: "no-such"}
	data, _ := json.Marshal(hb)
	req := httptest.NewRequest("POST", "/api/v1/heartbeat", strings.NewReader(string(data)))
	w := httptest.NewRecorder()
	coord.handleHeartbeat(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("heartbeat unknown: got %d, want 404", w.Code)
	}
}

func TestCoordinator_WorkCompleteBadPayload(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("POST", "/api/v1/work/complete", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	coord.handleWorkComplete(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("complete bad payload: got %d, want 400", w.Code)
	}
}

func TestCoordinator_WorkCompleteNotFound(t *testing.T) {
	coord := newTestCoordinator()

	payload := `{"work_item_id":"no-such","status":"completed"}`
	req := httptest.NewRequest("POST", "/api/v1/work/complete", strings.NewReader(payload))
	w := httptest.NewRecorder()
	coord.handleWorkComplete(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("complete not found: got %d, want 404", w.Code)
	}
}

func TestCoordinator_WorkSubmitBadPayload(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("POST", "/api/v1/work/submit", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	coord.handleWorkSubmit(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("submit bad payload: got %d, want 400", w.Code)
	}
}

func TestCoordinator_EventBatchBadPayload(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("POST", "/api/v1/events/batch", strings.NewReader("bad"))
	w := httptest.NewRecorder()
	coord.handleEventBatch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("event batch bad payload: got %d, want 400", w.Code)
	}
}

func TestCoordinator_WorkCompleteFailedExhausted(t *testing.T) {
	coord := newTestCoordinator()

	// Add item with no retries left
	item := &WorkItem{
		ID:           "fail-final",
		Status:       WorkAssigned,
		MaxRetries:   0,
		RetryCount:   0,
		MaxBudgetUSD: 5.0,
		AssignedTo:   "w1",
	}
	coord.queue.Push(item)
	coord.mu.Lock()
	coord.budget.ReservedUSD = 5.0
	coord.mu.Unlock()

	payload := `{"work_item_id":"fail-final","status":"failed","error":"permanent failure"}`
	req := httptest.NewRequest("POST", "/api/v1/work/complete", strings.NewReader(payload))
	w := httptest.NewRecorder()
	coord.handleWorkComplete(w, req)

	if w.Code != 200 {
		t.Fatalf("complete failed: got %d", w.Code)
	}

	// Item should be in DLQ
	if coord.DLQDepth() != 1 {
		t.Errorf("DLQ depth: got %d, want 1", coord.DLQDepth())
	}

	// Reserved budget should be released
	coord.mu.RLock()
	reserved := coord.budget.ReservedUSD
	coord.mu.RUnlock()
	if reserved != 0 {
		t.Errorf("reserved: got $%.2f, want $0", reserved)
	}
}

func TestCoordinator_SubmitWorkBudgetExceeded(t *testing.T) {
	coord := newTestCoordinator()
	coord.SetBudgetLimit(10)

	err := coord.SubmitWork(&WorkItem{
		RepoName:     "test",
		Prompt:       "big task",
		MaxBudgetUSD: 15,
	})
	if err == nil {
		t.Error("expected error for budget exceeded")
	}
}

func TestCoordinator_EventBatchNoBus(t *testing.T) {
	coord := NewCoordinator("test", "localhost", 0, "test", nil, nil)

	batch := EventBatch{
		WorkerID: "w1",
		Events:   []FleetEvent{{Type: "test"}},
	}
	data, _ := json.Marshal(batch)
	req := httptest.NewRequest("POST", "/api/v1/events/batch", strings.NewReader(string(data)))
	w := httptest.NewRecorder()
	coord.handleEventBatch(w, req)

	if w.Code != 200 {
		t.Fatalf("event batch no bus: got %d", w.Code)
	}
}

func TestCoordinator_StopNilServer(t *testing.T) {
	coord := newTestCoordinator()
	err := coord.Stop(context.Background())
	if err != nil {
		t.Fatalf("stop nil server: %v", err)
	}
}

func TestCoordinator_ExpireWorkersPreservesPaused(t *testing.T) {
	coord := newTestCoordinator()

	coord.mu.Lock()
	coord.workers["paused"] = &WorkerInfo{
		ID:            "paused",
		Status:        WorkerPaused,
		LastHeartbeat: time.Now().Add(-10 * time.Minute),
	}
	coord.mu.Unlock()

	coord.expireWorkers()

	coord.mu.RLock()
	defer coord.mu.RUnlock()
	if coord.workers["paused"].Status != WorkerPaused {
		t.Errorf("paused worker should stay paused, got %q", coord.workers["paused"].Status)
	}
}

func TestCoordinator_HandleStatusNilSessMgr(t *testing.T) {
	coord := NewCoordinator("test", "localhost", 0, "test", nil, nil)

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()
	coord.handleStatus(w, req)

	if w.Code != 200 {
		t.Fatalf("status nil sessmgr: got %d", w.Code)
	}
}

func TestCoordinator_WorkCompleteNoResult(t *testing.T) {
	coord := newTestCoordinator()

	item := &WorkItem{
		ID:         "no-result",
		Status:     WorkAssigned,
		AssignedTo: "w1",
	}
	coord.queue.Push(item)

	payload := `{"work_item_id":"no-result","status":"completed"}`
	req := httptest.NewRequest("POST", "/api/v1/work/complete", strings.NewReader(payload))
	w := httptest.NewRecorder()
	coord.handleWorkComplete(w, req)

	if w.Code != 200 {
		t.Fatalf("complete no result: got %d", w.Code)
	}
}

func TestCoordinator_SubmitWorkWithBudgetReservation(t *testing.T) {
	coord := newTestCoordinator()

	err := coord.SubmitWork(&WorkItem{
		RepoName:     "test",
		Prompt:       "task",
		MaxBudgetUSD: 10,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	coord.mu.RLock()
	reserved := coord.budget.ReservedUSD
	coord.mu.RUnlock()

	if reserved != 10 {
		t.Errorf("reserved: got $%.2f, want $10", reserved)
	}
}

func TestCoordinator_HandleWorkSubmitDefaults(t *testing.T) {
	coord := newTestCoordinator()

	// Submit with no ID, no status, no max_retries
	payload := `{"repo_name":"test","prompt":"do stuff"}`
	req := httptest.NewRequest("POST", "/api/v1/work/submit", strings.NewReader(payload))
	w := httptest.NewRecorder()
	coord.handleWorkSubmit(w, req)

	if w.Code != 200 {
		t.Fatalf("submit defaults: got %d, body: %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	workID, _ := resp["work_item_id"].(string)
	if workID == "" {
		t.Fatal("expected non-empty work_item_id")
	}

	item, ok := coord.queue.Get(workID)
	if !ok {
		t.Fatal("item should exist in queue")
	}
	if item.MaxRetries != 2 {
		t.Errorf("max_retries default: got %d, want 2", item.MaxRetries)
	}
	if item.Status != WorkPending {
		t.Errorf("status default: got %q, want pending", item.Status)
	}
}

func TestDrainWorker_HeartbeatPreservesStatus(t *testing.T) {
	coord := newTestCoordinator()

	// Register worker via handler
	payload := `{"hostname":"drain-test","tailscale_ip":"100.1.2.3","port":9473,"providers":["claude"],"repos":["test-repo"],"max_sessions":4}`
	req := httptest.NewRequest("POST", "/api/v1/register", strings.NewReader(payload))
	w := httptest.NewRecorder()
	coord.handleRegister(w, req)

	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	workerID, _ := resp["worker_id"].(string)

	// Drain the worker
	if err := coord.DrainWorker(workerID); err != nil {
		t.Fatalf("drain: %v", err)
	}

	// Send heartbeat — should not overwrite draining status
	hb := HeartbeatPayload{
		WorkerID:       workerID,
		ActiveSessions: 0,
		Providers:      []session.Provider{session.ProviderClaude},
	}
	hbData, _ := json.Marshal(hb)
	req2 := httptest.NewRequest("POST", "/api/v1/heartbeat", strings.NewReader(string(hbData)))
	w2 := httptest.NewRecorder()
	coord.handleHeartbeat(w2, req2)

	if w2.Code != 200 {
		t.Fatalf("heartbeat: got %d", w2.Code)
	}

	// Status should still be draining
	coord.mu.RLock()
	status := coord.workers[workerID].Status
	coord.mu.RUnlock()
	if status != WorkerDraining {
		t.Errorf("status after heartbeat: got %q, want draining", status)
	}

	// expireWorkers should also preserve draining status
	coord.expireWorkers()

	coord.mu.RLock()
	status = coord.workers[workerID].Status
	coord.mu.RUnlock()
	if status != WorkerDraining {
		t.Errorf("status after expireWorkers: got %q, want draining", status)
	}
}
