package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// WebhookConfig describes a single webhook endpoint.
type WebhookConfig struct {
	// URL is the HTTP(S) endpoint to POST events to.
	URL string `json:"url"`

	// Secret is the HMAC-SHA256 key for signing payloads.
	// If empty, no signature header is sent.
	Secret string `json:"secret,omitempty"`

	// Events filters which event types trigger this webhook.
	// An empty slice means all events are delivered.
	Events []string `json:"events,omitempty"`

	// Headers are additional HTTP headers sent with every request.
	Headers map[string]string `json:"headers,omitempty"`

	// Timeout for each HTTP request. Defaults to 10s if zero.
	Timeout time.Duration `json:"timeout,omitempty"`

	// MaxRetries is the number of retry attempts after a failed delivery.
	// Retries use exponential backoff (1s, 2s, 4s, ...). Defaults to 3
	// when left at zero. Set to -1 to disable retries entirely.
	MaxRetries int `json:"max_retries,omitempty"`
}

// WebhookPayload is the JSON body POSTed to webhook endpoints.
type WebhookPayload struct {
	Event     events.EventType `json:"event"`
	Timestamp time.Time        `json:"timestamp"`
	SessionID string           `json:"session_id,omitempty"`
	RepoName  string           `json:"repo_name,omitempty"`
	RepoPath  string           `json:"repo_path,omitempty"`
	Provider  string           `json:"provider,omitempty"`
	NodeID    string           `json:"node_id,omitempty"`
	Data      map[string]any   `json:"data,omitempty"`
}

// circuitState tracks consecutive failures for a single webhook.
type circuitState struct {
	mu                sync.Mutex
	consecutiveFails  int
	openUntil         time.Time
	failThreshold     int
	cooldownDuration  time.Duration
}

func newCircuitState() *circuitState {
	return &circuitState{
		failThreshold:    5,
		cooldownDuration: 5 * time.Minute,
	}
}

// isOpen returns true if the circuit breaker is tripped.
func (cs *circuitState) isOpen(now time.Time) bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.consecutiveFails >= cs.failThreshold && now.Before(cs.openUntil)
}

// recordSuccess resets the failure counter.
func (cs *circuitState) recordSuccess() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.consecutiveFails = 0
}

// recordFailure increments failures and opens the circuit if threshold is reached.
func (cs *circuitState) recordFailure(now time.Time) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.consecutiveFails++
	if cs.consecutiveFails >= cs.failThreshold {
		cs.openUntil = now.Add(cs.cooldownDuration)
	}
}

// ConsecutiveFailures returns the current consecutive failure count.
func (cs *circuitState) ConsecutiveFailures() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.consecutiveFails
}

// Dispatcher subscribes to the event bus and delivers matching events
// to configured webhook endpoints.
type Dispatcher struct {
	bus     *events.Bus
	configs []WebhookConfig
	client  *http.Client

	circuits map[string]*circuitState // keyed by config URL
	eventCh  <-chan events.Event
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// nowFunc enables time injection for testing.
	nowFunc func() time.Time
}

// NewDispatcher creates a webhook dispatcher that listens on bus and
// delivers to the provided webhook configurations.
func NewDispatcher(bus *events.Bus, configs []WebhookConfig) *Dispatcher {
	circuits := make(map[string]*circuitState, len(configs))
	for _, cfg := range configs {
		circuits[cfg.URL] = newCircuitState()
	}
	return &Dispatcher{
		bus:      bus,
		configs:  configs,
		circuits: circuits,
		nowFunc:  time.Now,
	}
}

// Start begins listening for events and dispatching webhooks.
// The dispatcher runs until Stop is called or the context is cancelled.
func (d *Dispatcher) Start(ctx context.Context) {
	ctx, d.cancel = context.WithCancel(ctx)

	// Build combined event type filter across all configs.
	var allTypes []events.EventType
	anyUnfiltered := false
	for _, cfg := range d.configs {
		if len(cfg.Events) == 0 {
			anyUnfiltered = true
			break
		}
		for _, e := range cfg.Events {
			allTypes = append(allTypes, events.EventType(e))
		}
	}

	subID := fmt.Sprintf("webhook-dispatcher-%d", time.Now().UnixNano())
	if anyUnfiltered {
		d.eventCh = d.bus.Subscribe(subID)
	} else {
		d.eventCh = d.bus.SubscribeFiltered(subID, allTypes...)
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		d.run(ctx, subID)
	}()
}

// Stop cancels the dispatcher and waits for all in-flight deliveries.
func (d *Dispatcher) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	d.wg.Wait()
}

// run is the main event loop.
func (d *Dispatcher) run(ctx context.Context, subID string) {
	defer d.bus.Unsubscribe(subID)

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-d.eventCh:
			if !ok {
				return
			}
			d.dispatch(ctx, ev)
		}
	}
}

// dispatch sends an event to all matching webhooks.
func (d *Dispatcher) dispatch(ctx context.Context, ev events.Event) {
	payload := WebhookPayload{
		Event:     ev.Type,
		Timestamp: ev.Timestamp,
		SessionID: ev.SessionID,
		RepoName:  ev.RepoName,
		RepoPath:  ev.RepoPath,
		Provider:  ev.Provider,
		NodeID:    ev.NodeID,
		Data:      ev.Data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Warn("webhook: failed to marshal payload", "event", ev.Type, "err", err)
		return
	}

	for i := range d.configs {
		cfg := &d.configs[i]
		if !d.matchesFilter(cfg, ev.Type) {
			continue
		}
		d.deliverWithRetry(ctx, cfg, body, ev.Type, ev.Timestamp)
	}
}

// matchesFilter returns true if the event type is accepted by this config.
func (d *Dispatcher) matchesFilter(cfg *WebhookConfig, evType events.EventType) bool {
	if len(cfg.Events) == 0 {
		return true // no filter = all events
	}
	for _, e := range cfg.Events {
		if events.EventType(e) == evType {
			return true
		}
	}
	return false
}

// deliverWithRetry attempts delivery with exponential backoff.
func (d *Dispatcher) deliverWithRetry(ctx context.Context, cfg *WebhookConfig, body []byte, evType events.EventType, ts time.Time) {
	cs := d.circuits[cfg.URL]
	now := d.nowFunc()

	if cs.isOpen(now) {
		slog.Debug("webhook: circuit open, skipping", "url", cfg.URL, "event", evType)
		return
	}

	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0 // -1 means no retries
	} else if maxRetries == 0 {
		maxRetries = 3 // default
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(math.Pow(2, float64(attempt-1))) * time.Second
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}

		err := d.deliver(ctx, cfg, body, evType, ts)
		if err == nil {
			cs.recordSuccess()
			return
		}

		slog.Warn("webhook: delivery failed",
			"url", cfg.URL, "event", evType,
			"attempt", attempt+1, "err", err)
	}

	// All retries exhausted.
	cs.recordFailure(d.nowFunc())
}

// deliver performs a single HTTP POST.
func (d *Dispatcher) deliver(ctx context.Context, cfg *WebhookConfig, body []byte, evType events.EventType, ts time.Time) error {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	client := d.client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Ralph-Event", string(evType))
	req.Header.Set("X-Ralph-Timestamp", ts.UTC().Format(time.RFC3339))

	if cfg.Secret != "" {
		sig := computeHMAC(body, []byte(cfg.Secret))
		req.Header.Set("X-Ralph-Signature", sig)
	}

	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("http %d from %s", resp.StatusCode, cfg.URL)
}

// computeHMAC returns the hex-encoded HMAC-SHA256 of data using key.
func computeHMAC(data, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks that a webhook payload signature is valid.
// This is exported so receivers can verify incoming webhook payloads.
func VerifySignature(body []byte, secret, signature string) bool {
	expected := computeHMAC(body, []byte(secret))
	return hmac.Equal([]byte(expected), []byte(signature))
}
