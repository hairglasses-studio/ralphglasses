package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/hairglasses-studio/mcpkit/registry"
	"github.com/mark3labs/mcp-go/server"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/config"
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
	Use:          "mcp",
	Short:        "Run as an MCP server on stdio",
	SilenceUsage: true,
	Long: fmt.Sprintf(`Start ralphglasses as a Model Context Protocol (MCP) server on stdio.

This exposes %d tools for managing ralph loops and multi-provider LLM sessions
programmatically from any MCP-capable client (for example Codex, Claude, or Gemini).

Read-only discovery is built in through ralph:///catalog/* resources, prompt templates,
and deferred tool-group loading.

Codex repo-local registration is already configured via .codex/config.toml and .mcp.json.
Other MCP clients can register this command directly, optionally setting
RALPHGLASSES_SCAN_PATH=%s for a custom scan path.`, mcpserver.TotalToolCount(), config.DefaultScanPath),
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

		if err := serveMCP(srv); err != nil {
			// MCP clients routinely cancel stdio transports during shutdown or
			// capability probing. Treat those as clean exits so Cobra does not
			// print usage/help text to stderr and confuse the client.
			if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		return nil
	},
}

var serveMCP = registry.ServeAuto

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

	rg := mcpserver.NewServerWithBus(sp, bus)
	rg.DeferredLoading = true
	rg.Version = version
	rg.Commit = commit
	rg.BuildDate = buildDate

	srv := registry.NewMCPServer(
		"ralphglasses",
		version+" ("+commit+")",
		server.WithInstructions(mcpserver.ServerInstructions()),
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
		server.WithToolHandlerMiddleware(rg.HooksMiddleware()),
		server.WithToolHandlerMiddleware(mcpserver.SecretSanitizationMiddleware()),
		server.WithToolHandlerMiddleware(mcpserver.ValidationMiddleware(sp)),
	)

	hookExec := hooks.NewExecutor(bus)
	hookExec.Start()
	// Enable MCP Sampling so the server can request LLM completions
	// from the host client (e.g., Claude Code) without separate API keys.
	srv.EnableSampling()
	rg.ToolRecorder = toolRec
	runtimeCleanup := configureMCPRuntime(sp, bus, rg)
	rg.Register(srv)
	mcpserver.RegisterResources(srv, rg)
	mcpserver.RegisterPrompts(srv, rg)

	cleanup := func() {
		runtimeCleanup()
		hookExec.Stop()
		toolRec.Close()
		otelShutdown(context.Background())
	}

	return srv, cleanup, nil
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
