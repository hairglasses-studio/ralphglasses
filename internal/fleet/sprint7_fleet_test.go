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

// --- NewRemoteA2AAdapter ---

func TestNewRemoteA2AAdapter_Construction(t *testing.T) {
	t.Parallel()

	adapter := NewRemoteA2AAdapter("http://example.com:9473")

	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if adapter.baseURL != "http://example.com:9473" {
		t.Errorf("baseURL: got %q, want %q", adapter.baseURL, "http://example.com:9473")
	}
	if adapter.client == nil {
		t.Fatal("expected non-nil HTTP client")
	}
	if adapter.client.Timeout != 30*time.Second {
		t.Errorf("client timeout: got %v, want 30s", adapter.client.Timeout)
	}
	if adapter.Card() != nil {
		t.Error("card should be nil before discovery")
	}
}

func TestNewRemoteA2AAdapterWithClient(t *testing.T) {
	t.Parallel()

	custom := &http.Client{Timeout: 10 * time.Second}
	adapter := NewRemoteA2AAdapterWithClient("http://custom.local", custom)

	if adapter.client != custom {
		t.Error("expected custom client to be used")
	}
	if adapter.baseURL != "http://custom.local" {
		t.Errorf("baseURL: got %q", adapter.baseURL)
	}
}

func TestRemoteA2AAdapter_Discover_MockServer(t *testing.T) {
	t.Parallel()

	card := AgentCard{
		Name:         "test-agent",
		Description:  "A test agent",
		URL:          "http://test:9473",
		Version:      "v1.0",
		Capabilities: AgentCapabilities{Streaming: true},
		Skills: []AgentSkill{
			{ID: "work_submit", Name: "Submit Work"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != AgentCardDiscoveryPath {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(card)
	}))
	defer srv.Close()

	adapter := NewRemoteA2AAdapterWithClient(srv.URL, srv.Client())
	discovered, err := adapter.Discover()
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if discovered.Name != "test-agent" {
		t.Errorf("name: got %q, want %q", discovered.Name, "test-agent")
	}
	// Card should be cached.
	if adapter.Card() == nil {
		t.Error("card should be cached after discovery")
	}
	if adapter.Card().Name != "test-agent" {
		t.Errorf("cached card name: got %q", adapter.Card().Name)
	}
}

func TestRemoteA2AAdapter_Discover_ServerError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	adapter := NewRemoteA2AAdapterWithClient(srv.URL, srv.Client())
	_, err := adapter.Discover()
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

// --- GetHostname ---

func TestGetHostname_ReturnsNonEmpty(t *testing.T) {
	t.Parallel()
	hostname := GetHostname()
	if hostname == "" {
		t.Error("expected non-empty hostname")
	}
}

// --- handleEventStream ---

func TestHandleEventStream_SSE(t *testing.T) {
	t.Parallel()

	bus := events.NewBus(100)
	coord := NewCoordinator("test-sse", "localhost", 0, "test", bus, session.NewManager())

	// Use a context with cancel to stop the SSE stream.
	ctx, cancel := context.WithCancel(context.Background())

	req := httptest.NewRequest("GET", "/api/v1/events/stream", nil)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	// Run handleEventStream in a goroutine since it blocks.
	done := make(chan struct{})
	go func() {
		coord.handleEventStream(w, req)
		close(done)
	}()

	// Give it a moment to start, then publish an event.
	time.Sleep(50 * time.Millisecond)
	bus.Publish(events.Event{
		Type:      "test.event",
		Timestamp: time.Now(),
		Data:      map[string]any{"key": "value"},
	})

	// Give time for event to be written.
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	// Check SSE headers.
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type: got %q, want text/event-stream", ct)
	}

	body := w.Body.String()
	if !strings.Contains(body, "data:") {
		t.Errorf("expected SSE data in response, got: %s", body)
	}
	if !strings.Contains(body, "test.event") {
		t.Errorf("expected test.event in body, got: %s", body)
	}
}

func TestHandleEventStream_NoBus(t *testing.T) {
	t.Parallel()

	coord := NewCoordinator("test-nobus", "localhost", 0, "test", nil, session.NewManager())

	req := httptest.NewRequest("GET", "/api/v1/events/stream", nil)
	w := httptest.NewRecorder()

	coord.handleEventStream(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

// --- WorkerAgent heartbeatLoop/pollLoop/executeWork ---

func TestWorkerAgent_Construction(t *testing.T) {
	t.Parallel()

	bus := events.NewBus(100)
	mgr := session.NewManager()

	agent := NewWorkerAgent("http://coordinator:9473", "test-host", 9474, "v1.0", "/tmp/scan", bus, mgr)

	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if agent.hostname != "test-host" {
		t.Errorf("hostname: got %q", agent.hostname)
	}
	if agent.port != 9474 {
		t.Errorf("port: got %d", agent.port)
	}
	if agent.NodeID() != "" {
		t.Error("NodeID should be empty before registration")
	}
}

func TestWorkerAgent_HeartbeatLoop_Cancel(t *testing.T) {
	t.Parallel()

	// Create a mock coordinator server.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	}))
	defer srv.Close()

	bus := events.NewBus(100)
	mgr := session.NewManager()
	agent := NewWorkerAgent(srv.URL, "test-host", 9474, "v1.0", "", bus, mgr)
	agent.nodeID = "test-worker"

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		agent.heartbeatLoop(ctx, []string{"repo1"}, []session.Provider{session.ProviderClaude})
		close(done)
	}()

	<-done
	// Should exit cleanly when context is cancelled.
}

func TestWorkerAgent_PollLoop_Cancel(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"item":null}`)
	}))
	defer srv.Close()

	bus := events.NewBus(100)
	mgr := session.NewManager()
	agent := NewWorkerAgent(srv.URL, "test-host", 9474, "v1.0", "", bus, mgr)
	agent.nodeID = "test-worker"

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() {
		agent.pollLoop(ctx)
		close(done)
	}()

	<-done
}

func TestWorkerAgent_DiscoverProviders_Sprint7(t *testing.T) {
	t.Parallel()

	bus := events.NewBus(100)
	mgr := session.NewManager()
	agent := NewWorkerAgent("http://unused:9473", "host", 9474, "v1.0", "", bus, mgr)

	providers := agent.discoverProviders()
	if len(providers) == 0 {
		t.Fatal("expected at least one provider")
	}
	// First provider depends on which CLI binaries are installed on PATH.
	known := map[session.Provider]bool{
		session.ProviderCodex:  true,
		session.ProviderGemini: true,
		session.ProviderClaude: true,
		"ollama": true,
	}
	if !known[providers[0]] {
		t.Errorf("first provider = %q, want one of codex/gemini/claude/ollama", providers[0])
	}
}

// --- BuildAgentCard ---

func TestBuildAgentCard(t *testing.T) {
	t.Parallel()

	bus := events.NewBus(100)
	coord := NewCoordinator("card-test", "localhost", 9473, "v1.0", bus, session.NewManager())

	card := BuildAgentCard(coord)
	if card.Name == "" {
		t.Error("expected non-empty card name")
	}
	if !strings.Contains(card.Name, "card-test") {
		t.Errorf("card name should contain nodeID, got: %s", card.Name)
	}
	if card.URL == "" {
		t.Error("expected non-empty URL")
	}
	if len(card.Skills) == 0 {
		t.Error("expected at least one skill")
	}
	if card.Provider.Organization != "hairglasses-studio" {
		t.Errorf("org: got %q", card.Provider.Organization)
	}
}

// --- GetLocalIP ---

func TestGetLocalIP_ReturnsValid(t *testing.T) {
	t.Parallel()
	ip := GetLocalIP()
	if ip == "" {
		t.Error("expected non-empty IP")
	}
}
