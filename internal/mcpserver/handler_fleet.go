package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Alert thresholds for fleet status.
const (
	fleetStaleThreshold      = 1 * time.Hour
	fleetBudgetWarnThreshold = 0.90
	fleetNoProgressThreshold = 3
)

// repoStaleThreshold is used by handleRepoHealth.
const repoStaleThreshold = time.Hour

// timeSince wraps time.Since for use across handler files.
var timeSince = time.Since

func (s *Server) handleFleetStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Auto-scan if needed
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}

	// Refresh all repos
	for _, r := range s.Repos {
		model.RefreshRepo(r)
	}

	// Gather sessions and teams
	allSessions := s.SessMgr.List("")
	allTeams := s.SessMgr.ListTeams()

	// --- Build per-repo summaries ---
	type repoSummary struct {
		Name            string  `json:"name"`
		Status          string  `json:"status"`
		Managed         bool    `json:"managed"`
		Paused          bool    `json:"paused"`
		LoopCount       int     `json:"loop_count"`
		Calls           string  `json:"calls"`
		Circuit         string  `json:"circuit"`
		SpendUSD        float64 `json:"spend_usd"`
		Model           string  `json:"model,omitempty"`
		LastUpdate      string  `json:"last_update"`
		SessionsRunning int     `json:"sessions_running"`
		SessionsTotal   int     `json:"sessions_total"`
		CompletedTasks  int     `json:"completed_tasks"`
		TotalTasks      int     `json:"total_tasks"`
	}

	repos := make([]repoSummary, 0, len(s.Repos))
	var totalLoopSpend float64
	var runningLoops, pausedLoops, openCircuits int

	for _, r := range s.Repos {
		managed := s.ProcMgr.IsRunning(r.Path)
		paused := s.ProcMgr.IsPaused(r.Path)

		loopCount := 0
		var spendUSD float64
		var mdl string
		if r.Status != nil {
			loopCount = r.Status.LoopCount
			spendUSD = r.Status.SessionSpendUSD
			mdl = r.Status.Model
		}
		totalLoopSpend += spendUSD

		if managed && !paused {
			runningLoops++
		}
		if paused {
			pausedLoops++
		}

		circuitStr := r.CircuitDisplay()
		if r.Circuit != nil && r.Circuit.State == "OPEN" {
			openCircuits++
		}

		// Count sessions for this repo
		var sessRunning, sessTotal int
		for _, sess := range allSessions {
			if sess.RepoPath == r.Path {
				sessTotal++
				sess.Lock()
				st := sess.Status
				sess.Unlock()
				if st == session.StatusRunning || st == session.StatusLaunching {
					sessRunning++
				}
			}
		}

		// Progress tasks
		var completedTasks, totalTasks int
		if r.Progress != nil {
			completedTasks = len(r.Progress.CompletedIDs)
			// Total tasks = completed + remaining iterations implied by log
			totalTasks = completedTasks
		}

		repos = append(repos, repoSummary{
			Name:            r.Name,
			Status:          r.StatusDisplay(),
			Managed:         managed,
			Paused:          paused,
			LoopCount:       loopCount,
			Calls:           r.CallsDisplay(),
			Circuit:         circuitStr,
			SpendUSD:        spendUSD,
			Model:           mdl,
			LastUpdate:      r.UpdatedDisplay(),
			SessionsRunning: sessRunning,
			SessionsTotal:   sessTotal,
			CompletedTasks:  completedTasks,
			TotalTasks:      totalTasks,
		})
	}

	// --- Build session summaries ---
	type sessionSummary struct {
		ID       string  `json:"id"`
		Provider string  `json:"provider"`
		Repo     string  `json:"repo"`
		Status   string  `json:"status"`
		Model    string  `json:"model,omitempty"`
		SpentUSD float64 `json:"spent_usd"`
		Turns    int     `json:"turns"`
		Agent    string  `json:"agent,omitempty"`
		Team     string  `json:"team,omitempty"`
	}

	// Provider breakdown accumulators
	type providerStats struct {
		Sessions int     `json:"sessions"`
		Running  int     `json:"running"`
		SpendUSD float64 `json:"spend_usd"`
	}
	providerMap := map[string]*providerStats{}

	var totalSessionSpend float64
	var runningSessions int
	sessions := make([]sessionSummary, 0, len(allSessions))

	for _, sess := range allSessions {
		sess.Lock()
		status := string(sess.Status)
		provider := string(sess.Provider)
		spent := sess.SpentUSD
		turns := sess.TurnCount
		isRunning := sess.Status == session.StatusRunning || sess.Status == session.StatusLaunching
		sess.Unlock()

		totalSessionSpend += spent
		if isRunning {
			runningSessions++
		}

		// Provider stats
		ps, ok := providerMap[provider]
		if !ok {
			ps = &providerStats{}
			providerMap[provider] = ps
		}
		ps.Sessions++
		ps.SpendUSD += spent
		if isRunning {
			ps.Running++
		}

		sessions = append(sessions, sessionSummary{
			ID:       sess.ID,
			Provider: provider,
			Repo:     sess.RepoName,
			Status:   status,
			Model:    sess.Model,
			SpentUSD: spent,
			Turns:    turns,
			Agent:    sess.AgentName,
			Team:     sess.TeamName,
		})
	}

	// --- Build team summaries ---
	type teamSummary struct {
		Name           string `json:"name"`
		Repo           string `json:"repo"`
		Status         string `json:"status"`
		TasksTotal     int    `json:"tasks_total"`
		TasksCompleted int    `json:"tasks_completed"`
		TasksPending   int    `json:"tasks_pending"`
	}

	teams := make([]teamSummary, 0, len(allTeams))
	for _, t := range allTeams {
		var completed, pending int
		for _, task := range t.Tasks {
			switch task.Status {
			case "completed":
				completed++
			case "pending":
				pending++
			}
		}
		teams = append(teams, teamSummary{
			Name:           t.Name,
			Repo:           filepath.Base(t.RepoPath),
			Status:         string(t.Status),
			TasksTotal:     len(t.Tasks),
			TasksCompleted: completed,
			TasksPending:   pending,
		})
	}

	// --- Generate alerts ---
	type alert struct {
		Severity string `json:"severity"`
		Repo     string `json:"repo"`
		Message  string `json:"message"`
	}

	var alerts []alert

	for _, r := range s.Repos {
		// Circuit breaker OPEN → critical
		if r.Circuit != nil && r.Circuit.State == "OPEN" {
			alerts = append(alerts, alert{
				Severity: "critical",
				Repo:     r.Name,
				Message:  fmt.Sprintf("Circuit breaker OPEN: %s", r.Circuit.Reason),
			})
		}

		// Loop stale → warning
		managed := s.ProcMgr.IsRunning(r.Path)
		if managed && r.Status != nil && !r.Status.Timestamp.IsZero() {
			if time.Since(r.Status.Timestamp) > fleetStaleThreshold {
				alerts = append(alerts, alert{
					Severity: "warning",
					Repo:     r.Name,
					Message:  fmt.Sprintf("Loop stale: last update %s", r.UpdatedDisplay()),
				})
			}
		}

		// Budget near limit → warning
		if r.Config != nil && r.Status != nil {
			if budgetStr, ok := r.Config.Values["RALPH_SESSION_BUDGET"]; ok {
				budget, err := strconv.ParseFloat(budgetStr, 64)
				if err == nil && budget > 0 {
					ratio := r.Status.SessionSpendUSD / budget
					if ratio >= fleetBudgetWarnThreshold {
						alerts = append(alerts, alert{
							Severity: "warning",
							Repo:     r.Name,
							Message:  fmt.Sprintf("Budget at %d%%: $%.2f/$%.2f", int(ratio*100), r.Status.SessionSpendUSD, budget),
						})
					}
				}
			}
		}

		// No-progress streak → warning
		if r.Circuit != nil && r.Circuit.State != "OPEN" && r.Circuit.ConsecutiveNoProgress >= fleetNoProgressThreshold {
			alerts = append(alerts, alert{
				Severity: "warning",
				Repo:     r.Name,
				Message:  fmt.Sprintf("No-progress streak: %d consecutive iterations", r.Circuit.ConsecutiveNoProgress),
			})
		}

		// Loop paused → info
		if s.ProcMgr.IsPaused(r.Path) {
			alerts = append(alerts, alert{
				Severity: "info",
				Repo:     r.Name,
				Message:  "Loop paused",
			})
		}
	}

	// Session errored → info
	for _, sess := range allSessions {
		sess.Lock()
		st := sess.Status
		errMsg := sess.Error
		sess.Unlock()
		if st == session.StatusErrored {
			msg := fmt.Sprintf("Session %s errored", sess.ID)
			if errMsg != "" {
				msg += ": " + errMsg
			}
			alerts = append(alerts, alert{
				Severity: "info",
				Repo:     sess.RepoName,
				Message:  msg,
			})
		}
	}

	if alerts == nil {
		alerts = []alert{}
	}

	// --- Assemble response ---
	totalSpend := totalLoopSpend + totalSessionSpend

	result := map[string]any{
		"summary": map[string]any{
			"total_repos":          len(s.Repos),
			"running_loops":        runningLoops,
			"paused_loops":         pausedLoops,
			"total_sessions":       len(allSessions),
			"running_sessions":     runningSessions,
			"total_loop_spend_usd": totalLoopSpend,
			"total_session_spend_usd": totalSessionSpend,
			"total_spend_usd":      totalSpend,
			"open_circuits":        openCircuits,
			"providers":            providerMap,
		},
		"repos":    repos,
		"sessions": sessions,
		"teams":    teams,
		"alerts":   alerts,
	}

	return jsonResult(result), nil
}

func (s *Server) handleEventList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.EventBus == nil {
		return errResult("event bus not initialized"), nil
	}

	typeFilter := events.EventType(getStringArg(req, "type"))
	repoFilter := getStringArg(req, "repo")
	limit := int(getNumberArg(req, "limit", 50))
	sinceStr := getStringArg(req, "since")

	var evts []events.Event
	if sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			return errResult(fmt.Sprintf("invalid since timestamp: %v", err)), nil
		}
		evts = s.EventBus.HistorySince(t)
	} else {
		evts = s.EventBus.History(typeFilter, limit)
	}

	// Apply filters
	var filtered []events.Event
	for _, e := range evts {
		if typeFilter != "" && e.Type != typeFilter {
			continue
		}
		if repoFilter != "" && e.RepoName != repoFilter {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	type eventOut struct {
		Type      string         `json:"type"`
		Timestamp string         `json:"timestamp"`
		RepoName  string         `json:"repo_name,omitempty"`
		SessionID string         `json:"session_id,omitempty"`
		Provider  string         `json:"provider,omitempty"`
		Data      map[string]any `json:"data,omitempty"`
	}
	out := make([]eventOut, len(filtered))
	for i, e := range filtered {
		out[i] = eventOut{
			Type:      string(e.Type),
			Timestamp: e.Timestamp.Format(time.RFC3339),
			RepoName:  e.RepoName,
			SessionID: e.SessionID,
			Provider:  e.Provider,
			Data:      e.Data,
		}
	}
	return jsonResult(out), nil
}

func (s *Server) handleFleetAnalytics(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoFilter := getStringArg(req, "repo")
	providerFilter := getStringArg(req, "provider")

	sessions := s.SessMgr.List("")

	type providerAnalytics struct {
		Sessions    int     `json:"sessions"`
		Running     int     `json:"running"`
		TotalSpend  float64 `json:"total_spend_usd"`
		AvgCostTurn float64 `json:"avg_cost_per_turn"`
		TotalTurns  int     `json:"total_turns"`
	}

	providers := make(map[string]*providerAnalytics)
	repos := make(map[string]float64)

	for _, sess := range sessions {
		sess.Lock()
		provider := string(sess.Provider)
		repoName := sess.RepoName
		spent := sess.SpentUSD
		turns := sess.TurnCount
		status := sess.Status
		sess.Unlock()

		if repoFilter != "" && repoName != repoFilter {
			continue
		}
		if providerFilter != "" && provider != providerFilter {
			continue
		}

		ps, ok := providers[provider]
		if !ok {
			ps = &providerAnalytics{}
			providers[provider] = ps
		}
		ps.Sessions++
		ps.TotalSpend += spent
		ps.TotalTurns += turns
		if status == session.StatusRunning || status == session.StatusLaunching {
			ps.Running++
		}
		repos[repoName] += spent
	}

	for _, ps := range providers {
		if ps.TotalTurns > 0 {
			ps.AvgCostTurn = ps.TotalSpend / float64(ps.TotalTurns)
		}
	}

	result := map[string]any{
		"providers":      providers,
		"repos":          repos,
		"total_sessions": len(sessions),
	}
	return jsonResult(result), nil
}
