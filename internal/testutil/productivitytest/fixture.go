package productivitytest

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Fixture provides a git-backed repo plus isolated session state for
// productivity and pressure tests.
type Fixture struct {
	RepoPath string
	StateDir string
	DocsDir  string
	ScanRoot string
}

// NewFixture creates a repo with minimal ralph scaffolding and an initialized
// git history so self-improvement acceptance flows can operate normally.
func NewFixture(t testing.TB) Fixture {
	t.Helper()

	root := t.TempDir()
	repoPath := filepath.Join(root, "repo")
	stateDir := filepath.Join(root, "state")
	docsDir := filepath.Join(root, "docs")

	mustMkdirAll(t, filepath.Join(repoPath, ".ralph"))
	mustMkdirAll(t, filepath.Join(repoPath, "docs"))
	mustMkdirAll(t, stateDir)
	mustMkdirAll(t, docsDir)

	writeFile(t, filepath.Join(repoPath, "README.md"), "# productive repo\n")
	writeFile(t, filepath.Join(repoPath, ".ralphrc"), "PROJECT_NAME=\"productive\"\n")
	writeFile(t, filepath.Join(repoPath, "ROADMAP.md"), "# Roadmap\n\n- [ ] Improve docs output\n")
	writeFile(t, filepath.Join(repoPath, "go.mod"), "module example.com/productive\n\ngo 1.22\n")

	runGit(t, repoPath, "init")
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "initial")

	return Fixture{
		RepoPath: repoPath,
		StateDir: stateDir,
		DocsDir:  docsDir,
		ScanRoot: root,
	}
}

// NewManager constructs a session manager bound to the fixture state dir.
func (f Fixture) NewManager() *session.Manager {
	mgr := session.NewManager()
	mgr.SetStateDir(f.StateDir)
	return mgr
}

// Gateway is a controllable ResearchGateway for productivity tests.
type Gateway struct {
	mu              sync.Mutex
	entries         []*session.ResearchEntry
	dequeued        []string
	completed       []string
	abandoned       []string
	written         []string
	commits         int
	dedupConfidence float64
	dedupRecommend  string
	failWrite       bool
}

func NewGateway(entries ...*session.ResearchEntry) *Gateway {
	return &Gateway{
		entries:        entries,
		dedupRecommend: "proceed",
	}
}

func (g *Gateway) SetDedup(confidence float64, recommendation string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.dedupConfidence = confidence
	g.dedupRecommend = recommendation
}

func (g *Gateway) SetFailWrite(v bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failWrite = v
}

func (g *Gateway) ExpireStale(context.Context) (int, error) { return 0, nil }

func (g *Gateway) DequeueNext(_ context.Context, _ string, _ int) (*session.ResearchEntry, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.entries) == 0 {
		return nil, nil
	}
	entry := g.entries[0]
	g.entries = g.entries[1:]
	g.dequeued = append(g.dequeued, entry.Topic)
	return entry, nil
}

func (g *Gateway) DedupCheck(_ context.Context, _, _ string) (float64, string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.dedupConfidence, g.dedupRecommend, nil
}

func (g *Gateway) Complete(_ context.Context, topic, _ string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.completed = append(g.completed, topic)
	return nil
}

func (g *Gateway) Abandon(_ context.Context, topic, _, reason string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.abandoned = append(g.abandoned, topic+":"+reason)
	return nil
}

func (g *Gateway) WriteResearch(_ context.Context, _, title, _ string, _ []string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.failWrite {
		return fmt.Errorf("write failed")
	}
	g.written = append(g.written, title)
	return nil
}

func (g *Gateway) CommitAndPush(_ context.Context, _ string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.commits++
	return nil
}

func mustMkdirAll(t testing.TB, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func runGit(t testing.TB, repoPath string, args ...string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	cmd.Env = []string{
		"HOME=" + repoPath,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_EDITOR=true",
		"GIT_AUTHOR_NAME=Test User",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test User",
		"GIT_COMMITTER_EMAIL=test@example.com",
		"PATH=" + os.Getenv("PATH"),
	}
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
