package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/config"
)

var configJSON bool

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage ralphglasses configuration",
}

var configListKeysCmd = &cobra.Command{
	Use:   "list-keys",
	Short: "Print all known config keys with types and constraints",
	RunE: func(cmd *cobra.Command, args []string) error {
		keys := config.KnownKeys()

		if configJSON {
			data, err := json.MarshalIndent(keys, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		fmt.Printf("%-25s  %-10s  %s\n", "KEY", "TYPE", "CONSTRAINTS")
		fmt.Println(strings.Repeat("-", 65))
		for _, k := range keys {
			constraint := ""
			if len(k.AllowedStr) > 0 {
				constraint = "enum: " + strings.Join(k.AllowedStr, ", ")
			} else if k.MaxInt > 0 {
				constraint = fmt.Sprintf("range: %d–%d", k.MinInt, k.MaxInt)
			}
			fmt.Printf("%-25s  %-10s  %s\n", k.Name, k.Type, constraint)
		}
		return nil
	},
}

var configInitCmd = &cobra.Command{
	Use:   "init [directory]",
	Short: "Generate a .ralphrc with all known keys and defaults",
	Long:  `Alias for 'ralphglasses init'. Creates a .ralphrc in the given directory.`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runInit,
}

func init() {
	configCmd.PersistentFlags().BoolVar(&configJSON, "json", false, "Output as JSON")
	configInitCmd.Flags().BoolVar(&initForce, "force", false, "Overwrite existing .ralphrc")
	configInitCmd.Flags().BoolVar(&initMinimal, "minimal", false, "Minimal config only")
	configCmd.AddCommand(configListKeysCmd, configInitCmd)
	rootCmd.AddCommand(configCmd)
}
