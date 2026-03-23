package fleet

import (
	"context"
	"encoding/json"
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
	json.Unmarshal(w.Body.Bytes(), &resp)
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
	json.Unmarshal(w.Body.Bytes(), &submitResp)
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
	json.Unmarshal(w2.Body.Bytes(), &pollResp)
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

func TestCoordinator_Status(t *testing.T) {
	coord := newTestCoordinator()

	req := httptest.NewRequest("GET", "/api/v1/status", nil)
	w := httptest.NewRecorder()
	coord.handleStatus(w, req)

	if w.Code != 200 {
		t.Fatalf("status: got %d", w.Code)
	}

	var status NodeStatus
	json.Unmarshal(w.Body.Bytes(), &status)
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
	coord.Stop(shutCtx)
}
