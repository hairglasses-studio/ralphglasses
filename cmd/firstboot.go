package cmd

import (
	"fmt"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/firstboot"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

var firstbootCmd = &cobra.Command{
	Use:   "firstboot",
	Short: "Run first-boot setup wizard for thin client configuration",
	Long: `Interactive wizard for first-time thin client setup.

Collects hostname, API keys, autonomy level, and fleet coordinator URL.
Writes configuration to ~/.ralphglasses/config.json and creates a
.firstboot-done marker file to prevent re-running.

Typically launched automatically by the ralphglasses-firstboot.service
	systemd unit on first boot.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configDir := firstboot.DefaultConfigDir()
		m := views.NewFirstBootModel(configDir)
		p := tea.NewProgram(m)
		finalModel, err := p.Run()
		if err != nil {
			return fmt.Errorf("firstboot wizard: %w", err)
		}

		// Check if user completed or cancelled
		_ = finalModel
		return nil
	},
}

func init() {
	rootCmd.AddCommand(firstbootCmd)
}
