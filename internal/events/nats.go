package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// NATSConnection abstracts the essential NATS JetStream operations so that
// NATSBus can be tested with a mock and swapped to any NATS client library
// without changing the bus code.
type NATSConnection interface {
	// Publish sends data to the given subject. The implementation must
	// persist the message in JetStream and return the sequence number
	// assigned by the stream, or an error.
	Publish(ctx context.Context, subject string, data []byte) (seq uint64, err error)

	// Subscribe creates a push-based consumer on the given subject pattern.
	// consumerName is used for durable consumers (empty = ephemeral).
	// startSeq, when > 0, requests replay starting from that sequence number;
	// zero means deliver only new messages.
	// The handler is called for each message; it must call Ack() on the
	// delivered Msg to acknowledge processing.
	Subscribe(ctx context.Context, subject string, consumerName string, startSeq uint64, handler func(NATSMsg)) (NATSSubscription, error)

	// Close tears down the underlying connection and all subscriptions.
	Close() error
}

// NATSMsg represents a single message delivered by the NATS connection.
type NATSMsg interface {
	// Data returns the message payload.
	Data() []byte

	// Subject returns the NATS subject the message was published to.
	Subject() string

	// Sequence returns the JetStream stream sequence number.
	Sequence() uint64

	// Ack acknowledges the message so the server won't redeliver it.
	Ack() error

	// Nak signals negative acknowledgement, requesting redelivery.
	Nak() error
}

// NATSSubscription represents an active consumer that can be stopped.
type NATSSubscription interface {
	// Stop halts message delivery. After Stop returns the handler will
	// not be called again.
	Stop()
}

// NATSBusConfig configures a NATSBus.
type NATSBusConfig struct {
	// SubjectPrefix is prepended to event subjects.
	// Default: "ralph.events".
	SubjectPrefix string

	// FleetID partitions subjects per fleet.
	// When non-empty, subjects become "<prefix>.<fleet_id>.<event_type>".
	// When empty, subjects are "<prefix>.<event_type>".
	FleetID string

	// ChannelBuffer is the capacity of subscriber delivery channels.
	// Default: 100.
	ChannelBuffer int
}

func (c *NATSBusConfig) defaults() {
	if c.SubjectPrefix == "" {
		c.SubjectPrefix = "ralph.events"
	}
	if c.ChannelBuffer <= 0 {
		c.ChannelBuffer = 100
	}
}

// natsBusSub tracks a single subscriber's state inside NATSBus.
type natsBusSub struct {
	ch     chan Event
	filter func(Event) bool
	sub    NATSSubscription
}

// NATSBus implements EventTransport using a NATSConnection interface.
// It supports durable consumers, replay from a stream sequence number,
// and fleet-partitioned subjects.
type NATSBus struct {
	conn NATSConnection
	cfg  NATSBusConfig

	mu   sync.RWMutex
	subs map[string]*natsBusSub

	// seq tracks the last published sequence for callers that need it.
	seq atomic.Uint64

	closed chan struct{}
}

// NewNATSBus creates a NATSBus that delegates publish/subscribe to the
// provided NATSConnection. The connection must already be established.
func NewNATSBus(conn NATSConnection, cfg NATSBusConfig) *NATSBus {
	cfg.defaults()
	return &NATSBus{
		conn:   conn,
		cfg:    cfg,
		subs:   make(map[string]*natsBusSub),
		closed: make(chan struct{}),
	}
}

// subjectFor builds the NATS subject for a given event type, incorporating
// the fleet partition when configured.
func (b *NATSBus) subjectFor(et EventType) string {
	if b.cfg.FleetID != "" {
		return b.cfg.SubjectPrefix + "." + b.cfg.FleetID + "." + string(et)
	}
	return b.cfg.SubjectPrefix + "." + string(et)
}

// subjectWildcard returns the wildcard subject that matches all events
// for this bus's fleet partition.
func (b *NATSBus) subjectWildcard() string {
	if b.cfg.FleetID != "" {
		return b.cfg.SubjectPrefix + "." + b.cfg.FleetID + ".>"
	}
	return b.cfg.SubjectPrefix + ".>"
}

// parseEventType extracts the EventType from a fully-qualified subject.
func (b *NATSBus) parseEventType(subject string) EventType {
	prefix := b.cfg.SubjectPrefix
	if b.cfg.FleetID != "" {
		prefix += "." + b.cfg.FleetID
	}
	if strings.HasPrefix(subject, prefix+".") {
		return EventType(subject[len(prefix)+1:])
	}
	return EventType(subject)
}

// Publish serializes the event and publishes it via the NATSConnection.
func (b *NATSBus) Publish(ctx context.Context, event Event) error {
	select {
	case <-b.closed:
		return fmt.Errorf("nats bus closed")
	default:
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Version == 0 {
		event.Version = 1
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("nats bus marshal: %w", err)
	}

	subject := b.subjectFor(event.Type)
	seq, err := b.conn.Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("nats bus publish %s: %w", subject, err)
	}
	b.seq.Store(seq)
	return nil
}

// Subscribe creates a consumer that delivers all events to the returned
// channel. The subscriber name is used as a durable consumer name.
// If filter is non-nil, only matching events are sent to the channel.
func (b *NATSBus) Subscribe(ctx context.Context, subscriber string, filter func(Event) bool) (<-chan Event, error) {
	return b.SubscribeFrom(ctx, subscriber, 0, filter)
}

// SubscribeFrom creates a durable consumer that replays events starting from
// the given JetStream sequence number. When startSeq is 0, only new messages
// are delivered (equivalent to Subscribe).
func (b *NATSBus) SubscribeFrom(ctx context.Context, subscriber string, startSeq uint64, filter func(Event) bool) (<-chan Event, error) {
	select {
	case <-b.closed:
		return nil, fmt.Errorf("nats bus closed")
	default:
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if existing, ok := b.subs[subscriber]; ok {
		return existing.ch, nil
	}

	ch := make(chan Event, b.cfg.ChannelBuffer)

	sub, err := b.conn.Subscribe(ctx, b.subjectWildcard(), subscriber, startSeq, func(msg NATSMsg) {
		var event Event
		if err := json.Unmarshal(msg.Data(), &event); err != nil {
			slog.Warn("nats bus unmarshal failed", "subscriber", subscriber, "err", err)
			// Ack to avoid infinite redelivery of malformed messages.
			_ = msg.Ack()
			return
		}

		// Fill type from subject if not set in payload.
		if event.Type == "" {
			event.Type = b.parseEventType(msg.Subject())
		}

		if filter != nil && !filter(event) {
			_ = msg.Ack()
			return
		}

		select {
		case ch <- event:
			_ = msg.Ack()
		default:
			// Channel full — nak for redelivery to avoid silent loss
			// on durable consumers. Callers consuming fast enough will
			// never hit this path.
			_ = msg.Nak()
		}
	})
	if err != nil {
		close(ch)
		return nil, fmt.Errorf("nats bus subscribe %q: %w", subscriber, err)
	}

	b.subs[subscriber] = &natsBusSub{
		ch:     ch,
		filter: filter,
		sub:    sub,
	}
	return ch, nil
}

// Unsubscribe stops the consumer for the given subscriber and closes its
// delivery channel.
func (b *NATSBus) Unsubscribe(subscriber string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub, ok := b.subs[subscriber]
	if !ok {
		return
	}
	delete(b.subs, subscriber)
	sub.sub.Stop()
	close(sub.ch)
}

// Close stops all consumers, closes all channels, and closes the
// underlying NATSConnection.
func (b *NATSBus) Close() error {
	select {
	case <-b.closed:
		return nil
	default:
		close(b.closed)
	}

	b.mu.Lock()
	for id, sub := range b.subs {
		sub.sub.Stop()
		close(sub.ch)
		delete(b.subs, id)
	}
	b.mu.Unlock()

	return b.conn.Close()
}

// LastSequence returns the sequence number of the most recently published
// message. Useful for callers that want to checkpoint and later replay
// via SubscribeFrom.
func (b *NATSBus) LastSequence() uint64 {
	return b.seq.Load()
}

// SubjectFor exposes the subject mapping for external callers. It is safe
// to call from any goroutine.
func (b *NATSBus) SubjectFor(et EventType) string {
	return b.subjectFor(et)
}

// FleetID returns the fleet partition, or empty string if unpartitioned.
func (b *NATSBus) FleetID() string {
	return b.cfg.FleetID
}

// Verify NATSBus satisfies EventTransport at compile time.
var _ EventTransport = (*NATSBus)(nil)
