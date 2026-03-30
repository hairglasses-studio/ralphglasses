package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/eval"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// abTestMinGroupSize is the minimum number of observations required in each
// group for an A/B test to produce meaningful results.
const abTestMinGroupSize = 5

// changepointBurnIn is the number of initial observations to ignore when
// reporting changepoints, as early indices produce false positives due to
// insufficient baseline data.
const changepointBurnIn = 5

// filterChangepointBurnIn removes changepoints at indices below the burn-in
// threshold. Returns an empty (non-nil) slice if all are filtered out.
func filterChangepointBurnIn(cps []eval.Changepoint, burnIn int) []eval.Changepoint {
	filtered := make([]eval.Changepoint, 0, len(cps))
	for _, cp := range cps {
		if cp.Index >= burnIn {
			filtered = append(filtered, cp)
		}
	}
	return filtered
}

func (s *Server) handleEvalCounterfactual(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	hours := getNumberArg(req, "hours", 168)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	obsPath := session.ObservationPath(r.Path)
	observations, err := session.LoadObservations(obsPath, since)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("load observations: %v", err)), nil
	}

	if len(observations) == 0 {
		return emptyResult("counterfactuals"), nil
	}

	policy := getStringArg(req, "policy")
	var policyFn eval.PolicyFunc

	switch policy {
	case "cascade_threshold":
		threshold := getNumberArg(req, "threshold", 0.6)
		policyFn = eval.CascadeThresholdPolicy(threshold)
	case "provider_routing":
		taskType := getStringArg(req, "task_type")
		provider := getStringArg(req, "provider")
		if taskType == "" || provider == "" {
			return codedError(ErrInvalidParams, "provider_routing policy requires task_type and provider"), nil
		}
		policyFn = eval.ProviderRoutingPolicy(taskType, provider)
	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unknown policy: %q (use cascade_threshold or provider_routing)", policy)), nil
	}

	result := eval.EvaluatePolicy(observations, policyFn)

	return jsonResult(map[string]any{
		"repo":                     repoName,
		"hours":                    hours,
		"policy":                   policy,
		"observations":             len(observations),
		"estimated_completion_rate": result.EstimatedCompletionRate,
		"estimated_avg_cost":       result.EstimatedAvgCost,
		"sample_size":              result.SampleSize,
		"effective_sample_size":    result.EffectiveSampleSize,
		"confidence_95":            result.Confidence95,
	}), nil
}

func (s *Server) handleEvalABTest(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	hours := getNumberArg(req, "hours", 168)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	obsPath := session.ObservationPath(r.Path)
	observations, err := session.LoadObservations(obsPath, since)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("load observations: %v", err)), nil
	}

	if len(observations) == 0 {
		return emptyResult("ab_tests"), nil
	}

	mode := getStringArg(req, "mode")

	switch mode {
	case "providers":
		providerA := getStringArg(req, "provider_a")
		providerB := getStringArg(req, "provider_b")
		if providerA == "" || providerB == "" {
			return codedError(ErrInvalidParams, "providers mode requires provider_a and provider_b"), nil
		}

		// FINDING-105: Check minimum group sizes before computing posteriors.
		var groupA, groupB []session.LoopObservation
		for _, o := range observations {
			switch o.WorkerProvider {
			case providerA:
				groupA = append(groupA, o)
			case providerB:
				groupB = append(groupB, o)
			}
		}
		if len(groupA) < abTestMinGroupSize || len(groupB) < abTestMinGroupSize {
			insufficientGroup := providerA
			insufficientCount := len(groupA)
			if len(groupB) < len(groupA) {
				insufficientGroup = providerB
				insufficientCount = len(groupB)
			}
			return jsonResult(map[string]any{
				"status":           "insufficient_data",
				"message":          fmt.Sprintf("group %s has %d observations (minimum %d required)", insufficientGroup, insufficientCount, abTestMinGroupSize),
				"group_a_count":    len(groupA),
				"group_b_count":    len(groupB),
				"minimum_required": abTestMinGroupSize,
			}), nil
		}

		result := eval.CompareProviders(observations, providerA, providerB)
		return jsonResult(map[string]any{
			"repo":         repoName,
			"hours":        hours,
			"mode":         mode,
			"observations": len(observations),
			"comparison":   result,
		}), nil

	case "periods":
		splitHoursAgo := getNumberArg(req, "split_hours_ago", 0)
		if splitHoursAgo <= 0 {
			return codedError(ErrInvalidParams, "periods mode requires split_hours_ago > 0"), nil
		}
		splitTime := time.Now().Add(-time.Duration(splitHoursAgo) * time.Hour)

		// FINDING-105: Check minimum group sizes before computing posteriors.
		var before, after []session.LoopObservation
		for _, o := range observations {
			if o.Timestamp.Before(splitTime) {
				before = append(before, o)
			} else {
				after = append(after, o)
			}
		}
		if len(before) < abTestMinGroupSize || len(after) < abTestMinGroupSize {
			insufficientGroup := "before"
			insufficientCount := len(before)
			if len(after) < len(before) {
				insufficientGroup = "after"
				insufficientCount = len(after)
			}
			return jsonResult(map[string]any{
				"status":           "insufficient_data",
				"message":          fmt.Sprintf("group %s has %d observations (minimum %d required)", insufficientGroup, insufficientCount, abTestMinGroupSize),
				"group_a_count":    len(before),
				"group_b_count":    len(after),
				"minimum_required": abTestMinGroupSize,
			}), nil
		}

		successFn := func(obs session.LoopObservation) bool {
			return obs.VerifyPassed
		}
		result := eval.ComparePeriods(observations, splitTime, successFn)
		return jsonResult(map[string]any{
			"repo":            repoName,
			"hours":           hours,
			"mode":            mode,
			"split_hours_ago": splitHoursAgo,
			"observations":    len(observations),
			"comparison":      result,
		}), nil

	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unknown mode: %q (use providers or periods)", mode)), nil
	}
}

func (s *Server) handleEvalChangepoints(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	hours := getNumberArg(req, "hours", 168)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	obsPath := session.ObservationPath(r.Path)
	observations, err := session.LoadObservations(obsPath, since)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("load observations: %v", err)), nil
	}

	if len(observations) == 0 {
		return emptyResult("changepoints"), nil
	}

	metricName := getStringArg(req, "metric")

	if metricName != "" {
		metrics := eval.StandardMetrics()
		metricFn, ok := metrics[metricName]
		if !ok {
			names := make([]string, 0, len(metrics))
			for k := range metrics {
				names = append(names, k)
			}
			return codedError(ErrInvalidParams, fmt.Sprintf("unknown metric: %q (available: %v)", metricName, names)), nil
		}
		result := eval.DetectChangepoints(observations, metricFn, metricName)
		// FINDING-106: Filter out false positives at low indices (burn-in period).
		result = filterChangepointBurnIn(result, changepointBurnIn)
		return jsonResult(map[string]any{
			"repo":         repoName,
			"hours":        hours,
			"metric":       metricName,
			"observations": len(observations),
			"changepoints": result,
			"burn_in":      changepointBurnIn,
		}), nil
	}

	results := eval.DetectAllChangepoints(observations)
	// FINDING-106: Filter out false positives at low indices (burn-in period).
	for metric, cps := range results {
		results[metric] = filterChangepointBurnIn(cps, changepointBurnIn)
	}
	return jsonResult(map[string]any{
		"repo":         repoName,
		"hours":        hours,
		"observations": len(observations),
		"changepoints": results,
		"burn_in":      changepointBurnIn,
	}), nil
}

func (s *Server) handleEvalSignificance(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	hours := getNumberArg(req, "hours", 168)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	obsPath := session.ObservationPath(r.Path)
	observations, err := session.LoadObservations(obsPath, since)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("load observations: %v", err)), nil
	}

	if len(observations) == 0 {
		return emptyResult("significance"), nil
	}

	mode := getStringArg(req, "mode")

	switch mode {
	case "providers":
		providerA := getStringArg(req, "provider_a")
		providerB := getStringArg(req, "provider_b")
		if providerA == "" || providerB == "" {
			return codedError(ErrInvalidParams, "providers mode requires provider_a and provider_b"), nil
		}

		var groupA, groupB []float64
		for _, o := range observations {
			switch o.WorkerProvider {
			case providerA:
				if o.VerifyPassed {
					groupA = append(groupA, 1)
				} else {
					groupA = append(groupA, 0)
				}
			case providerB:
				if o.VerifyPassed {
					groupB = append(groupB, 1)
				} else {
					groupB = append(groupB, 0)
				}
			}
		}

		if len(groupA) < abTestMinGroupSize || len(groupB) < abTestMinGroupSize {
			return jsonResult(map[string]any{
				"status":           "insufficient_data",
				"group_a_count":    len(groupA),
				"group_b_count":    len(groupB),
				"minimum_required": abTestMinGroupSize,
			}), nil
		}

		successFn := func(v float64) bool { return v == 1 }
		report := eval.GenerateReport(groupA, groupB, successFn)

		return jsonResult(map[string]any{
			"repo":           repoName,
			"hours":          hours,
			"mode":           mode,
			"provider_a":     providerA,
			"provider_b":     providerB,
			"observations":   len(observations),
			"report":         report,
		}), nil

	case "periods":
		splitHoursAgo := getNumberArg(req, "split_hours_ago", 0)
		if splitHoursAgo <= 0 {
			return codedError(ErrInvalidParams, "periods mode requires split_hours_ago > 0"), nil
		}
		splitTime := time.Now().Add(-time.Duration(splitHoursAgo) * time.Hour)

		var groupA, groupB []float64
		for _, o := range observations {
			val := 0.0
			if o.VerifyPassed {
				val = 1.0
			}
			if o.Timestamp.Before(splitTime) {
				groupA = append(groupA, val)
			} else {
				groupB = append(groupB, val)
			}
		}

		if len(groupA) < abTestMinGroupSize || len(groupB) < abTestMinGroupSize {
			return jsonResult(map[string]any{
				"status":           "insufficient_data",
				"group_a_count":    len(groupA),
				"group_b_count":    len(groupB),
				"minimum_required": abTestMinGroupSize,
			}), nil
		}

		successFn := func(v float64) bool { return v == 1 }
		report := eval.GenerateReport(groupA, groupB, successFn)

		return jsonResult(map[string]any{
			"repo":            repoName,
			"hours":           hours,
			"mode":            mode,
			"split_hours_ago": splitHoursAgo,
			"observations":    len(observations),
			"report":          report,
		}), nil

	case "cost":
		providerA := getStringArg(req, "provider_a")
		providerB := getStringArg(req, "provider_b")
		if providerA == "" || providerB == "" {
			return codedError(ErrInvalidParams, "cost mode requires provider_a and provider_b"), nil
		}

		var groupA, groupB []float64
		for _, o := range observations {
			switch o.WorkerProvider {
			case providerA:
				groupA = append(groupA, o.TotalCostUSD)
			case providerB:
				groupB = append(groupB, o.TotalCostUSD)
			}
		}

		if len(groupA) < abTestMinGroupSize || len(groupB) < abTestMinGroupSize {
			return jsonResult(map[string]any{
				"status":           "insufficient_data",
				"group_a_count":    len(groupA),
				"group_b_count":    len(groupB),
				"minimum_required": abTestMinGroupSize,
			}), nil
		}

		tResult := eval.WelchTTest(groupA, groupB)

		return jsonResult(map[string]any{
			"repo":         repoName,
			"hours":        hours,
			"mode":         mode,
			"provider_a":   providerA,
			"provider_b":   providerB,
			"observations": len(observations),
			"t_test":       tResult,
		}), nil

	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unknown mode: %q (use providers, periods, or cost)", mode)), nil
	}
}

// handleBanditStatus returns current multi-armed bandit arm statistics
// for provider selection. Reports whether bandit is configured and, if so,
// the cascade router's bandit configuration status and recent results.
func (s *Server) handleBanditStatus(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.SessMgr == nil {
		return codedError(ErrInternal, "session manager not initialized"), nil
	}

	cr := s.SessMgr.GetCascadeRouter()
	if cr == nil {
		return jsonResult(map[string]any{
			"status":  "not_configured",
			"message": "cascade router not configured; bandit policy unavailable",
		}), nil
	}

	if !cr.BanditConfigured() {
		return jsonResult(map[string]any{
			"status":  "not_configured",
			"message": "bandit policy not configured on cascade router",
			"cascade": cr.Stats(),
		}), nil
	}

	// Bandit is configured — return cascade stats which reflect bandit-influenced decisions.
	stats := cr.Stats()
	recent := cr.RecentResults(10)

	// Build per-provider summary from recent results.
	providerHits := make(map[string]int)
	providerEscalations := make(map[string]int)
	for _, r := range recent {
		p := string(r.UsedProvider)
		providerHits[p]++
		if r.Escalated {
			providerEscalations[p]++
		}
	}

	providerSummary := make(map[string]any)
	for p, hits := range providerHits {
		esc := providerEscalations[p]
		successRate := 0.0
		if hits > 0 {
			successRate = float64(hits-esc) / float64(hits)
		}
		providerSummary[p] = map[string]any{
			"pulls":        hits,
			"escalations":  esc,
			"success_rate": successRate,
		}
	}

	return jsonResult(map[string]any{
		"status":           "active",
		"cascade_stats":    stats,
		"provider_summary": providerSummary,
	}), nil
}

// handleConfidenceCalibration returns the decision model's training status,
// weights, and feature importances.
func (s *Server) handleConfidenceCalibration(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.SessMgr == nil {
		return codedError(ErrInternal, "session manager not initialized"), nil
	}

	cr := s.SessMgr.GetCascadeRouter()
	if cr == nil {
		return jsonResult(map[string]any{
			"status":  "not_configured",
			"message": "cascade router not configured; decision model unavailable",
		}), nil
	}

	dmStats := cr.DecisionModelStats()
	if dmStats == nil {
		return jsonResult(map[string]any{
			"status":  "not_configured",
			"message": "decision model not configured on cascade router; using heuristic confidence scoring",
		}), nil
	}

	return jsonResult(map[string]any{
		"status": "active",
		"model":  dmStats,
	}), nil
}
