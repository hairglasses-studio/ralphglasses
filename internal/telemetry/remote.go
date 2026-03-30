package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"
)

// RemoteClient batches telemetry events and flushes them via HTTP POST.
type RemoteClient struct {
	endpoint string
	apiKey   string

	batchSize     int
	flushInterval time.Duration
	httpTimeout   time.Duration
	maxRetries    int

	mu     sync.Mutex
	queue  []Event
	client *http.Client

	done   chan struct{}
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// RemoteOption configures a RemoteClient.
type RemoteOption func(*RemoteClient)

// WithAPIKey sets the Authorization bearer token.
func WithAPIKey(key string) RemoteOption {
	return func(c *RemoteClient) { c.apiKey = key }
}

// WithBatchSize sets the maximum events per flush (default 100).
func WithBatchSize(n int) RemoteOption {
	return func(c *RemoteClient) {
		if n > 0 {
			c.batchSize = n
		}
	}
}

// WithFlushInterval sets the periodic flush interval (default 30s).
func WithFlushInterval(d time.Duration) RemoteOption {
	return func(c *RemoteClient) {
		if d > 0 {
			c.flushInterval = d
		}
	}
}

// WithHTTPTimeout sets the HTTP request timeout (default 10s).
func WithHTTPTimeout(d time.Duration) RemoteOption {
	return func(c *RemoteClient) {
		if d > 0 {
			c.httpTimeout = d
		}
	}
}

// WithMaxRetries sets the maximum retry attempts on server errors (default 3).
func WithMaxRetries(n int) RemoteOption {
	return func(c *RemoteClient) {
		if n >= 0 {
			c.maxRetries = n
		}
	}
}

// NewRemoteClient creates a RemoteClient that sends events to the given endpoint.
// The background flusher starts immediately; call Close to shut it down.
func NewRemoteClient(endpoint string, opts ...RemoteOption) *RemoteClient {
	c := &RemoteClient{
		endpoint:      endpoint,
		batchSize:     100,
		flushInterval: 30 * time.Second,
		httpTimeout:   10 * time.Second,
		maxRetries:    3,
		queue:         make([]Event, 0, 100),
		done:          make(chan struct{}),
		stopCh:        make(chan struct{}),
	}
	for _, o := range opts {
		o(c)
	}
	c.client = &http.Client{Timeout: c.httpTimeout}

	c.wg.Add(1)
	go c.loop()
	return c
}

// Send queues an event for batched delivery. It is non-blocking.
// Returns an error only if the client has been closed.
func (c *RemoteClient) Send(_ context.Context, event Event) error {
	select {
	case <-c.done:
		return fmt.Errorf("telemetry: client closed")
	default:
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	c.mu.Lock()
	c.queue = append(c.queue, event)
	full := len(c.queue) >= c.batchSize
	c.mu.Unlock()

	if full {
		// Best-effort flush; errors are swallowed since periodic flush retries.
		_ = c.Flush(context.Background())
	}
	return nil
}

// Flush sends all queued events via HTTP POST. It respects context cancellation.
func (c *RemoteClient) Flush(ctx context.Context) error {
	c.mu.Lock()
	if len(c.queue) == 0 {
		c.mu.Unlock()
		return nil
	}
	batch := c.queue
	c.queue = make([]Event, 0, c.batchSize)
	c.mu.Unlock()

	return c.sendBatch(ctx, batch)
}

// Close flushes remaining events and stops the background flusher.
func (c *RemoteClient) Close() error {
	select {
	case <-c.done:
		return nil // already closed
	default:
	}
	close(c.done)
	close(c.stopCh)
	c.wg.Wait()

	// Final flush with a generous timeout.
	ctx, cancel := context.WithTimeout(context.Background(), c.httpTimeout)
	defer cancel()
	return c.Flush(ctx)
}

func (c *RemoteClient) loop() {
	defer c.wg.Done()
	ticker := time.NewTicker(c.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			_ = c.Flush(context.Background())
		}
	}
}

// payload is the JSON body sent to the endpoint.
type payload struct {
	Events []Event `json:"events"`
}

func (c *RemoteClient) sendBatch(ctx context.Context, events []Event) error {
	body, err := json.Marshal(payload{Events: events})
	if err != nil {
		return fmt.Errorf("telemetry: marshal: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			// Re-queue on cancellation so events are not lost.
			c.requeue(events)
			return fmt.Errorf("telemetry: context cancelled: %w", err)
		}

		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * 100 * time.Millisecond
			select {
			case <-ctx.Done():
				c.requeue(events)
				return fmt.Errorf("telemetry: context cancelled during backoff: %w", ctx.Err())
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("telemetry: new request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return nil
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("telemetry: server error %d", resp.StatusCode)
			continue
		}
		// 4xx — do not retry client errors.
		return fmt.Errorf("telemetry: client error %d", resp.StatusCode)
	}

	// All retries exhausted — re-queue so a future flush can try again.
	c.requeue(events)
	return fmt.Errorf("telemetry: retries exhausted: %w", lastErr)
}

func (c *RemoteClient) requeue(events []Event) {
	c.mu.Lock()
	// Prepend so ordering is preserved.
	c.queue = append(events, c.queue...)
	c.mu.Unlock()
}
