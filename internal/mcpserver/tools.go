package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
)

// Server holds state for the MCP server.
type Server struct {
	ScanPath string
	Repos    []*model.Repo
	ProcMgr  *process.Manager
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
		_ = s.scan()
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
		_ = s.scan()
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
		_ = s.scan()
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
		_ = s.scan()
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
		_ = s.scan()
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
		_ = s.scan()
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
