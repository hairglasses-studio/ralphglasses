package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/repofiles"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Handlers

func (s *Server) handleScan(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.scan(); err != nil {
		return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
	}
	repos := s.reposCopy()
	if s.EventBus != nil {
		s.EventBus.Publish(events.Event{
			Type: events.ScanComplete,
			Data: map[string]any{"repo_count": len(repos)},
		})
	}
	return textResult(fmt.Sprintf("Found %d ralph-enabled repos", len(repos))), nil
}

func (s *Server) handleList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	repos := s.reposCopy()
	for _, r := range repos {
		if errs := model.RefreshRepo(r); len(errs) > 0 {
			for _, e := range errs {
				slog.Warn("handleList: refresh failed", "repo", r.Path, "err", e)
			}
		}
	}

	type repoSummary struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Loop    int    `json:"loop_count"`
		Calls   string `json:"calls"`
		Circuit string `json:"circuit"`
		Running bool   `json:"managed"`
	}

	summaries := make([]repoSummary, len(repos))
	for i, r := range repos {
		loop := 0
		if r.Status != nil {
			loop = r.Status.LoopCount
		}
		summaries[i] = repoSummary{
			Name:    r.Name,
			Status:  r.StatusDisplay(),
			Loop:    loop,
			Calls:   r.CallsDisplay(),
			Circuit: r.CircuitDisplay(),
			Running: s.ProcMgr.IsRunning(r.Path),
		}
	}
	return jsonResult(summaries), nil
}

func (s *Server) handleStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return invalidParams("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return internalErr(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return notFound(fmt.Sprintf("repo not found: %s", name)), nil
	}
	if errs := model.RefreshRepo(r); len(errs) > 0 {
		for _, e := range errs {
			slog.Warn("handleStatus: refresh failed", "repo", r.Path, "err", e)
		}
	}

	detail := map[string]any{
		"name":    r.Name,
		"path":    r.Path,
		"managed": s.ProcMgr.IsRunning(r.Path),
		"paused":  s.ProcMgr.IsPaused(r.Path),
	}
	if pid := s.ProcMgr.PidForRepo(r.Path); pid != 0 {
		detail["pid"] = pid
	}
	if r.Status != nil {
		detail["status"] = r.Status
	}
	if r.Circuit != nil {
		detail["circuit_breaker"] = r.Circuit
	}
	if r.Progress != nil {
		detail["progress"] = r.Progress
	}
	if r.Config != nil {
		detail["config"] = r.Config.Values
	}
	return jsonResult(detail), nil
}

func (s *Server) handleStart(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return invalidParams("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return internalErr(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return notFound(fmt.Sprintf("repo not found: %s", name)), nil
	}
	if err := s.ProcMgr.Start(r.Path); err != nil {
		return internalErr(fmt.Sprintf("start failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Started ralph loop for %s", name)), nil
}

func (s *Server) handleStop(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return invalidParams("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return internalErr(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return notFound(fmt.Sprintf("repo not found: %s", name)), nil
	}
	if err := s.ProcMgr.Stop(r.Path); err != nil {
		return internalErr(fmt.Sprintf("stop failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Stopped ralph loop for %s", name)), nil
}

func (s *Server) handleStopAll(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.ProcMgr.StopAll()
	return textResult("All managed loops stopped"), nil
}

func (s *Server) handlePause(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", name)), nil
	}
	paused, err := s.ProcMgr.TogglePause(r.Path)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("pause toggle failed: %v", err)), nil
	}
	if paused {
		return textResult(fmt.Sprintf("Paused loop for %s", name)), nil
	}
	return textResult(fmt.Sprintf("Resumed loop for %s", name)), nil
}

func (s *Server) handleLogs(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", name)), nil
	}

	maxLines := int(getNumberArg(req, "lines", 50))
	if maxLines > 500 {
		maxLines = 500
	}

	allLines, err := process.ReadFullLog(r.Path)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("read log: %v", err)), nil
	}

	start := 0
	if len(allLines) > maxLines {
		start = len(allLines) - maxLines
	}
	return textResult(strings.Join(allLines[start:], "\n")), nil
}

func (s *Server) handleConfig(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", name)), nil
	}

	if r.Config == nil {
		return codedError(ErrConfigInvalid, fmt.Sprintf("no .ralphrc found for %s", name)), nil
	}

	key := getStringArg(req, "key")
	value := getStringArg(req, "value")

	// List all
	if key == "" {
		return jsonResult(r.Config.Values), nil
	}

	// Set value
	if value != "" {
		r.Config.Values[key] = value
		if err := r.Config.Save(); err != nil {
			return codedError(ErrFilesystem, fmt.Sprintf("save config: %v", err)), nil
		}
		return textResult(fmt.Sprintf("Set %s=%s for %s", key, value, name)), nil
	}

	// Get value
	v := r.Config.Get(key, "")
	if v == "" {
		return codedError(ErrConfigInvalid, fmt.Sprintf("key not found: %s", key)), nil
	}
	return textResult(fmt.Sprintf("%s=%s", key, v)), nil
}

// Repo file handlers

func (s *Server) handleRepoScaffold(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return codedError(ErrInvalidParams, "path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
	}

	opts := repofiles.ScaffoldOptions{
		ProjectType: getStringArg(req, "project_type"),
		ProjectName: getStringArg(req, "project_name"),
		Force:       getStringArg(req, "force") == "true",
	}

	result, err := repofiles.Scaffold(path, opts)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("scaffold: %v", err)), nil
	}
	return jsonResult(result), nil
}

func (s *Server) handleRepoOptimize(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return codedError(ErrInvalidParams, "path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
	}

	opts := repofiles.OptimizeOptions{
		DryRun: getStringArg(req, "dry_run") != "false",
		Focus:  getStringArg(req, "focus"),
	}

	result, err := repofiles.Optimize(path, opts)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("optimize: %v", err)), nil
	}
	return jsonResult(result), nil
}

// Alert thresholds for fleet status.
const (
	fleetStaleThreshold      = 1 * time.Hour
	fleetBudgetWarnThreshold = 0.90
	fleetNoProgressThreshold = 3
)

func (s *Server) handleFleetStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Auto-scan if needed
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}

	// Refresh all repos
	for _, r := range s.Repos {
		if errs := model.RefreshRepo(r); len(errs) > 0 {
			for _, e := range errs {
				slog.Warn("handleFleetStatus: refresh failed", "repo", r.Path, "err", e)
			}
		}
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
		if run.Status == "running" {
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

	result := map[string]any{
		"summary": map[string]any{
			"total_repos":             len(s.Repos),
			"running_loops":           runningLoops,
			"paused_loops":            pausedLoops,
			"total_sessions":          len(allSessions),
			"running_sessions":        runningSessions,
			"total_loop_runs":         len(loopRuns),
			"running_loop_runs":       runningLoopRuns,
			"total_loop_spend_usd":    totalLoopSpend,
			"total_session_spend_usd": totalSessionSpend,
			"total_spend_usd":         totalSpend,
			"open_circuits":           openCircuits,
			"providers":               providerMap,
		},
		"repos":    repos,
		"sessions": sessions,
		"teams":    teams,
		"loops":    loops,
		"alerts":   alerts,
	}

	return jsonResult(result), nil
}

func (s *Server) handleConfigBulk(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key := getStringArg(req, "key")
	if key == "" {
		return codedError(ErrInvalidParams, "key required"), nil
	}
	value := getStringArg(req, "value")
	reposStr := getStringArg(req, "repos")

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}

	allRepos := s.reposCopy()
	var targetNames map[string]bool
	if reposStr != "" {
		targetNames = make(map[string]bool)
		for _, name := range strings.Split(reposStr, ",") {
			targetNames[strings.TrimSpace(name)] = true
		}
	}

	results := make(map[string]any)
	for _, r := range allRepos {
		if targetNames != nil && !targetNames[r.Name] {
			continue
		}
		if errs := model.RefreshRepo(r); len(errs) > 0 {
			for _, e := range errs {
				slog.Warn("handleConfigBulk: refresh failed", "repo", r.Path, "err", e)
			}
		}
		if r.Config == nil {
			results[r.Name] = "no .ralphrc"
			continue
		}
		if value == "" {
			results[r.Name] = r.Config.Values[key]
		} else {
			r.Config.Values[key] = value
			if err := r.Config.Save(); err != nil {
				results[r.Name] = fmt.Sprintf("save error: %v", err)
			} else {
				results[r.Name] = "updated"
				if s.EventBus != nil {
					s.EventBus.Publish(events.Event{
						Type:     events.ConfigChanged,
						RepoPath: r.Path,
						RepoName: r.Name,
						Data:     map[string]any{"key": key, "value": value},
					})
				}
			}
		}
	}
	return jsonResult(results), nil
}

func (s *Server) handleRepoHealth(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", name)), nil
	}
	if errs := model.RefreshRepo(r); len(errs) > 0 {
		for _, e := range errs {
			slog.Warn("handleRepoHealth: refresh failed", "repo", r.Path, "err", e)
		}
	}

	score := 100
	var issues []string

	// Circuit breaker
	cbState := "CLOSED"
	if r.Circuit != nil {
		cbState = r.Circuit.State
		if cbState == "OPEN" {
			score -= 30
			issues = append(issues, fmt.Sprintf("circuit breaker OPEN: %s", r.Circuit.Reason))
		} else if cbState == "HALF_OPEN" {
			score -= 10
			issues = append(issues, "circuit breaker HALF_OPEN")
		}
	}

	// Staleness
	if r.Status != nil && !r.Status.Timestamp.IsZero() {
		age := time.Since(r.Status.Timestamp)
		if age > time.Hour {
			score -= 15
			issues = append(issues, fmt.Sprintf("status stale (%.0f min)", age.Minutes()))
		}
	}

	// Budget
	if r.Status != nil && r.Status.BudgetStatus == "exceeded" {
		score -= 20
		issues = append(issues, "budget exceeded")
	}

	// Active sessions
	activeSessions := 0
	erroredSessions := 0
	totalSpend := 0.0
	for _, sess := range s.SessMgr.List("") {
		sess.Lock()
		if sess.RepoName == name || filepath.Base(sess.RepoPath) == name {
			if sess.Status == session.StatusRunning {
				activeSessions++
			}
			if sess.Status == session.StatusErrored {
				erroredSessions++
				score -= 5
			}
			totalSpend += sess.SpentUSD
		}
		sess.Unlock()
	}

	if erroredSessions > 0 {
		issues = append(issues, fmt.Sprintf("%d errored sessions", erroredSessions))
	}

	// CLAUDE.md health
	claudeMDPath := filepath.Join(r.Path, "CLAUDE.md")
	var claudeMDFindings []enhancer.ClaudeMDResult
	if claudeResults, err := enhancer.CheckClaudeMD(claudeMDPath); err == nil {
		claudeMDFindings = claudeResults
		warningCount := 0
		for _, finding := range claudeResults {
			if finding.Severity == "warn" {
				warningCount++
			}
		}
		if warningCount > 3 {
			score -= 10
			issues = append(issues, fmt.Sprintf("CLAUDE.md: %d warnings", warningCount))
		}
	}

	if score < 0 {
		score = 0
	}

	return jsonResult(map[string]any{
		"repo":              name,
		"health_score":      score,
		"circuit_breaker":   cbState,
		"active_sessions":   activeSessions,
		"errored_sessions":  erroredSessions,
		"total_spend_usd":   totalSpend,
		"loop_running":      s.ProcMgr.IsRunning(r.Path),
		"issues":            issues,
		"claudemd_findings": claudeMDFindings,
	}), nil
}
