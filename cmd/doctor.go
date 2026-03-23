package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check required tools and environment",
	Long:  `Verify that all required binaries are installed and the environment is configured correctly.`,
	Example: `  # Run all checks
  ralphglasses doctor

  # Run with debug output
  ralphglasses doctor --debug`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)
		util.Debug.Debugf("doctor: scan-path=%s", sp)

		okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))   // green
		warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11")) // yellow
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))   // red

		type result struct {
			name     string
			status   string
			message  string
			required bool
		}

		var results []result
		anyFailed := false

		check := func(name string, required bool, fn func() (string, string)) {
			status, msg := fn()
			if required && status == "MISSING" {
				anyFailed = true
			}
			results = append(results, result{name, status, msg, required})
		}

		binaryCheck := func(bin string) func() (string, string) {
			return func() (string, string) {
				path, err := exec.LookPath(bin)
				if err != nil {
					return "MISSING", bin + " not found in PATH"
				}
				return "OK", path
			}
		}

		// Required checks
		check("ralph", true, binaryCheck("ralph"))
		check("claude", true, binaryCheck("claude"))
		check("git", true, binaryCheck("git"))
		check("ANTHROPIC_API_KEY", true, func() (string, string) {
			if os.Getenv("ANTHROPIC_API_KEY") != "" {
				return "OK", "set"
			}
			return "WARN", "not set (Claude Code uses OAuth if missing)"
		})
		check("scan-path exists", true, func() (string, string) {
			info, err := os.Stat(sp)
			if err != nil {
				return "MISSING", "path not found: " + sp
			}
			if !info.IsDir() {
				return "MISSING", "not a directory: " + sp
			}
			return "OK", sp
		})

		// Optional checks
		check("gemini", false, binaryCheck("gemini"))
		check("codex", false, binaryCheck("codex"))
		check("goreleaser", false, binaryCheck("goreleaser"))
		check("golangci-lint", false, binaryCheck("golangci-lint"))

		// Print table
		fmt.Printf("%-25s  %-8s  %s\n", "CHECK", "STATUS", "DETAILS")
		fmt.Println(strings.Repeat("-", 65))
		for _, r := range results {
			var statusStr string
			switch r.status {
			case "OK":
				statusStr = okStyle.Render("OK")
			case "WARN":
				statusStr = warnStyle.Render("WARN")
			default:
				if r.required {
					statusStr = errStyle.Render("MISSING")
				} else {
					statusStr = warnStyle.Render("MISSING")
				}
			}
			fmt.Printf("%-25s  %-17s  %s\n", r.name, statusStr, r.message)
		}

		if anyFailed {
			os.Exit(1)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
