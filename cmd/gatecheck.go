package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

var (
	gateBaselinePath string
	gateHours        float64
	gateJSON         bool
)

var gateCheckCmd = &cobra.Command{
	Use:   "gate-check",
	Short: "Evaluate regression gates against loop observations",
	Long: `Load recent loop observations and evaluate them against a baseline.
Exits 0 on pass/warn/skip, exits 1 on fail.`,
	Example: `  # Check against saved baseline
  ralphglasses gate-check --baseline .ralph/loop_baseline.json --hours 24

  # JSON output for automation
  ralphglasses gate-check --json --hours 24

  # Check with default baseline path
  ralphglasses gate-check`,
	RunE: func(cmd *cobra.Command, args []string) error {
		sp := util.ExpandHome(scanPath)

		// Load baseline
		blPath := gateBaselinePath
		if blPath == "" {
			blPath = e2e.BaselinePath(sp)
		}

		baseline, err := e2e.LoadBaseline(blPath)
		if err != nil {
			if os.IsNotExist(err) {
				result := &e2e.GateReport{
					Timestamp: time.Now(),
					Overall:   e2e.VerdictSkip,
					Results: []e2e.GateResult{{
						Metric:  "baseline",
						Verdict: e2e.VerdictSkip,
					}},
				}
				return outputGateReport(result)
			}
			return fmt.Errorf("load baseline: %w", err)
		}

		// Load observations
		since := time.Now().Add(-time.Duration(gateHours) * time.Hour)
		obsPath := session.ObservationPath(sp)
		observations, err := session.LoadObservations(obsPath, since)
		if err != nil {
			if os.IsNotExist(err) {
				result := &e2e.GateReport{
					Timestamp: time.Now(),
					Overall:   e2e.VerdictSkip,
					Results: []e2e.GateResult{{
						Metric:  "observations",
						Verdict: e2e.VerdictSkip,
					}},
				}
				return outputGateReport(result)
			}
			return fmt.Errorf("load observations: %w", err)
		}

		// Evaluate gates
		thresholds := e2e.DefaultGateThresholds()
		report := e2e.EvaluateGates(observations, baseline, thresholds)

		if err := outputGateReport(report); err != nil {
			return err
		}

		if report.Overall == e2e.VerdictFail {
			os.Exit(1)
		}
		return nil
	},
}

func outputGateReport(report *e2e.GateReport) error {
	if gateJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Human-readable output
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

	fmt.Printf("Gate Check (%d samples)\n", report.SampleCount)
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
	return nil
}

func init() {
	gateCheckCmd.Flags().StringVar(&gateBaselinePath, "baseline", "", "path to baseline JSON (default: .ralph/loop_baseline.json)")
	gateCheckCmd.Flags().Float64Var(&gateHours, "hours", 24, "look-back window in hours")
	gateCheckCmd.Flags().BoolVar(&gateJSON, "json", false, "output JSON for automation")
	rootCmd.AddCommand(gateCheckCmd)
}
