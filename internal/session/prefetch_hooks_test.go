package session

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- Mock hooks for testing ---

type mockHook struct {
	name      string
	shouldRun bool
	content   string
	delay     time.Duration
	err       error
}

func (m *mockHook) Name() string { return m.name }

func (m *mockHook) ShouldRun(_ context.Context, _ PrefetchSessionInfo) bool { return m.shouldRun }

func (m *mockHook) Fetch(ctx context.Context, _ PrefetchSessionInfo) (PrefetchResult, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return PrefetchResult{}, ctx.Err()
		}
	}
	if m.err != nil {
		return PrefetchResult{}, m.err
	}
	return PrefetchResult{
		Name:          m.name,
		Content:       m.content,
		TokenEstimate: estimateTokens(m.content),
	}, nil
}

// --- PrefetchRunner tests ---

func TestPrefetchRunner_MockHooks(t *testing.T) {
	t.Parallel()
	r := NewPrefetchRunner(5 * time.Second)
	r.Register(&mockHook{name: "hook_a", shouldRun: true, content: "data A"})
	r.Register(&mockHook{name: "hook_b", shouldRun: true, content: "data B"})

	results := r.RunAll(context.Background(), PrefetchSessionInfo{RepoPath: "/tmp/test"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Name != "hook_a" {
		t.Errorf("expected first result to be hook_a, got %q", results[0].Name)
	}
	if results[1].Name != "hook_b" {
		t.Errorf("expected second result to be hook_b, got %q", results[1].Name)
	}
}

func TestPrefetchRunner_ShouldRunFiltering(t *testing.T) {
	t.Parallel()
	r := NewPrefetchRunner(5 * time.Second)
	r.Register(&mockHook{name: "active", shouldRun: true, content: "ok"})
	r.Register(&mockHook{name: "skipped", shouldRun: false, content: "never"})
	r.Register(&mockHook{name: "also_active", shouldRun: true, content: "ok too"})

	results := r.RunAll(context.Background(), PrefetchSessionInfo{RepoPath: "/tmp"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results (1 skipped), got %d", len(results))
	}
	for _, res := range results {
		if res.Name == "skipped" {
			t.Error("skipped hook should not appear in results")
		}
	}
}

func TestPrefetchRunner_TimeoutEnforcement(t *testing.T) {
	t.Parallel()
	r := NewPrefetchRunner(50 * time.Millisecond)
	r.Register(&mockHook{name: "fast", shouldRun: true, content: "quick"})
	r.Register(&mockHook{name: "slow", shouldRun: true, content: "late", delay: 5 * time.Second})

	start := time.Now()
	results := r.RunAll(context.Background(), PrefetchSessionInfo{RepoPath: "/tmp"})
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("RunAll took %v, expected timeout to cap it", elapsed)
	}

	// Only the fast hook should succeed.
	if len(results) != 1 {
		t.Fatalf("expected 1 result (slow timed out), got %d", len(results))
	}
	if results[0].Name != "fast" {
		t.Errorf("expected fast hook, got %q", results[0].Name)
	}
}

func TestPrefetchRunner_ErrorHandling(t *testing.T) {
	t.Parallel()
	r := NewPrefetchRunner(5 * time.Second)
	r.Register(&mockHook{name: "good", shouldRun: true, content: "fine"})
	r.Register(&mockHook{name: "bad", shouldRun: true, err: errors.New("boom")})
	r.Register(&mockHook{name: "also_good", shouldRun: true, content: "also fine"})

	results := r.RunAll(context.Background(), PrefetchSessionInfo{RepoPath: "/tmp"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results (1 errored), got %d", len(results))
	}
	for _, res := range results {
		if res.Name == "bad" {
			t.Error("errored hook should not appear in results")
		}
	}
}

func TestPrefetchRunner_EmptyHooks(t *testing.T) {
	t.Parallel()
	r := NewPrefetchRunner(5 * time.Second)
	results := r.RunAll(context.Background(), PrefetchSessionInfo{})
	if results != nil {
		t.Errorf("expected nil results for empty hooks, got %v", results)
	}
}

func TestPrefetchRunner_RegisterRunAllLifecycle(t *testing.T) {
	t.Parallel()
	r := NewPrefetchRunner(5 * time.Second)

	// RunAll with no hooks.
	results := r.RunAll(context.Background(), PrefetchSessionInfo{})
	if results != nil {
		t.Error("expected nil for no hooks")
	}

	// Register one hook.
	r.Register(&mockHook{name: "first", shouldRun: true, content: "a"})
	results = r.RunAll(context.Background(), PrefetchSessionInfo{RepoPath: "/tmp"})
	if len(results) != 1 {
		t.Fatalf("expected 1 result after register, got %d", len(results))
	}

	// Register another.
	r.Register(&mockHook{name: "second", shouldRun: true, content: "b"})
	results = r.RunAll(context.Background(), PrefetchSessionInfo{RepoPath: "/tmp"})
	if len(results) != 2 {
		t.Fatalf("expected 2 results after second register, got %d", len(results))
	}
}

func TestPrefetchRunner_ConcurrentRunAllSafety(t *testing.T) {
	t.Parallel()
	r := NewPrefetchRunner(5 * time.Second)
	r.Register(&mockHook{name: "a", shouldRun: true, content: "data"})
	r.Register(&mockHook{name: "b", shouldRun: true, content: "data"})

	var wg sync.WaitGroup
	var errCount atomic.Int32

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results := r.RunAll(context.Background(), PrefetchSessionInfo{RepoPath: "/tmp"})
			if len(results) != 2 {
				errCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if errCount.Load() > 0 {
		t.Errorf("%d concurrent RunAll calls returned unexpected results", errCount.Load())
	}
}

func TestPrefetchRunner_Hooks(t *testing.T) {
	t.Parallel()
	r := NewPrefetchRunner(5 * time.Second)
	r.Register(&mockHook{name: "alpha", shouldRun: true})
	r.Register(&mockHook{name: "beta", shouldRun: true})

	names := r.Hooks()
	if len(names) != 2 {
		t.Fatalf("expected 2 hook names, got %d", len(names))
	}
	if names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("unexpected hook names: %v", names)
	}
}

func TestPrefetchRunner_DefaultTimeout(t *testing.T) {
	t.Parallel()
	r := NewPrefetchRunner(0)
	if r.timeout != 5*time.Second {
		t.Errorf("expected default 5s timeout, got %v", r.timeout)
	}
}

// --- FormatContext tests ---

func TestFormatContext_Output(t *testing.T) {
	t.Parallel()
	results := []PrefetchResult{
		{Name: "git_status", Content: "On branch main\n3 modified files", TokenEstimate: 150},
		{Name: "claude_md", Content: "# Project Name", TokenEstimate: 500},
	}
	formatted := FormatContext(results)

	if !strings.Contains(formatted, "<prefetched_context>") {
		t.Error("expected <prefetched_context> opening tag")
	}
	if !strings.Contains(formatted, "</prefetched_context>") {
		t.Error("expected </prefetched_context> closing tag")
	}
	if !strings.Contains(formatted, `name="git_status"`) {
		t.Error("expected git_status context block")
	}
	if !strings.Contains(formatted, `tokens="~150"`) {
		t.Error("expected token estimate ~150")
	}
	if !strings.Contains(formatted, `name="claude_md"`) {
		t.Error("expected claude_md context block")
	}
	if !strings.Contains(formatted, "# Project Name") {
		t.Error("expected claude_md content")
	}
}

func TestFormatContext_Empty(t *testing.T) {
	t.Parallel()
	if FormatContext(nil) != "" {
		t.Error("expected empty string for nil results")
	}
	if FormatContext([]PrefetchResult{}) != "" {
		t.Error("expected empty string for empty results")
	}
}

// --- Built-in hook tests (require filesystem/git) ---

func TestGitStatusHook_ShouldRun(t *testing.T) {
	t.Parallel()
	h := GitStatusHook{}
	if h.ShouldRun(context.Background(), PrefetchSessionInfo{}) {
		t.Error("should not run with empty RepoPath")
	}
	if !h.ShouldRun(context.Background(), PrefetchSessionInfo{RepoPath: "/tmp"}) {
		t.Error("should run with non-empty RepoPath")
	}
}

func TestGitStatusHook_Fetch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initTestGitRepo(t, dir)

	h := GitStatusHook{}
	res, err := h.Fetch(context.Background(), PrefetchSessionInfo{RepoPath: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Name != "git_status" {
		t.Errorf("expected name git_status, got %q", res.Name)
	}
	if !strings.Contains(res.Content, "initial") {
		t.Errorf("expected git log with 'initial', got %q", res.Content)
	}
	if res.TokenEstimate <= 0 {
		t.Error("expected positive token estimate")
	}
}

func TestClaudeMDHook_ShouldRun(t *testing.T) {
	t.Parallel()
	h := ClaudeMDHook{}

	// No repo path.
	if h.ShouldRun(context.Background(), PrefetchSessionInfo{}) {
		t.Error("should not run with empty RepoPath")
	}

	// No CLAUDE.md file.
	dir := t.TempDir()
	if h.ShouldRun(context.Background(), PrefetchSessionInfo{RepoPath: dir}) {
		t.Error("should not run without CLAUDE.md")
	}

	// With CLAUDE.md.
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Test"), 0644); err != nil {
		t.Fatal(err)
	}
	if !h.ShouldRun(context.Background(), PrefetchSessionInfo{RepoPath: dir}) {
		t.Error("should run when CLAUDE.md exists")
	}
}

func TestClaudeMDHook_Fetch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "# My Project\n\nBuild: go build ./...\n"
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	h := ClaudeMDHook{}
	res, err := h.Fetch(context.Background(), PrefetchSessionInfo{RepoPath: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Content != content {
		t.Errorf("expected CLAUDE.md content, got %q", res.Content)
	}
	if res.TokenEstimate <= 0 {
		t.Error("expected positive token estimate")
	}
}

func TestTestInventoryHook_ShouldRun(t *testing.T) {
	t.Parallel()
	h := TestInventoryHook{}
	if h.ShouldRun(context.Background(), PrefetchSessionInfo{}) {
		t.Error("should not run with empty RepoPath")
	}
	if !h.ShouldRun(context.Background(), PrefetchSessionInfo{RepoPath: "/tmp"}) {
		t.Error("should run with non-empty RepoPath")
	}
}

func TestTestInventoryHook_Fetch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Create some test files.
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "pkg", "util_test.go"), []byte("package pkg"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "pkg", "util.go"), []byte("package pkg"), 0644)

	h := TestInventoryHook{}
	res, err := h.Fetch(context.Background(), PrefetchSessionInfo{RepoPath: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Content, "2") {
		t.Errorf("expected 2 test files in content, got %q", res.Content)
	}
}

func TestDirStructureHook_ShouldRun(t *testing.T) {
	t.Parallel()
	h := DirStructureHook{}
	if h.ShouldRun(context.Background(), PrefetchSessionInfo{}) {
		t.Error("should not run with empty RepoPath")
	}
	if !h.ShouldRun(context.Background(), PrefetchSessionInfo{RepoPath: "/tmp"}) {
		t.Error("should run with non-empty RepoPath")
	}
}

func TestDirStructureHook_Fetch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "cmd"), 0755)
	_ = os.MkdirAll(filepath.Join(dir, "internal"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)

	h := DirStructureHook{}
	res, err := h.Fetch(context.Background(), PrefetchSessionInfo{RepoPath: dir})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Content, "cmd/") {
		t.Errorf("expected cmd/ in listing, got %q", res.Content)
	}
	if !strings.Contains(res.Content, "go.mod") {
		t.Errorf("expected go.mod in listing, got %q", res.Content)
	}
}

func TestDefaultPrefetchRunner(t *testing.T) {
	t.Parallel()
	r := DefaultPrefetchRunner()
	names := r.Hooks()
	if len(names) != 4 {
		t.Fatalf("expected 4 default hooks, got %d: %v", len(names), names)
	}
	expected := []string{"git_status", "claude_md", "test_inventory", "dir_structure"}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("hook[%d] = %q, want %q", i, names[i], want)
		}
	}
}

func TestEstimateTokens_Prefetch(t *testing.T) {
	t.Parallel()
	if estimateTokens("") != 0 {
		t.Error("empty string should be 0 tokens")
	}
	// 12 chars -> 3 tokens
	if got := estimateTokens("hello world!"); got != 3 {
		t.Errorf("expected 3 tokens for 12 chars, got %d", got)
	}
	// 13 chars -> 4 tokens (rounding up)
	if got := estimateTokens("hello world!!"); got != 4 {
		t.Errorf("expected 4 tokens for 13 chars, got %d", got)
	}
}

// initTestGitRepo creates a minimal git repo for testing.
func initTestGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"commit", "--allow-empty", "-m", "initial commit"},
	} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
			"GIT_CONFIG_NOSYSTEM=1",
			"GIT_CONFIG_GLOBAL=/dev/null",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}
