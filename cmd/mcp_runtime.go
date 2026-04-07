package cmd

import (
	"github.com/hairglasses-studio/ralphglasses/internal/bootstrap"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
)

// configureMCPRuntime replaces the default in-memory session manager with the
// normal store-backed runtime and wires optional fleet and autonomy subsystems.
func configureMCPRuntime(scanRoot string, bus *events.Bus, rg *mcpserver.Server) func() {
	return bootstrap.ConfigureMCPRuntime(scanRoot, bus, rg)
}
