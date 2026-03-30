package review

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// GitHubReviewer posts review comments to GitHub PRs via the `gh` CLI.
type GitHubReviewer struct {
	// Repo is the owner/repo string (e.g. "hairglasses-studio/ralphglasses").
	// If empty, gh uses the current repo context.
	Repo string

	// DryRun when true prevents actual gh calls and records commands instead.
	DryRun bool

	// Commands records the gh commands that would be executed in DryRun mode.
	Commands [][]string

	// execCommand is the function used to run commands. Defaults to exec.Command.
	// Overridable for testing.
	execCommand func(name string, args ...string) *exec.Cmd
}

// NewGitHubReviewer creates a reviewer for the given repository.
func NewGitHubReviewer(repo string) *GitHubReviewer {
	return &GitHubReviewer{
		Repo:        repo,
		execCommand: exec.Command,
	}
}

// ghComment is the JSON body for a PR review comment via gh api.
type ghComment struct {
	Body     string `json:"body"`
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`
	Side     string `json:"side,omitempty"`
	CommitID string `json:"commit_id,omitempty"`
}

// PostReview posts inline review comments for each finding to a PR.
// It groups findings into a single review submission when possible,
// falling back to individual comments on error.
func (g *GitHubReviewer) PostReview(prNumber int, result *ReviewResult, commitSHA string) error {
	if len(result.Findings) == 0 {
		return nil
	}

	// Post summary comment first.
	if err := g.postComment(prNumber, g.formatSummary(result)); err != nil {
		return fmt.Errorf("posting summary comment: %w", err)
	}

	// Post inline comments for each finding.
	var errs []string
	for _, f := range result.Findings {
		if f.File == "" || f.Line == 0 {
			continue
		}
		if err := g.postInlineComment(prNumber, f, commitSHA); err != nil {
			errs = append(errs, fmt.Sprintf("%s:%d: %v", f.File, f.Line, err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to post %d comment(s): %s", len(errs), strings.Join(errs, "; "))
	}
	return nil
}

// postComment posts a top-level PR comment.
func (g *GitHubReviewer) postComment(prNumber int, body string) error {
	args := []string{"pr", "comment", fmt.Sprintf("%d", prNumber), "--body", body}
	if g.Repo != "" {
		args = append(args, "--repo", g.Repo)
	}
	return g.runGH(args...)
}

// postInlineComment posts a review comment on a specific file and line.
func (g *GitHubReviewer) postInlineComment(prNumber int, f Finding, commitSHA string) error {
	comment := ghComment{
		Body: g.formatFinding(f),
		Path: f.File,
		Line: f.Line,
		Side: "RIGHT",
	}
	if commitSHA != "" {
		comment.CommitID = commitSHA
	}

	bodyJSON, err := json.Marshal(comment)
	if err != nil {
		return fmt.Errorf("marshaling comment: %w", err)
	}

	endpoint := fmt.Sprintf("repos/{owner}/{repo}/pulls/%d/comments", prNumber)
	args := []string{"api", endpoint, "--method", "POST", "--input", "-"}
	if g.Repo != "" {
		args = []string{"api", fmt.Sprintf("repos/%s/pulls/%d/comments", g.Repo, prNumber),
			"--method", "POST", "--input", "-"}
	}

	return g.runGHWithStdin(string(bodyJSON), args...)
}

// formatSummary produces a markdown summary of the review.
func (g *GitHubReviewer) formatSummary(result *ReviewResult) string {
	var b strings.Builder
	b.WriteString("## Automated Code Review\n\n")
	b.WriteString(fmt.Sprintf("**%s**\n\n", result.Summary))

	if len(result.Findings) > 0 {
		b.WriteString("| Severity | File | Line | Rule | Message |\n")
		b.WriteString("|----------|------|------|------|--------|\n")
		for _, f := range result.Findings {
			b.WriteString(fmt.Sprintf("| %s | `%s` | %d | %s | %s |\n",
				f.Severity, f.File, f.Line, f.CriterionID, f.Message))
		}
	}
	return b.String()
}

// formatFinding formats a single finding as a markdown comment body.
func (g *GitHubReviewer) formatFinding(f Finding) string {
	icon := "info"
	switch f.Severity {
	case SeverityError:
		icon = "x"
	case SeverityWarning:
		icon = "warning"
	}
	return fmt.Sprintf("**[%s]** :%s: `%s` — %s", f.CriterionID, icon, f.Name, f.Message)
}

// runGH executes a gh command.
func (g *GitHubReviewer) runGH(args ...string) error {
	if g.DryRun {
		g.Commands = append(g.Commands, append([]string{"gh"}, args...))
		return nil
	}

	cmd := g.execCommand("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gh %s: %w: %s", strings.Join(args[:min(len(args), 3)], " "), err, out)
	}
	return nil
}

// runGHWithStdin executes a gh command with stdin data.
func (g *GitHubReviewer) runGHWithStdin(stdin string, args ...string) error {
	if g.DryRun {
		g.Commands = append(g.Commands, append([]string{"gh"}, args...))
		return nil
	}

	cmd := g.execCommand("gh", args...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gh %s: %w: %s", strings.Join(args[:min(len(args), 3)], " "), err, out)
	}
	return nil
}
