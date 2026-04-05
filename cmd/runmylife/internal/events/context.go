package events

import "context"

type contextKey struct{}

// WithBus stores the event bus in the context.
// MCP tools use init() registration and can't accept constructor deps;
// context injection bridges the gap.
func WithBus(ctx context.Context, bus *Bus) context.Context {
	return context.WithValue(ctx, contextKey{}, bus)
}

// BusFromContext retrieves the event bus from the context.
// Returns nil if no bus was stored.
func BusFromContext(ctx context.Context) *Bus {
	bus, _ := ctx.Value(contextKey{}).(*Bus)
	return bus
}

// EmitterFromContext returns a type-safe emitter wrapping the bus in context.
// Returns nil if no bus was stored.
func EmitterFromContext(ctx context.Context) *Emitter {
	bus := BusFromContext(ctx)
	if bus == nil {
		return nil
	}
	return NewEmitter(bus)
}
