package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/hooks"
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

// mcpCmd runs as a long-lived MCP server on stdio. Code changes require
// restarting the server:
//   claude mcp remove ralphglasses && claude mcp add ralphglasses -- go run . mcp
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run as an MCP server on stdio",
	Long: `Start ralphglasses as a Model Context Protocol (MCP) server on stdio.

This exposes 80+ tools for managing ralph loops and multi-provider LLM sessions
programmatically from any MCP-capable client (e.g., Claude Code).

Install via claude CLI:
  claude mcp add ralphglasses -- go run . mcp

Or with a custom scan path:
  claude mcp add ralphglasses -e RALPHGLASSES_SCAN_PATH=~/hairglasses-studio -- go run . mcp`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)

		// Wire structured logging to file (uses canonical path from process package).
		logDir := process.LogDirPath(sp)
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return err
		}
		logFile, err := os.OpenFile(process.LogFilePath(sp), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer logFile.Close()
		slog.SetDefault(slog.New(newLogHandler(logFile)))

		// Tool call recorder: writes to <scanPath>/.ralph/tool_benchmarks.jsonl
		benchPath := filepath.Join(sp, ".ralph", "tool_benchmarks.jsonl")
		toolRec := mcpserver.NewToolCallRecorder(benchPath, nil, 50)
		defer toolRec.Close()

		bus := events.NewBus(1000)

		srv := server.NewMCPServer(
			"ralphglasses",
			version+" ("+commit+")",
			server.WithToolCapabilities(true),
			server.WithResourceCapabilities(false, false),
			server.WithPromptCapabilities(true),
			server.WithRecovery(),
			// Outermost → innermost: trace → timeout → instrumentation → event bus → validation → handler
			server.WithToolHandlerMiddleware(mcpserver.TraceMiddleware()),
			server.WithToolHandlerMiddleware(mcpserver.TimeoutMiddleware(30*time.Second, map[string]time.Duration{
				"ralphglasses_loop_step":       10 * time.Minute,
				"ralphglasses_coverage_report": 5 * time.Minute,
				"ralphglasses_merge_verify":    5 * time.Minute,
				"ralphglasses_self_test":       0, // exempt
				"ralphglasses_self_improve":    0, // exempt
			})),
			server.WithToolHandlerMiddleware(mcpserver.InstrumentationMiddleware(toolRec)),
			server.WithToolHandlerMiddleware(mcpserver.EventBusMiddleware(bus)),
			server.WithToolHandlerMiddleware(mcpserver.ValidationMiddleware(sp)),
		)

		hookExec := hooks.NewExecutor(bus)
		hookExec.Start()
		defer hookExec.Stop()
		// Enable MCP Sampling so the server can request LLM completions
		// from the host client (e.g., Claude Code) without separate API keys.
		srv.EnableSampling()

		rg := mcpserver.NewServerWithBus(sp, bus)
		rg.ToolRecorder = toolRec
		rg.InitSelfImprovement(filepath.Join(sp, ".ralph"), 0)
		rg.Register(srv)
		mcpserver.RegisterResources(srv, rg)
		mcpserver.RegisterPrompts(srv, rg)

		return server.ServeStdio(srv)
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
