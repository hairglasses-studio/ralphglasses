package events

import (
	"context"
	"database/sql"
	"log"
	"sync"
)

// Bus is an in-process publish/subscribe event bus.
// Optionally persists events to SQLite for audit/replay.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]Handler
	db          *sql.DB // nil = no persistence
}

// NewBus creates an event bus. Pass nil for db to disable persistence.
func NewBus(db *sql.DB) *Bus {
	return &Bus{
		subscribers: make(map[EventType][]Handler),
		db:          db,
	}
}

// Subscribe registers a handler for the given event type.
func (b *Bus) Subscribe(t EventType, h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[t] = append(b.subscribers[t], h)
}

// SubscribeAll registers a handler for every event type.
func (b *Bus) SubscribeAll(h Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Use empty string as wildcard key
	b.subscribers["*"] = append(b.subscribers["*"], h)
}

// Publish dispatches an event to all matching subscribers synchronously.
// Persists the event to SQLite if a DB is configured.
func (b *Bus) Publish(ctx context.Context, e Event) {
	if b.db != nil {
		if err := PersistEvent(ctx, b.db, e); err != nil {
			log.Printf("[events] persist error: %v", err)
		}
	}

	b.mu.RLock()
	handlers := make([]Handler, 0, len(b.subscribers[e.Type])+len(b.subscribers["*"]))
	handlers = append(handlers, b.subscribers[e.Type]...)
	handlers = append(handlers, b.subscribers["*"]...)
	b.mu.RUnlock()

	for _, h := range handlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[events] panic in handler for %s: %v", e.Type, r)
				}
			}()
			h(e)
		}()
	}
}

// PublishAsync dispatches an event to all matching subscribers in goroutines.
// Persists the event to SQLite if a DB is configured.
func (b *Bus) PublishAsync(ctx context.Context, e Event) {
	if b.db != nil {
		if err := PersistEvent(ctx, b.db, e); err != nil {
			log.Printf("[events] persist error: %v", err)
		}
	}

	b.mu.RLock()
	handlers := make([]Handler, 0, len(b.subscribers[e.Type])+len(b.subscribers["*"]))
	handlers = append(handlers, b.subscribers[e.Type]...)
	handlers = append(handlers, b.subscribers["*"]...)
	b.mu.RUnlock()

	for _, h := range handlers {
		go func(fn Handler) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[events] panic in async handler for %s: %v", e.Type, r)
				}
			}()
			fn(e)
		}(h)
	}
}
