package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/repofiles"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleRepoHealth(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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

	// Ensure slices marshal as [] not null
	if issues == nil {
		issues = []string{}
	}
	if claudeMDFindings == nil {
		claudeMDFindings = []enhancer.ClaudeMDResult{}
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

func (s *Server) handleRepoOptimize(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return codedError(ErrInvalidParams, "path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid path: %v", err)), nil
	}

	opts := repofiles.OptimizeOptions{
		DryRun: getStringArg(req, "dry_run") != "false",
		Focus:  getStringArg(req, "focus"),
	}

	result, err := repofiles.Optimize(path, opts)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("optimize: %v", err)), nil
	}

	// Ensure slices marshal as [] not null
	if result.Issues == nil {
		result.Issues = []repofiles.OptimizeIssue{}
	}
	if result.Optimizations == nil {
		result.Optimizations = []repofiles.OptimizeAction{}
	}

	return jsonResult(result), nil
}
