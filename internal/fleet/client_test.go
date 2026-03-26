package fleet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func newTestServer(t *testing.T) (*httptest.Server, *Coordinator) {
	t.Helper()
	bus := events.NewBus(100)
	coord := NewCoordinator("test-coord", "localhost", 0, "test", bus, session.NewManager())

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/register", coord.handleRegister)
	mux.HandleFunc("POST /api/v1/heartbeat", coord.handleHeartbeat)
	mux.HandleFunc("POST /api/v1/work/poll", coord.handleWorkPoll)
	mux.HandleFunc("POST /api/v1/work/complete", coord.handleWorkComplete)
	mux.HandleFunc("POST /api/v1/work/submit", coord.handleWorkSubmit)
	mux.HandleFunc("POST /api/v1/events/batch", coord.handleEventBatch)
	mux.HandleFunc("GET /api/v1/status", coord.handleStatus)
	mux.HandleFunc("GET /api/v1/fleet", coord.handleFleetState)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, coord
}

func TestClient_Register(t *testing.T) {
	ts, _ := newTestServer(t)
	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	workerID, err := client.Register(ctx, RegisterPayload{
		Hostname:    "test-host",
		TailscaleIP: "100.1.2.3",
		Port:        9473,
		Providers:   []session.Provider{session.ProviderClaude},
		MaxSessions: 4,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if workerID == "" {
		t.Fatal("expected non-empty worker ID")
	}
}

func TestClient_Heartbeat(t *testing.T) {
	ts, coord := newTestServer(t)
	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	// Register first
	workerID, _ := client.Register(ctx, RegisterPayload{
		Hostname: "hb-host", Port: 9473,
		Providers: []session.Provider{session.ProviderClaude},
	})

	err := client.Heartbeat(ctx, HeartbeatPayload{
		WorkerID:       workerID,
		ActiveSessions: 2,
		SpentUSD:       1.50,
		Providers:      []session.Provider{session.ProviderClaude},
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	coord.mu.RLock()
	w := coord.workers[workerID]
	coord.mu.RUnlock()
	if w.ActiveSessions != 2 {
		t.Errorf("active sessions: got %d, want 2", w.ActiveSessions)
	}
}

func TestClient_SubmitAndPollWork(t *testing.T) {
	ts, coord := newTestServer(t)
	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	// Register worker
	workerID, _ := client.Register(ctx, RegisterPayload{
		Hostname: "poll-host", Port: 9473,
		Providers:   []session.Provider{session.ProviderClaude},
		Repos:       []string{"test-repo"},
		MaxSessions: 4,
	})

	// Submit work
	workID, err := client.SubmitWork(ctx, WorkItem{
		RepoName: "test-repo",
		Prompt:   "fix lint",
		Priority: 5,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if workID == "" {
		t.Fatal("expected work item ID")
	}

	// Ensure worker health is tracked
	coord.health.RecordHeartbeat(workerID)

	// Poll for work
	item, err := client.PollWork(ctx, workerID)
	if err != nil {
		t.Fatalf("poll: %v", err)
	}
	if item == nil {
		t.Fatal("expected work item")
	}
	if item.ID != workID {
		t.Errorf("got %q, want %q", item.ID, workID)
	}
}

func TestClient_CompleteWork(t *testing.T) {
	ts, _ := newTestServer(t)
	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	// Submit work first
	workID, _ := client.SubmitWork(ctx, WorkItem{
		RepoName: "test-repo",
		Prompt:   "do stuff",
	})

	err := client.CompleteWork(ctx, WorkCompletePayload{
		WorkItemID: workID,
		Status:     WorkCompleted,
		Result:     &WorkResult{SessionID: "s1", SpentUSD: 1.0},
	})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
}

func TestClient_SendEvents(t *testing.T) {
	ts, _ := newTestServer(t)
	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	err := client.SendEvents(ctx, EventBatch{
		WorkerID: "w1",
		Events: []FleetEvent{
			{NodeID: "w1", Type: "test.event", Timestamp: time.Now()},
		},
	})
	if err != nil {
		t.Fatalf("send events: %v", err)
	}
}

func TestClient_Status(t *testing.T) {
	ts, _ := newTestServer(t)
	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	status, err := client.Status(ctx)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Role != "coordinator" {
		t.Errorf("role: got %q, want coordinator", status.Role)
	}
}

func TestClient_FleetState(t *testing.T) {
	ts, _ := newTestServer(t)
	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	state, err := client.FleetState(ctx)
	if err != nil {
		t.Fatalf("fleet state: %v", err)
	}
	if state == nil {
		t.Fatal("expected fleet state")
	}
}

func TestClient_Ping(t *testing.T) {
	ts, _ := newTestServer(t)
	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestClient_PingCoordinator(t *testing.T) {
	ts, _ := newTestServer(t)
	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	if err := client.PingCoordinator(ctx); err != nil {
		t.Fatalf("ping coordinator: %v", err)
	}
}

func TestClient_PingCoordinatorError(t *testing.T) {
	// Server that returns 500
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	err := client.PingCoordinator(ctx)
	if err == nil {
		t.Fatal("expected error from 500 response")
	}
}

func TestClient_PostErrorResponse(t *testing.T) {
	// Server that returns 400 with body
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer ts.Close()

	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	err := client.Heartbeat(ctx, HeartbeatPayload{WorkerID: "w1"})
	if err == nil {
		t.Fatal("expected error from 400 response")
	}
}

func TestClient_GetErrorResponse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()

	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	_, err := client.Status(ctx)
	if err == nil {
		t.Fatal("expected error from 404 response")
	}
}

func TestClient_ConnectionError(t *testing.T) {
	client := NewClient("http://127.0.0.1:1") // port 1 should not be open
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := client.PingCoordinator(ctx)
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestNewClient(t *testing.T) {
	c := NewClient("http://localhost:9473")
	if c.baseURL != "http://localhost:9473" {
		t.Errorf("baseURL: got %q", c.baseURL)
	}
}

// Verify JSON round-trip through the client
func TestClient_RegisterRoundTrip(t *testing.T) {
	var receivedPayload RegisterPayload
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedPayload)
		writeJSON(w, map[string]any{"worker_id": "w-test", "status": "registered"})
	}))
	defer ts.Close()

	client := NewClientWithTransport(ts.URL, ts.Client().Transport)
	ctx := context.Background()

	id, err := client.Register(ctx, RegisterPayload{
		Hostname:    "rt-host",
		Port:        9999,
		MaxSessions: 8,
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if id != "w-test" {
		t.Errorf("got %q, want w-test", id)
	}
	if receivedPayload.Hostname != "rt-host" {
		t.Errorf("hostname: got %q", receivedPayload.Hostname)
	}
}
