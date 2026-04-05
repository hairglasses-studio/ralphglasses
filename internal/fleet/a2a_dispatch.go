package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// Sentinel errors for A2A dispatch operations.
var (
	ErrNoAgents          = errors.New("a2a dispatch: no agents available")
	ErrNoCapableAgent    = errors.New("a2a dispatch: no agent has required capabilities")
	ErrDispatchFailed    = errors.New("a2a dispatch: task submission failed")
	ErrAgentUnreachable  = errors.New("a2a dispatch: agent unreachable")
	ErrAgentCardNotFound = errors.New("a2a dispatch: agent card not found at well-known URL")
)

// HTTPDoer abstracts the HTTP client interface for testing.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// DispatchStrategy controls how the dispatcher selects an agent.
type DispatchStrategy string

const (
	// StrategyFirstMatch picks the first agent whose skills match.
	StrategyFirstMatch DispatchStrategy = "first_match"
	// StrategyRoundRobin distributes across matching agents evenly.
	StrategyRoundRobin DispatchStrategy = "round_robin"
	// StrategyBestFit picks the agent with the most matching skills.
	StrategyBestFit DispatchStrategy = "best_fit"
)

// DispatchRequest describes a task to route to a remote A2A agent.
type DispatchRequest struct {
	TaskID           string   `json:"task_id"`
	Prompt           string   `json:"prompt"`
	RequiredSkills   []string `json:"required_skills,omitempty"`
	PreferredAgentID string   `json:"preferred_agent_id,omitempty"`
	MaxBudgetUSD     float64  `json:"max_budget_usd,omitempty"`
	Timeout          time.Duration
}

// DispatchResult captures the outcome of a dispatch operation.
type DispatchResult struct {
	AgentName    string    `json:"agent_name"`
	AgentURL     string    `json:"agent_url"`
	TaskID       string    `json:"task_id"`
	RemoteID     string    `json:"remote_id"`
	Status       TaskState `json:"status"`
	DispatchedAt time.Time `json:"dispatched_at"`
}

// cachedCard holds an agent card with a TTL for cache invalidation.
type cachedCard struct {
	card      AgentCard
	fetchedAt time.Time
}

// A2ADispatcher routes tasks to remote A2A agents based on capability matching.
// It discovers agents via well-known URLs and caches agent cards.
type A2ADispatcher struct {
	mu       sync.RWMutex
	client   HTTPDoer
	cards    map[string]*cachedCard // agent URL -> cached card
	agents   []string               // known agent base URLs
	cacheTTL time.Duration
	strategy DispatchStrategy
	rrIndex  uint64 // round-robin counter
}

// A2ADispatcherConfig configures the dispatcher.
type A2ADispatcherConfig struct {
	Client   HTTPDoer
	CacheTTL time.Duration
	Strategy DispatchStrategy
}

// NewA2ADispatcher creates a dispatcher with the given configuration.
// If config is nil, sensible defaults are used.
func NewA2ADispatcher(cfg *A2ADispatcherConfig) *A2ADispatcher {
	d := &A2ADispatcher{
		cards:    make(map[string]*cachedCard),
		cacheTTL: 5 * time.Minute,
		strategy: StrategyFirstMatch,
	}
	if cfg != nil {
		if cfg.Client != nil {
			d.client = cfg.Client
		}
		if cfg.CacheTTL > 0 {
			d.cacheTTL = cfg.CacheTTL
		}
		if cfg.Strategy != "" {
			d.strategy = cfg.Strategy
		}
	}
	if d.client == nil {
		d.client = &http.Client{Timeout: 30 * time.Second}
	}
	return d
}

// AddAgent registers a remote agent base URL for discovery.
func (d *A2ADispatcher) AddAgent(baseURL string) {
	baseURL = strings.TrimRight(baseURL, "/")
	d.mu.Lock()
	defer d.mu.Unlock()
	if slices.Contains(d.agents, baseURL) {
		return
	}
	d.agents = append(d.agents, baseURL)
}

// RemoveAgent unregisters an agent and clears its cached card.
func (d *A2ADispatcher) RemoveAgent(baseURL string) {
	baseURL = strings.TrimRight(baseURL, "/")
	d.mu.Lock()
	defer d.mu.Unlock()
	for i, u := range d.agents {
		if u == baseURL {
			d.agents = append(d.agents[:i], d.agents[i+1:]...)
			break
		}
	}
	delete(d.cards, baseURL)
}

// Agents returns the list of registered agent URLs.
func (d *A2ADispatcher) Agents() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	result := make([]string, len(d.agents))
	copy(result, d.agents)
	return result
}

// DiscoverAgent fetches and caches the agent card from a remote agent's
// well-known endpoint. Returns the cached version if still within TTL.
func (d *A2ADispatcher) DiscoverAgent(ctx context.Context, baseURL string) (*AgentCard, error) {
	baseURL = strings.TrimRight(baseURL, "/")

	d.mu.RLock()
	if cached, ok := d.cards[baseURL]; ok && time.Since(cached.fetchedAt) < d.cacheTTL {
		card := cached.card
		d.mu.RUnlock()
		return &card, nil
	}
	d.mu.RUnlock()

	card, err := d.fetchAgentCard(ctx, baseURL)
	if err != nil {
		return nil, err
	}

	d.mu.Lock()
	d.cards[baseURL] = &cachedCard{card: *card, fetchedAt: time.Now()}
	d.mu.Unlock()

	return card, nil
}

// DiscoverAll refreshes agent cards for all registered agents.
// Returns the number of successfully discovered agents.
func (d *A2ADispatcher) DiscoverAll(ctx context.Context) (int, error) {
	agents := d.Agents()
	discovered := 0
	var lastErr error
	for _, url := range agents {
		if _, err := d.DiscoverAgent(ctx, url); err != nil {
			lastErr = err
			continue
		}
		discovered++
	}
	if discovered == 0 && lastErr != nil {
		return 0, fmt.Errorf("a2a dispatch: failed to discover any agents: %w", lastErr)
	}
	return discovered, nil
}

// GetCachedCard returns the cached agent card for a URL, or nil if not cached or expired.
func (d *A2ADispatcher) GetCachedCard(baseURL string) *AgentCard {
	baseURL = strings.TrimRight(baseURL, "/")
	d.mu.RLock()
	defer d.mu.RUnlock()
	cached, ok := d.cards[baseURL]
	if !ok || time.Since(cached.fetchedAt) >= d.cacheTTL {
		return nil
	}
	card := cached.card
	return &card
}

// InvalidateCache removes the cached card for a specific agent.
func (d *A2ADispatcher) InvalidateCache(baseURL string) {
	baseURL = strings.TrimRight(baseURL, "/")
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.cards, baseURL)
}

// InvalidateAllCaches clears all cached agent cards.
func (d *A2ADispatcher) InvalidateAllCaches() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cards = make(map[string]*cachedCard)
}

// Dispatch routes a task to the best matching remote agent based on required
// skills and the configured dispatch strategy. It discovers agent cards as
// needed, matches capabilities, submits the task via A2A JSON-RPC, and returns
// the dispatch result.
func (d *A2ADispatcher) Dispatch(ctx context.Context, req DispatchRequest) (*DispatchResult, error) {
	if len(d.Agents()) == 0 {
		return nil, ErrNoAgents
	}

	// Build candidate list: (url, card, matchScore).
	type candidate struct {
		url   string
		card  AgentCard
		score int
	}

	var candidates []candidate

	for _, agentURL := range d.Agents() {
		card, err := d.DiscoverAgent(ctx, agentURL)
		if err != nil {
			continue
		}

		// If a preferred agent is specified, skip others.
		if req.PreferredAgentID != "" && card.Name != req.PreferredAgentID {
			continue
		}

		score := matchSkills(card, req.RequiredSkills)
		if len(req.RequiredSkills) > 0 && score == 0 {
			continue
		}

		candidates = append(candidates, candidate{url: agentURL, card: *card, score: score})
	}

	if len(candidates) == 0 {
		if req.PreferredAgentID != "" {
			return nil, fmt.Errorf("%w: preferred agent %q not found or unreachable", ErrNoCapableAgent, req.PreferredAgentID)
		}
		return nil, ErrNoCapableAgent
	}

	// Select based on strategy.
	var selected candidate
	switch d.strategy {
	case StrategyBestFit:
		best := candidates[0]
		for _, c := range candidates[1:] {
			if c.score > best.score {
				best = c
			}
		}
		selected = best
	case StrategyRoundRobin:
		d.mu.Lock()
		idx := d.rrIndex % uint64(len(candidates))
		d.rrIndex++
		d.mu.Unlock()
		selected = candidates[idx]
	default: // StrategyFirstMatch
		selected = candidates[0]
	}

	// Submit the task to the selected agent.
	remoteID, err := d.submitTask(ctx, selected.url, req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDispatchFailed, err)
	}

	return &DispatchResult{
		AgentName:    selected.card.Name,
		AgentURL:     selected.url,
		TaskID:       req.TaskID,
		RemoteID:     remoteID,
		Status:       TaskStateQueued,
		DispatchedAt: time.Now(),
	}, nil
}

// matchSkills returns the number of required skills that an agent's skills or
// tags satisfy. A skill matches if any AgentSkill.ID, AgentSkill.Tags, or the
// card-level Tags contain the required string (case-insensitive).
func matchSkills(card *AgentCard, required []string) int {
	if len(required) == 0 {
		return 1 // no requirements = universal match with score 1
	}

	matched := 0
	for _, req := range required {
		reqLower := strings.ToLower(req)
		if skillMatches(card, reqLower) {
			matched++
		}
	}
	return matched
}

func skillMatches(card *AgentCard, reqLower string) bool {
	// Check skill IDs and tags.
	for _, skill := range card.Skills {
		if strings.ToLower(skill.ID) == reqLower {
			return true
		}
		for _, tag := range skill.Tags {
			if strings.ToLower(tag) == reqLower {
				return true
			}
		}
	}
	// Check card-level tags.
	for _, tag := range card.Tags {
		if strings.ToLower(tag) == reqLower {
			return true
		}
	}
	return false
}

// fetchAgentCard retrieves the agent card from the well-known URL.
func (d *A2ADispatcher) fetchAgentCard(ctx context.Context, baseURL string) (*AgentCard, error) {
	endpoint := baseURL + AgentCardDiscoveryPath

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("a2a dispatch: build request for %s: %w", endpoint, err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrAgentUnreachable, baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%w: %s", ErrAgentCardNotFound, baseURL)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("a2a dispatch: agent card at %s returned status %d: %s", baseURL, resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("a2a dispatch: read agent card from %s: %w", baseURL, err)
	}

	var card AgentCard
	if err := json.Unmarshal(body, &card); err != nil {
		return nil, fmt.Errorf("a2a dispatch: parse agent card from %s: %w", baseURL, err)
	}

	return &card, nil
}

// submitTask sends the task to the remote agent's A2A task/send endpoint.
func (d *A2ADispatcher) submitTask(ctx context.Context, agentURL string, dr DispatchRequest) (string, error) {
	taskReq := A2ATaskSendRequest{
		ID: dr.TaskID,
		Message: Message{
			Role:  MessageRoleUser,
			Parts: []Part{NewTextPart(dr.Prompt)},
		},
		Metadata: A2ATaskMetadata{
			MaxBudgetUSD: dr.MaxBudgetUSD,
		},
	}

	data, err := json.Marshal(taskReq)
	if err != nil {
		return "", fmt.Errorf("marshal task request: %w", err)
	}

	endpoint := agentURL + "/api/v1/a2a/task/send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytesReader(data))
	if err != nil {
		return "", fmt.Errorf("build submit request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("submit to %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("submit to %s: status %d: %s", endpoint, resp.StatusCode, string(body))
	}

	var result A2ATaskResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse submit response from %s: %w", endpoint, err)
	}

	return result.ID, nil
}
