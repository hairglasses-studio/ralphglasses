package cmd

import (
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run as an MCP server on stdio",
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)

		srv := server.NewMCPServer(
			"ralphglasses",
			"0.1.0",
			server.WithToolCapabilities(true),
		)

		bus := events.NewBus(1000)
		rg := mcpserver.NewServerWithBus(sp, bus)
		rg.Register(srv)

		return server.ServeStdio(srv)
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
