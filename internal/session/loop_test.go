package session

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultLoopProfile(t *testing.T) {
	profile := DefaultLoopProfile()
	if profile.PlannerProvider != ProviderCodex {
		t.Fatalf("planner provider = %q", profile.PlannerProvider)
	}
	if profile.PlannerModel != "o1-pro" {
		t.Fatalf("planner model = %q", profile.PlannerModel)
	}
	if profile.WorkerModel != "gpt-5.4-xhigh" {
		t.Fatalf("worker model = %q", profile.WorkerModel)
	}
	if len(profile.VerifyCommands) != 1 || profile.VerifyCommands[0] != "./scripts/dev/ci.sh" {
		t.Fatalf("verify commands = %#v", profile.VerifyCommands)
	}
}

func TestLoopStepSuccess(t *testing.T) {
	repoPath := setupLoopRepo(t)

	m := NewManager()
	m.SetStateDir(t.TempDir())
	m.SetHooksForTesting(
		func(_ context.Context, opts LaunchOptions) (*Session, error) {
			sess := &Session{
				ID:         sanitizeLoopName(opts.SessionName),
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   filepath.Base(opts.RepoPath),
				Prompt:     opts.Prompt,
				Model:      opts.Model,
				Status:     StatusCompleted,
				OutputCh:   make(chan string, 1),
				LaunchedAt: time.Now(),
			}
			if opts.Model == "o1-pro" {
				sess.LastOutput = `{"title":"Add README note","prompt":"Append a loop-generated marker comment to README.md."}`
				sess.OutputHistory = []string{sess.LastOutput}
			} else {
				sess.LastOutput = "worker complete"
				sess.OutputHistory = []string{"worker complete"}
			}
			return sess, nil
		},
		func(_ context.Context, sess *Session) error {
			sess.Lock()
			sess.Status = StatusCompleted
			now := time.Now()
			sess.EndedAt = &now
			sess.Unlock()
			return nil
		},
	)

	run, err := m.StartLoop(context.Background(), repoPath, LoopProfile{
		VerifyCommands: []string{"test -f README.md"},
	})
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	if err := m.StepLoop(context.Background(), run.ID); err != nil {
		t.Fatalf("StepLoop: %v", err)
	}

	run, ok := m.GetLoop(run.ID)
	if !ok {
		t.Fatal("loop not found after step")
	}

	run.Lock()
	defer run.Unlock()

	if run.Status != "idle" {
		t.Fatalf("run status = %q", run.Status)
	}
	if len(run.Iterations) != 1 {
		t.Fatalf("iterations = %d", len(run.Iterations))
	}
	iter := run.Iterations[0]
	if iter.Status != "idle" {
		t.Fatalf("iteration status = %q", iter.Status)
	}
	if iter.WorktreePath == "" {
		t.Fatal("expected worktree path")
	}
	if _, err := os.Stat(iter.WorktreePath); err != nil {
		t.Fatalf("stat worktree: %v", err)
	}
	if iter.Task.Title != "Add README note" {
		t.Fatalf("task title = %q", iter.Task.Title)
	}
	if len(iter.Verification) != 1 || iter.Verification[0].Status != "completed" {
		t.Fatalf("verification = %#v", iter.Verification)
	}
}

func TestLoopStepVerificationFailure(t *testing.T) {
	repoPath := setupLoopRepo(t)

	m := NewManager()
	m.SetStateDir(t.TempDir())
	m.SetHooksForTesting(
		func(_ context.Context, opts LaunchOptions) (*Session, error) {
			sess := &Session{
				ID:         sanitizeLoopName(opts.SessionName),
				Provider:   opts.Provider,
				RepoPath:   opts.RepoPath,
				RepoName:   filepath.Base(opts.RepoPath),
				Prompt:     opts.Prompt,
				Model:      opts.Model,
				Status:     StatusCompleted,
				OutputCh:   make(chan string, 1),
				LaunchedAt: time.Now(),
			}
			if opts.Model == "o1-pro" {
				sess.LastOutput = `{"title":"Break verify","prompt":"Do some work that will fail verification."}`
			}
			return sess, nil
		},
		func(_ context.Context, sess *Session) error {
			sess.Lock()
			sess.Status = StatusCompleted
			now := time.Now()
			sess.EndedAt = &now
			sess.Unlock()
			return nil
		},
	)

	run, err := m.StartLoop(context.Background(), repoPath, LoopProfile{
		VerifyCommands: []string{"exit 7"},
	})
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}

	err = m.StepLoop(context.Background(), run.ID)
	if err == nil {
		t.Fatal("expected verification failure")
	}

	run, ok := m.GetLoop(run.ID)
	if !ok {
		t.Fatal("loop not found after failure")
	}

	run.Lock()
	defer run.Unlock()

	if run.Status != "failed" {
		t.Fatalf("run status = %q", run.Status)
	}
	if run.LastError == "" {
		t.Fatal("expected last error")
	}
	if len(run.Iterations) != 1 {
		t.Fatalf("iterations = %d", len(run.Iterations))
	}
	if run.Iterations[0].Status != "failed" {
		t.Fatalf("iteration status = %q", run.Iterations[0].Status)
	}
}

func TestStopLoopPreventsFurtherSteps(t *testing.T) {
	repoPath := setupLoopRepo(t)

	m := NewManager()
	m.SetStateDir(t.TempDir())
	run, err := m.StartLoop(context.Background(), repoPath, LoopProfile{})
	if err != nil {
		t.Fatalf("StartLoop: %v", err)
	}
	if err := m.StopLoop(run.ID); err != nil {
		t.Fatalf("StopLoop: %v", err)
	}
	if err := m.StepLoop(context.Background(), run.ID); err == nil {
		t.Fatal("expected stopped loop to refuse stepping")
	}
}

func setupLoopRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	repoPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("# loop repo\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repoPath, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".ralphrc"), []byte("PROJECT_NAME=\"test\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, "ROADMAP.md"), []byte("# Loop Roadmap\n\n## Phase 1\n\n### 1.1\n- [ ] 1.1.1 — Add README note\n"), 0644); err != nil {
		t.Fatal(err)
	}

	runGitLoop(t, repoPath, "init")
	runGitLoop(t, repoPath, "config", "user.email", "test@example.com")
	runGitLoop(t, repoPath, "config", "user.name", "Test User")
	runGitLoop(t, repoPath, "config", "commit.gpgsign", "false")
	runGitLoop(t, repoPath, "add", ".")
	runGitLoop(t, repoPath, "commit", "-m", "initial")
	return repoPath
}

func runGitLoop(t *testing.T, repoPath string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repoPath}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
