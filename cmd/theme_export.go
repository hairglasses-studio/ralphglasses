package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/parity"
)

var themeExportCmd = &cobra.Command{
	Use:   "theme-export [format] [theme-name]",
	Short: "Export a theme as a config snippet for another tool",
	Long: `Export a ralphglasses theme in a format suitable for other tools.

Supported formats: ghostty, starship, k9s

Examples:
  ralphglasses theme-export ghostty dracula
  ralphglasses theme-export starship nord
  ralphglasses theme-export k9s gruvbox`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		format, name := args[0], args[1]
		export, err := parity.ExportTheme(format, name)
		if err != nil {
			return err
		}
		fmt.Println(export.Content)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(themeExportCmd)
}
