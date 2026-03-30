package events

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// NATSConfig holds configuration for the NATS JetStream transport.
type NATSConfig struct {
	// URL is the NATS server URL (e.g. "nats://localhost:4222").
	URL string

	// StreamName is the JetStream stream name. Defaults to "RALPH_EVENTS".
	StreamName string

	// SubjectPrefix is prepended to event type subjects.
	// For example, prefix "ralph.events" yields subjects like "ralph.events.session.started".
	// Defaults to "ralph.events".
	SubjectPrefix string

	// Credentials is the path to a NATS credentials file (.creds).
	// Empty means no authentication.
	Credentials string

	// TLS configures TLS for the NATS connection. Nil means no TLS.
	TLS *tls.Config

	// MaxReconnects is the maximum number of reconnect attempts. Defaults to 60.
	MaxReconnects int

	// ReconnectWait is the base interval between reconnect attempts. Defaults to 2s.
	// The NATS client applies jitter automatically.
	ReconnectWait time.Duration

	// ConnectTimeout is the timeout for the initial connection. Defaults to 5s.
	ConnectTimeout time.Duration

	// AckWait is how long the server waits for an ack before redelivery. Defaults to 30s.
	AckWait time.Duration
}

// defaults fills in zero-value fields with sensible defaults.
func (c *NATSConfig) defaults() {
	if c.StreamName == "" {
		c.StreamName = "RALPH_EVENTS"
	}
	if c.SubjectPrefix == "" {
		c.SubjectPrefix = "ralph.events"
	}
	if c.MaxReconnects < 0 {
		c.MaxReconnects = 0 // nats.MaxReconnects(-1) means infinite in the client
	}
	if c.MaxReconnects == 0 {
		c.MaxReconnects = 60
	}
	if c.ReconnectWait == 0 {
		c.ReconnectWait = 2 * time.Second
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 5 * time.Second
	}
	if c.AckWait == 0 {
		c.AckWait = 30 * time.Second
	}
}

// natsSubscription tracks a single subscriber's consumer and delivery channel.
type natsSubscription struct {
	ch     chan Event
	filter func(Event) bool
	cons   jetstream.ConsumeContext
}

// NATSTransport implements EventTransport over NATS JetStream.
// It publishes events to JetStream subjects and delivers them to local subscribers
// via push-based consumers.
type NATSTransport struct {
	cfg NATSConfig
	nc  *nats.Conn
	js  jetstream.JetStream

	mu   sync.RWMutex
	subs map[string]*natsSubscription

	closed chan struct{}
}

// NewNATSTransport connects to NATS, ensures the JetStream stream exists,
// and returns a ready transport. If NATS is unreachable, it returns an error;
// callers should fall back to the in-memory transport.
func NewNATSTransport(cfg NATSConfig) (*NATSTransport, error) {
	cfg.defaults()

	opts := []nats.Option{
		nats.MaxReconnects(cfg.MaxReconnects),
		nats.ReconnectWait(cfg.ReconnectWait),
		nats.Timeout(cfg.ConnectTimeout),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			slog.Warn("nats disconnected", "err", err)
		}),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			slog.Info("nats reconnected")
		}),
		nats.ClosedHandler(func(_ *nats.Conn) {
			slog.Info("nats connection closed")
		}),
	}

	if cfg.Credentials != "" {
		opts = append(opts, nats.UserCredentials(cfg.Credentials))
	}
	if cfg.TLS != nil {
		opts = append(opts, nats.Secure(cfg.TLS))
	}

	nc, err := nats.Connect(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect %s: %w", cfg.URL, err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats jetstream init: %w", err)
	}

	// Ensure the stream exists. CreateOrUpdate is idempotent.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     cfg.StreamName,
		Subjects: []string{cfg.SubjectPrefix + ".>"},
		// Retain up to 10k messages or 100MB, whichever comes first
		MaxMsgs:  10000,
		MaxBytes: 100 * 1024 * 1024,
		// Discard old messages when limits are hit
		Discard:   jetstream.DiscardOld,
		Retention: jetstream.LimitsPolicy,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats stream ensure %q: %w", cfg.StreamName, err)
	}

	return &NATSTransport{
		cfg:    cfg,
		nc:     nc,
		js:     js,
		subs:   make(map[string]*natsSubscription),
		closed: make(chan struct{}),
	}, nil
}

// subjectForEvent maps an event type to a NATS subject.
// E.g. EventType "session.started" -> "ralph.events.session.started"
func (t *NATSTransport) subjectForEvent(et EventType) string {
	return t.cfg.SubjectPrefix + "." + string(et)
}

// Publish serializes the event as JSON and publishes to the JetStream
// subject derived from the event type.
func (t *NATSTransport) Publish(ctx context.Context, event Event) error {
	select {
	case <-t.closed:
		return fmt.Errorf("nats transport closed")
	default:
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("nats publish marshal: %w", err)
	}

	subject := t.subjectForEvent(event.Type)

	_, err = t.js.Publish(ctx, subject, data)
	if err != nil {
		slog.Warn("nats publish failed", "subject", subject, "err", err)
		return fmt.Errorf("nats publish %s: %w", subject, err)
	}
	return nil
}

// Subscribe creates a push-based JetStream consumer and returns a channel
// that receives matching events. The subscriber name is used as a durable
// consumer name (sanitized for NATS). If filter is non-nil, only events
// passing the filter are delivered to the channel.
func (t *NATSTransport) Subscribe(ctx context.Context, subscriber string, filter func(Event) bool) (<-chan Event, error) {
	select {
	case <-t.closed:
		return nil, fmt.Errorf("nats transport closed")
	default:
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// If already subscribed, return existing channel
	if sub, ok := t.subs[subscriber]; ok {
		return sub.ch, nil
	}

	ch := make(chan Event, 100)

	// Create an ephemeral ordered consumer that receives all stream messages.
	cons, err := t.js.OrderedConsumer(ctx, t.cfg.StreamName, jetstream.OrderedConsumerConfig{
		FilterSubjects: []string{t.cfg.SubjectPrefix + ".>"},
		DeliverPolicy:  jetstream.DeliverNewPolicy,
	})
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("nats consumer create %q: %w", subscriber, err)
	}

	// Start consuming messages
	consumeCtx, err := cons.Consume(func(msg jetstream.Msg) {
		var event Event
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			slog.Warn("nats event unmarshal failed", "subscriber", subscriber, "err", err)
			return
		}

		// Apply local filter
		if filter != nil && !filter(event) {
			return
		}

		// Non-blocking send, same semantics as memTransport
		select {
		case ch <- event:
		default:
			// Subscriber is too slow, drop
		}
	})
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("nats consume start %q: %w", subscriber, err)
	}

	t.subs[subscriber] = &natsSubscription{
		ch:     ch,
		filter: filter,
		cons:   consumeCtx,
	}

	return ch, nil
}

// Unsubscribe stops the consumer for the given subscriber and closes its channel.
func (t *NATSTransport) Unsubscribe(subscriber string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	sub, ok := t.subs[subscriber]
	if !ok {
		return
	}
	delete(t.subs, subscriber)

	// Stop the consumer first, then close the channel
	sub.cons.Stop()
	close(sub.ch)
}

// Close stops all consumers, closes all subscriber channels, and closes
// the NATS connection.
func (t *NATSTransport) Close() error {
	// Signal closed
	select {
	case <-t.closed:
		return nil // already closed
	default:
		close(t.closed)
	}

	t.mu.Lock()
	for id, sub := range t.subs {
		sub.cons.Stop()
		close(sub.ch)
		delete(t.subs, id)
	}
	t.mu.Unlock()

	if t.nc != nil {
		t.nc.Close()
	}
	return nil
}

// sanitizeConsumerName converts a subscriber ID into a valid NATS consumer name.
// NATS consumer names must be alphanumeric, dash, or underscore.
func sanitizeConsumerName(name string) string {
	b := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			b = append(b, c)
		} else {
			b = append(b, '_')
		}
	}
	return string(b)
}

// SubjectForEvent exposes the subject mapping for testing.
func SubjectForEvent(prefix string, et EventType) string {
	return prefix + "." + string(et)
}

// NATSTransportFromConn creates a NATSTransport using an existing NATS connection.
// This is primarily useful for testing with embedded NATS servers.
func NATSTransportFromConn(nc *nats.Conn, cfg NATSConfig) (*NATSTransport, error) {
	cfg.defaults()

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("nats jetstream init: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     cfg.StreamName,
		Subjects: []string{cfg.SubjectPrefix + ".>"},
		MaxMsgs:  10000,
		MaxBytes: 100 * 1024 * 1024,
		Discard:  jetstream.DiscardOld,
	})
	if err != nil {
		return nil, fmt.Errorf("nats stream ensure %q: %w", cfg.StreamName, err)
	}

	return &NATSTransport{
		cfg:    cfg,
		nc:     nc,
		js:     js,
		subs:   make(map[string]*natsSubscription),
		closed: make(chan struct{}),
	}, nil
}

// Connected returns true if the underlying NATS connection is active.
func (t *NATSTransport) Connected() bool {
	return t.nc != nil && t.nc.IsConnected()
}

// Stats returns NATS connection statistics for monitoring.
func (t *NATSTransport) Stats() nats.Statistics {
	if t.nc == nil {
		return nats.Statistics{}
	}
	return t.nc.Stats()
}

// SubjectPrefix returns the configured subject prefix. It is safe to call
// from any goroutine because the prefix is immutable after construction.
func (t *NATSTransport) SubjectPrefix() string {
	return t.cfg.SubjectPrefix
}

// StreamName returns the configured JetStream stream name.
func (t *NATSTransport) StreamName() string {
	return t.cfg.StreamName
}

// parseEventTypeFromSubject extracts the EventType from a NATS subject by
// stripping the configured prefix. For example, given prefix "ralph.events"
// and subject "ralph.events.session.started", it returns "session.started".
func parseEventTypeFromSubject(prefix, subject string) EventType {
	if !strings.HasPrefix(subject, prefix+".") {
		return EventType(subject)
	}
	return EventType(subject[len(prefix)+1:])
}
