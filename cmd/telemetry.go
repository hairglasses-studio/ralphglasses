package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/parity"
)

var telemetryFormat string

var telemetryCmd = &cobra.Command{
	Use:   "telemetry",
	Short: "Manage local telemetry data",
}

var telemetryExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export telemetry events as JSON or CSV",
	RunE: func(cmd *cobra.Command, args []string) error {
		events, err := parity.LoadTelemetry(parity.TelemetryOptions{})
		if err != nil {
			return err
		}
		if len(events) == 0 {
			fmt.Println("No telemetry data found.")
			return nil
		}

		if telemetryFormat == "csv" {
			out, err := parity.TelemetryCSV(events)
			if err != nil {
				return err
			}
			fmt.Println(out)
			return nil
		}

		out, err := parity.TelemetryJSON(events)
		if err != nil {
			return err
		}
		fmt.Println(strings.TrimSpace(out))
		return nil
	},
}

func init() {
	telemetryExportCmd.Flags().StringVar(&telemetryFormat, "format", "json", "Output format (json, csv)")
	telemetryCmd.AddCommand(telemetryExportCmd)
	rootCmd.AddCommand(telemetryCmd)
}
