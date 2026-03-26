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

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/config"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/hooks"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var (
	scanPath   string
	themeName  string
	notifyFlag bool
	debugMode  bool
	logLevel   string
	logFormat  string
	version    = "dev"
	commit     = "unknown"
	buildDate  = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "ralphglasses",
	Short: "Command-and-control TUI for parallel ralph loops",
	Long: `ralphglasses is a k9s-style TUI for managing parallel multi-LLM agent fleets.

It scans a directory tree for ralph-enabled repos (containing .ralphrc or .ralph/)
and provides a live dashboard to start, stop, monitor, and configure LLM coding loops
across Claude, Gemini, and Codex providers.

It also runs as an MCP server (ralphglasses mcp) exposing 48 tools for programmatic
fleet management from any MCP-capable client.`,
	Version: version,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		util.Debug.Enabled = debugMode

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

		// Wire structured logging to file.
		logDir := filepath.Join(scanPath, ".ralph", "logs")
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return err
		}
		logFile, err := os.OpenFile(filepath.Join(logDir, "ralph.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer logFile.Close()
		slog.SetDefault(slog.New(newLogHandler(logFile)))

		util.Debug.Debugf("scan-path: %s", scanPath)

		// Apply theme
		if themes := styles.DefaultThemes(); themes[themeName] != nil {
			styles.ApplyTheme(themes[themeName])
		} else if themeName != "k9s" {
			// Try loading as file path
			if t, err := styles.LoadTheme(themeName); err == nil {
				styles.ApplyTheme(t)
			}
		}

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

		// Graceful shutdown: stop all managed processes on SIGINT/SIGTERM.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			cancel()
			m.ProcMgr.StopAll(ctx)
			os.Exit(0)
		}()

		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err = p.Run()
		return err
	},
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for ralphglasses.

To install completions:

  Bash:
    ralphglasses completion bash > /etc/bash_completion.d/ralphglasses
    # or for current user:
    ralphglasses completion bash > ~/.bash_completion

  Zsh:
    ralphglasses completion zsh > "${fpath[1]}/_ralphglasses"
    # or add to ~/.zshrc:
    source <(ralphglasses completion zsh)

  Fish:
    ralphglasses completion fish > ~/.config/fish/completions/ralphglasses.fish`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return rootCmd.GenBashCompletion(os.Stdout)
		case "zsh":
			return rootCmd.GenZshCompletion(os.Stdout)
		case "fish":
			return rootCmd.GenFishCompletion(os.Stdout, true)
		default:
			return fmt.Errorf("unsupported shell: %s (use bash, zsh, or fish)", args[0])
		}
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
	rootCmd.Flags().StringVar(&themeName, "theme", "k9s",
		"Color theme (k9s, dracula, gruvbox, nord, or path to YAML)")
	rootCmd.Flags().BoolVar(&notifyFlag, "notify", false,
		"Enable desktop notifications for critical alerts")
	rootCmd.AddCommand(completionCmd)
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
