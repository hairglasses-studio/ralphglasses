package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Alert thresholds for fleet status.
const (
	fleetStaleThreshold      = 1 * time.Hour
	fleetBudgetWarnThreshold = 0.90
	fleetNoProgressThreshold = 3
)

func (s *Server) handleFleetStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Auto-scan if needed
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}

	// Refresh all repos
	for _, r := range s.Repos {
		if errs := model.RefreshRepo(ctx, r); len(errs) > 0 {
			for _, e := range errs {
				slog.Warn("handleFleetStatus: refresh failed", "repo", r.Path, "err", e)
			}
		}
	}

	// Pagination and filter params
	limit := int(getNumberArg(req, "limit", 50))
	offset := int(getNumberArg(req, "offset", 0))
	repoFilter := getStringArg(req, "repo")

	if limit < 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	// Apply repo filter to s.Repos
	filteredRepos := s.Repos
	if repoFilter != "" {
		filteredRepos = nil
		for _, r := range s.Repos {
			if r.Name == repoFilter {
				filteredRepos = append(filteredRepos, r)
			}
		}
	}

	// Summary-only: compact JSON with repo names, session counts, and total spend.
	if getBoolArg(req, "summary_only") {
		allSessions := s.SessMgr.List("")
		var totalSpend float64
		var runningSessions int
		repoSessionCounts := make(map[string]int)
		for _, sess := range allSessions {
			sess.Lock()
			totalSpend += sess.SpentUSD
			repoSessionCounts[sess.RepoName]++
			if sess.Status == session.StatusRunning || sess.Status == session.StatusLaunching {
				runningSessions++
			}
			sess.Unlock()
		}
		repoNames := make([]string, 0, len(filteredRepos))
		for _, r := range filteredRepos {
			repoNames = append(repoNames, r.Name)
		}
		// Filter session counts to matching repos when repo filter is set
		if repoFilter != "" {
			filtered := make(map[string]int)
			for k, v := range repoSessionCounts {
				if k == repoFilter {
					filtered[k] = v
				}
			}
			repoSessionCounts = filtered
			// Recount totals for filtered view
			totalSpend = 0
			runningSessions = 0
			var filteredTotal int
			for _, sess := range allSessions {
				sess.Lock()
				if sess.RepoName == repoFilter {
					totalSpend += sess.SpentUSD
					filteredTotal++
					if sess.Status == session.StatusRunning || sess.Status == session.StatusLaunching {
						runningSessions++
					}
				}
				sess.Unlock()
			}
			return jsonResult(map[string]any{
				"repos":            repoNames,
				"repo_sessions":    repoSessionCounts,
				"total_sessions":   filteredTotal,
				"running_sessions": runningSessions,
				"total_spend_usd":  totalSpend,
			}), nil
		}
		return jsonResult(map[string]any{
			"repos":            repoNames,
			"repo_sessions":    repoSessionCounts,
			"total_sessions":   len(allSessions),
			"running_sessions": runningSessions,
			"total_spend_usd":  totalSpend,
		}), nil
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

	repos := make([]repoSummary, 0, len(filteredRepos))
	var totalLoopSpend float64
	var runningLoops, pausedLoops, openCircuits int

	for _, r := range filteredRepos {
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
		repoName := sess.RepoName
		status := string(sess.Status)
		provider := string(sess.Provider)
		spent := sess.SpentUSD
		turns := sess.TurnCount
		isRunning := sess.Status == session.StatusRunning || sess.Status == session.StatusLaunching
		sess.Unlock()

		// Apply repo filter to sessions
		if repoFilter != "" && repoName != repoFilter {
			continue
		}

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
			Repo:     repoName,
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
		teamRepo := filepath.Base(t.RepoPath)
		if repoFilter != "" && teamRepo != repoFilter {
			continue
		}
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
			Repo:           teamRepo,
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

	alerts := make([]alert, 0)

	for _, r := range filteredRepos {
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
		repoName := sess.RepoName
		st := sess.Status
		errMsg := sess.Error
		sess.Unlock()
		if repoFilter != "" && repoName != repoFilter {
			continue
		}
		if st == session.StatusErrored {
			msg := fmt.Sprintf("Session %s errored", sess.ID)
			if errMsg != "" {
				msg += ": " + errMsg
			}
			alerts = append(alerts, alert{
				Severity: "info",
				Repo:     repoName,
				Message:  msg,
			})
		}
	}

	// --- Truncate session list for output size control (FINDING-173) ---
	totalSessionCount := len(sessions)
	if limit > 0 && len(sessions) > limit {
		sessions = sessions[:limit]
	}

	// --- Assemble response ---
	totalSpend := totalLoopSpend + totalSessionSpend
	loopRuns := s.SessMgr.ListLoops()

	type loopSummary struct {
		ID             string `json:"id"`
		Repo           string `json:"repo"`
		Status         string `json:"status"`
		Iterations     int    `json:"iterations"`
		LastError      string `json:"last_error,omitempty"`
		PlannerModel   string `json:"planner_model"`
		WorkerModel    string `json:"worker_model"`
		WorktreePolicy string `json:"worktree_policy,omitempty"`
	}

	loops := make([]loopSummary, 0, len(loopRuns))
	runningLoopRuns := 0
	for _, run := range loopRuns {
		run.Lock()
		runRepoName := run.RepoName
		runStatus := run.Status
		run.Unlock()

		if repoFilter != "" && runRepoName != repoFilter {
			continue
		}

		run.Lock()
		if runStatus == "running" {
			runningLoopRuns++
		}
		loops = append(loops, loopSummary{
			ID:             run.ID,
			Repo:           run.RepoName,
			Status:         run.Status,
			Iterations:     len(run.Iterations),
			LastError:      run.LastError,
			PlannerModel:   run.Profile.PlannerModel,
			WorkerModel:    run.Profile.WorkerModel,
			WorktreePolicy: run.Profile.WorktreePolicy,
		})
		run.Unlock()
	}

	// Apply pagination to repos
	totalRepoCount := len(repos)
	hasMore := false
	if offset >= len(repos) {
		repos = repos[:0]
	} else {
		repos = repos[offset:]
		if len(repos) > limit {
			repos = repos[:limit]
			hasMore = true
		}
	}

	result := map[string]any{
		"summary": map[string]any{
			"total_repos":             len(filteredRepos),
			"running_loops":           runningLoops,
			"paused_loops":            pausedLoops,
			"total_sessions":          len(sessions),
			"running_sessions":        runningSessions,
			"total_loop_runs":         len(loops),
			"running_loop_runs":       runningLoopRuns,
			"total_loop_spend_usd":    totalLoopSpend,
			"total_session_spend_usd": totalSessionSpend,
			"total_spend_usd":         totalSpend,
			"open_circuits":           openCircuits,
			"providers":               providerMap,
		},
		"repos":               repos,
		"sessions":            sessions,
		"teams":               teams,
		"loops":               loops,
		"alerts":              alerts,
		"has_more":            hasMore,
		"total_count":         totalRepoCount,
		"total_session_count": totalSessionCount,
	}

	return jsonResult(result), nil
}
