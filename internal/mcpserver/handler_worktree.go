package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/parity"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleWorktreeCreate(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "worktree name required"), nil
	}
	if err := validateSafePath(name); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("worktree name: %v", err)), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	wtPath, branch, err := session.CreateWorktree(r.Path, name)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("create worktree: %v", err)), nil
	}

	type createResult struct {
		Repo   string `json:"repo"`
		Path   string `json:"path"`
		Branch string `json:"branch"`
	}
	return jsonResult(createResult{Repo: repoName, Path: wtPath, Branch: branch}), nil
}

func (s *Server) handleWorktreeCleanup(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	maxAgeHours := getNumberArg(req, "max_age_hours", 24)
	if maxAgeHours < 1 {
		maxAgeHours = 1
	}
	maxAge := time.Duration(maxAgeHours) * time.Hour

	if getBoolArg(req, "dry_run") {
		worktrees, err := parity.PreviewWorktreeCleanup(r.Path, maxAge)
		if err != nil {
			return codedError(ErrFilesystem, fmt.Sprintf("cleanup preview failed: %v", err)), nil
		}
		return jsonResult(map[string]any{
			"repo":      repoName,
			"dry_run":   true,
			"max_age":   maxAge.String(),
			"cleaned":   0,
			"count":     len(worktrees),
			"worktrees": worktrees,
			"message":   fmt.Sprintf("would clean %d stale worktrees older than %s", len(worktrees), maxAge),
		}), nil
	}

	cleaned, err := session.CleanupStaleWorktrees(r.Path, maxAge)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("cleanup failed: %v", err)), nil
	}

	type cleanupResult struct {
		Repo    string `json:"repo"`
		Cleaned int    `json:"cleaned"`
		MaxAge  string `json:"max_age"`
		Message string `json:"message"`
	}
	result := cleanupResult{
		Repo:    repoName,
		Cleaned: cleaned,
		MaxAge:  maxAge.String(),
		Message: fmt.Sprintf("cleaned %d stale worktrees older than %s", cleaned, maxAge),
	}

	return jsonResult(result), nil
}
