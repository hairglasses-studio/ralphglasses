package cmd

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var (
	scanPath    string
	themeName   string
	notifyFlag  bool
	version     = "dev"
)

var rootCmd = &cobra.Command{
	Use:     "ralphglasses",
	Short:   "Command-and-control TUI for parallel ralph loops",
	Version: version,
	RunE: func(cmd *cobra.Command, args []string) error {
		scanPath = util.ExpandHome(scanPath)

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
		sessMgr := session.NewManagerWithBus(bus)
		m := tui.NewModel(scanPath, sessMgr)
		m.NotifyEnabled = notifyFlag
		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish]",
	Short: "Generate shell completion scripts",
	Args:  cobra.ExactArgs(1),
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
	rootCmd.Flags().StringVar(&scanPath, "scan-path", "~/hairglasses-studio",
		"Root directory to scan for ralph-enabled repos")
	rootCmd.Flags().StringVar(&themeName, "theme", "k9s",
		"Color theme (k9s, dracula, gruvbox, nord, or path to YAML)")
	rootCmd.Flags().BoolVar(&notifyFlag, "notify", false,
		"Enable desktop notifications for critical alerts")
	rootCmd.AddCommand(completionCmd)
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
