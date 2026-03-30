package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
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
		t := styles.ResolveTheme(name)
		if t == nil {
			return fmt.Errorf("theme %q not found", name)
		}

		switch format {
		case "ghostty":
			fmt.Printf("# Ghostty palette from ralphglasses theme %q\n", t.Name)
			fmt.Printf("palette = 0=%s\n", t.DarkBg)
			fmt.Printf("palette = 1=%s\n", t.Red)
			fmt.Printf("palette = 2=%s\n", t.Green)
			fmt.Printf("palette = 3=%s\n", t.Yellow)
			fmt.Printf("palette = 4=%s\n", t.Primary)
			fmt.Printf("palette = 5=%s\n", t.Accent)
			fmt.Printf("palette = 6=%s\n", t.Primary)
			fmt.Printf("palette = 7=%s\n", t.BrightFg)
			fmt.Printf("background = %s\n", t.DarkBg)
			fmt.Printf("foreground = %s\n", t.BrightFg)
		case "starship":
			fmt.Printf("# Starship color overrides from ralphglasses theme %q\n", t.Name)
			fmt.Printf("[palette.%s]\n", t.Name)
			fmt.Printf("primary = \"%s\"\n", t.Primary)
			fmt.Printf("accent = \"%s\"\n", t.Accent)
			fmt.Printf("green = \"%s\"\n", t.Green)
			fmt.Printf("yellow = \"%s\"\n", t.Yellow)
			fmt.Printf("red = \"%s\"\n", t.Red)
		case "k9s":
			fmt.Printf("# k9s skin.yml from ralphglasses theme %q\n", t.Name)
			fmt.Printf("k9s:\n")
			fmt.Printf("  body:\n")
			fmt.Printf("    fgColor: \"%s\"\n", t.BrightFg)
			fmt.Printf("    bgColor: \"%s\"\n", t.DarkBg)
			fmt.Printf("    logoColor: \"%s\"\n", t.Primary)
			fmt.Printf("  info:\n")
			fmt.Printf("    fgColor: \"%s\"\n", t.Accent)
			fmt.Printf("    sectionColor: \"%s\"\n", t.Primary)
			fmt.Printf("  frame:\n")
			fmt.Printf("    border:\n")
			fmt.Printf("      fgColor: \"%s\"\n", t.Primary)
			fmt.Printf("      focusColor: \"%s\"\n", t.Accent)
		default:
			return fmt.Errorf("unsupported format %q (try: ghostty, starship, k9s)", format)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(themeExportCmd)
}
