package bootstrap

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// DefaultDocsRoot resolves the shared docs repo path for a workspace scan root.
func DefaultDocsRoot(scanRoot string) string {
	return filepath.Join(filepath.Dir(scanRoot), "docs")
}

type storeInitResult struct {
	store      session.Store
	path       string
	err        error
	persistent bool
}

// InitStore creates a SQLite-backed session store at ~/.ralphglasses/state.db.
// On failure it logs a warning and returns a MemoryStore so the process can
// still start without persistence.
func InitStore() session.Store {
	return initStore().store
}

func initStore() storeInitResult {
	dbPath := ralphpath.SQLiteStorePath()
	store, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		slog.Warn("sqlite store: falling back to memory store", "path", dbPath, "error", err)
		return storeInitResult{store: session.NewMemoryStore(), path: dbPath, err: err}
	}
	return storeInitResult{store: store, path: dbPath, persistent: true}
}

func publishBootstrapError(bus *events.Bus, component string, err error, extra map[string]any) {
	if bus == nil || err == nil {
		return
	}

	data := map[string]any{
		"component": component,
		"error":     err.Error(),
	}
	for key, value := range extra {
		data[key] = value
	}

	bus.Publish(events.Event{
		Type:      events.SessionError,
		Timestamp: time.Now(),
		Data:      data,
	})
}

func publishStoreFallback(bus *events.Bus, result storeInitResult) {
	if result.persistent || result.err == nil {
		return
	}

	data := map[string]any{
		"backend":          "sqlite",
		"fallback_backend": "memory",
	}
	if result.path != "" {
		data["path"] = result.path
	}
	publishBootstrapError(bus, "bootstrap.store", result.err, data)
}

// InitManagerWithStore creates a session manager backed by SQLite persistence.
// If bus is nil, the manager will operate without event publishing.
func InitManagerWithStore(bus *events.Bus) *session.Manager {
	result := initStore()
	publishStoreFallback(bus, result)
	if bus != nil {
		return session.NewManagerWithStore(result.store, bus)
	}
	mgr := session.NewManager()
	mgr.SetStore(result.store)
	return mgr
}

// InitManagerRuntime returns a fully initialized manager with store-backed
// persistence, optional scan-root config applied, and startup hygiene completed.
func InitManagerRuntime(scanRoot string, bus *events.Bus) *session.Manager {
	mgr := InitManagerWithStore(bus)
	if scanRoot != "" {
		configPath := filepath.Join(scanRoot, ".ralphrc")
		if _, err := os.Stat(configPath); err == nil {
			cfg, cfgErr := model.LoadConfig(context.Background(), scanRoot)
			if cfgErr != nil {
				slog.Warn("manager bootstrap: failed to load scan-root config", "path", scanRoot, "error", cfgErr)
				publishBootstrapError(bus, "bootstrap.config", cfgErr, map[string]any{"path": configPath})
			} else {
				mgr.ApplyConfig(cfg)
			}
		}
	}
	mgr.Init()
	return mgr
}

// ConfigureMCPRuntime replaces the default in-memory session manager with the
// normal store-backed runtime and wires optional fleet and autonomy subsystems.
func ConfigureMCPRuntime(scanRoot string, bus *events.Bus, rg *mcpserver.Server) func() {
	if rg == nil {
		return func() {}
	}

	mgr := InitManagerRuntime(scanRoot, bus)
	rg.SessMgr = mgr
	rg.InitSelfImprovement(filepath.Join(scanRoot, ".ralph"), 0)
	rg.WireAutoOptimizer(mgr)

	var cleanups []func()

	if fleetURL := strings.TrimSpace(os.Getenv("RALPH_FLEET_URL")); fleetURL != "" {
		client := fleet.NewClient(fleetURL)
		rg.InitFleetTools(nil, client, rg.HITLTracker, rg.DecisionLog, rg.FeedbackAnalyzer)
		mgr.SetStructuredTeamBackend(fleet.NewStructuredTeamBackend(nil, client))
	}

	docsRoot := DefaultDocsRoot(scanRoot)
	if _, err := os.Stat(filepath.Join(docsRoot, ".docs.sqlite")); err == nil {
		gateway, gwErr := session.NewDocsResearchGateway(docsRoot)
		if gwErr != nil {
			slog.Warn("mcp: research gateway unavailable", "docs_root", docsRoot, "error", gwErr)
			publishBootstrapError(bus, "bootstrap.research_gateway", gwErr, map[string]any{"docs_root": docsRoot})
		} else {
			mgr.SetResearchGateway(gateway)
			cleanups = append(cleanups, func() {
				_ = gateway.Close()
			})
		}
	}

	if bus != nil {
		mgr.SetCrashRecovery(session.NewCrashRecoveryOrchestrator(mgr, bus, mgr.Store()))
	}

	// Restore the persisted autonomy level only after all supervisor subsystems
	// are attached so reactivated supervisors come up fully wired.
	mgr.RestoreAutonomyLevel()
	if level := mgr.GetAutonomyLevel(); level >= session.LevelAutoOptimize {
		mgr.SetAutonomyLevel(level, scanRoot)
	}

	return func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
}
