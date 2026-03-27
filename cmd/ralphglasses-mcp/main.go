package main

import (
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"

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
func setup(scanPath string) (*server.MCPServer, func(), error) {
	srv := server.NewMCPServer(
		"ralphglasses",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	bus := events.NewBus(1000)
	hookExec := hooks.NewExecutor(bus)
	hookExec.Start()

	rg := mcpserver.NewServerWithBus(scanPath, bus)
	rg.Register(srv)

	cleanup := func() {
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

	if err := server.ServeStdio(srv); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
}
