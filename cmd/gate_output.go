package cmd

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/charmbracelet/lipgloss"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
)

// ErrGateFailed is returned when gate evaluation results in a fail verdict.
// Cobra callers should return this instead of calling os.Exit(1).
var ErrGateFailed = errors.New("gate check failed")

// outputGateReport renders a GateReport as either JSON or human-readable text.
// The jsonOutput flag controls the format. The title is used in the
// human-readable header (e.g. "Gate Check" or "Self-Test Gate").
func outputGateReport(report *e2e.GateReport, title string, jsonOutput bool) error {
	if jsonOutput {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	// Human-readable output: delegate to e2e.FormatGateReport for the tabular
	// body, then colorize the overall verdict with lipgloss.
	fmt.Printf("%s (%d samples)\n", title, report.SampleCount)
	fmt.Print(e2e.FormatGateReport(report))

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

	fmt.Printf("\nOverall: %s\n", verdictStyle(report.Overall).Render(string(report.Overall)))
	return nil
}
