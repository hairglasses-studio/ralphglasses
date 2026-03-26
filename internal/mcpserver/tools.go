package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/bandit"
	"github.com/hairglasses-studio/ralphglasses/internal/blackboard"
	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ToolGroup represents a namespace of related tools.
type ToolGroup struct {
	Name        string
	Description string
	Tools       []ToolEntry
}

// ToolEntry pairs a tool definition with its handler.
type ToolEntry struct {
	Tool    mcp.Tool
	Handler server.ToolHandlerFunc
}

// Server holds state for the MCP server.
type Server struct {
	mu           sync.RWMutex
	ScanPath     string
	Repos        []*model.Repo
	ProcMgr      *process.Manager
	SessMgr      *session.Manager
	EventBus     *events.Bus
	HTTPClient   *http.Client
	Engine       *enhancer.HybridEngine
	engineOnce   sync.Once
	ToolRecorder *ToolCallRecorder

	// DeferredLoading controls whether only core tools are registered on startup.
	// When true, RegisterCoreTools is called instead of RegisterAllTools.
	DeferredLoading bool

	// loadedGroups tracks which tool groups have been registered (for deferred loading).
	loadedGroups map[string]bool

	// mcpSrv holds a reference to the MCPServer for deferred group loading.
	mcpSrv *server.MCPServer

	// Fleet and HITL infrastructure (set via InitFleetTools / InitSelfImprovement).
	FleetCoordinator *fleet.Coordinator
	FleetClient      *fleet.Client
	HITLTracker      *session.HITLTracker
	DecisionLog      *session.DecisionLog
	FeedbackAnalyzer *session.FeedbackAnalyzer
	AutoOptimizer    *session.AutoOptimizer

	// Phase H subsystems (set via setter methods).
	Blackboard    *blackboard.Blackboard
	A2A           *fleet.A2AAdapter
	CostPredictor *fleet.CostPredictor

	// Bandit: provider selection independent of cascade routing.
	Bandit *bandit.Selector
}

// NewServer creates a new MCP server instance.
func NewServer(scanPath string) *Server {
	return &Server{
		ScanPath:   scanPath,
		ProcMgr:    process.NewManager(),
		SessMgr:    session.NewManager(),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewServerWithBus creates a new MCP server instance with an event bus.
func NewServerWithBus(scanPath string, bus *events.Bus) *Server {
	return &Server{
		ScanPath:   scanPath,
		ProcMgr:    process.NewManagerWithBus(bus),
		SessMgr:    session.NewManagerWithBus(bus),
		EventBus:   bus,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// ToolGroupNames lists all valid tool group names in registration order.
var ToolGroupNames = []string{
	"core", "session", "loop", "prompt", "fleet",
	"repo", "roadmap", "team", "awesome", "advanced", "eval", "fleet_h",
	"observability",
}

func (s *Server) scan() error {
	repos, err := discovery.Scan(context.Background(), s.ScanPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.Repos = repos
	s.mu.Unlock()
	return nil
}

// RACE FIX: return a shallow copy of the Repo struct so that callers
// (e.g. handleStatus → RefreshRepo) can safely mutate fields without
// racing with reposCopy or other concurrent readers of s.Repos.
func (s *Server) findRepo(name string) *model.Repo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.Repos {
		if r.Name == name {
			rc := *r
			return &rc
		}
	}
	return nil
}

func (s *Server) reposCopy() []*model.Repo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]*model.Repo, len(s.Repos))
	for i, r := range s.Repos {
		rc := *r
		cp[i] = &rc
	}
	return cp
}

func (s *Server) reposNil() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Repos == nil
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: text,
		}},
	}
}

func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: msg,
		}},
	}
}

// errCode returns a structured error result with an error_code field.
// error_code values: "invalid_params", "not_found", "internal_error"
func errCode(code, msg string) *mcp.CallToolResult {
	data, _ := json.Marshal(map[string]string{
		"error":      msg,
		"error_code": code,
	})
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: string(data),
		}},
	}
}

func invalidParams(msg string) *mcp.CallToolResult { return errCode("invalid_params", msg) }
func notFound(msg string) *mcp.CallToolResult      { return errCode("not_found", msg) }
func internalErr(msg string) *mcp.CallToolResult   { return errCode("internal_error", msg) }

func jsonResult(v any) *mcp.CallToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult(fmt.Sprintf("json marshal: %v", err))
	}
	return textResult(string(data))
}

func argsMap(req mcp.CallToolRequest) map[string]any {
	if m, ok := req.Params.Arguments.(map[string]any); ok {
		return m
	}
	return nil
}

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func getStringArg(req mcp.CallToolRequest, key string) string {
	m := argsMap(req)
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getNumberArg(req mcp.CallToolRequest, key string, defaultVal float64) float64 {
	m := argsMap(req)
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok {
		if n, ok := v.(float64); ok {
			return n
		}
	}
	return defaultVal
}

func getBoolArg(req mcp.CallToolRequest, key string) bool {
	m := argsMap(req)
	if m == nil {
		return false
	}
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// Handlers

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

	type providerStats struct {
		Sessions    int     `json:"sessions"`
		Running     int     `json:"running"`
		TotalSpend  float64 `json:"total_spend_usd"`
		AvgCostTurn float64 `json:"avg_cost_per_turn"`
		TotalTurns  int     `json:"total_turns"`
	}

	providers := make(map[string]*providerStats)
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
			ps = &providerStats{}
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

func (s *Server) handleWorkflowDefine(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	name := getStringArg(req, "name")
	yamlStr := getStringArg(req, "yaml")
	if repoName == "" || name == "" || yamlStr == "" {
		return errResult("repo, name, and yaml are required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	wf, err := session.ParseWorkflow(name, []byte(yamlStr))
	if err != nil {
		return errResult(fmt.Sprintf("invalid workflow YAML: %v", err)), nil
	}

	if err := session.SaveWorkflow(r.Path, wf); err != nil {
		return errResult(fmt.Sprintf("save failed: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"name":  wf.Name,
		"steps": len(wf.Steps),
		"saved": true,
	}), nil
}

func (s *Server) handleWorkflowRun(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	name := getStringArg(req, "name")
	if repoName == "" || name == "" {
		return errResult("repo and name are required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	wf, err := session.LoadWorkflow(r.Path, name)
	if err != nil {
		return errResult(fmt.Sprintf("load workflow: %v", err)), nil
	}

	run, err := s.SessMgr.RunWorkflow(ctx, r.Path, *wf)
	if err != nil {
		return errResult(fmt.Sprintf("run workflow: %v", err)), nil
	}

	run.Lock()
	result := map[string]any{
		"run_id":     run.ID,
		"workflow":   run.Name,
		"repo_path":  run.RepoPath,
		"status":     run.Status,
		"created_at": run.CreatedAt,
		"updated_at": run.UpdatedAt,
		"steps":      append([]session.WorkflowStepResult(nil), run.Steps...),
	}
	run.Unlock()

	return jsonResult(result), nil
}

func (s *Server) handleSnapshot(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := getStringArg(req, "action")
	if action == "" {
		action = "save"
	}
	name := getStringArg(req, "name")

	if action == "list" {
		if s.reposNil() {
			if err := s.scan(); err != nil {
				return errResult(fmt.Sprintf("scan failed: %v", err)), nil
			}
		}
		allRepos := s.reposCopy()
		var snapshots []string
		for _, r := range allRepos {
			snaps, _ := filepath.Glob(filepath.Join(r.Path, ".ralph", "snapshots", "*.json"))
			for _, snap := range snaps {
				snapshots = append(snapshots, filepath.Base(snap))
			}
		}
		return jsonResult(map[string]any{"snapshots": snapshots}), nil
	}

	// Save snapshot
	if name == "" {
		name = fmt.Sprintf("snapshot-%s", time.Now().Format("20060102-150405"))
	}

	allSessions := s.SessMgr.List("")
	type sessionSnap struct {
		ID       string  `json:"id"`
		Provider string  `json:"provider"`
		Repo     string  `json:"repo"`
		Status   string  `json:"status"`
		SpentUSD float64 `json:"spent_usd"`
		Turns    int     `json:"turns"`
	}
	var sessSnaps []sessionSnap
	for _, sess := range allSessions {
		sess.Lock()
		sessSnaps = append(sessSnaps, sessionSnap{
			ID:       sess.ID,
			Provider: string(sess.Provider),
			Repo:     sess.RepoName,
			Status:   string(sess.Status),
			SpentUSD: sess.SpentUSD,
			Turns:    sess.TurnCount,
		})
		sess.Unlock()
	}

	teams := s.SessMgr.ListTeams()
	snapshot := map[string]any{
		"name":      name,
		"timestamp": time.Now().Format(time.RFC3339),
		"sessions":  sessSnaps,
		"teams":     teams,
	}

	data, _ := json.MarshalIndent(snapshot, "", "  ")

	// Save to first repo's .ralph/snapshots/
	if s.reposNil() {
		_ = s.scan()
	}
	allRepos := s.reposCopy()
	if len(allRepos) > 0 {
		dir := filepath.Join(allRepos[0].Path, ".ralph", "snapshots")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, name+".json"), data, 0o644)
	}

	return jsonResult(snapshot), nil
}

func (s *Server) handleJournalRead(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	limit := int(getNumberArg(req, "limit", 10))
	entries, err := session.ReadRecentJournal(r.Path, limit)
	if err != nil {
		return errResult(fmt.Sprintf("read journal: %v", err)), nil
	}

	synthesis := session.SynthesizeContext(entries)

	return jsonResult(map[string]any{
		"entries":   entries,
		"count":     len(entries),
		"synthesis": synthesis,
	}), nil
}

func (s *Server) handleJournalWrite(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	entry := session.JournalEntry{
		Timestamp: time.Now(),
		SessionID: getStringArg(req, "session_id"),
		RepoName:  r.Name,
	}
	if w := getStringArg(req, "worked"); w != "" {
		entry.Worked = splitCSV(w)
	}
	if f := getStringArg(req, "failed"); f != "" {
		entry.Failed = splitCSV(f)
	}
	if sg := getStringArg(req, "suggest"); sg != "" {
		entry.Suggest = splitCSV(sg)
	}

	if err := session.WriteJournalEntryManual(r.Path, entry); err != nil {
		return errResult(fmt.Sprintf("write journal: %v", err)), nil
	}

	if s.EventBus != nil {
		s.EventBus.Publish(events.Event{
			Type:      events.JournalWritten,
			RepoName:  r.Name,
			RepoPath:  r.Path,
			SessionID: entry.SessionID,
		})
	}

	return jsonResult(map[string]any{
		"status":  "written",
		"repo":    r.Name,
		"worked":  len(entry.Worked),
		"failed":  len(entry.Failed),
		"suggest": len(entry.Suggest),
	}), nil
}

func (s *Server) handleJournalPrune(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	keep := int(getNumberArg(req, "keep", 100))
	dryRun := getStringArg(req, "dry_run") != "false"

	// Read current count
	entries, err := session.ReadRecentJournal(r.Path, 100000)
	if err != nil {
		return errResult(fmt.Sprintf("read journal: %v", err)), nil
	}

	wouldPrune := len(entries) - keep
	if wouldPrune < 0 {
		wouldPrune = 0
	}

	if dryRun {
		return jsonResult(map[string]any{
			"dry_run":     true,
			"total":       len(entries),
			"keep":        keep,
			"would_prune": wouldPrune,
		}), nil
	}

	pruned, err := session.PruneJournal(r.Path, keep)
	if err != nil {
		return errResult(fmt.Sprintf("prune journal: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"dry_run":   false,
		"pruned":    pruned,
		"remaining": len(entries) - pruned,
	}), nil
}

func splitCSV(s string) []string {
	var result []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

// Awesome-list handlers

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
		staleList    []map[string]any
		alerts       []map[string]any
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

	// Team summaries
	var teamSummaries []map[string]any
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
		"alerts": alerts,
	}), nil
}

func (s *Server) handleToolBenchmark(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.ToolRecorder == nil {
		return errResult("tool benchmarking not configured"), nil
	}

	hours := getNumberArg(req, "hours", 24)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	entries, err := s.ToolRecorder.LoadEntries(since)
	if err != nil {
		return internalErr(fmt.Sprintf("loading benchmark data: %v", err)), nil
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
