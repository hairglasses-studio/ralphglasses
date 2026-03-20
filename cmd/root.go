package cmd

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/tui"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var scanPath string

var rootCmd = &cobra.Command{
	Use:   "ralphglasses",
	Short: "Command-and-control TUI for parallel ralph loops",
	RunE: func(cmd *cobra.Command, args []string) error {
		scanPath = util.ExpandHome(scanPath)

		m := tui.NewModel(scanPath)
		p := tea.NewProgram(m, tea.WithAltScreen())
		_, err := p.Run()
		return err
	},
}

func init() {
	rootCmd.Flags().StringVar(&scanPath, "scan-path", "~/hairglasses-studio",
		"Root directory to scan for ralph-enabled repos")
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
