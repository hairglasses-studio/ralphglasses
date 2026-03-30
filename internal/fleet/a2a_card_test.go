package fleet

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestAgentCard_Build(t *testing.T) {
	c := NewCoordinator("test-node", "localhost", 9473, "1.0.0", nil, nil)

	// Register a worker to populate capabilities.
	c.mu.Lock()
	c.workers["worker-1"] = &WorkerInfo{
		ID:        "worker-1",
		Hostname:  "host-1",
		Providers: []session.Provider{session.ProviderClaude, session.ProviderGemini},
		Repos:     []string{"ralphglasses"},
	}
	c.mu.Unlock()

	card := BuildAgentCard(c)

	if card.Name != "ralphglasses-test-node" {
		t.Errorf("expected name 'ralphglasses-test-node', got %q", card.Name)
	}
	if card.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", card.Version)
	}
	if card.URL != "http://localhost:9473" {
		t.Errorf("expected URL 'http://localhost:9473', got %q", card.URL)
	}
	if card.Provider.Organization != "hairglasses-studio" {
		t.Errorf("expected organization 'hairglasses-studio', got %q", card.Provider.Organization)
	}
	if len(card.Skills) != 3 {
		t.Errorf("expected 3 skills, got %d", len(card.Skills))
	}

	// Capabilities struct should reflect supported features.
	if !card.Capabilities.Streaming {
		t.Error("expected Streaming capability to be true")
	}
	if !card.Capabilities.StateTransitionHistory {
		t.Error("expected StateTransitionHistory capability to be true")
	}

	// Tags should include task_delegation, work_queue, and provider:* entries.
	if len(card.Tags) < 3 {
		t.Errorf("expected at least 3 tags, got %d", len(card.Tags))
	}

	hasTaskDelegation := false
	for _, tag := range card.Tags {
		if tag == "task_delegation" {
			hasTaskDelegation = true
		}
	}
	if !hasTaskDelegation {
		t.Error("expected 'task_delegation' tag")
	}

	// SupportedInterfaces should include a2a/v1.
	if len(card.SupportedInterfaces) == 0 {
		t.Error("expected non-empty SupportedInterfaces")
	}
}

func TestAgentCard_BuildNoWorkers(t *testing.T) {
	c := NewCoordinator("empty-node", "10.0.0.1", 8080, "0.1.0", nil, nil)
	card := BuildAgentCard(c)

	if card.Name != "ralphglasses-empty-node" {
		t.Errorf("expected name 'ralphglasses-empty-node', got %q", card.Name)
	}
	// Should still have base tags even without workers.
	if len(card.Tags) != 2 {
		t.Errorf("expected 2 base tags, got %d: %v", len(card.Tags), card.Tags)
	}
}

func TestAgentCard_Endpoint(t *testing.T) {
	c := NewCoordinator("endpoint-test", "localhost", 9999, "2.0.0", nil, nil)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/agent-card.json", c.handleAgentCard)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/.well-known/agent-card.json")
	if err != nil {
		t.Fatalf("GET /.well-known/agent-card.json error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		t.Fatalf("decode AgentCard: %v", err)
	}

	if card.Name != "ralphglasses-endpoint-test" {
		t.Errorf("expected name 'ralphglasses-endpoint-test', got %q", card.Name)
	}
	if card.Version != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %q", card.Version)
	}
}

func TestAgentCard_Discover(t *testing.T) {
	// Set up a mock server that serves an AgentCard at the well-known path.
	expectedCard := AgentCard{
		Name:        "remote-agent",
		Description: "A remote test agent",
		URL:         "http://remote:9473",
		Version:     "3.0.0",
		Capabilities: AgentCapabilities{
			Streaming: true,
		},
		Skills: []AgentSkill{
			{
				ID:          "test_skill",
				Name:        "Test",
				Description: "A test skill",
				InputModes:  []string{"application/json"},
				OutputModes: []string{"application/json"},
			},
		},
		Provider: AgentProvider{Organization: "test-org"},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent-card.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedCard)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	card, err := DiscoverAgent(srv.URL)
	if err != nil {
		t.Fatalf("DiscoverAgent error: %v", err)
	}

	if card.Name != "remote-agent" {
		t.Errorf("expected name 'remote-agent', got %q", card.Name)
	}
	if card.Version != "3.0.0" {
		t.Errorf("expected version '3.0.0', got %q", card.Version)
	}
	if card.Provider.Organization != "test-org" {
		t.Errorf("expected organization 'test-org', got %q", card.Provider.Organization)
	}
	if len(card.Skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(card.Skills))
	}
}

func TestAgentCard_DiscoverNotFound(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	_, err := DiscoverAgent(srv.URL)
	if err == nil {
		t.Fatal("expected error for 404 response")
	}
}

func TestRemoteA2A_SubmitTask(t *testing.T) {
	var receivedItem WorkItem

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/work/submit", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if err := json.NewDecoder(r.Body).Decode(&receivedItem); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"work_item_id": "remote-work-123",
			"status":       "pending",
		})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	adapter := NewRemoteA2AAdapterWithClient(srv.URL, srv.Client())

	offer := TaskOffer{
		ID:           "test-offer",
		OfferingNode: "local-node",
		TaskType:     "code_review",
		Prompt:       "Review the auth module",
		Constraints: DelegationConstraints{
			RequireProvider: "claude",
			MaxBudgetUSD:    2.50,
			RequireRepo:     "ralphglasses",
			PreferLocal:     true,
		},
	}

	workID, err := adapter.SubmitTask(offer)
	if err != nil {
		t.Fatalf("SubmitTask error: %v", err)
	}

	if workID != "remote-work-123" {
		t.Errorf("expected work ID 'remote-work-123', got %q", workID)
	}

	// Verify the submitted work item was properly mapped.
	if receivedItem.Prompt != "Review the auth module" {
		t.Errorf("expected prompt 'Review the auth module', got %q", receivedItem.Prompt)
	}
	if receivedItem.RepoName != "ralphglasses" {
		t.Errorf("expected repo 'ralphglasses', got %q", receivedItem.RepoName)
	}
	if receivedItem.MaxBudgetUSD != 2.50 {
		t.Errorf("expected max budget 2.50, got %.2f", receivedItem.MaxBudgetUSD)
	}
}

func TestRemoteA2A_SubmitTaskError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/work/submit", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "insufficient budget", http.StatusPaymentRequired)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	adapter := NewRemoteA2AAdapterWithClient(srv.URL, srv.Client())

	_, err := adapter.SubmitTask(TaskOffer{Prompt: "test"})
	if err == nil {
		t.Fatal("expected error for payment required response")
	}
}

func TestRemoteA2A_GetStatus(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/a2a/task/task-456", func(w http.ResponseWriter, r *http.Request) {
		offer := TaskOffer{
			ID:           "task-456",
			OfferingNode: "remote-node",
			TaskType:     "loop_task",
			Prompt:       "run tests",
			Status:       "accepted",
			AcceptedBy:   "worker-x",
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(offer)
	})
	mux.HandleFunc("/api/v1/a2a/task/not-found", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "task not found", http.StatusNotFound)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	adapter := NewRemoteA2AAdapterWithClient(srv.URL, srv.Client())

	// Test successful status retrieval.
	offer, err := adapter.GetTaskStatus("task-456")
	if err != nil {
		t.Fatalf("GetTaskStatus error: %v", err)
	}
	if offer.ID != "task-456" {
		t.Errorf("expected ID 'task-456', got %q", offer.ID)
	}
	if offer.Status != "accepted" {
		t.Errorf("expected status 'accepted', got %q", offer.Status)
	}
	if offer.AcceptedBy != "worker-x" {
		t.Errorf("expected AcceptedBy 'worker-x', got %q", offer.AcceptedBy)
	}

	// Test not found.
	_, err = adapter.GetTaskStatus("not-found")
	if err == nil {
		t.Fatal("expected error for not-found task")
	}
}

func TestRemoteA2A_Discover(t *testing.T) {
	card := AgentCard{
		Name:    "discoverable-agent",
		Version: "1.0.0",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent-card.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(card)
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	adapter := NewRemoteA2AAdapterWithClient(srv.URL, srv.Client())

	// Card should be nil before discovery.
	if adapter.Card() != nil {
		t.Error("expected nil card before discovery")
	}

	discovered, err := adapter.Discover()
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if discovered.Name != "discoverable-agent" {
		t.Errorf("expected name 'discoverable-agent', got %q", discovered.Name)
	}

	// Card should be cached after discovery.
	cached := adapter.Card()
	if cached == nil {
		t.Fatal("expected cached card after discovery")
	}
	if cached.Name != "discoverable-agent" {
		t.Errorf("expected cached name 'discoverable-agent', got %q", cached.Name)
	}
}
