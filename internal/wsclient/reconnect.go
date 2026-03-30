package wsclient

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"
)

// Connection state constants for the ReconnectingClient.
const (
	StateDisconnected = "disconnected"
	StateConnecting   = "connecting"
	StateConnected    = "connected"
	StateReconnecting = "reconnecting"
	StateClosed       = "closed"
)

// Default backoff parameters.
const (
	DefaultInitialBackoff = 1 * time.Second
	DefaultMaxBackoff     = 60 * time.Second
	DefaultBackoffFactor  = 2.0
	DefaultMaxRetries     = 0 // 0 means unlimited retries.
)

// ReconnectEvent describes a reconnection lifecycle event.
type ReconnectEvent struct {
	// State is the new connection state.
	State string

	// Attempt is the current retry attempt (1-indexed). Zero for non-retry events.
	Attempt int

	// Delay is the backoff delay before this attempt (zero for first connect).
	Delay time.Duration

	// Err is the error that triggered reconnection, or nil on success.
	Err error
}

// ReconnectCallback is called on reconnection lifecycle events.
type ReconnectCallback func(ReconnectEvent)

// ReconnectOption configures a ReconnectingClient.
type ReconnectOption func(*ReconnectingClient)

// WithInitialBackoff sets the initial backoff duration (default 1s).
func WithInitialBackoff(d time.Duration) ReconnectOption {
	return func(rc *ReconnectingClient) {
		if d > 0 {
			rc.initialBackoff = d
		}
	}
}

// WithMaxBackoff sets the maximum backoff duration (default 60s).
func WithMaxBackoff(d time.Duration) ReconnectOption {
	return func(rc *ReconnectingClient) {
		if d > 0 {
			rc.maxBackoff = d
		}
	}
}

// WithBackoffFactor sets the exponential backoff multiplier (default 2.0).
func WithBackoffFactor(f float64) ReconnectOption {
	return func(rc *ReconnectingClient) {
		if f > 1.0 {
			rc.backoffFactor = f
		}
	}
}

// WithMaxRetries sets the maximum number of reconnection attempts.
// 0 means unlimited (default).
func WithMaxRetries(n int) ReconnectOption {
	return func(rc *ReconnectingClient) {
		rc.maxRetries = n
	}
}

// WithOnReconnect sets a callback for reconnection events.
func WithOnReconnect(cb ReconnectCallback) ReconnectOption {
	return func(rc *ReconnectingClient) {
		rc.onReconnect = cb
	}
}

// ReconnectingClient wraps a base *Client and adds automatic reconnection
// with exponential backoff on disconnect. It tracks connection state and
// fires callbacks on state transitions.
type ReconnectingClient struct {
	client *Client

	// Backoff configuration.
	initialBackoff time.Duration
	maxBackoff     time.Duration
	backoffFactor  float64
	maxRetries     int

	// State tracking.
	mu    sync.RWMutex
	state string

	// Callback.
	onReconnect ReconnectCallback

	// closed is set to true once Close is called to prevent further reconnects.
	closed bool
}

// NewReconnectingClient wraps an existing *Client with automatic reconnection.
func NewReconnectingClient(client *Client, opts ...ReconnectOption) *ReconnectingClient {
	rc := &ReconnectingClient{
		client:         client,
		initialBackoff: DefaultInitialBackoff,
		maxBackoff:     DefaultMaxBackoff,
		backoffFactor:  DefaultBackoffFactor,
		maxRetries:     DefaultMaxRetries,
		state:          StateDisconnected,
	}
	for _, opt := range opts {
		opt(rc)
	}
	return rc
}

// State returns the current connection state.
func (rc *ReconnectingClient) State() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.state
}

// setState updates the state and fires the callback.
func (rc *ReconnectingClient) setState(state string, evt ReconnectEvent) {
	rc.mu.Lock()
	rc.state = state
	cb := rc.onReconnect
	rc.mu.Unlock()

	evt.State = state
	if cb != nil {
		cb(evt)
	}
}

// Connect establishes the initial connection with retry support.
func (rc *ReconnectingClient) Connect(ctx context.Context) error {
	rc.mu.RLock()
	if rc.closed {
		rc.mu.RUnlock()
		return errors.New("wsclient: reconnecting client is closed")
	}
	rc.mu.RUnlock()

	rc.setState(StateConnecting, ReconnectEvent{})

	if err := rc.client.Connect(ctx); err != nil {
		rc.setState(StateDisconnected, ReconnectEvent{Err: err})
		return err
	}

	rc.setState(StateConnected, ReconnectEvent{})
	return nil
}

// Send sends a request, automatically reconnecting on failure with exponential
// backoff. If the underlying connection drops, Send retries the connection up
// to maxRetries times (unlimited when maxRetries == 0).
func (rc *ReconnectingClient) Send(ctx context.Context, req *Request) (*Response, error) {
	rc.mu.RLock()
	if rc.closed {
		rc.mu.RUnlock()
		return nil, errors.New("wsclient: reconnecting client is closed")
	}
	rc.mu.RUnlock()

	// Try sending directly first.
	resp, err := rc.client.Send(ctx, req)
	if err == nil {
		// If we were reconnecting, update state to connected.
		if rc.State() != StateConnected {
			rc.setState(StateConnected, ReconnectEvent{})
		}
		return resp, nil
	}

	// If context is already done, don't attempt reconnection.
	if ctx.Err() != nil {
		return nil, err
	}

	// Enter reconnection loop.
	return rc.reconnectAndSend(ctx, req, err)
}

// reconnectAndSend runs the reconnection loop with exponential backoff,
// then retries the send.
func (rc *ReconnectingClient) reconnectAndSend(ctx context.Context, req *Request, lastErr error) (*Response, error) {
	attempt := 0

	for {
		attempt++

		// Check max retries.
		if rc.maxRetries > 0 && attempt > rc.maxRetries {
			rc.setState(StateDisconnected, ReconnectEvent{
				Attempt: attempt - 1,
				Err:     lastErr,
			})
			return nil, lastErr
		}

		// Check if closed.
		rc.mu.RLock()
		closed := rc.closed
		rc.mu.RUnlock()
		if closed {
			return nil, errors.New("wsclient: reconnecting client is closed")
		}

		// Calculate backoff.
		delay := rc.backoffDelay(attempt)

		rc.setState(StateReconnecting, ReconnectEvent{
			Attempt: attempt,
			Delay:   delay,
			Err:     lastErr,
		})

		// Wait for backoff or context cancellation.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}

		// Close any stale connection and try to reconnect.
		_ = rc.client.closeConn()

		if err := rc.client.Connect(ctx); err != nil {
			lastErr = err
			continue
		}

		// Connection restored, try sending.
		resp, err := rc.client.Send(ctx, req)
		if err != nil {
			lastErr = err
			continue
		}

		rc.setState(StateConnected, ReconnectEvent{
			Attempt: attempt,
		})
		return resp, nil
	}
}

// backoffDelay computes the delay for a given attempt using capped exponential
// backoff: min(initialBackoff * factor^(attempt-1), maxBackoff).
func (rc *ReconnectingClient) backoffDelay(attempt int) time.Duration {
	backoff := float64(rc.initialBackoff) * math.Pow(rc.backoffFactor, float64(attempt-1))
	if backoff > float64(rc.maxBackoff) {
		backoff = float64(rc.maxBackoff)
	}
	return time.Duration(backoff)
}

// Close permanently shuts down the client, preventing further reconnections.
func (rc *ReconnectingClient) Close() error {
	rc.mu.Lock()
	rc.closed = true
	rc.state = StateClosed
	cb := rc.onReconnect
	rc.mu.Unlock()

	if cb != nil {
		cb(ReconnectEvent{State: StateClosed})
	}

	return rc.client.Close()
}

// Client returns the underlying *Client. Useful for direct access when
// reconnection is not needed.
func (rc *ReconnectingClient) Client() *Client {
	return rc.client
}
