package cmd

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/hairglasses-studio/ralphglasses/internal/telemetry"
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
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		path := filepath.Join(home, ".ralphglasses", "telemetry.jsonl")
		f, err := os.Open(path)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No telemetry data found.")
				return nil
			}
			return err
		}
		defer f.Close()

		var events []telemetry.Event
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			var ev telemetry.Event
			if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
				continue
			}
			events = append(events, ev)
		}

		if telemetryFormat == "csv" {
			w := csv.NewWriter(os.Stdout)
			w.Write([]string{"timestamp", "type", "session_id", "provider", "repo_name"})
			for _, ev := range events {
				w.Write([]string{
					ev.Timestamp.Format("2006-01-02T15:04:05Z"),
					string(ev.Type),
					ev.SessionID,
					ev.Provider,
					ev.RepoName,
				})
			}
			w.Flush()
			return w.Error()
		}

		// JSON output
		data, err := json.MarshalIndent(events, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	},
}

func init() {
	telemetryExportCmd.Flags().StringVar(&telemetryFormat, "format", "json", "Output format (json, csv)")
	telemetryCmd.AddCommand(telemetryExportCmd)
	rootCmd.AddCommand(telemetryCmd)
}
