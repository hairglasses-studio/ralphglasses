package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

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

		repos, err := discovery.Scan(sp)
		if err != nil {
			return fmt.Errorf("scan failed: %w", err)
		}

		hasError := false
		fmt.Printf("%-30s  %-7s  %s\n", "REPO", "STATUS", "ISSUES")
		fmt.Println(strings.Repeat("-", 72))

		for _, repo := range repos {
			if !repo.HasRC {
				continue
			}
			cfg, err := model.LoadConfig(repo.Path)
			if err != nil {
				fmt.Printf("%-30s  %-7s  %s\n", repo.Name, "ERROR", "cannot read .ralphrc: "+err.Error())
				hasError = true
				continue
			}

			issues := validateConfig(cfg)
			status := "OK"
			if len(issues) > 0 {
				for _, iss := range issues {
					if strings.HasPrefix(iss, "ERROR") {
						status = "ERROR"
						hasError = true
						break
					}
				}
				if status != "ERROR" {
					status = "WARN"
				}
				fmt.Printf("%-30s  %-7s  %s\n", repo.Name, status, issues[0])
				for _, iss := range issues[1:] {
					fmt.Printf("%-30s  %-7s  %s\n", "", "", iss)
				}
			} else {
				fmt.Printf("%-30s  %-7s\n", repo.Name, status)
			}
		}

		if hasError {
			return ErrChecksFailed
		}
		return nil
	},
}

// validateConfig returns a slice of issue strings (prefixed ERROR or WARN).
func validateConfig(cfg *model.RalphConfig) []string {
	var issues []string

	// Required keys
	if cfg.Get("PROJECT_NAME", "") == "" {
		issues = append(issues, "ERROR: PROJECT_NAME is required but not set")
	}

	// MAX_CALLS_PER_HOUR: must be a positive integer
	if v := cfg.Get("MAX_CALLS_PER_HOUR", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			issues = append(issues, "ERROR: MAX_CALLS_PER_HOUR is not a valid integer: "+v)
		} else if n <= 0 {
			issues = append(issues, "ERROR: MAX_CALLS_PER_HOUR must be > 0, got: "+v)
		}
	}

	// CLAUDE_TIMEOUT_MINUTES: must be a positive integer if set
	if v := cfg.Get("CLAUDE_TIMEOUT_MINUTES", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			issues = append(issues, "WARN: CLAUDE_TIMEOUT_MINUTES is not a valid integer: "+v)
		} else if n <= 0 {
			issues = append(issues, "WARN: CLAUDE_TIMEOUT_MINUTES should be > 0, got: "+v)
		}
	}

	// CB_* threshold keys: must be 0-100 if set
	cbKeys := []string{
		"CB_FAILURE_THRESHOLD",
		"CB_SUCCESS_THRESHOLD",
		"CB_HALF_OPEN_MAX_CALLS",
	}
	for _, key := range cbKeys {
		if v := cfg.Get(key, ""); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				issues = append(issues, "ERROR: "+key+" is not a valid integer: "+v)
			} else if n < 0 || n > 100 {
				issues = append(issues, "WARN: "+key+" should be 0-100, got: "+v)
			}
		}
	}

	return issues
}

func init() {
	rootCmd.AddCommand(validateCmd)
}
