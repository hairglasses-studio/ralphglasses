package mcpserver

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleSweepReport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	sweepID, errResult := p.RequireString("sweep_id")
	if errResult != nil {
		return errResult, nil
	}
	format := p.OptionalString("format", "markdown")

	sessions := s.sweepSessions(sweepID)
	if len(sessions) == 0 {
		return emptyResult("sweep_sessions"), nil
	}

	var totalCost float64
	var completed, errored, running int
	var items []map[string]any

	for _, sess := range sessions {
		sess.Lock()
		status := sess.Status
		spent := sess.SpentUSD
		repo := sess.RepoName
		repoPath := sess.RepoPath
		turns := sess.TurnCount
		launched := sess.LaunchedAt
		lastOut := sess.LastOutput
		sess.Unlock()

		totalCost += spent
		switch {
		case status == session.StatusCompleted:
			completed++
		case status == session.StatusErrored:
			errored++
		case status == session.StatusRunning || status == session.StatusLaunching:
			running++
		}

		// Count commits since session launch.
		commits := countCommitsSince(repoPath, launched)
		diffStat := gitDiffStat(repoPath, commits)

		item := map[string]any{
			"repo":      repo,
			"status":    status,
			"cost_usd":  spent,
			"turns":     turns,
			"commits":   commits,
			"changes":   diffStat,
			"last_output": lastOut,
		}
		items = append(items, item)
	}

	total := len(sessions)

	switch format {
	case "json":
		return jsonResult(map[string]any{
			"sweep_id":  sweepID,
			"total":     total,
			"completed": completed,
			"errored":   errored,
			"running":   running,
			"cost_usd":  totalCost,
			"repos":     items,
		}), nil

	default: // markdown
		var b strings.Builder
		b.WriteString(fmt.Sprintf("## Sweep Report: %s\n\n", sweepID))
		b.WriteString("### Summary\n\n")
		b.WriteString(fmt.Sprintf("- Repos: %d, Completed: %d, Errored: %d, Running: %d\n", total, completed, errored, running))
		b.WriteString(fmt.Sprintf("- Total cost: $%.2f\n", totalCost))

		totalCommits := 0
		for _, item := range items {
			totalCommits += item["commits"].(int)
		}
		b.WriteString(fmt.Sprintf("- Total commits: %d\n\n", totalCommits))

		b.WriteString("### Per-Repo Results\n\n")
		b.WriteString("| Repo | Status | Commits | Cost | Turns | Changes |\n")
		b.WriteString("|------|--------|---------|------|-------|---------|\n")
		for _, item := range items {
			b.WriteString(fmt.Sprintf("| %s | %s | %d | $%.2f | %d | %s |\n",
				item["repo"], item["status"], item["commits"],
				item["cost_usd"], item["turns"], item["changes"]))
		}

		return textResult(b.String()), nil
	}
}

func (s *Server) handleSweepRetry(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	sweepID, errResult := p.RequireString("sweep_id")
	if errResult != nil {
		return errResult, nil
	}
	budgetOverride := p.OptionalNumber("budget_usd", 0)

	sessions := s.sweepSessions(sweepID)
	if len(sessions) == 0 {
		return emptyResult("sweep_sessions"), nil
	}

	var retried []map[string]any
	var skipped int

	for _, sess := range sessions {
		sess.Lock()
		status := sess.Status
		repoPath := sess.RepoPath
		repo := sess.RepoName
		prompt := sess.Prompt
		model := sess.Model
		budget := sess.BudgetUSD
		permMode := sess.PermissionMode
		noPerist := false // can't read from session, default to false for retries
		sess.Unlock()

		if status != session.StatusErrored && status != session.StatusStopped {
			skipped++
			continue
		}

		if budgetOverride > 0 {
			budget = budgetOverride
		}

		opts := session.LaunchOptions{
			Provider:             session.ProviderClaude,
			RepoPath:             repoPath,
			Prompt:               prompt,
			Model:                model,
			MaxBudgetUSD:         budget,
			PermissionMode:       permMode,
			SweepID:              sweepID,
			NoSessionPersistence: noPerist,
		}

		newSess, err := s.SessMgr.Launch(context.Background(), opts)
		if err != nil {
			retried = append(retried, map[string]any{
				"repo":  repo,
				"error": err.Error(),
			})
			continue
		}

		retried = append(retried, map[string]any{
			"repo":       repo,
			"session_id": newSess.ID,
			"status":     "relaunched",
		})
	}

	return jsonResult(map[string]any{
		"sweep_id": sweepID,
		"retried":  len(retried),
		"skipped":  skipped,
		"details":  retried,
	}), nil
}

func (s *Server) handleSweepPush(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	sweepID, errResult := p.RequireString("sweep_id")
	if errResult != nil {
		return errResult, nil
	}
	dryRun := p.OptionalBool("dry_run", false)

	sessions := s.sweepSessions(sweepID)
	if len(sessions) == 0 {
		return emptyResult("sweep_sessions"), nil
	}

	// Deduplicate repos (multiple sessions may target the same repo).
	seen := make(map[string]bool)
	var results []map[string]any

	for _, sess := range sessions {
		sess.Lock()
		repoPath := sess.RepoPath
		repo := sess.RepoName
		sess.Unlock()

		if seen[repoPath] {
			continue
		}
		seen[repoPath] = true

		// Check for unpushed commits.
		unpushed := countUnpushedCommits(repoPath)
		if unpushed == 0 {
			results = append(results, map[string]any{
				"repo":   repo,
				"status": "up_to_date",
			})
			continue
		}

		if dryRun {
			results = append(results, map[string]any{
				"repo":     repo,
				"status":   "would_push",
				"unpushed": unpushed,
			})
			continue
		}

		cmd := exec.Command("git", "push", "origin", "HEAD")
		cmd.Dir = repoPath
		out, err := cmd.CombinedOutput()
		if err != nil {
			results = append(results, map[string]any{
				"repo":   repo,
				"status": "push_failed",
				"error":  strings.TrimSpace(string(out)),
			})
		} else {
			results = append(results, map[string]any{
				"repo":     repo,
				"status":   "pushed",
				"unpushed": unpushed,
			})
		}
	}

	return jsonResult(map[string]any{
		"sweep_id": sweepID,
		"dry_run":  dryRun,
		"repos":    results,
	}), nil
}

// countCommitsSince returns the number of commits in a repo since a given time.
func countCommitsSince(repoPath string, since time.Time) int {
	cmd := exec.Command("git", "log", "--oneline", "--since="+since.Format(time.RFC3339))
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return 0
	}
	return len(lines)
}

// gitDiffStat returns a compact diff stat string for N commits.
func gitDiffStat(repoPath string, commits int) string {
	if commits == 0 {
		return "no changes"
	}
	cmd := exec.Command("git", "diff", "--stat", fmt.Sprintf("HEAD~%d", commits), "HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return "no changes"
	}
	// Return the summary line (last line: "N files changed, N insertions(+), N deletions(-)")
	return strings.TrimSpace(lines[len(lines)-1])
}

// countUnpushedCommits returns how many commits are ahead of the remote.
func countUnpushedCommits(repoPath string) int {
	cmd := exec.Command("git", "rev-list", "--count", "@{u}..HEAD")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	var count int
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &count)
	return count
}
