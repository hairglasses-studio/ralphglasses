package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var mcpCallCmd = &cobra.Command{
	Use:   "mcp-call <tool-name> [--param key=value ...]",
	Short: "Call an MCP tool directly and print the result",
	Long: `Invokes a registered MCP tool in-process without starting a full MCP server.
Useful for scripts and CI.

Example:
  ralphglasses mcp-call ralphglasses_scan
  ralphglasses mcp-call ralphglasses_self_improve -p repo=myrepo -p budget_usd=10`,
	Args: cobra.MinimumNArgs(1),
	RunE: runMCPCall,
}

var mcpCallParams []string

func init() {
	rootCmd.AddCommand(mcpCallCmd)
	mcpCallCmd.Flags().StringArrayVarP(&mcpCallParams, "param", "p", nil, "Tool parameter as key=value (repeatable)")
}

func runMCPCall(cmd *cobra.Command, args []string) error {
	toolName := args[0]

	// Parse params into map
	params := make(map[string]any)
	for _, p := range mcpCallParams {
		parts := strings.SplitN(p, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid param format %q, expected key=value", p)
		}
		key, val := parts[0], parts[1]
		// Try to parse as number first, then bool, then keep as string
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			params[key] = f
		} else if b, err := strconv.ParseBool(val); err == nil {
			params[key] = b
		} else {
			params[key] = val
		}
	}

	sp := util.ExpandHome(scanPath)

	// Create MCP server in-process with the same setup as the "mcp" subcommand
	bus := events.NewBus(1000)
	srv := server.NewMCPServer(
		"ralphglasses",
		version+" ("+commit+")",
		server.WithToolCapabilities(true),
		server.WithRecovery(),
	)

	rg := mcpserver.NewServerWithBus(sp, bus)
	rg.InitSelfImprovement(filepath.Join(sp, ".ralph"), 0)
	rg.Register(srv)

	// Look up the tool and call its handler directly
	tool := srv.GetTool(toolName)
	if tool == nil {
		return fmt.Errorf("tool %q not found", toolName)
	}

	req := mcp.CallToolRequest{
		Request: mcp.Request{
			Method: "tools/call",
		},
		Params: mcp.CallToolParams{
			Name:      toolName,
			Arguments: params,
		},
	}

	ctx := context.Background()
	result, err := tool.Handler(ctx, req)
	if err != nil {
		return fmt.Errorf("call tool %s: %w", toolName, err)
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}
