package cmd

import (
	"github.com/hairglasses-studio/ralphglasses/internal/bootstrap"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// initStore creates a SQLite-backed session store at ~/.ralphglasses/state.db.
// On failure it logs a warning and returns a MemoryStore so the process can
// still start without persistence.
func initStore() session.Store {
	return bootstrap.InitStore()
}

// initManagerWithStore creates a session manager backed by SQLite persistence.
// If bus is nil, the manager will operate without event publishing.
func initManagerWithStore(bus *events.Bus) *session.Manager {
	return bootstrap.InitManagerWithStore(bus)
}

// initManagerRuntime returns a fully initialized manager with store-backed
// persistence, optional scan-root config applied, and startup hygiene completed.
func initManagerRuntime(scanRoot string, bus *events.Bus) *session.Manager {
	return bootstrap.InitManagerRuntime(scanRoot, bus)
}
