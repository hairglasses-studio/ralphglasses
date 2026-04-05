package cmd

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/hooks"
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
	"github.com/hairglasses-studio/ralphglasses/internal/observability"
	"github.com/hairglasses-studio/ralphglasses/internal/tracing"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

// mcpCmd runs as a long-lived MCP server on stdio. Code changes require
// restarting any client registration that points at this command.
var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run as an MCP server on stdio",
	Long: `Start ralphglasses as a Model Context Protocol (MCP) server on stdio.

This exposes 80+ tools for managing ralph loops and multi-provider LLM sessions
programmatically from any MCP-capable client (for example Codex, Claude, or Gemini).

Codex repo-local registration is already configured via .codex/config.toml and .mcp.json.
Other MCP clients can register this command directly, optionally setting
RALPHGLASSES_SCAN_PATH=~/hairglasses-studio for a custom scan path.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// In MCP mode, stderr IS the transport — any writes corrupt the
		// protocol. Immediately silence the default slog handler (which
		// targets stderr) before doing anything that might log.
		slog.SetDefault(slog.New(slog.NewJSONHandler(io.Discard, nil)))

		// Also ensure util.Debug never writes to stderr in MCP mode.
		util.Debug.Enabled = false

		sp := util.ExpandHome(scanPath)

		logFile, err := initLogging(sp)
		if err != nil {
			return err
		}
		defer logFile.Close()

		srv, cleanup, err := setupMCP(sp)
		if err != nil {
			return err
		}
		defer cleanup()

		return server.ServeStdio(srv)
	},
}

// setupMCP creates and configures the full MCP server with middleware, tools,
// resources, and prompts. Returns the server, a cleanup function, and any error.
func setupMCP(sp string) (*server.MCPServer, func(), error) {
	// Initialize OpenTelemetry provider. When OTEL_EXPORTER_OTLP_ENDPOINT is
	// set, spans are exported to the configured collector (Jaeger, Tempo, etc.).
	// Otherwise noop providers are used and all instrumentation is zero-cost.
	otelProvider, otelShutdown, err := observability.NewProvider("ralphglasses", "")
	if err != nil {
		return nil, nil, err
	}

	// Bridge the tracing.Recorder interface to the official OTel SDK so that
	// session spans created in runner.go are exported as real OTel spans.
	if !otelProvider.IsNoop() {
		otelRec := tracing.NewOTelRecorder()
		promRec := tracing.NewPrometheusRecorder(otelRec)
		tracing.SetRecorder(promRec)
	}

	// Tool call recorder: writes to <scanPath>/.ralph/tool_benchmarks.jsonl
	benchPath := filepath.Join(sp, ".ralph", "tool_benchmarks.jsonl")
	toolRec := mcpserver.NewToolCallRecorder(benchPath, nil, 50)

	bus := events.NewBus(1000)

	srv := server.NewMCPServer(
		"ralphglasses",
		version+" ("+commit+")",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, false),
		server.WithPromptCapabilities(true),
		server.WithRecovery(),
		// Outermost → innermost: concurrency → trace → timeout → instrumentation → event bus → validation → handler
		server.WithToolHandlerMiddleware(mcpserver.ConcurrencyMiddleware(mcpserver.DefaultMaxConcurrent)),
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
	// Enable MCP Sampling so the server can request LLM completions
	// from the host client (e.g., Claude Code) without separate API keys.
	srv.EnableSampling()

	rg := mcpserver.NewServerWithBus(sp, bus)
	rg.ToolRecorder = toolRec
	rg.InitSelfImprovement(filepath.Join(sp, ".ralph"), 0)
	rg.Register(srv)
	mcpserver.RegisterResources(srv, rg)
	mcpserver.RegisterPrompts(srv, rg)

	cleanup := func() {
		hookExec.Stop()
		toolRec.Close()
		otelShutdown(context.Background())
	}

	return srv, cleanup, nil
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
