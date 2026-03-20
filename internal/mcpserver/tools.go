package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/repofiles"
	"github.com/hairglasses-studio/ralphglasses/internal/roadmap"
)

// Server holds state for the MCP server.
type Server struct {
	ScanPath   string
	Repos      []*model.Repo
	ProcMgr    *process.Manager
	HTTPClient *http.Client
}

// NewServer creates a new MCP server instance.
func NewServer(scanPath string) *Server {
	return &Server{
		ScanPath: scanPath,
		ProcMgr:  process.NewManager(),
	}
}

// Register adds all ralphglasses tools to the MCP server.
func (s *Server) Register(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("ralphglasses_scan",
		mcp.WithDescription("Scan for ralph-enabled repos and return their current status"),
	), s.handleScan)

	srv.AddTool(mcp.NewTool("ralphglasses_list",
		mcp.WithDescription("List all discovered repos with status summary"),
	), s.handleList)

	srv.AddTool(mcp.NewTool("ralphglasses_status",
		mcp.WithDescription("Get detailed status for a specific repo including loop status, circuit breaker, progress, and config"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name (basename of directory)")),
	), s.handleStatus)

	srv.AddTool(mcp.NewTool("ralphglasses_start",
		mcp.WithDescription("Start a ralph loop for a repo"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name to start loop for")),
	), s.handleStart)

	srv.AddTool(mcp.NewTool("ralphglasses_stop",
		mcp.WithDescription("Stop a running ralph loop for a repo"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name to stop loop for")),
	), s.handleStop)

	srv.AddTool(mcp.NewTool("ralphglasses_stop_all",
		mcp.WithDescription("Stop all running ralph loops"),
	), s.handleStopAll)

	srv.AddTool(mcp.NewTool("ralphglasses_pause",
		mcp.WithDescription("Pause or resume a running ralph loop"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name to pause/resume")),
	), s.handlePause)

	srv.AddTool(mcp.NewTool("ralphglasses_logs",
		mcp.WithDescription("Get recent log lines from a repo's ralph log"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithNumber("lines", mcp.Description("Number of lines to return (default 50, max 500)")),
	), s.handleLogs)

	srv.AddTool(mcp.NewTool("ralphglasses_config",
		mcp.WithDescription("Get or set .ralphrc config values for a repo"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("key", mcp.Description("Config key to get/set (omit to list all)")),
		mcp.WithString("value", mcp.Description("Value to set (omit to get current value)")),
	), s.handleConfig)

	// Roadmap automation tools

	srv.AddTool(mcp.NewTool("ralphglasses_roadmap_parse",
		mcp.WithDescription("Parse ROADMAP.md into structured JSON (phases, sections, tasks, deps, completion stats)"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root or direct .md path")),
		mcp.WithString("file", mcp.Description("Override filename (default: ROADMAP.md)")),
	), s.handleRoadmapParse)

	srv.AddTool(mcp.NewTool("ralphglasses_roadmap_analyze",
		mcp.WithDescription("Compare roadmap vs codebase — find gaps, stale checkboxes, ready tasks, orphaned code"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
		mcp.WithString("file", mcp.Description("Override filename (default: ROADMAP.md)")),
	), s.handleRoadmapAnalyze)

	srv.AddTool(mcp.NewTool("ralphglasses_roadmap_research",
		mcp.WithDescription("Search GitHub for relevant repos and tools that unlock new capabilities"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
		mcp.WithString("topics", mcp.Description("Search topics (inferred from go.mod/README if omitted)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 10)")),
	), s.handleRoadmapResearch)

	srv.AddTool(mcp.NewTool("ralphglasses_roadmap_expand",
		mcp.WithDescription("Generate proposed roadmap expansions from analysis gaps and research findings"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
		mcp.WithString("file", mcp.Description("Override filename (default: ROADMAP.md)")),
		mcp.WithString("research", mcp.Description("Research topics to include (runs research internally)")),
		mcp.WithString("style", mcp.Description("Expansion style: conservative, balanced, aggressive (default: balanced)")),
	), s.handleRoadmapExpand)

	srv.AddTool(mcp.NewTool("ralphglasses_roadmap_export",
		mcp.WithDescription("Export roadmap items as structured task specs for ralph loop consumption"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
		mcp.WithString("file", mcp.Description("Override filename (default: ROADMAP.md)")),
		mcp.WithString("format", mcp.Description("Output format: rdcycle, fix_plan, progress (default: rdcycle)")),
		mcp.WithString("phase", mcp.Description("Filter by phase name (default: all)")),
		mcp.WithString("section", mcp.Description("Filter by section name (default: all)")),
		mcp.WithNumber("max_tasks", mcp.Description("Max tasks to export (default 20)")),
		mcp.WithString("respect_deps", mcp.Description("Skip tasks with unmet deps (default: true)")),
	), s.handleRoadmapExport)

	// Repo file management tools

	srv.AddTool(mcp.NewTool("ralphglasses_repo_scaffold",
		mcp.WithDescription("Create/initialize ralph config files (.ralph/, .ralphrc, PROMPT.md, AGENT.md, fix_plan.md) for a repo"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
		mcp.WithString("project_type", mcp.Description("Project type override (auto-detected from go.mod, package.json, etc.)")),
		mcp.WithString("project_name", mcp.Description("Project name override (defaults to directory name)")),
		mcp.WithString("force", mcp.Description("Overwrite existing files: true/false (default: false)")),
	), s.handleRepoScaffold)

	srv.AddTool(mcp.NewTool("ralphglasses_repo_optimize",
		mcp.WithDescription("Analyze and optimize ralph config files — detect misconfigs, missing settings, stale plans"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
		mcp.WithString("focus", mcp.Description("Focus area: config, prompt, plan, all (default: all)")),
		mcp.WithString("dry_run", mcp.Description("Report only, don't modify: true/false (default: true)")),
	), s.handleRepoOptimize)
}

func (s *Server) scan() error {
	repos, err := discovery.Scan(s.ScanPath)
	if err != nil {
		return err
	}
	s.Repos = repos
	return nil
}

func (s *Server) findRepo(name string) *model.Repo {
	for _, r := range s.Repos {
		if r.Name == name {
			return r
		}
	}
	return nil
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

// Handlers

func (s *Server) handleScan(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.scan(); err != nil {
		return errResult(fmt.Sprintf("scan failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Found %d ralph-enabled repos", len(s.Repos))), nil
}

func (s *Server) handleList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.Repos == nil {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	// Refresh all
	for _, r := range s.Repos {
		model.RefreshRepo(r)
	}

	type repoSummary struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Loop    int    `json:"loop_count"`
		Calls   string `json:"calls"`
		Circuit string `json:"circuit"`
		Running bool   `json:"managed"`
	}

	summaries := make([]repoSummary, len(s.Repos))
	for i, r := range s.Repos {
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
		return errResult("repo name required"), nil
	}
	if s.Repos == nil {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}
	model.RefreshRepo(r)

	detail := map[string]any{
		"name":    r.Name,
		"path":    r.Path,
		"managed": s.ProcMgr.IsRunning(r.Path),
		"paused":  s.ProcMgr.IsPaused(r.Path),
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
		return errResult("repo name required"), nil
	}
	if s.Repos == nil {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}
	if err := s.ProcMgr.Start(r.Path); err != nil {
		return errResult(fmt.Sprintf("start failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Started ralph loop for %s", name)), nil
}

func (s *Server) handleStop(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if s.Repos == nil {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}
	if err := s.ProcMgr.Stop(r.Path); err != nil {
		return errResult(fmt.Sprintf("stop failed: %v", err)), nil
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
		return errResult("repo name required"), nil
	}
	if s.Repos == nil {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}
	paused, err := s.ProcMgr.TogglePause(r.Path)
	if err != nil {
		return errResult(fmt.Sprintf("pause toggle failed: %v", err)), nil
	}
	if paused {
		return textResult(fmt.Sprintf("Paused loop for %s", name)), nil
	}
	return textResult(fmt.Sprintf("Resumed loop for %s", name)), nil
}

func (s *Server) handleLogs(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if s.Repos == nil {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	maxLines := int(getNumberArg(req, "lines", 50))
	if maxLines > 500 {
		maxLines = 500
	}

	allLines, err := process.ReadFullLog(r.Path)
	if err != nil {
		return errResult(fmt.Sprintf("read log: %v", err)), nil
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
		return errResult("repo name required"), nil
	}
	if s.Repos == nil {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	if r.Config == nil {
		return errResult(fmt.Sprintf("no .ralphrc found for %s", name)), nil
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
			return errResult(fmt.Sprintf("save config: %v", err)), nil
		}
		return textResult(fmt.Sprintf("Set %s=%s for %s", key, value, name)), nil
	}

	// Get value
	v := r.Config.Get(key, "")
	if v == "" {
		return errResult(fmt.Sprintf("key not found: %s", key)), nil
	}
	return textResult(fmt.Sprintf("%s=%s", key, v)), nil
}

// Roadmap handlers

func (s *Server) handleRoadmapParse(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}
	file := getStringArg(req, "file")
	rmPath := roadmap.ResolvePath(path, file)

	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return errResult(fmt.Sprintf("parse roadmap: %v", err)), nil
	}
	return jsonResult(rm), nil
}

func (s *Server) handleRoadmapAnalyze(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}
	file := getStringArg(req, "file")
	rmPath := roadmap.ResolvePath(path, file)

	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return errResult(fmt.Sprintf("parse roadmap: %v", err)), nil
	}

	analysis, err := roadmap.Analyze(rm, path)
	if err != nil {
		return errResult(fmt.Sprintf("analyze: %v", err)), nil
	}
	return jsonResult(analysis), nil
}

func (s *Server) handleRoadmapResearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}
	topics := getStringArg(req, "topics")
	limit := int(getNumberArg(req, "limit", 10))

	results, err := roadmap.Research(ctx, s.HTTPClient, path, topics, limit)
	if err != nil {
		return errResult(fmt.Sprintf("research: %v", err)), nil
	}
	return jsonResult(results), nil
}

func (s *Server) handleRoadmapExpand(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}
	file := getStringArg(req, "file")
	style := getStringArg(req, "style")
	researchTopics := getStringArg(req, "research")

	rmPath := roadmap.ResolvePath(path, file)
	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return errResult(fmt.Sprintf("parse roadmap: %v", err)), nil
	}

	analysis, err := roadmap.Analyze(rm, path)
	if err != nil {
		return errResult(fmt.Sprintf("analyze: %v", err)), nil
	}

	var research *roadmap.ResearchResults
	if researchTopics != "" {
		research, _ = roadmap.Research(ctx, s.HTTPClient, path, researchTopics, 10)
	}

	expansion, err := roadmap.Expand(rm, analysis, research, style)
	if err != nil {
		return errResult(fmt.Sprintf("expand: %v", err)), nil
	}
	return jsonResult(expansion), nil
}

func (s *Server) handleRoadmapExport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}
	file := getStringArg(req, "file")
	format := getStringArg(req, "format")
	phase := getStringArg(req, "phase")
	section := getStringArg(req, "section")
	maxTasks := int(getNumberArg(req, "max_tasks", 20))
	respectDeps := getStringArg(req, "respect_deps") != "false"

	rmPath := roadmap.ResolvePath(path, file)
	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return errResult(fmt.Sprintf("parse roadmap: %v", err)), nil
	}

	output, err := roadmap.Export(rm, format, phase, section, maxTasks, respectDeps)
	if err != nil {
		return errResult(fmt.Sprintf("export: %v", err)), nil
	}
	return textResult(output), nil
}

// Repo file handlers

func (s *Server) handleRepoScaffold(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}

	opts := repofiles.ScaffoldOptions{
		ProjectType: getStringArg(req, "project_type"),
		ProjectName: getStringArg(req, "project_name"),
		Force:       getStringArg(req, "force") == "true",
	}

	result, err := repofiles.Scaffold(path, opts)
	if err != nil {
		return errResult(fmt.Sprintf("scaffold: %v", err)), nil
	}
	return jsonResult(result), nil
}

func (s *Server) handleRepoOptimize(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}

	opts := repofiles.OptimizeOptions{
		DryRun: getStringArg(req, "dry_run") != "false",
		Focus:  getStringArg(req, "focus"),
	}

	result, err := repofiles.Optimize(path, opts)
	if err != nil {
		return errResult(fmt.Sprintf("optimize: %v", err)), nil
	}
	return jsonResult(result), nil
}
