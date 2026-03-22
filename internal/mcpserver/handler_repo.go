package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/repofiles"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

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

func (s *Server) handleRepoHealth(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	model.RefreshRepo(r)

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

func (s *Server) handleConfigBulk(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key := getStringArg(req, "key")
	if key == "" {
		return errResult("key required"), nil
	}
	value := getStringArg(req, "value")
	reposStr := getStringArg(req, "repos")

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
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
		model.RefreshRepo(r)
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
