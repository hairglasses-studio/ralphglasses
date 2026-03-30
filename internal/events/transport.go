package events

import "context"

// EventTransport abstracts the publish/subscribe mechanism for events.
// The default in-memory ring buffer (used by Bus) implements this interface.
// Future implementations (e.g. NATS, Redis) can be plugged in via
// WithTransport to distribute events across processes.
type EventTransport interface {
	// Publish sends an event to all matching subscribers.
	Publish(ctx context.Context, event Event) error

	// Subscribe returns a channel that receives events matching the filter.
	// If filter is nil, all events are delivered.
	Subscribe(ctx context.Context, subscriber string, filter func(Event) bool) (<-chan Event, error)

	// Unsubscribe removes a subscriber and closes its channel.
	Unsubscribe(subscriber string)

	// Close shuts down the transport and releases resources.
	Close() error
}
