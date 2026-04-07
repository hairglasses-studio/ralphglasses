package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/hooks"
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

// resolveScanPath returns the scan path from the environment or default.
func resolveScanPath() string {
	sp := os.Getenv("RALPHGLASSES_SCAN_PATH")
	if sp == "" {
		sp = "~/hairglasses-studio"
	}
	return util.ExpandHome(sp)
}

// setup creates and configures the MCP server with all tools registered.
// It returns the server, a cleanup function, and any error.
func setup(scanPath string) (*server.MCPServer, func(), error) {
	srv := registry.NewMCPServer(
		"ralphglasses",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	bus := events.NewBus(1000)
	hookExec := hooks.NewExecutor(bus)
	hookExec.Start()

	rg := mcpserver.NewServerWithBus(scanPath, bus)
	runtimeCleanup := configureRuntime(scanPath, bus, rg)
	rg.Register(srv)

	cleanup := func() {
		runtimeCleanup()
		hookExec.Stop()
	}

	return srv, cleanup, nil
}

func main() {
	scanPath := resolveScanPath()

	srv, cleanup, err := setup(scanPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "setup error: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	if err := registry.ServeAuto(srv); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
}

func configureRuntime(scanPath string, bus *events.Bus, rg *mcpserver.Server) func() {
	mgr := initManagerRuntime(scanPath, bus)
	rg.SessMgr = mgr
	rg.InitSelfImprovement(filepath.Join(scanPath, ".ralph"), 0)
	rg.WireAutoOptimizer(mgr)
	mgr.RestoreAutonomyLevel()

	var cleanups []func()
	if fleetURL := strings.TrimSpace(os.Getenv("RALPH_FLEET_URL")); fleetURL != "" {
		client := fleet.NewClient(fleetURL)
		rg.InitFleetTools(nil, client, rg.HITLTracker, rg.DecisionLog, rg.FeedbackAnalyzer)
		mgr.SetStructuredTeamBackend(fleet.NewStructuredTeamBackend(nil, client))
	}
	docsRoot := filepath.Join(filepath.Dir(scanPath), "docs")
	if _, err := os.Stat(filepath.Join(docsRoot, ".docs.sqlite")); err == nil {
		gateway, gwErr := session.NewDocsResearchGateway(docsRoot)
		if gwErr != nil {
			slog.Warn("mcp-main: research gateway unavailable", "docs_root", docsRoot, "error", gwErr)
		} else {
			mgr.SetResearchGateway(gateway)
			cleanups = append(cleanups, func() {
				_ = gateway.Close()
			})
		}
	}
	mgr.SetCrashRecovery(session.NewCrashRecoveryOrchestrator(mgr, bus, mgr.Store()))

	return func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
}

func initManagerRuntime(scanRoot string, bus *events.Bus) *session.Manager {
	mgr := initManagerWithStore(bus)
	if scanRoot != "" {
		if _, err := os.Stat(filepath.Join(scanRoot, ".ralphrc")); err == nil {
			cfg, cfgErr := model.LoadConfig(context.Background(), scanRoot)
			if cfgErr != nil {
				slog.Warn("mcp-main: failed to load scan-root config", "path", scanRoot, "error", cfgErr)
			} else {
				mgr.ApplyConfig(cfg)
			}
		}
	}
	mgr.Init()
	return mgr
}

func initManagerWithStore(bus *events.Bus) *session.Manager {
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Warn("mcp-main: cannot resolve home dir, using memory store", "error", err)
		if bus != nil {
			return session.NewManagerWithBus(bus)
		}
		return session.NewManager()
	}
	dbPath := filepath.Join(home, ".ralphglasses", "state.db")
	store, err := session.NewSQLiteStore(dbPath)
	if err != nil {
		slog.Warn("mcp-main: sqlite store unavailable, using memory store", "path", dbPath, "error", err)
		if bus != nil {
			return session.NewManagerWithBus(bus)
		}
		return session.NewManager()
	}
	if bus != nil {
		return session.NewManagerWithStore(store, bus)
	}
	mgr := session.NewManager()
	mgr.SetStore(store)
	return mgr
}
