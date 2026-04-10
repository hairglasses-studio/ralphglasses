package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// validDispatchActions enumerates the actions accepted by handleDispatch.
var validDispatchActions = map[string]bool{
	"send":   true,
	"stop":   true,
	"pause":  true,
	"resume": true,
	"retry":  true,
}

// handleDispatch is a unified cross-provider dispatch tool for mobile remote
// control. It collapses rc_send, rc_act(stop/pause/resume/retry) into a single
// entry point with optional runtime provider selection when provider is omitted
// or set to "auto".
func (s *Server) handleDispatch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParamParser(argsMap(req))

	repo := pp.StringOpt("repo", "")
	if repo == "" {
		return codedError(ErrInvalidParams, "repo required"), nil
	}
	if err := ValidateRepoName(repo); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
	}

	action := pp.StringOpt("action", "")
	if action == "" {
		return codedError(ErrInvalidParams, "action required"), nil
	}
	if !validDispatchActions[action] {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid action %q (valid: send, stop, pause, resume, retry)", action)), nil
	}

	providerStr := pp.StringOpt("provider", "auto")

	switch action {
	case "send":
		return s.dispatchSend(ctx, pp, repo, providerStr)
	case "stop":
		return s.dispatchStop(repo)
	case "pause":
		return s.dispatchPause(repo)
	case "resume":
		return s.dispatchResume(ctx, repo)
	case "retry":
		return s.dispatchRetry(repo)
	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unhandled action: %s", action)), nil
	}
}

// dispatchSend launches a new session on the given repo. When provider is
// omitted or set to "auto", the runtime chooses the effective provider.
func (s *Server) dispatchSend(ctx context.Context, pp *ParamParser, repo, providerStr string) (*mcp.CallToolResult, error) {
	prompt := SanitizeString(pp.StringOpt("prompt", ""))
	if prompt == "" {
		return codedError(ErrInvalidParams, "prompt required for send action"), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repo)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repo)), nil
	}

	provider, _, err := parseOptionalLaunchProvider(providerStr)
	if err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid provider: %v", err)), nil
	}

	// Stop existing running sessions on this repo.
	existing := s.SessMgr.FindByRepo(repo)
	for _, sess := range existing {
		sess.Lock()
		st := sess.Status
		sid := sess.ID
		sess.Unlock()
		if st == session.StatusRunning || st == session.StatusLaunching {
			if err := s.SessMgr.Stop(sid); err != nil {
				slog.Warn("dispatch: failed to stop existing session", "session", sid, "error", err)
			}
		}
	}

	budget := 5.0
	opts := session.LaunchOptions{
		Provider:     provider,
		RepoPath:     r.Path,
		Prompt:       prompt,
		MaxBudgetUSD: budget,
	}

	// Inject journal context.
	journal, _ := session.ReadRecentJournal(r.Path, 5)
	if len(journal) > 0 {
		journalCtx := session.SynthesizeContext(journal)
		if journalCtx != "" {
			opts.Prompt = journalCtx + "\n\n---\n\n" + opts.Prompt
		}
	}

	sess, err := s.SessMgr.Launch(context.Background(), opts)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("launch failed: %v", err)), nil
	}

	return textResult(fmt.Sprintf("Sent to %s/%s (budget: %s, id: %s)",
		repo, sess.Provider, formatCost(budget), shortID(sess.ID)) + launchSelectionSuffix(sess)), nil
}

// dispatchStop stops the most active session on the given repo.
func (s *Server) dispatchStop(repo string) (*mcp.CallToolResult, error) {
	sess, errRes := s.resolveTarget(repo)
	if errRes != nil {
		return errRes, nil
	}
	sess.Lock()
	sid := sess.ID
	repoName := sess.RepoName
	cost := sess.SpentUSD
	turns := sess.TurnCount
	sess.Unlock()

	if err := s.SessMgr.Stop(sid); err != nil {
		return codedError(ErrInternal, fmt.Sprintf("stop failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Stopped %s (%s, %dt)", repoName, formatCost(cost), turns)), nil
}

// dispatchPause toggles pause on the repo's loop.
func (s *Server) dispatchPause(repo string) (*mcp.CallToolResult, error) {
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repo)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repo)), nil
	}
	nowPaused, err := s.ProcMgr.TogglePause(r.Path)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("pause toggle failed: %v", err)), nil
	}
	if nowPaused {
		return textResult(fmt.Sprintf("Paused %s", repo)), nil
	}
	return textResult(fmt.Sprintf("Resumed %s", repo)), nil
}

// dispatchResume resumes a stopped session by its provider session ID.
func (s *Server) dispatchResume(ctx context.Context, repo string) (*mcp.CallToolResult, error) {
	sess, errRes := s.resolveTarget(repo)
	if errRes != nil {
		return errRes, nil
	}
	sess.Lock()
	psid := sess.ProviderSessionID
	repoPath := sess.RepoPath
	provider := sess.Provider
	sess.Unlock()

	if psid == "" {
		return codedError(ErrInvalidParams, "session has no provider session ID to resume"), nil
	}
	newSess, err := s.SessMgr.Resume(ctx, repoPath, provider, psid, "")
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("resume failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Resumed %s (id: %s)", repo, shortID(newSess.ID))), nil
}

// dispatchRetry re-launches the most recent session on the repo with same options.
func (s *Server) dispatchRetry(repo string) (*mcp.CallToolResult, error) {
	sess, errRes := s.resolveTarget(repo)
	if errRes != nil {
		return errRes, nil
	}
	sess.Lock()
	opts := session.LaunchOptions{
		Provider:     sess.Provider,
		RepoPath:     sess.RepoPath,
		Prompt:       sess.Prompt,
		Model:        sess.Model,
		MaxBudgetUSD: sess.BudgetUSD,
		MaxTurns:     sess.MaxTurns,
		Agent:        sess.AgentName,
		TeamName:     sess.TeamName,
	}
	sess.Unlock()

	newSess, err := s.SessMgr.Launch(context.Background(), opts)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("retry failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Retried %s (id: %s)", repo, shortID(newSess.ID))), nil
}

// resolveProvider maps a provider string to an explicit session.Provider. When
// omitted or set to "auto", runtime selection stays unset.
func (s *Server) resolveProvider(providerStr string) session.Provider {
	provider, _, _ := parseOptionalLaunchProvider(providerStr)
	return provider
}

// handleFleetSummary returns a compact one-call fleet overview formatted as
// readable text for mobile use. It reports total sessions, status breakdown,
// total cost, and a per-repo status line.
func (s *Server) handleFleetSummary(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	all := s.SessMgr.List("")

	var (
		totalCost float64
		active    int
		paused    int
		stopped   int
	)

	// repoStats collects per-repo status.
	type repoStat struct {
		status   string
		provider session.Provider
		cost     float64
		turns    int
		idle     time.Duration
	}
	repoMap := make(map[string]repoStat)
	now := time.Now()

	for _, sess := range all {
		sess.Lock()
		st := sess.Status
		cost := sess.SpentUSD
		repoName := sess.RepoName
		provider := sess.Provider
		turns := sess.TurnCount
		la := sess.LastActivity
		sess.Unlock()

		totalCost += cost

		switch {
		case st == session.StatusRunning || st == session.StatusLaunching:
			active++
		case st == session.StatusStopped:
			stopped++
		case st == session.StatusCompleted:
			stopped++
		case st == session.StatusErrored:
			stopped++
		}

		// Keep the most recently active session per repo.
		existing, exists := repoMap[repoName]
		idle := now.Sub(la)
		if !exists || idle < existing.idle {
			repoMap[repoName] = repoStat{
				status:   string(st),
				provider: provider,
				cost:     cost,
				turns:    turns,
				idle:     idle,
			}
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Fleet: %d total | %d active | %d stopped | %s spent",
		len(all), active, stopped, formatCost(totalCost))

	if paused > 0 {
		fmt.Fprintf(&b, " | %d paused", paused)
	}

	if len(repoMap) > 0 {
		b.WriteString("\n")
		for repo, rs := range repoMap {
			fmt.Fprintf(&b, "\n  %s: [%s] %s %s %dt %s idle",
				repo, rs.status, rs.provider, formatCost(rs.cost), rs.turns, formatDuration(rs.idle))
		}
	}

	if len(all) == 0 {
		b.WriteString("\nNo sessions.")
	}

	return textResult(b.String()), nil
}
