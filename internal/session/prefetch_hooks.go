package session

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PrefetchHook defines a deterministic context-gathering step that runs
// before the LLM invocation. Hooks pre-fetch data the model will almost
// certainly need, eliminating wasted tool-call round trips.
// This implements 12-Factor Agent principle #13: Pre-Fetch Context.
type PrefetchHook interface {
	// Name returns a short, unique identifier for this hook (e.g. "git_status").
	Name() string
	// ShouldRun decides whether this hook applies given the session info.
	ShouldRun(ctx context.Context, info PrefetchSessionInfo) bool
	// Fetch gathers context and returns it. Implementations must respect
	// ctx cancellation for timeout enforcement.
	Fetch(ctx context.Context, info PrefetchSessionInfo) (PrefetchResult, error)
}

// PrefetchSessionInfo carries launch-time metadata that hooks use to decide
// what context to fetch.
type PrefetchSessionInfo struct {
	RepoPath string
	Branch   string
	Provider string
	Model    string
	Prompt   string
}

// PrefetchResult holds the output of a single hook execution.
type PrefetchResult struct {
	Name          string `json:"name"`
	Content       string `json:"content"`
	TokenEstimate int    `json:"token_estimate"`
}

// PrefetchRunner manages a set of hooks and executes them concurrently
// with per-hook timeout enforcement.
type PrefetchRunner struct {
	mu      sync.RWMutex
	hooks   []PrefetchHook
	timeout time.Duration
}

// NewPrefetchRunner creates a runner with the given per-hook timeout.
// If timeout is zero, a default of 5 seconds is used.
func NewPrefetchRunner(timeout time.Duration) *PrefetchRunner {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &PrefetchRunner{
		timeout: timeout,
	}
}

// Register adds a hook to the runner. It is safe to call before RunAll
// but must not be called concurrently with RunAll.
func (r *PrefetchRunner) Register(hook PrefetchHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks = append(r.hooks, hook)
}

// Hooks returns a snapshot of all registered hook names.
func (r *PrefetchRunner) Hooks() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, len(r.hooks))
	for i, h := range r.hooks {
		names[i] = h.Name()
	}
	return names
}

// RunAll executes all matching hooks concurrently. Hooks that return
// false from ShouldRun are skipped. Hooks that error or exceed the
// per-hook timeout are silently skipped — one failing hook must never
// block the others.
func (r *PrefetchRunner) RunAll(ctx context.Context, info PrefetchSessionInfo) []PrefetchResult {
	r.mu.RLock()
	hooks := make([]PrefetchHook, len(r.hooks))
	copy(hooks, r.hooks)
	r.mu.RUnlock()

	if len(hooks) == 0 {
		return nil
	}

	type indexed struct {
		idx    int
		result PrefetchResult
	}

	ch := make(chan indexed, len(hooks))
	var wg sync.WaitGroup

	for i, h := range hooks {
		if !h.ShouldRun(ctx, info) {
			continue
		}
		wg.Add(1)
		go func(idx int, hook PrefetchHook) {
			defer wg.Done()
			hookCtx, cancel := context.WithTimeout(ctx, r.timeout)
			defer cancel()
			res, err := hook.Fetch(hookCtx, info)
			if err != nil {
				return
			}
			ch <- indexed{idx: idx, result: res}
		}(i, h)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	// Collect results preserving registration order via index.
	collected := make(map[int]PrefetchResult)
	for item := range ch {
		collected[item.idx] = item.result
	}

	results := make([]PrefetchResult, 0, len(collected))
	for i := range hooks {
		if res, ok := collected[i]; ok {
			results = append(results, res)
		}
	}
	return results
}

// FormatContext renders a slice of PrefetchResults as an XML context block
// suitable for prepending to an LLM prompt.
func FormatContext(results []PrefetchResult) string {
	if len(results) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("<prefetched_context>\n")
	for _, r := range results {
		fmt.Fprintf(&b, "  <context name=%q tokens=\"~%d\">\n", r.Name, r.TokenEstimate)
		// Indent content lines for readability.
		for _, line := range strings.Split(strings.TrimRight(r.Content, "\n"), "\n") {
			b.WriteString("    ")
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteString("  </context>\n")
	}
	b.WriteString("</prefetched_context>")
	return b.String()
}

// estimateTokens provides a rough 4-chars-per-token estimate.
func estimateTokens(s string) int {
	if len(s) == 0 {
		return 0
	}
	return (len(s) + 3) / 4
}

// --- Built-in hooks ---

// GitStatusHook fetches `git status --porcelain` and `git log --oneline -5`.
type GitStatusHook struct{}

func (GitStatusHook) Name() string { return "git_status" }

func (GitStatusHook) ShouldRun(_ context.Context, info PrefetchSessionInfo) bool {
	return info.RepoPath != ""
}

func (GitStatusHook) Fetch(ctx context.Context, info PrefetchSessionInfo) (PrefetchResult, error) {
	var parts []string

	if out, err := prefetchGitCmd(ctx, info.RepoPath, "status", "--porcelain"); err == nil && len(out) > 0 {
		parts = append(parts, "Status:\n"+out)
	}

	if out, err := prefetchGitCmd(ctx, info.RepoPath, "log", "--oneline", "-5"); err == nil && len(out) > 0 {
		parts = append(parts, "Recent commits:\n"+out)
	}

	content := strings.Join(parts, "\n\n")
	return PrefetchResult{
		Name:          "git_status",
		Content:       content,
		TokenEstimate: estimateTokens(content),
	}, nil
}

// ClaudeMDHook reads CLAUDE.md from the repo root if it exists.
type ClaudeMDHook struct{}

func (ClaudeMDHook) Name() string { return "claude_md" }

func (ClaudeMDHook) ShouldRun(_ context.Context, info PrefetchSessionInfo) bool {
	if info.RepoPath == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(info.RepoPath, "CLAUDE.md"))
	return err == nil
}

func (ClaudeMDHook) Fetch(_ context.Context, info PrefetchSessionInfo) (PrefetchResult, error) {
	data, err := os.ReadFile(filepath.Join(info.RepoPath, "CLAUDE.md"))
	if err != nil {
		return PrefetchResult{}, err
	}
	content := string(data)
	return PrefetchResult{
		Name:          "claude_md",
		Content:       content,
		TokenEstimate: estimateTokens(content),
	}, nil
}

// TestInventoryHook counts test files in the repository.
// For Go projects it counts *_test.go files; this is a fast filesystem walk.
type TestInventoryHook struct{}

func (TestInventoryHook) Name() string { return "test_inventory" }

func (TestInventoryHook) ShouldRun(_ context.Context, info PrefetchSessionInfo) bool {
	return info.RepoPath != ""
}

func (TestInventoryHook) Fetch(_ context.Context, info PrefetchSessionInfo) (PrefetchResult, error) {
	count := 0
	_ = filepath.WalkDir(info.RepoPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "vendor" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), "_test.go") {
			count++
		}
		return nil
	})
	content := fmt.Sprintf("Test files found: %d (*_test.go)", count)
	return PrefetchResult{
		Name:          "test_inventory",
		Content:       content,
		TokenEstimate: estimateTokens(content),
	}, nil
}

// DirStructureHook captures the top-level directory listing.
type DirStructureHook struct{}

func (DirStructureHook) Name() string { return "dir_structure" }

func (DirStructureHook) ShouldRun(_ context.Context, info PrefetchSessionInfo) bool {
	return info.RepoPath != ""
}

func (DirStructureHook) Fetch(_ context.Context, info PrefetchSessionInfo) (PrefetchResult, error) {
	entries, err := os.ReadDir(info.RepoPath)
	if err != nil {
		return PrefetchResult{}, err
	}
	var lines []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		lines = append(lines, name)
	}
	content := strings.Join(lines, "\n")
	return PrefetchResult{
		Name:          "dir_structure",
		Content:       content,
		TokenEstimate: estimateTokens(content),
	}, nil
}

// prefetchGitCmd runs a git command in the given repo and returns trimmed stdout.
func prefetchGitCmd(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// DefaultPrefetchRunner creates a PrefetchRunner pre-loaded with all
// built-in hooks.
func DefaultPrefetchRunner() *PrefetchRunner {
	r := NewPrefetchRunner(5 * time.Second)
	r.Register(GitStatusHook{})
	r.Register(ClaudeMDHook{})
	r.Register(TestInventoryHook{})
	r.Register(DirStructureHook{})
	return r
}
