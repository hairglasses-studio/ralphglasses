package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/config"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/hooks"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var (
	scanPath    string
	themeName   string
	notifyFlag  bool
	debugMode   bool
	logLevel    string
	logFormat   string
	scanTimeout string
	version     = "dev"
	commit      = "unknown"
	buildDate   = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "ralphglasses",
	Short: "Command-and-control TUI for parallel ralph loops",
	Long: `ralphglasses is a k9s-style TUI for managing parallel multi-LLM agent fleets.

It scans a directory tree for ralph-enabled repos (containing .ralphrc or .ralph/)
and provides a live dashboard to start, stop, monitor, and configure LLM coding loops
across Claude, Gemini, and Codex providers.

It also runs as an MCP server (ralphglasses mcp) exposing 110 tools for programmatic
fleet management from any MCP-capable client.`,
	Version: version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		util.Debug.Enabled = debugMode

		// Configure structured logging from flags before anything else logs.
		slog.SetDefault(slog.New(newLogHandler(os.Stderr)))

		// Validate --scan-timeout is a valid duration.
		if _, err := time.ParseDuration(scanTimeout); err != nil {
			return fmt.Errorf("invalid --scan-timeout %q: %w", scanTimeout, err)
		}

		// Load and validate runtime config (warnings only, non-fatal).
		home, _ := os.UserHomeDir()
		if home != "" {
			cfgPath := filepath.Join(home, ".ralphglasses", "config.json")
			_, _ = config.Load(cfgPath) // validation warnings logged inside Load
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		scanPath = util.ExpandHome(scanPath)

		logFile, err := initLogging(scanPath)
		if err != nil {
			return err
		}
		defer logFile.Close()

		util.Debug.Debugf("scan-path: %s", scanPath)

		applyTheme(themeName)

		bus := events.NewBus(1000)
		hookExec := hooks.NewExecutor(bus)
		hookExec.Start()
		defer hookExec.Stop()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		sessMgr := session.NewManagerWithBus(bus)
		m := tui.NewModel(scanPath, sessMgr)
		m.Ctx = ctx
		m.EventBus = bus
		m.NotifyEnabled = notifyFlag

		p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())

		// Graceful shutdown: on SIGINT/SIGTERM, quit the TUI and stop processes.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			slog.Info("received shutdown signal, stopping TUI")
			cancel()
			p.Kill()
		}()

		_, err = p.Run()

		// After TUI exits (whether from signal or user quit), gracefully
		// stop all managed processes with a 5-second timeout.
		slog.Info("shutting down managed processes")
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		killed := m.ProcMgr.StopAllGraceful(shutdownCtx)
		if killed > 0 {
			slog.Warn("force-killed processes that did not exit in time", "count", killed)
		}
		slog.Info("shutdown complete")
		return err
	},
}

func init() {
	rootCmd.SetVersionTemplate(fmt.Sprintf(
		"ralphglasses version %s (commit: %s, built: %s)\n",
		version, commit, buildDate,
	))
	rootCmd.PersistentFlags().StringVar(&scanPath, "scan-path", "~/hairglasses-studio",
		"Root directory to scan for ralph-enabled repos")
	rootCmd.PersistentFlags().BoolVar(&debugMode, "debug", false,
		"Enable verbose debug logging to stderr")
	rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "info",
		"Log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "json",
		"Log format (json, text)")
	rootCmd.PersistentFlags().StringVar(&scanTimeout, "scan-timeout", "30s",
		"Timeout for repository scanning (e.g. 30s, 1m)")
	rootCmd.Flags().StringVar(&themeName, "theme", "k9s",
		"Color theme (k9s, dracula, gruvbox, nord, or path to YAML)")
	rootCmd.Flags().BoolVar(&notifyFlag, "notify", false,
		"Enable desktop notifications for critical alerts")
	rootCmd.AddCommand(completionCmd)
}

// ScanTimeoutDuration returns the parsed --scan-timeout duration.
// It panics if called before PersistentPreRunE has validated the value.
func ScanTimeoutDuration() time.Duration {
	d, err := time.ParseDuration(scanTimeout)
	if err != nil {
		return 30 * time.Second
	}
	return d
}

// parseLogLevel converts a string level name to slog.Level.
func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// newLogHandler creates a slog.Handler writing to w, respecting the
// --log-level and --log-format flags.
func newLogHandler(w io.Writer) slog.Handler {
	opts := &slog.HandlerOptions{Level: parseLogLevel(logLevel)}
	if strings.ToLower(logFormat) == "text" {
		return slog.NewTextHandler(w, opts)
	}
	return slog.NewJSONHandler(w, opts)
}

// initLogging sets up structured logging to a file under the scan path.
// Returns the log file handle (caller must close) and any error.
func initLogging(sp string) (*os.File, error) {
	logDir := process.LogDirPath(sp)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	logFile, err := os.OpenFile(process.LogFilePath(sp), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	slog.SetDefault(slog.New(newLogHandler(logFile)))
	return logFile, nil
}

// applyTheme applies the named theme to the TUI styles.
// Falls back to RALPH_THEME env var when no --theme flag is given.
func applyTheme(name string) {
	if name == "k9s" {
		if envTheme := os.Getenv("RALPH_THEME"); envTheme != "" {
			name = envTheme
		}
	}
	if t := styles.ResolveTheme(name); t != nil {
		styles.ApplyTheme(t)
	}
}

// Execute runs the root command.
func Execute() {
	// Silence cobra's default error printing; we handle it below.
	rootCmd.SilenceErrors = true
	if err := rootCmd.Execute(); err != nil {
		// Sentinel errors: commands already printed diagnostics; exit silently.
		if errors.Is(err, ErrChecksFailed) || errors.Is(err, ErrGateFailed) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
