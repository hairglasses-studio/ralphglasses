package fleet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client communicates with a remote fleet node (coordinator or worker).
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a fleet HTTP client for the given base URL.
// It uses a pooled transport so that connections to the same host
// are reused across requests instead of dialing fresh each time.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: DefaultTransport(),
		},
	}
}

// NewClientWithTransport creates a fleet HTTP client with a caller-supplied
// transport, useful for testing or custom TLS configurations.
func NewClientWithTransport(baseURL string, transport http.RoundTripper) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

// PingCoordinator performs a lightweight health check against the
// coordinator's /api/v1/status endpoint. It returns nil on success
// or an error describing the failure.
func (c *Client) PingCoordinator(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/status", nil)
	if err != nil {
		return fmt.Errorf("create health check request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("coordinator health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("coordinator returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Register sends a registration payload to the coordinator.
func (c *Client) Register(ctx context.Context, payload RegisterPayload) (string, error) {
	var resp struct {
		WorkerID string `json:"worker_id"`
	}
	if err := c.post(ctx, "/api/v1/register", payload, &resp); err != nil {
		return "", err
	}
	return resp.WorkerID, nil
}

// Heartbeat sends a heartbeat to the coordinator.
func (c *Client) Heartbeat(ctx context.Context, payload HeartbeatPayload) error {
	var resp struct{}
	return c.post(ctx, "/api/v1/heartbeat", payload, &resp)
}

// PollWork requests work from the coordinator.
func (c *Client) PollWork(ctx context.Context, workerID string) (*WorkItem, error) {
	var resp WorkPollResponse
	if err := c.post(ctx, "/api/v1/work/poll", map[string]string{"worker_id": workerID}, &resp); err != nil {
		return nil, err
	}
	return resp.Item, nil
}

// CompleteWork reports work completion to the coordinator.
func (c *Client) CompleteWork(ctx context.Context, payload WorkCompletePayload) error {
	var resp struct{}
	return c.post(ctx, "/api/v1/work/complete", payload, &resp)
}

// SubmitWork submits a new work item to the coordinator.
func (c *Client) SubmitWork(ctx context.Context, item WorkItem) (string, error) {
	var resp struct {
		WorkItemID string `json:"work_item_id"`
	}
	if err := c.post(ctx, "/api/v1/work/submit", item, &resp); err != nil {
		return "", err
	}
	return resp.WorkItemID, nil
}

// SendEvents forwards a batch of events to the coordinator.
func (c *Client) SendEvents(ctx context.Context, batch EventBatch) error {
	var resp struct{}
	return c.post(ctx, "/api/v1/events/batch", batch, &resp)
}

// Status fetches the node status.
func (c *Client) Status(ctx context.Context) (*NodeStatus, error) {
	var status NodeStatus
	if err := c.get(ctx, "/api/v1/status", &status); err != nil {
		return nil, err
	}
	return &status, nil
}

// FleetState fetches the fleet state from the coordinator.
func (c *Client) FleetState(ctx context.Context) (*FleetState, error) {
	var state FleetState
	if err := c.get(ctx, "/api/v1/fleet", &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// Ping checks if the node is reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Status(ctx)
	return err
}

func (c *Client) post(ctx context.Context, path string, body, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, result any) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
