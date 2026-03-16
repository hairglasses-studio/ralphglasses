package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run as an MCP server on stdio",
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := scanPath
		if len(sp) >= 2 && sp[:2] == "~/" {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("expand home: %w", err)
			}
			sp = filepath.Join(home, sp[2:])
		}

		srv := server.NewMCPServer(
			"ralphglasses",
			"0.1.0",
			server.WithToolCapabilities(true),
		)

		rg := mcpserver.NewServer(sp)
		rg.Register(srv)

		return server.ServeStdio(srv)
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
