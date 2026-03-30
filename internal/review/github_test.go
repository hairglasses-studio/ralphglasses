package review

import (
	"os/exec"
	"strings"
	"testing"
)

func TestNewGitHubReviewer(t *testing.T) {
	r := NewGitHubReviewer("org/repo")
	if r.Repo != "org/repo" {
		t.Errorf("Repo = %q, want org/repo", r.Repo)
	}
	if r.DryRun {
		t.Error("DryRun should default to false")
	}
	if r.execCommand == nil {
		t.Error("execCommand should default to exec.Command")
	}
}

func TestNewGitHubReviewer_EmptyRepo(t *testing.T) {
	r := NewGitHubReviewer("")
	if r.Repo != "" {
		t.Errorf("Repo = %q, want empty", r.Repo)
	}
}

func TestGitHubReviewer_PostReview_EmptyFindings(t *testing.T) {
	r := NewGitHubReviewer("org/repo")
	r.DryRun = true

	err := r.PostReview(1, &ReviewResult{}, "sha123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Commands) != 0 {
		t.Errorf("Commands = %d, want 0", len(r.Commands))
	}
}

func TestGitHubReviewer_PostReview_DryRun_WithRepo(t *testing.T) {
	r := NewGitHubReviewer("hairglasses-studio/ralphglasses")
	r.DryRun = true

	result := &ReviewResult{
		FilesCount: 1,
		Summary:    "test summary",
		Findings: []Finding{
			{
				CriterionID: "SEC001",
				Name:        "hardcoded-secret",
				Category:    CategorySecurity,
				Severity:    SeverityError,
				File:        "main.go",
				Line:        42,
				Message:     "Possible hardcoded secret.",
				DiffLine:    `password = "hunter2"`,
			},
		},
	}

	err := r.PostReview(123, result, "abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 summary comment + 1 inline comment.
	if len(r.Commands) != 2 {
		t.Fatalf("Commands = %d, want 2", len(r.Commands))
	}

	// Summary comment should use "pr comment".
	sumCmd := strings.Join(r.Commands[0], " ")
	if !strings.Contains(sumCmd, "pr comment") {
		t.Errorf("first command should be 'pr comment': %s", sumCmd)
	}
	if !strings.Contains(sumCmd, "--repo") {
		t.Errorf("first command should contain --repo: %s", sumCmd)
	}

	// Inline comment should use "api".
	inlineCmd := strings.Join(r.Commands[1], " ")
	if !strings.Contains(inlineCmd, "api") {
		t.Errorf("second command should use 'api': %s", inlineCmd)
	}
	if !strings.Contains(inlineCmd, "hairglasses-studio/ralphglasses") {
		t.Errorf("second command should contain repo in API path: %s", inlineCmd)
	}
}

func TestGitHubReviewer_PostReview_DryRun_NoRepo(t *testing.T) {
	r := NewGitHubReviewer("")
	r.DryRun = true

	result := &ReviewResult{
		FilesCount: 1,
		Summary:    "test",
		Findings: []Finding{
			{
				CriterionID: "COR003",
				Name:        "panic-in-library",
				Category:    CategoryCorrectness,
				Severity:    SeverityError,
				File:        "lib.go",
				Line:        10,
				Message:     "panic in library",
			},
		},
	}

	err := r.PostReview(5, result, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(r.Commands) != 2 {
		t.Fatalf("Commands = %d, want 2", len(r.Commands))
	}

	// Without repo, pr comment should not have --repo flag.
	sumCmd := strings.Join(r.Commands[0], " ")
	if strings.Contains(sumCmd, "--repo") {
		t.Errorf("command should not contain --repo when repo is empty: %s", sumCmd)
	}

	// Without repo, API path should use {owner}/{repo} placeholder.
	inlineCmd := strings.Join(r.Commands[1], " ")
	if !strings.Contains(inlineCmd, "{owner}/{repo}") {
		t.Errorf("API path should use placeholder when repo is empty: %s", inlineCmd)
	}
}

func TestGitHubReviewer_PostReview_SkipsFindingsWithoutFileOrLine(t *testing.T) {
	r := NewGitHubReviewer("org/repo")
	r.DryRun = true

	result := &ReviewResult{
		FilesCount: 1,
		Summary:    "test",
		Findings: []Finding{
			{CriterionID: "A", File: "", Line: 0, Message: "no file or line"},
			{CriterionID: "B", File: "a.go", Line: 0, Message: "no line"},
			{CriterionID: "C", File: "", Line: 5, Message: "no file"},
		},
	}

	err := r.PostReview(1, result, "sha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only 1 summary comment, no inline comments (all skipped).
	if len(r.Commands) != 1 {
		t.Errorf("Commands = %d, want 1 (summary only)", len(r.Commands))
	}
}

func TestGitHubReviewer_PostReview_MultipleFindings(t *testing.T) {
	r := NewGitHubReviewer("org/repo")
	r.DryRun = true

	result := &ReviewResult{
		FilesCount: 2,
		Summary:    "multiple",
		Findings: []Finding{
			{CriterionID: "A", File: "a.go", Line: 1, Message: "msg1"},
			{CriterionID: "B", File: "b.go", Line: 2, Message: "msg2"},
			{CriterionID: "C", File: "c.go", Line: 3, Message: "msg3"},
		},
	}

	err := r.PostReview(10, result, "sha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1 summary + 3 inline.
	if len(r.Commands) != 4 {
		t.Errorf("Commands = %d, want 4", len(r.Commands))
	}
}

func TestGitHubReviewer_FormatSummary_NoFindings(t *testing.T) {
	r := NewGitHubReviewer("")
	result := &ReviewResult{
		FilesCount: 1,
		Summary:    "No issues found in 1 file(s).",
	}

	summary := r.formatSummary(result)
	if !strings.Contains(summary, "Automated Code Review") {
		t.Error("summary should contain header")
	}
	// No table when no findings.
	if strings.Contains(summary, "| Severity") {
		t.Error("summary should not contain table with no findings")
	}
}

func TestGitHubReviewer_FormatSummary_WithFindings(t *testing.T) {
	r := NewGitHubReviewer("")
	result := &ReviewResult{
		FilesCount: 1,
		Summary:    "1 finding(s)",
		Findings: []Finding{
			{CriterionID: "SEC001", Severity: SeverityError, File: "a.go", Line: 1, Message: "secret found"},
		},
	}

	summary := r.formatSummary(result)
	if !strings.Contains(summary, "SEC001") {
		t.Error("summary table should contain criterion ID")
	}
	if !strings.Contains(summary, "| error |") {
		t.Error("summary table should contain severity")
	}
	if !strings.Contains(summary, "`a.go`") {
		t.Error("summary table should contain file name")
	}
}

func TestGitHubReviewer_FormatFinding(t *testing.T) {
	r := NewGitHubReviewer("")

	tests := []struct {
		name     string
		finding  Finding
		wantIcon string
		wantID   string
	}{
		{
			name:     "error severity",
			finding:  Finding{CriterionID: "SEC001", Name: "hardcoded-secret", Severity: SeverityError, Message: "secret"},
			wantIcon: ":x:",
			wantID:   "SEC001",
		},
		{
			name:     "warning severity",
			finding:  Finding{CriterionID: "COR001", Name: "unchecked-error", Severity: SeverityWarning, Message: "error discarded"},
			wantIcon: ":warning:",
			wantID:   "COR001",
		},
		{
			name:     "info severity",
			finding:  Finding{CriterionID: "STY001", Name: "long-function", Severity: SeverityInfo, Message: "too long"},
			wantIcon: ":info:",
			wantID:   "STY001",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.formatFinding(tt.finding)
			if !strings.Contains(got, tt.wantIcon) {
				t.Errorf("formatFinding missing icon %q in: %s", tt.wantIcon, got)
			}
			if !strings.Contains(got, tt.wantID) {
				t.Errorf("formatFinding missing ID %q in: %s", tt.wantID, got)
			}
			if !strings.Contains(got, tt.finding.Name) {
				t.Errorf("formatFinding missing name %q in: %s", tt.finding.Name, got)
			}
			if !strings.Contains(got, tt.finding.Message) {
				t.Errorf("formatFinding missing message %q in: %s", tt.finding.Message, got)
			}
		})
	}
}

func TestGitHubReviewer_RunGH_DryRun(t *testing.T) {
	r := NewGitHubReviewer("org/repo")
	r.DryRun = true

	err := r.runGH("pr", "comment", "1", "--body", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Commands) != 1 {
		t.Fatalf("Commands = %d, want 1", len(r.Commands))
	}
	if r.Commands[0][0] != "gh" {
		t.Errorf("Commands[0][0] = %q, want gh", r.Commands[0][0])
	}
}

func TestGitHubReviewer_RunGHWithStdin_DryRun(t *testing.T) {
	r := NewGitHubReviewer("org/repo")
	r.DryRun = true

	err := r.runGHWithStdin(`{"body":"hello"}`, "api", "repos/org/repo/pulls/1/comments", "--method", "POST", "--input", "-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(r.Commands) != 1 {
		t.Fatalf("Commands = %d, want 1", len(r.Commands))
	}
}

func TestGitHubReviewer_RunGH_ExecFailure(t *testing.T) {
	r := NewGitHubReviewer("org/repo")
	r.DryRun = false
	// Override execCommand to return a command that will fail.
	r.execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false")
	}

	err := r.runGH("pr", "comment", "1")
	if err == nil {
		t.Error("expected error from failed command")
	}
}

func TestGitHubReviewer_RunGHWithStdin_ExecFailure(t *testing.T) {
	r := NewGitHubReviewer("org/repo")
	r.DryRun = false
	r.execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false")
	}

	err := r.runGHWithStdin(`{"body":"x"}`, "api", "endpoint")
	if err == nil {
		t.Error("expected error from failed command")
	}
}

func TestGitHubReviewer_PostReview_SummaryCommentError(t *testing.T) {
	r := NewGitHubReviewer("org/repo")
	r.DryRun = false
	r.execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("false")
	}

	result := &ReviewResult{
		Findings: []Finding{
			{CriterionID: "A", File: "a.go", Line: 1, Message: "msg"},
		},
		Summary: "test",
	}

	err := r.PostReview(1, result, "sha")
	if err == nil {
		t.Error("expected error when summary comment fails")
	}
	if !strings.Contains(err.Error(), "posting summary comment") {
		t.Errorf("error should mention summary comment: %v", err)
	}
}

func TestGitHubReviewer_PostReview_InlineCommentError(t *testing.T) {
	r := NewGitHubReviewer("org/repo")
	r.DryRun = false

	callCount := 0
	r.execCommand = func(name string, args ...string) *exec.Cmd {
		callCount++
		if callCount == 1 {
			// Summary comment succeeds.
			return exec.Command("true")
		}
		// Inline comments fail.
		return exec.Command("false")
	}

	result := &ReviewResult{
		Findings: []Finding{
			{CriterionID: "A", File: "a.go", Line: 1, Message: "msg"},
		},
		Summary: "test",
	}

	err := r.PostReview(1, result, "sha")
	if err == nil {
		t.Error("expected error when inline comment fails")
	}
	if !strings.Contains(err.Error(), "failed to post") {
		t.Errorf("error should mention failed comments: %v", err)
	}
}

func TestGitHubReviewer_RunGH_ExecSuccess(t *testing.T) {
	r := NewGitHubReviewer("org/repo")
	r.DryRun = false
	r.execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("true")
	}

	err := r.runGH("pr", "list")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGitHubReviewer_RunGHWithStdin_ExecSuccess(t *testing.T) {
	r := NewGitHubReviewer("org/repo")
	r.DryRun = false
	r.execCommand = func(name string, args ...string) *exec.Cmd {
		return exec.Command("true")
	}

	err := r.runGHWithStdin(`{"body":"x"}`, "api", "endpoint")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
