package cmd

import (
	"fmt"
	"os"
	"time"

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
				return outputGateReport(result, "Gate Check", gateJSON)
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
				return outputGateReport(result, "Gate Check", gateJSON)
			}
			return fmt.Errorf("load observations: %w", err)
		}
		filtered := make([]session.LoopObservation, 0, len(observations))
		for _, obs := range observations {
			if session.ObservationEligibleForBaseline(obs) {
				filtered = append(filtered, obs)
			}
		}
		observations = filtered

		// Evaluate gates
		thresholds := e2e.DefaultGateThresholds()
		report := e2e.EvaluateGates(observations, baseline, thresholds)

		if err := outputGateReport(report, "Gate Check", gateJSON); err != nil {
			return err
		}

		if report.Overall == e2e.VerdictFail {
			return ErrGateFailed
		}
		return nil
	},
}

func init() {
	gateCheckCmd.Flags().StringVar(&gateBaselinePath, "baseline", "", "path to baseline JSON (default: .ralph/loop_baseline.json)")
	gateCheckCmd.Flags().Float64Var(&gateHours, "hours", 24, "look-back window in hours")
	gateCheckCmd.Flags().BoolVar(&gateJSON, "json", false, "output JSON for automation")
	rootCmd.AddCommand(gateCheckCmd)
}
