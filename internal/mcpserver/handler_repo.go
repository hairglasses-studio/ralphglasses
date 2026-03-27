package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
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
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
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

func (s *Server) handleStart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
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
	if err := s.ProcMgr.Start(ctx, r.Path); err != nil {
		return codedError(ErrInternal, fmt.Sprintf("start failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Started ralph loop for %s", name)), nil
}

func (s *Server) handleStop(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
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
	if err := s.ProcMgr.Stop(ctx, r.Path); err != nil {
		if strings.Contains(err.Error(), "no running loop") {
			return codedError(ErrNotRunning, fmt.Sprintf("no running loop for %s", name)), nil
		}
		return codedError(ErrInternal, fmt.Sprintf("stop failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Stopped ralph loop for %s", name)), nil
}

func (s *Server) handleStopAll(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.ProcMgr.StopAll(ctx)
	return textResult("All managed loops stopped"), nil
}

func (s *Server) handlePause(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
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
		if strings.Contains(err.Error(), "no running loop") {
			return codedError(ErrNotRunning, fmt.Sprintf("no running loop for %s", name)), nil
		}
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
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
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
		if errors.Is(err, os.ErrNotExist) {
			return emptyResult("log_lines"), nil
		}
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
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
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
