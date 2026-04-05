package common

import (
	"sync"

	"github.com/hairglasses-studio/runmylife/internal/events"
)

var (
	eventBus   *events.Bus
	eventBusMu sync.RWMutex
)

// SetBus stores the event bus for MCP tool access.
// Called once during MCP server initialization.
func SetBus(bus *events.Bus) {
	eventBusMu.Lock()
	defer eventBusMu.Unlock()
	eventBus = bus
}

// GetBus returns the event bus, or nil if not configured.
func GetBus() *events.Bus {
	eventBusMu.RLock()
	defer eventBusMu.RUnlock()
	return eventBus
}

// GetEmitter returns a type-safe emitter wrapping the event bus.
// Returns nil if no bus was configured.
func GetEmitter() *events.Emitter {
	bus := GetBus()
	if bus == nil {
		return nil
	}
	return events.NewEmitter(bus)
}
