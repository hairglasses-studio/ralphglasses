package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// configureMCPRuntime replaces the default in-memory session manager with the
// normal store-backed runtime and wires optional fleet and autonomy subsystems.
func configureMCPRuntime(scanRoot string, bus *events.Bus, rg *mcpserver.Server) func() {
	if rg == nil {
		return func() {}
	}

	mgr := initManagerRuntime(scanRoot, bus)
	rg.SessMgr = mgr
	rg.InitSelfImprovement(filepath.Join(scanRoot, ".ralph"), 0)
	rg.WireAutoOptimizer(mgr)
	mgr.RestoreAutonomyLevel()

	var cleanups []func()

	if fleetURL := strings.TrimSpace(os.Getenv("RALPH_FLEET_URL")); fleetURL != "" {
		client := fleet.NewClient(fleetURL)
		rg.InitFleetTools(nil, client, rg.HITLTracker, rg.DecisionLog, rg.FeedbackAnalyzer)
		mgr.SetStructuredTeamBackend(fleet.NewStructuredTeamBackend(nil, client))
	}

	docsRoot := defaultServeDocsRoot(scanRoot)
	if _, err := os.Stat(filepath.Join(docsRoot, ".docs.sqlite")); err == nil {
		gateway, gwErr := session.NewDocsResearchGateway(docsRoot)
		if gwErr != nil {
			slog.Warn("mcp: research gateway unavailable", "docs_root", docsRoot, "error", gwErr)
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

	return func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
}
