package cmd

import (
	"log/slog"
	"os"
	"path/filepath"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// initStore creates a SQLite-backed session store at ~/.ralphglasses/state.db.
// On failure it logs a warning and returns a MemoryStore so the process can
// still start without persistence.
func initStore() session.Store {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("sqlite store: cannot resolve home dir, using memory store", "error", err)
		return session.NewMemoryStore()
	}
	dbPath := filepath.Join(home, ".ralphglasses", "state.db")
	store, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		slog.Warn("sqlite store: falling back to memory store", "path", dbPath, "error", err)
		return session.NewMemoryStore()
	}
	return store
}

// initManagerWithStore creates a session manager backed by SQLite persistence.
// If bus is nil, the manager will operate without event publishing.
func initManagerWithStore(bus *events.Bus) *session.Manager {
	store := initStore()
	if bus != nil {
		return session.NewManagerWithStore(store, bus)
	}
	mgr := session.NewManager()
	mgr.SetStore(store)
	return mgr
}
