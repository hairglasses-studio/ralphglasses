package fleet

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// AgentCardDiscoveryPath is the well-known path for A2A agent card discovery
// per the A2A v1.0 specification.
const AgentCardDiscoveryPath = "/.well-known/agent-card.json"

// AgentCard describes an A2A agent's identity, capabilities, and endpoint
// following Google's Agent-to-Agent protocol v1.0 specification.
type AgentCard struct {
	Name                string                    `json:"name"`
	Description         string                    `json:"description"`
	URL                 string                    `json:"url"`
	Version             string                    `json:"version"`
	DocumentationURL    string                    `json:"documentationUrl,omitempty"`
	SupportedInterfaces []string                  `json:"supportedInterfaces,omitempty"`
	Capabilities        AgentCapabilities         `json:"capabilities"`
	Skills              []AgentSkill              `json:"skills"`
	SecuritySchemes     map[string]SecurityScheme `json:"securitySchemes,omitempty"`
	Security            []map[string][]string     `json:"security,omitempty"`
	Provider            AgentProvider             `json:"provider"`

	// SupportsA2A is a convenience field for backward compatibility;
	// it lists string tags like "task_delegation", "provider:claude", etc.
	// These are not part of the A2A v1.0 spec but are retained for internal use.
	Tags []string `json:"tags,omitempty"`
}

// AgentCapabilities describes what protocol features the agent supports.
type AgentCapabilities struct {
	Streaming              bool `json:"streaming"`
	PushNotifications      bool `json:"pushNotifications"`
	StateTransitionHistory bool `json:"stateTransitionHistory"`
}

// AgentSkill describes a specific capability an agent can perform.
type AgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	InputModes  []string `json:"inputModes"`
	OutputModes []string `json:"outputModes"`
	Examples    []string `json:"examples,omitempty"`
}

// SecurityScheme describes an authentication mechanism per OpenAPI-style definition.
type SecurityScheme struct {
	Type   string `json:"type"`             // "apiKey", "http", "oauth2", "openIdConnect"
	In     string `json:"in,omitempty"`     // "header", "query", "cookie" (for apiKey)
	Name   string `json:"name,omitempty"`   // header/query name (for apiKey)
	Scheme string `json:"scheme,omitempty"` // "bearer", "basic" (for http)
}

// AgentProvider identifies the organization operating the agent.
type AgentProvider struct {
	Organization string `json:"organization"`
	URL          string `json:"url,omitempty"`
}

// BuildAgentCard constructs an AgentCard from the coordinator's current state.
func BuildAgentCard(c *Coordinator) AgentCard {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Collect unique provider names across all workers for tags.
	providerSet := make(map[string]bool)
	for _, w := range c.workers {
		for _, p := range w.Providers {
			providerSet[string(p)] = true
		}
	}

	tags := make([]string, 0, len(providerSet)+2)
	tags = append(tags, "task_delegation", "work_queue")
	for p := range providerSet {
		tags = append(tags, "provider:"+p)
	}

	skills := []AgentSkill{
		{
			ID:          "work_submit",
			Name:        "Submit Work",
			Description: "Submit a work item to the fleet queue for execution",
			Tags:        []string{"fleet", "work-queue"},
			InputModes:  []string{"application/json"},
			OutputModes: []string{"application/json"},
		},
		{
			ID:          "a2a_offer",
			Name:        "Task Offer",
			Description: "Publish a task offer for other agents to accept",
			Tags:        []string{"a2a", "delegation"},
			InputModes:  []string{"application/json"},
			OutputModes: []string{"application/json"},
		},
		{
			ID:          "fleet_status",
			Name:        "Fleet Status",
			Description: "Get current fleet state including workers and queue depth",
			Tags:        []string{"fleet", "monitoring"},
			InputModes:  []string{"application/json"},
			OutputModes: []string{"application/json"},
		},
	}

	agentURL := fmt.Sprintf("http://%s:%d", c.hostname, c.port)

	return AgentCard{
		Name:                "ralphglasses-" + c.nodeID,
		Description:         "Ralphglasses fleet coordinator managing multi-LLM agent sessions",
		URL:                 agentURL,
		Version:             c.version,
		SupportedInterfaces: []string{"a2a/v1"},
		Capabilities: AgentCapabilities{
			Streaming:              true,
			PushNotifications:      false,
			StateTransitionHistory: true,
		},
		Skills: skills,
		SecuritySchemes: map[string]SecurityScheme{
			"bearer": {
				Type:   "http",
				Scheme: "bearer",
			},
			"apiKey": {
				Type: "apiKey",
				In:   "header",
				Name: "X-API-Key",
			},
		},
		Security: []map[string][]string{
			{"bearer": {}},
			{"apiKey": {}},
		},
		Tags: tags,
		Provider: AgentProvider{
			Organization: "hairglasses-studio",
		},
	}
}

// DiscoverAgent fetches and parses the AgentCard from a remote agent's
// well-known endpoint at {url}/.well-known/agent-card.json (A2A v1.0 spec).
func DiscoverAgent(url string) (*AgentCard, error) {
	return DiscoverAgentWithClient(http.DefaultClient, url)
}

// DiscoverAgentWithClient fetches an AgentCard using the provided HTTP client.
func DiscoverAgentWithClient(client *http.Client, url string) (*AgentCard, error) {
	endpoint := url + AgentCardDiscoveryPath

	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("a2a: discover agent at %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("a2a: discover agent at %s: status %d", endpoint, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("a2a: read agent card from %s: %w", endpoint, err)
	}

	var card AgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		return nil, fmt.Errorf("a2a: parse agent card from %s: %w", endpoint, err)
	}

	return &card, nil
}

// RemoteA2AAdapter enables cross-fleet task delegation over HTTP using the
// A2A protocol. It communicates with a remote coordinator's API endpoints.
type RemoteA2AAdapter struct {
	client  *http.Client
	baseURL string
	card    *AgentCard // cached after discovery
}

// NewRemoteA2AAdapter creates an adapter for communicating with a remote
// fleet coordinator at the given base URL.
func NewRemoteA2AAdapter(baseURL string) *RemoteA2AAdapter {
	return &RemoteA2AAdapter{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: baseURL,
	}
}

// NewRemoteA2AAdapterWithClient creates an adapter with a custom HTTP client.
func NewRemoteA2AAdapterWithClient(baseURL string, client *http.Client) *RemoteA2AAdapter {
	return &RemoteA2AAdapter{
		client:  client,
		baseURL: baseURL,
	}
}

// Discover fetches and caches the remote agent's AgentCard.
func (r *RemoteA2AAdapter) Discover() (*AgentCard, error) {
	card, err := DiscoverAgentWithClient(r.client, r.baseURL)
	if err != nil {
		return nil, err
	}
	r.card = card
	return card, nil
}

// Card returns the cached AgentCard, or nil if not yet discovered.
func (r *RemoteA2AAdapter) Card() *AgentCard {
	return r.card
}

// SubmitTask sends a task offer to the remote coordinator's work submission
// endpoint and returns the assigned work item ID.
func (r *RemoteA2AAdapter) SubmitTask(offer TaskOffer) (string, error) {
	data, err := json.Marshal(WorkItem{
		Type:     WorkTypeLoopTask,
		Priority: 1,
		Prompt:   offer.Prompt,
		RepoName: offer.Constraints.RequireRepo,
		Constraints: WorkConstraints{
			RequireProvider: session.Provider(offer.Constraints.RequireProvider),
			RequireLocal:    offer.Constraints.PreferLocal,
		},
		MaxBudgetUSD: offer.Constraints.MaxBudgetUSD,
	})
	if err != nil {
		return "", fmt.Errorf("a2a: marshal task: %w", err)
	}

	endpoint := r.baseURL + "/api/v1/work/submit"
	resp, err := r.client.Post(endpoint, "application/json", bytesReader(data))
	if err != nil {
		return "", fmt.Errorf("a2a: submit task to %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("a2a: submit task: status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		WorkItemID string `json:"work_item_id"`
		Status     string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("a2a: parse submit response: %w", err)
	}

	return result.WorkItemID, nil
}

func (r *RemoteA2AAdapter) SubmitStructuredTask(req session.TeamBackendSubmitRequest) (string, error) {
	taskID := generateID()
	payload := A2ATaskSendRequest{
		ID: taskID,
		Message: Message{
			Role:  MessageRoleUser,
			Parts: []Part{NewTextPart(req.Prompt)},
		},
		Metadata: A2ATaskMetadata{
			RepoName:         req.RepoName,
			RepoPath:         req.RepoPath,
			Source:           WorkSourceStructuredCodexTeam,
			Provider:         string(req.Provider),
			Model:            req.Model,
			MaxBudgetUSD:     req.MaxBudgetUSD,
			Priority:         1,
			TeamName:         req.TeamName,
			TeamTaskID:       req.TaskID,
			PlannerSessionID: req.PlannerSessionID,
			SessionName:      req.SessionName,
			PermissionMode:   req.PermissionMode,
			OutputSchema:     req.OutputSchema,
			WorktreePolicy:   req.WorktreePolicy,
			TargetBranch:     req.TargetBranch,
			HumanContext:     append([]string(nil), req.HumanContext...),
			OwnedPaths:       append([]string(nil), req.OwnedPaths...),
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("a2a: marshal structured task: %w", err)
	}
	endpoint := r.baseURL + "/api/v1/a2a/task/send"
	resp, err := r.client.Post(endpoint, "application/json", bytesReader(data))
	if err != nil {
		return "", fmt.Errorf("a2a: submit structured task to %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("a2a: submit structured task: status %d: %s", resp.StatusCode, string(body))
	}
	var result A2ATaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("a2a: parse structured submit response: %w", err)
	}
	return result.ID, nil
}

// GetTaskStatus retrieves the current state of a submitted task from the
// remote coordinator.
func (r *RemoteA2AAdapter) GetTaskStatus(taskID string) (*TaskOffer, error) {
	endpoint := fmt.Sprintf("%s/api/v1/a2a/task/%s", r.baseURL, taskID)
	resp, err := r.client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("a2a: get task status from %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrOfferNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("a2a: get task status: status %d: %s", resp.StatusCode, string(body))
	}

	var offer TaskOffer
	if err := json.NewDecoder(resp.Body).Decode(&offer); err != nil {
		return nil, fmt.Errorf("a2a: parse task status: %w", err)
	}

	return &offer, nil
}

func (r *RemoteA2AAdapter) GetTaskResponse(taskID string) (*A2ATaskResponse, error) {
	endpoint := fmt.Sprintf("%s/api/v1/a2a/task/%s", r.baseURL, taskID)
	resp, err := r.client.Get(endpoint)
	if err != nil {
		return nil, fmt.Errorf("a2a: get task response from %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrOfferNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("a2a: get task response: status %d: %s", resp.StatusCode, string(body))
	}

	var result A2ATaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("a2a: parse task response: %w", err)
	}
	return &result, nil
}

func (r *RemoteA2AAdapter) CancelTask(taskID string) error {
	endpoint := fmt.Sprintf("%s/api/v1/a2a/task/%s/cancel", r.baseURL, taskID)
	resp, err := r.client.Post(endpoint, "application/json", nil)
	if err != nil {
		return fmt.Errorf("a2a: cancel task at %s: %w", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrOfferNotFound
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("a2a: cancel task: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// bytesReader wraps a byte slice in an io.Reader for HTTP request bodies.
func bytesReader(data []byte) io.Reader {
	return &byteReaderWrapper{data: data, pos: 0}
}

type byteReaderWrapper struct {
	data []byte
	pos  int
}

func (b *byteReaderWrapper) Read(p []byte) (n int, err error) {
	if b.pos >= len(b.data) {
		return 0, io.EOF
	}
	n = copy(p, b.data[b.pos:])
	b.pos += n
	return n, nil
}
