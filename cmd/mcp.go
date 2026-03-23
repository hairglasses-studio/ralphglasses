package cmd

import (
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/hooks"
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run as an MCP server on stdio",
	Long: `Start ralphglasses as a Model Context Protocol (MCP) server on stdio.

This exposes 48 tools for managing ralph loops and multi-provider LLM sessions
programmatically from any MCP-capable client (e.g., Claude Code).

Install via claude CLI:
  claude mcp add ralphglasses -- go run . mcp

Or with a custom scan path:
  claude mcp add ralphglasses -e RALPHGLASSES_SCAN_PATH=~/hairglasses-studio -- go run . mcp`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)

		srv := server.NewMCPServer(
			"ralphglasses",
			version+" ("+commit+")",
			server.WithToolCapabilities(true),
		)

		bus := events.NewBus(1000)
		hookExec := hooks.NewExecutor(bus)
		hookExec.Start()
		defer hookExec.Stop()
		rg := mcpserver.NewServerWithBus(sp, bus)
		rg.Register(srv)

		return server.ServeStdio(srv)
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
