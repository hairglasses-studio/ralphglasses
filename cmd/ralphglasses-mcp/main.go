package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/hairglasses-studio/mcpkit/observability"
	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/hairglasses-studio/mcpkit/slogcfg"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/bootstrap"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/hooks"
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
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
func setup(ctx context.Context, scanPath string) (*server.MCPServer, func(), error) {
	// --- Logging ---
	slogcfg.Init(slogcfg.Config{
		ServiceName:  "ralphglasses",
		ExtraHandler: slogcfg.WithTracing,
	})

	// Initialize mcpkit observability
	obsCfg := observability.Config{
		ServiceName:    "ralphglasses",
		ServiceVersion: "0.1.0",
		EnableTracing:  true,
		EnableMetrics:  true,
		EnableLogs:     true,
		OTLPEndpoint:   os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		PrometheusPort: "9091",
	}
	if p := os.Getenv("PROMETHEUS_PORT"); p != "" {
		obsCfg.PrometheusPort = p
	}
	obsProvider, obsShutdown, err := observability.Init(ctx, obsCfg)
	if err != nil {
		slog.Warn("failed to initialize observability", "error", err)
	}

	srv := registry.NewMCPServer(
		"ralphglasses",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	bus := events.NewBus(1000)
	hookExec := hooks.NewExecutor(bus)
	hookExec.Start()

	rg := mcpserver.NewServerWithBus(scanPath, bus)
	if obsProvider != nil {
		rg.Observability = obsProvider
	}

	runtimeCleanup := bootstrap.ConfigureMCPRuntime(scanPath, bus, rg)
	rg.Register(srv)

	cleanup := func() {
		runtimeCleanup()
		hookExec.Stop()
		if obsShutdown != nil {
			_ = obsShutdown(context.Background())
		}
	}

	return srv, cleanup, nil
}

func main() {
	ctx := context.Background()
	scanPath := resolveScanPath()

	srv, cleanup, err := setup(ctx, scanPath)
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
