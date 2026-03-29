package mcpserver

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleLoopBenchmark(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	hours := getNumberArg(req, "hours", 48)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	obsPath := session.ObservationPath(r.Path)
	observations, err := session.LoadObservations(obsPath, since)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("load observations: %v", err)), nil
	}

	if len(observations) == 0 {
		return jsonResult(map[string]any{
			"repo":         repoName,
			"hours":        hours,
			"observations": 0,
			"message":      "no observations in window",
		}), nil
	}

	bl := e2e.BuildBaseline(observations, 0)

	// Compute actual window span from observation timestamps.
	actualHours := observations[len(observations)-1].Timestamp.Sub(observations[0].Timestamp).Hours()

	result := map[string]any{
		"repo":              repoName,
		"hours":             hours,
		"observation_count": len(observations),
		"window_type":       "rolling",
		"window_size":       actualHours,
	}
	if bl.Aggregate != nil {
		result["cost_p50"] = bl.Aggregate.CostP50
		result["cost_p95"] = bl.Aggregate.CostP95
		result["latency_p50_ms"] = bl.Aggregate.LatencyP50
		result["latency_p95_ms"] = bl.Aggregate.LatencyP95
	}
	if bl.Rates != nil {
		result["completion_rate"] = bl.Rates.CompletionRate
		result["verify_pass_rate"] = bl.Rates.VerifyPassRate
		result["error_rate"] = bl.Rates.ErrorRate
	}
	result["per_scenario"] = bl.Entries

	// Load stored baseline and compute divergence warnings.
	blPath := e2e.BaselinePath(r.Path)
	storedBL, _ := e2e.LoadBaseline(blPath)
	if storedBL != nil {
		warnings := computeDivergenceWarnings(bl, storedBL)
		if len(warnings) > 0 {
			result["divergence_warnings"] = warnings
		}
	}

	return jsonResult(result), nil
}

func (s *Server) handleLoopBaseline(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	action := getStringArg(req, "action")
	if action == "" {
		action = "view"
	}

	blPath := e2e.BaselinePath(r.Path)

	switch action {
	case "view":
		bl, err := e2e.LoadBaseline(blPath)
		if err != nil {
			return codedError(ErrFilesystem, fmt.Sprintf("load baseline: %v", err)), nil
		}
		return jsonResult(bl), nil

	case "refresh":
		hours := getNumberArg(req, "hours", 48)
		bl, err := e2e.RefreshBaseline(r.Path, hours)
		if err != nil {
			return codedError(ErrGateFailed, fmt.Sprintf("refresh baseline: %v", err)), nil
		}
		enrichBaselineWindow(bl, hours)
		if err := e2e.SaveBaseline(blPath, bl); err != nil {
			return codedError(ErrFilesystem, fmt.Sprintf("save baseline: %v", err)), nil
		}
		return jsonResult(map[string]any{
			"action":      "refresh",
			"path":        blPath,
			"samples":     bl.Aggregate.SampleCount,
			"window_type": windowType(hours),
			"window_hours": bl.WindowHours,
		}), nil

	case "pin":
		hours := getNumberArg(req, "hours", 48)
		bl, err := e2e.RefreshBaseline(r.Path, hours)
		if err != nil {
			return codedError(ErrGateFailed, fmt.Sprintf("refresh baseline: %v", err)), nil
		}
		enrichBaselineWindow(bl, hours)
		if err := e2e.SaveBaseline(blPath, bl); err != nil {
			return codedError(ErrFilesystem, fmt.Sprintf("save baseline: %v", err)), nil
		}
		return jsonResult(map[string]any{
			"action":       "pin",
			"path":         blPath,
			"samples":      bl.Aggregate.SampleCount,
			"window_type":  windowType(hours),
			"window_hours": bl.WindowHours,
			"message":      "baseline pinned — future observations measured against this snapshot",
		}), nil

	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unknown action: %s (use view, refresh, or pin)", action)), nil
	}
}

func (s *Server) handleLoopGates(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	hours := getNumberArg(req, "hours", 24)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	obsPath := session.ObservationPath(r.Path)
	observations, err := session.LoadObservations(obsPath, since)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("load observations: %v", err)), nil
	}

	blPath := e2e.BaselinePath(r.Path)
	baseline, _ := e2e.LoadBaseline(blPath) // nil baseline is ok — gates still evaluate rates

	thresholds := e2e.DefaultGateThresholds()
	report := e2e.EvaluateGates(observations, baseline, thresholds)

	// Wrap the report with human-readable and markdown-formatted summaries.
	type gateResponse struct {
		Report   *e2e.GateReport `json:"report"`
		Summary  string          `json:"summary"`
		Markdown string          `json:"markdown"`
	}
	resp := gateResponse{
		Report:   report,
		Summary:  e2e.FormatGateReport(report),
		Markdown: e2e.FormatGateReportMarkdown(report),
	}

	return jsonResult(resp), nil
}

// windowType returns "rolling" if a window was explicitly specified, "all" otherwise.
func windowType(hours float64) string {
	if hours > 0 {
		return "rolling"
	}
	return "all"
}

// enrichBaselineWindow replaces a zero WindowHours in a LoopBaseline with the
// actual time span computed from the earliest and latest entry sample counts,
// using GeneratedAt as the upper bound. When hours <= 0 (meaning "all data"),
// the WindowHours stored in the baseline already reflects the BuildBaseline
// input. We recompute from the baseline's GeneratedAt and the requested window.
func enrichBaselineWindow(bl *e2e.LoopBaseline, requestedHours float64) {
	if bl == nil {
		return
	}
	if requestedHours <= 0 {
		// "all" mode — keep whatever BuildBaseline stored but ensure it is
		// not the ambiguous zero. Use the aggregate sample count as a hint;
		// the true span is unknown without raw observations, so we leave
		// WindowHours as-is (BuildBaseline sets it to 0 for "all").
		// We do NOT overwrite here — the window_type field clarifies semantics.
		return
	}
	// For rolling windows, ensure WindowHours reflects the requested value.
	bl.WindowHours = requestedHours
}

// computeDivergenceWarnings compares current benchmark metrics against a stored
// baseline and returns warnings for any metric that diverges by more than 20%.
func computeDivergenceWarnings(current, stored *e2e.LoopBaseline) []string {
	if current == nil || stored == nil {
		return nil
	}

	var warnings []string

	// Compare aggregate cost/latency metrics.
	if current.Aggregate != nil && stored.Aggregate != nil {
		type metricPair struct {
			name    string
			current float64
			stored  float64
		}
		pairs := []metricPair{
			{"cost_p50", current.Aggregate.CostP50, stored.Aggregate.CostP50},
			{"cost_p95", current.Aggregate.CostP95, stored.Aggregate.CostP95},
			{"latency_p50_ms", current.Aggregate.LatencyP50, stored.Aggregate.LatencyP50},
			{"latency_p95_ms", current.Aggregate.LatencyP95, stored.Aggregate.LatencyP95},
		}
		for _, p := range pairs {
			if pctDivergence(p.current, p.stored) > 20 {
				warnings = append(warnings, fmt.Sprintf("%s diverged: baseline=%.4f current=%.4f", p.name, p.stored, p.current))
			}
		}
	}

	// Compare rates.
	if current.Rates != nil && stored.Rates != nil {
		type ratePair struct {
			name    string
			current float64
			stored  float64
		}
		pairs := []ratePair{
			{"verify_pass_rate", current.Rates.VerifyPassRate, stored.Rates.VerifyPassRate},
			{"completion_rate", current.Rates.CompletionRate, stored.Rates.CompletionRate},
			{"error_rate", current.Rates.ErrorRate, stored.Rates.ErrorRate},
		}
		for _, p := range pairs {
			if pctDivergence(p.current, p.stored) > 20 {
				warnings = append(warnings, fmt.Sprintf("%s diverged: baseline=%.4f current=%.4f", p.name, p.stored, p.current))
			}
		}
	}

	return warnings
}

// pctDivergence returns the absolute percentage difference between two values.
// Returns 0 when both values are zero to avoid division-by-zero.
func pctDivergence(a, b float64) float64 {
	if b == 0 && a == 0 {
		return 0
	}
	if b == 0 {
		return 100 // infinite divergence capped at 100%
	}
	return math.Abs(a-b) / math.Abs(b) * 100
}
