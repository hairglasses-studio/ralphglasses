package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleEventList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.EventBus == nil {
		return codedError(ErrNotRunning, "event bus not initialized"), nil
	}

	// Build query from params.
	q := events.EventQuery{
		Limit:     int(getNumberArg(req, "limit", 50)),
		Offset:    int(getNumberArg(req, "offset", 0)),
		SessionID: getStringArg(req, "session_id"),
		RepoName:  getStringArg(req, "repo"),
		Provider:  getStringArg(req, "provider"),
	}

	// Parse comma-separated type filter.
	if typesStr := getStringArg(req, "types"); typesStr != "" {
		for _, t := range strings.Split(typesStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				q.Types = append(q.Types, events.EventType(t))
			}
		}
	} else if t := getStringArg(req, "type"); t != "" {
		q.Types = []events.EventType{events.EventType(t)}
	}

	// Parse time range.
	if sinceStr := getStringArg(req, "since"); sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid since timestamp: %v", err)), nil
		}
		q.Since = &t
	}
	if untilStr := getStringArg(req, "until"); untilStr != "" {
		t, err := time.Parse(time.RFC3339, untilStr)
		if err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid until timestamp: %v", err)), nil
		}
		q.Until = &t
	}

	result := s.EventBus.Query(q)

	type eventOut struct {
		Type      string         `json:"type"`
		Timestamp string         `json:"timestamp"`
		RepoName  string         `json:"repo_name,omitempty"`
		SessionID string         `json:"session_id,omitempty"`
		Provider  string         `json:"provider,omitempty"`
		Data      map[string]any `json:"data,omitempty"`
	}
	out := make([]eventOut, len(result.Events))
	for i, e := range result.Events {
		out[i] = eventOut{
			Type:      string(e.Type),
			Timestamp: e.Timestamp.Format(time.RFC3339),
			RepoName:  e.RepoName,
			SessionID: e.SessionID,
			Provider:  e.Provider,
			Data:      e.Data,
		}
	}

	return jsonResult(map[string]any{
		"events":      out,
		"total_count": result.TotalCount,
		"has_more":    result.HasMore,
	}), nil
}

func (s *Server) handleFleetAnalytics(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoFilter := getStringArg(req, "repo")
	providerFilter := getStringArg(req, "provider")

	sessions := s.SessMgr.List("")

	type providerStats struct {
		Sessions            int     `json:"sessions"`
		Running             int     `json:"running"`
		TotalSpend          float64 `json:"total_spend_usd"`
		AvgCostTurn         float64 `json:"avg_cost_per_turn"`
		TotalTurns          int     `json:"total_turns"`
		CacheReadTokens     int     `json:"cache_read_tokens"`
		CacheWriteTokens    int     `json:"cache_write_tokens"`
		CacheReadWriteRatio float64 `json:"cache_read_write_ratio"`
		CacheAnomalies      int     `json:"cache_anomaly_sessions"`
	}

	providers := make(map[string]*providerStats)
	repos := make(map[string]float64)
	totalCacheRead := 0
	totalCacheWrite := 0
	cacheAnomalySessions := 0

	for _, sess := range sessions {
		sess.Lock()
		provider := string(sess.Provider)
		repoName := sess.RepoName
		spent := sess.SpentUSD
		turns := sess.TurnCount
		status := sess.Status
		cacheRead := sess.CacheReadTokens
		cacheWrite := sess.CacheWriteTokens
		cacheAnomaly := sess.CacheAnomaly
		sess.Unlock()

		if repoFilter != "" && repoName != repoFilter {
			continue
		}
		if providerFilter != "" && provider != providerFilter {
			continue
		}

		ps, ok := providers[provider]
		if !ok {
			ps = &providerStats{}
			providers[provider] = ps
		}
		ps.Sessions++
		ps.TotalSpend += spent
		ps.TotalTurns += turns
		ps.CacheReadTokens += cacheRead
		ps.CacheWriteTokens += cacheWrite
		if status == session.StatusRunning || status == session.StatusLaunching {
			ps.Running++
		}
		if cacheAnomaly != "" {
			ps.CacheAnomalies++
			cacheAnomalySessions++
		}
		totalCacheRead += cacheRead
		totalCacheWrite += cacheWrite
		repos[repoName] += spent
	}

	for _, ps := range providers {
		if ps.TotalTurns > 0 {
			ps.AvgCostTurn = ps.TotalSpend / float64(ps.TotalTurns)
		}
		if ps.CacheWriteTokens > 0 {
			ps.CacheReadWriteRatio = float64(ps.CacheReadTokens) / float64(ps.CacheWriteTokens)
		}
	}

	cacheRatio := 0.0
	if totalCacheWrite > 0 {
		cacheRatio = float64(totalCacheRead) / float64(totalCacheWrite)
	}

	result := map[string]any{
		"providers":              providers,
		"repos":                  repos,
		"total_sessions":         len(sessions),
		"cache_read_tokens":      totalCacheRead,
		"cache_write_tokens":     totalCacheWrite,
		"cache_read_write_ratio": cacheRatio,
		"cache_anomaly_sessions": cacheAnomalySessions,
	}

	// Parse window for metrics.
	windowStr := getStringArg(req, "window")
	window := time.Hour
	if windowStr != "" {
		if d, err := time.ParseDuration(windowStr); err == nil && d > 0 {
			window = d
		}
	}

	// If FleetAnalytics is available, enrich with rolling-window metrics.
	if s.FleetAnalytics != nil {
		snap := s.FleetAnalytics.Snapshot(window)
		result["metrics"] = map[string]any{
			"window":             window.String(),
			"completions":        snap.TotalCompletions,
			"failures":           snap.TotalFailures,
			"failure_rate":       snap.FailureRate,
			"latency_p50_ms":     snap.LatencyP50Ms,
			"latency_p95_ms":     snap.LatencyP95Ms,
			"latency_p99_ms":     snap.LatencyP99Ms,
			"total_cost_usd":     snap.TotalCostUSD,
			"cost_per_provider":  snap.CostPerProvider,
			"worker_utilization": snap.WorkerUtilization,
		}
		if forecast := s.FleetAnalytics.CostForecast(24 * time.Hour); forecast > 0 {
			result["cost_forecast_24h_usd"] = forecast
		}
		result["data_source"] = "fleet_coordinator"
	} else {
		// FINDING-237: Fallback — aggregate metrics from observation store when
		// FleetAnalytics is nil (standalone MCP mode).
		metrics, obsCount := s.aggregateObservationMetrics(window, repoFilter, providerFilter)
		if obsCount > 0 {
			result["metrics"] = metrics
			result["observation_count"] = obsCount
			result["data_source"] = "observation_store"
		} else if len(sessions) == 0 {
			// FINDING-237: No fleet coordinator, no sessions, no observations —
			// return a warning instead of misleading all-zero metrics.
			return jsonResult(map[string]any{
				"warning":   "fleet not initialized — start a fleet session to collect analytics",
				"analytics": map[string]any{},
			}), nil
		} else {
			result["observation_count"] = obsCount
			result["data_source"] = "observation_store"
		}
	}

	return jsonResult(result), nil
}

// aggregateObservationMetrics reads observation JSONL files from known repos
// and computes fleet-style metrics (completions, failures, latency percentiles,
// cost) for the given time window. Returns the metrics map and total observation
// count.
func (s *Server) aggregateObservationMetrics(window time.Duration, repoFilter, providerFilter string) (map[string]any, int) {
	since := time.Now().Add(-window)

	s.mu.RLock()
	repos := s.Repos
	s.mu.RUnlock()

	var allObs []session.LoopObservation
	for _, r := range repos {
		if repoFilter != "" && r.Name != repoFilter {
			continue
		}
		obsPath := session.ObservationPath(r.Path)
		obs, err := session.LoadObservations(obsPath, since)
		if err != nil || len(obs) == 0 {
			continue
		}
		allObs = append(allObs, obs...)
	}

	if len(allObs) == 0 {
		return map[string]any{
			"window":            window.String(),
			"completions":       0,
			"failures":          0,
			"failure_rate":      0.0,
			"latency_p50_ms":    0.0,
			"latency_p95_ms":    0.0,
			"latency_p99_ms":    0.0,
			"total_cost_usd":    0.0,
			"cost_per_provider": map[string]float64{},
		}, 0
	}

	var (
		completions  int
		failures     int
		totalCostUSD float64
		costByProv   = make(map[string]float64)
		latencies    []float64
	)

	for _, obs := range allObs {
		// Apply provider filter.
		if providerFilter != "" {
			if obs.PlannerProvider != providerFilter && obs.WorkerProvider != providerFilter {
				continue
			}
		}

		if obs.Status == "failed" || obs.Error != "" {
			failures++
		} else {
			completions++
		}

		totalCostUSD += obs.TotalCostUSD
		if obs.PlannerProvider != "" {
			costByProv[obs.PlannerProvider] += obs.PlannerCostUSD
		}
		if obs.WorkerProvider != "" {
			costByProv[obs.WorkerProvider] += obs.WorkerCostUSD
		}

		if obs.TotalLatencyMs > 0 {
			latencies = append(latencies, float64(obs.TotalLatencyMs))
		}
	}

	total := completions + failures
	var failureRate float64
	if total > 0 {
		failureRate = float64(failures) / float64(total)
	}

	var p50, p95, p99 float64
	if len(latencies) > 0 {
		sort.Float64s(latencies)
		p50 = obsPercentile(latencies, 50)
		p95 = obsPercentile(latencies, 95)
		p99 = obsPercentile(latencies, 99)
	}

	metrics := map[string]any{
		"window":            window.String(),
		"completions":       completions,
		"failures":          failures,
		"failure_rate":      failureRate,
		"latency_p50_ms":    p50,
		"latency_p95_ms":    p95,
		"latency_p99_ms":    p99,
		"total_cost_usd":    totalCostUSD,
		"cost_per_provider": costByProv,
	}

	return metrics, completions + failures
}

// obsPercentile returns the p-th percentile from a sorted slice.
func obsPercentile(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := len(sorted) * p / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func (s *Server) handleMarathonDashboard(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	staleMin := getNumberArg(req, "stale_threshold_min", 5)
	staleThreshold := time.Duration(staleMin) * time.Minute

	allSessions := s.SessMgr.List("")
	allTeams := s.SessMgr.ListTeams()

	var (
		totalUSD     float64
		runningCount int
		staleCount   int
		erroredCount int
		staleList    = make([]map[string]any, 0)
		alerts       = make([]map[string]any, 0)
		byProvider   = make(map[string]float64)
	)

	now := time.Now()

	for _, sess := range allSessions {
		sess.Lock()
		totalUSD += sess.SpentUSD
		byProvider[string(sess.Provider)] += sess.SpentUSD

		isRunning := sess.Status == session.StatusRunning || sess.Status == session.StatusLaunching
		if isRunning {
			runningCount++
			idle := now.Sub(sess.LastActivity)
			if idle > staleThreshold {
				staleCount++
				staleList = append(staleList, map[string]any{
					"id":           sess.ID,
					"repo":         sess.RepoName,
					"idle_minutes": int(idle.Minutes()),
				})
				alerts = append(alerts, map[string]any{
					"severity": "warning",
					"type":     "stale_session",
					"message":  fmt.Sprintf("Session %s idle %.0f min", sess.ID[:min(8, len(sess.ID))], idle.Minutes()),
				})
			}
		}

		if sess.Status == session.StatusErrored {
			erroredCount++
			alerts = append(alerts, map[string]any{
				"severity": "critical",
				"type":     "session_error",
				"message":  fmt.Sprintf("Session %s errored: %s", sess.ID[:min(8, len(sess.ID))], truncateForAlert(sess.Error, 80)),
			})
		}

		if sess.BudgetUSD > 0 && sess.SpentUSD/sess.BudgetUSD >= 0.80 {
			alerts = append(alerts, map[string]any{
				"severity": "warning",
				"type":     "budget_warning",
				"message":  fmt.Sprintf("Session %s at %.0f%% budget ($%.2f/$%.2f)", sess.ID[:min(8, len(sess.ID))], sess.SpentUSD/sess.BudgetUSD*100, sess.SpentUSD, sess.BudgetUSD),
			})
		}
		sess.Unlock()
	}

	// Burn rate: total spend / total hours of running sessions
	var burnRate float64
	var hoursEst float64
	var totalBudget float64
	for _, sess := range allSessions {
		sess.Lock()
		if sess.Status == session.StatusRunning {
			elapsed := now.Sub(sess.LaunchedAt).Hours()
			if elapsed > 0 && sess.SpentUSD > 0 {
				burnRate += sess.SpentUSD / elapsed
			}
		}
		totalBudget += sess.BudgetUSD
		sess.Unlock()
	}
	remaining := totalBudget - totalUSD
	if remaining < 0 {
		remaining = 0
	}
	if burnRate > 0 {
		hoursEst = remaining / burnRate
	}

	// Team summaries — init as empty to marshal as [] not null.
	teamSummaries := make([]map[string]any, 0)
	var tasksCompleted, tasksTotal int
	for _, team := range allTeams {
		completed := 0
		for _, t := range team.Tasks {
			tasksTotal++
			if t.Status == "completed" {
				completed++
				tasksCompleted++
			}
		}
		teamSummaries = append(teamSummaries, map[string]any{
			"name":      team.Name,
			"status":    string(team.Status),
			"tasks":     len(team.Tasks),
			"completed": completed,
		})
	}

	return jsonResult(map[string]any{
		"timestamp": now.Format(time.RFC3339),
		"cost": map[string]any{
			"total_usd":   totalUSD,
			"burn_rate":   burnRate,
			"remaining":   remaining,
			"hours_est":   hoursEst,
			"by_provider": byProvider,
		},
		"sessions": map[string]any{
			"total":      len(allSessions),
			"running":    runningCount,
			"stale":      staleCount,
			"errored":    erroredCount,
			"stale_list": staleList,
		},
		"teams": map[string]any{
			"summary":         teamSummaries,
			"tasks_completed": tasksCompleted,
			"tasks_total":     tasksTotal,
		},
		"alerts":     alerts,
		"automation": s.SessMgr.SubscriptionAutomationStatus(""),
	}), nil
}

func (s *Server) handleToolBenchmark(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.ToolRecorder == nil {
		return codedError(ErrNotRunning, "tool benchmarking not configured"), nil
	}

	hours := getNumberArg(req, "hours", 24)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	entries, err := s.ToolRecorder.LoadEntries(since)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("loading benchmark data: %v", err)), nil
	}

	toolFilter := getStringArg(req, "tool")
	if toolFilter != "" {
		filtered := entries[:0]
		for _, e := range entries {
			if e.ToolName == toolFilter {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	summaries := Summarize(entries)

	// Build sorted list for stable output.
	summaryList := make([]*ToolBenchmarkSummary, 0, len(summaries))
	for _, s := range summaries {
		summaryList = append(summaryList, s)
	}

	result := map[string]any{
		"summaries":    summaryList,
		"window_hours": hours,
		"total_calls":  len(entries),
	}

	compare := getStringArg(req, "compare")
	if compare == "true" {
		// Baseline: previous window of same duration.
		baselineSince := since.Add(-time.Duration(hours) * time.Hour)
		baselineEntries, err := s.ToolRecorder.LoadEntries(baselineSince)
		if err == nil {
			// Filter baseline to only entries before 'since'.
			baselineFiltered := baselineEntries[:0]
			for _, e := range baselineEntries {
				if e.Timestamp.Before(since) {
					baselineFiltered = append(baselineFiltered, e)
				}
			}
			baselineSummaries := Summarize(baselineFiltered)
			regressions := CompareRuns(baselineSummaries, summaries)
			result["regressions"] = regressions
		}
	}

	return jsonResult(result), nil
}
