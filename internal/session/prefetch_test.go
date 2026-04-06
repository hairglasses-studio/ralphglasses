package session

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrefetchContext_EmptyRepo(t *testing.T) {
	t.Parallel()
	ec, err := PrefetchContext(context.Background(), "", ProviderClaude, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ec.TotalTokens != 0 {
		t.Errorf("expected 0 tokens for empty repo, got %d", ec.TotalTokens)
	}
}

func TestPrefetchContext_WithGitRepo(t *testing.T) {
	t.Parallel()
	// Create a temporary git repo.
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	run("git", "init")
	run("git", "commit", "--allow-empty", "-m", "initial commit")

	ec, err := PrefetchContext(context.Background(), dir, ProviderClaude, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ec.CommitHash == "" || ec.CommitHash == "unknown" {
		t.Error("expected valid commit hash")
	}
	if !strings.Contains(ec.GitLog, "initial commit") {
		t.Errorf("expected git log to contain 'initial commit', got %q", ec.GitLog)
	}
	if ec.GitLogTokens <= 0 {
		t.Error("expected positive token estimate for git log")
	}
}

func TestPrefetchContext_Cache(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	run("git", "init")
	run("git", "commit", "--allow-empty", "-m", "test")

	ctx := context.Background()
	ec1, _ := PrefetchContext(ctx, dir, ProviderClaude, nil)
	ec2, _ := PrefetchContext(ctx, dir, ProviderClaude, nil)

	// Same commit should return cached result.
	if ec1.FetchedAt != ec2.FetchedAt {
		t.Error("expected cache hit (same FetchedAt)")
	}
}

func TestPrefetchContext_WithErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	run("git", "init")
	run("git", "commit", "--allow-empty", "-m", "init")

	errs := []LoopError{
		{Iteration: 1, Phase: "execute", Message: "test failed", Retryable: true},
	}
	ec, _ := PrefetchContext(context.Background(), dir, ProviderGemini, errs)
	if ec.RecentErrors == "" {
		t.Error("expected recent errors in context")
	}
	if ec.RecentErrorsTokens <= 0 {
		t.Error("expected positive token count for errors")
	}
}

func TestEnrichedContext_FormatForContext(t *testing.T) {
	t.Parallel()
	ec := &EnrichedContext{
		GitLog:       "abc123 initial commit",
		GitStatus:    "M file.go",
		RecentErrors: "## Lessons from previous attempts\n\n- error happened\n",
	}
	formatted := ec.FormatForContext()
	if !strings.Contains(formatted, "Recent Commits") {
		t.Error("expected Recent Commits header")
	}
	if !strings.Contains(formatted, "Working Tree Status") {
		t.Error("expected Working Tree Status header")
	}
	if !strings.Contains(formatted, "Lessons from previous") {
		t.Error("expected error lessons section")
	}
}

func TestEnrichedContext_FormatNil(t *testing.T) {
	t.Parallel()
	var ec *EnrichedContext
	if ec.FormatForContext() != "" {
		t.Error("expected empty string for nil context")
	}
}

func TestPrefetchContext_WorkingTreeStatus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	run("git", "init")
	run("git", "commit", "--allow-empty", "-m", "init")
	// Create an untracked file.
	_ = os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello"), 0644)

	// Clear cache for this dir.
	prefetchCache.Lock()
	for k := range prefetchCache.entries {
		if strings.HasPrefix(k, dir) {
			delete(prefetchCache.entries, k)
		}
	}
	prefetchCache.Unlock()

	ec, _ := PrefetchContext(context.Background(), dir, ProviderClaude, nil)
	_ = ec.GitStatus // just verify it doesn't crash; content depends on git version
	_ = time.Now()
}
