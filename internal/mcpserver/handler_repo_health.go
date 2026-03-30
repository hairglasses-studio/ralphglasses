package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/repofiles"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// HealthWeights controls the penalty applied per health check category.
// Each field is a multiplier on the default penalty for that category.
// A value of 1.0 means the default penalty; 0 disables the check.
type HealthWeights struct {
	CircuitBreakerOpen     float64 `json:"circuit_breaker_open"`      // default penalty: 30
	CircuitBreakerHalfOpen float64 `json:"circuit_breaker_half_open"` // default penalty: 10
	Staleness              float64 `json:"staleness"`                 // default penalty: 15
	BudgetExceeded         float64 `json:"budget_exceeded"`           // default penalty: 20
	ErroredSession         float64 `json:"errored_session"`           // default penalty: 5 per session
	ConfigParseError       float64 `json:"config_parse_error"`        // default penalty: 5
	MissingDirectory       float64 `json:"missing_directory"`         // default penalty: 5 per dir
	StaleLockFile          float64 `json:"stale_lock_file"`           // default penalty: 10
	ClaudeMDWarnings       float64 `json:"claudemd_warnings"`         // default penalty: 10
}

// DefaultHealthWeights returns sensible default weights (all 1.0).
func DefaultHealthWeights() HealthWeights {
	return HealthWeights{
		CircuitBreakerOpen:     1.0,
		CircuitBreakerHalfOpen: 1.0,
		Staleness:              1.0,
		BudgetExceeded:         1.0,
		ErroredSession:         1.0,
		ConfigParseError:       1.0,
		MissingDirectory:       1.0,
		StaleLockFile:          1.0,
		ClaudeMDWarnings:       1.0,
	}
}

// weightedPenalty computes a penalty as int(basePenalty * weight), clamped to >= 0.
func weightedPenalty(base int, weight float64) int {
	p := int(float64(base) * weight)
	if p < 0 {
		return 0
	}
	return p
}

// computeHealthScore computes a repo health score (0-100) given a set of
// issue booleans/counts and the provided weights. It returns the score and
// the list of issue strings. This is extracted for testability.
func computeHealthScore(params healthParams, w HealthWeights) (int, []string) {
	score := 100
	var issues []string

	// Circuit breaker
	if params.cbState == "OPEN" {
		score -= weightedPenalty(30, w.CircuitBreakerOpen)
		issues = append(issues, fmt.Sprintf("circuit breaker OPEN: %s", params.cbReason))
	} else if params.cbState == "HALF_OPEN" {
		score -= weightedPenalty(10, w.CircuitBreakerHalfOpen)
		issues = append(issues, "circuit breaker HALF_OPEN")
	}

	// Staleness
	if params.staleMinutes > 60 {
		score -= weightedPenalty(15, w.Staleness)
		issues = append(issues, fmt.Sprintf("status stale (%.0f min)", params.staleMinutes))
	}

	// Budget
	if params.budgetExceeded {
		score -= weightedPenalty(20, w.BudgetExceeded)
		issues = append(issues, "budget exceeded")
	}

	// Errored sessions
	for i := 0; i < params.erroredSessions; i++ {
		score -= weightedPenalty(5, w.ErroredSession)
	}
	if params.erroredSessions > 0 {
		issues = append(issues, fmt.Sprintf("%d errored sessions", params.erroredSessions))
	}

	// Config parse error
	if params.configParseError != "" {
		score -= weightedPenalty(5, w.ConfigParseError)
		issues = append(issues, fmt.Sprintf(".ralphrc parse error: %s", params.configParseError))
	}

	// Deprecated config keys
	issues = append(issues, params.deprecatedKeyIssues...)

	// Missing directories
	for _, dir := range params.missingDirs {
		score -= weightedPenalty(5, w.MissingDirectory)
		issues = append(issues, fmt.Sprintf("missing directory: %s", dir))
	}

	// Stale lock file
	if params.staleLockMinutes > 60 {
		score -= weightedPenalty(10, w.StaleLockFile)
		issues = append(issues, fmt.Sprintf("stale .git/index.lock (age: %.0f min)", params.staleLockMinutes))
	}

	// CLAUDE.md warnings
	if params.claudeMDWarnings > 3 {
		score -= weightedPenalty(10, w.ClaudeMDWarnings)
		issues = append(issues, fmt.Sprintf("CLAUDE.md: %d warnings", params.claudeMDWarnings))
	}

	if score < 0 {
		score = 0
	}
	return score, issues
}

// healthParams captures the raw data needed for health score computation.
type healthParams struct {
	cbState             string
	cbReason            string
	staleMinutes        float64
	budgetExceeded      bool
	erroredSessions     int
	configParseError    string
	deprecatedKeyIssues []string
	missingDirs         []string
	staleLockMinutes    float64
	claudeMDWarnings    int
}

func (s *Server) handleRepoHealth(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
	if errs := model.RefreshRepo(ctx, r); len(errs) > 0 {
		for _, e := range errs {
			slog.Warn("handleRepoHealth: refresh failed", "repo", r.Path, "err", e)
		}
	}

	weights := DefaultHealthWeights()

	var params healthParams

	// Circuit breaker
	params.cbState = "CLOSED"
	if r.Circuit != nil {
		params.cbState = r.Circuit.State
		params.cbReason = r.Circuit.Reason
	}

	// Staleness
	if r.Status != nil && !r.Status.Timestamp.IsZero() {
		age := time.Since(r.Status.Timestamp)
		params.staleMinutes = age.Minutes()
	}

	// Budget
	if r.Status != nil && r.Status.BudgetStatus == "exceeded" {
		params.budgetExceeded = true
	}

	// Active sessions
	activeSessions := 0
	totalSpend := 0.0
	for _, sess := range s.SessMgr.List("") {
		sess.Lock()
		if sess.RepoName == name || filepath.Base(sess.RepoPath) == name {
			if sess.Status == session.StatusRunning {
				activeSessions++
			}
			if sess.Status == session.StatusErrored {
				params.erroredSessions++
			}
			totalSpend += sess.SpentUSD
		}
		sess.Unlock()
	}

	// .ralphrc parse check
	if _, err := model.LoadConfig(ctx, r.Path); err != nil {
		if !os.IsNotExist(err) {
			params.configParseError = err.Error()
		}
	} else if r.Config != nil {
		configWarnings, _ := model.ValidateConfig(r.Config)
		for _, w := range configWarnings {
			if _, deprecated := model.DeprecatedKeys[w.Key]; deprecated {
				params.deprecatedKeyIssues = append(params.deprecatedKeyIssues,
					fmt.Sprintf("deprecated config key %s: %s", w.Key, w.Message))
			}
		}
	}

	// Missing directories
	for _, dir := range []string{".ralph", filepath.Join(".ralph", "logs")} {
		dirPath := filepath.Join(r.Path, dir)
		if _, err := os.Stat(dirPath); os.IsNotExist(err) {
			params.missingDirs = append(params.missingDirs, dir)
		}
	}

	// Stale lock files
	lockPath := filepath.Join(r.Path, ".git", "index.lock")
	if info, err := os.Stat(lockPath); err == nil {
		params.staleLockMinutes = time.Since(info.ModTime()).Minutes()
	}

	// CLAUDE.md health
	claudeMDPath := filepath.Join(r.Path, "CLAUDE.md")
	var claudeMDFindings []enhancer.ClaudeMDResult
	if claudeResults, err := enhancer.CheckClaudeMD(claudeMDPath); err == nil {
		claudeMDFindings = claudeResults
		for _, finding := range claudeResults {
			if finding.Severity == "warn" {
				params.claudeMDWarnings++
			}
		}
	}

	cbState := params.cbState
	score, issues := computeHealthScore(params, weights)

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
		"errored_sessions":  params.erroredSessions,
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
