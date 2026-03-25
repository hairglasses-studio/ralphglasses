package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var (
	selftestIterations int
	selftestBudget     float64
	selftestRepoPath   string
	selftestJSON       bool
	selftestGateOnly   bool
)

var selftestCmd = &cobra.Command{
	Use:   "selftest",
	Short: "Run recursive self-test iterations and evaluate regression gates",
	Long: `Run self-test iterations against the current repository and evaluate
regression gates against a baseline. In --gate mode, only evaluates the
gate without running iterations.

Exits 0 on pass/warn/skip, exits 1 on fail.`,
	Example: `  # Run 2 iterations with $2 budget
  ralphglasses selftest --iterations 2 --budget 2.0

  # Gate-check only (no iterations)
  ralphglasses selftest --gate

  # JSON output for CI
  ralphglasses selftest --gate --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoPath := util.ExpandHome(selftestRepoPath)

		if selftestGateOnly {
			// Gate-only mode: evaluate observations against baseline, no iterations.
			report, err := e2e.EvaluateFromObservations(repoPath, e2e.DefaultGateThresholds(), 0)
			if err != nil {
				return fmt.Errorf("gate evaluation: %w", err)
			}
			return outputSelftestGateReport(report)
		}

		// Full mode: prepare → run → evaluate.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()

		cfg := e2e.DefaultSelfTestConfig(repoPath)
		cfg.MaxIterations = selftestIterations
		cfg.BudgetUSD = selftestBudget

		runner, err := e2e.Prepare(ctx, cfg)
		if err != nil {
			return fmt.Errorf("prepare self-test: %w", err)
		}

		result, err := runner.Run(ctx)
		if err != nil {
			return fmt.Errorf("run self-test: %w", err)
		}

		// After run, evaluate gates.
		report, err := e2e.EvaluateFromObservations(repoPath, e2e.DefaultGateThresholds(), 0)
		if err != nil {
			// Non-fatal: we still have the run result.
			report = nil
		}

		if selftestJSON {
			out := map[string]any{
				"iterations_run": result.Iterations,
				"cost_usd":      result.TotalCostUSD,
				"duration_ms":   result.Duration.Milliseconds(),
				"binary_hash":   result.BinaryHash,
			}
			if report != nil {
				out["gate"] = report
			}
			data, err := json.MarshalIndent(out, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
		} else {
			fmt.Printf("Self-test complete: %d iterations, $%.4f spent, %s elapsed\n",
				result.Iterations, result.TotalCostUSD, result.Duration.Round(time.Second))
			if report != nil {
				return outputSelftestGateReport(report)
			}
		}

		if report != nil && report.Overall == e2e.VerdictFail {
			os.Exit(1)
		}
		return nil
	},
}

func outputSelftestGateReport(report *e2e.GateReport) error {
	if selftestJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		if report.Overall == e2e.VerdictFail {
			os.Exit(1)
		}
		return nil
	}

	// Human-readable output (same style as gate-check).
	okStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	skipStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))

	verdictStyle := func(v e2e.GateVerdict) lipgloss.Style {
		switch v {
		case e2e.VerdictPass:
			return okStyle
		case e2e.VerdictWarn:
			return warnStyle
		case e2e.VerdictFail:
			return errStyle
		default:
			return skipStyle
		}
	}

	fmt.Printf("Self-Test Gate (%d samples)\n", report.SampleCount)
	fmt.Println("─────────────────────────────────────")

	for _, r := range report.Results {
		style := verdictStyle(r.Verdict)
		if r.BaselineVal > 0 {
			fmt.Printf("  %-20s %s  (current=%.3f baseline=%.3f delta=%+.1f%%)\n",
				r.Metric, style.Render(string(r.Verdict)), r.CurrentVal, r.BaselineVal, r.DeltaPct)
		} else {
			fmt.Printf("  %-20s %s  (current=%.3f)\n",
				r.Metric, style.Render(string(r.Verdict)), r.CurrentVal)
		}
	}

	fmt.Printf("\nOverall: %s\n", verdictStyle(report.Overall).Render(string(report.Overall)))

	if report.Overall == e2e.VerdictFail {
		os.Exit(1)
	}
	return nil
}

func init() {
	selftestCmd.Flags().IntVar(&selftestIterations, "iterations", 2, "number of self-test iterations")
	selftestCmd.Flags().Float64Var(&selftestBudget, "budget", 2.0, "API budget in USD")
	selftestCmd.Flags().StringVar(&selftestRepoPath, "repo-path", ".", "repository path to test")
	selftestCmd.Flags().BoolVar(&selftestJSON, "json", false, "output JSON for automation")
	selftestCmd.Flags().BoolVar(&selftestGateOnly, "gate", false, "gate-check only, skip iterations")
	rootCmd.AddCommand(selftestCmd)
}
