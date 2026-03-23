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

func main() {
	scanPath := os.Getenv("RALPHGLASSES_SCAN_PATH")
	if scanPath == "" {
		scanPath = "~/hairglasses-studio"
	}
	scanPath = util.ExpandHome(scanPath)

	srv := server.NewMCPServer(
		"ralphglasses",
		"0.1.0",
		server.WithToolCapabilities(true),
	)

	bus := events.NewBus(1000)
	hookExec := hooks.NewExecutor(bus)
	hookExec.Start()
	defer hookExec.Stop()
	rg := mcpserver.NewServerWithBus(scanPath, bus)
	rg.Register(srv)

	if err := server.ServeStdio(srv); err != nil {
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
}
