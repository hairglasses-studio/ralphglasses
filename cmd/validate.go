package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/parity"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var validateJSON bool

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate .ralphrc files in scan-path repos",
	Long:  `Scan repos under --scan-path and validate each .ralphrc configuration file.`,
	Example: `  # Validate all repos under default scan-path
  ralphglasses validate

  # Validate with a custom scan-path
  ralphglasses validate --scan-path ~/projects`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)
		util.Debug.Debugf("validate: scanning %s", sp)

		results, err := parity.ValidateRepos(cmd.Context(), parity.ValidateOptions{
			ScanPath:     sp,
			IncludeClean: true,
		})
		if err != nil {
			return fmt.Errorf("validate failed: %w", err)
		}

		if validateJSON {
			data, err := json.MarshalIndent(results, "", "  ")
			if err != nil {
				return fmt.Errorf("json marshal: %w", err)
			}
			fmt.Println(string(data))
		} else {
			fmt.Println(parity.FormatValidateResults(results))
		}

		if parity.ValidationHasError(results) {
			return ErrChecksFailed
		}
		return nil
	},
}

// validateConfig returns a slice of issue strings (prefixed ERROR or WARN).
func validateConfig(cfg *model.RalphConfig) []string {
	return parity.ValidateConfig(cfg)
}

func init() {
	validateCmd.Flags().BoolVar(&validateJSON, "json", false, "Output results as JSON")
	rootCmd.AddCommand(validateCmd)
}
