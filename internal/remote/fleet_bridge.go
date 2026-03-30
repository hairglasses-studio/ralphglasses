package remote

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// RemoteSession represents a session running on a remote host.
type RemoteSession struct {
	ID        string    `json:"id"`
	Host      string    `json:"host"`
	Status    string    `json:"status"`
	Provider  string    `json:"provider"`
	StartedAt time.Time `json:"started_at"`
	CostUSD   float64   `json:"cost_usd"`
	LastSeen  time.Time `json:"last_seen"`
}

// FleetBridgeStats holds aggregate statistics across all remote sessions.
type FleetBridgeStats struct {
	TotalSessions int            `json:"total_sessions"`
	TotalCostUSD  float64        `json:"total_cost_usd"`
	Providers     map[string]int `json:"providers"`
	Healthy       int            `json:"healthy"`
	Unhealthy     int            `json:"unhealthy"`
}

// FleetBridge aggregates remote session data from multiple endpoints for the
// fleet view. It polls each endpoint's /sessions JSON API and merges results
// into a unified session list.
type FleetBridge struct {
	mu        sync.RWMutex
	endpoints []string
	sessions  map[string][]RemoteSession // endpoint -> sessions
	health    map[string]bool            // endpoint -> reachable
	client    *http.Client
	interval  time.Duration
}

// NewFleetBridge creates a FleetBridge that polls the given HTTP endpoints.
func NewFleetBridge(endpoints []string) *FleetBridge {
	eps := make([]string, len(endpoints))
	copy(eps, endpoints)
	return &FleetBridge{
		endpoints: eps,
		sessions:  make(map[string][]RemoteSession),
		health:    make(map[string]bool),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		interval: 15 * time.Second,
	}
}

// SetInterval configures the polling interval used by Poll.
func (fb *FleetBridge) SetInterval(d time.Duration) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.interval = d
}

// SetHTTPClient replaces the default HTTP client used for polling.
func (fb *FleetBridge) SetHTTPClient(c *http.Client) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.client = c
}

// Poll fetches session data from all endpoints once and updates internal state.
// It returns an error only if the context is canceled; individual endpoint
// failures are recorded in the health map but do not cause Poll to fail.
func (fb *FleetBridge) Poll(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	type result struct {
		endpoint string
		sessions []RemoteSession
		err      error
	}

	fb.mu.RLock()
	client := fb.client
	endpoints := fb.endpoints
	fb.mu.RUnlock()

	results := make(chan result, len(endpoints))
	var wg sync.WaitGroup

	for _, ep := range endpoints {
		wg.Add(1)
		go func(endpoint string) {
			defer wg.Done()

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/sessions", nil)
			if err != nil {
				results <- result{endpoint: endpoint, err: err}
				return
			}

			resp, err := client.Do(req)
			if err != nil {
				results <- result{endpoint: endpoint, err: err}
				return
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				results <- result{endpoint: endpoint, err: fmt.Errorf("unexpected status %d", resp.StatusCode)}
				return
			}

			var sessions []RemoteSession
			if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
				results <- result{endpoint: endpoint, err: err}
				return
			}

			results <- result{endpoint: endpoint, sessions: sessions}
		}(ep)
	}

	// Close results channel once all goroutines finish.
	go func() {
		wg.Wait()
		close(results)
	}()

	newSessions := make(map[string][]RemoteSession)
	newHealth := make(map[string]bool)

	for r := range results {
		if r.err != nil {
			newHealth[r.endpoint] = false
			continue
		}
		newSessions[r.endpoint] = r.sessions
		newHealth[r.endpoint] = true
	}

	fb.mu.Lock()
	fb.sessions = newSessions
	fb.health = newHealth
	fb.mu.Unlock()

	return nil
}

// Sessions returns all known remote sessions across every endpoint.
func (fb *FleetBridge) Sessions() []RemoteSession {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	var all []RemoteSession
	for _, ss := range fb.sessions {
		all = append(all, ss...)
	}
	return all
}

// Healthy returns true if every configured endpoint is reachable.
func (fb *FleetBridge) Healthy() bool {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	if len(fb.endpoints) == 0 {
		return true
	}
	for _, ep := range fb.endpoints {
		if !fb.health[ep] {
			return false
		}
	}
	return true
}

// Stats returns aggregate statistics across all remote sessions.
func (fb *FleetBridge) Stats() FleetBridgeStats {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	stats := FleetBridgeStats{
		Providers: make(map[string]int),
	}

	for _, ss := range fb.sessions {
		stats.TotalSessions += len(ss)
		for _, s := range ss {
			stats.TotalCostUSD += s.CostUSD
			stats.Providers[s.Provider]++
		}
	}

	for _, ep := range fb.endpoints {
		if fb.health[ep] {
			stats.Healthy++
		} else {
			stats.Unhealthy++
		}
	}

	return stats
}
