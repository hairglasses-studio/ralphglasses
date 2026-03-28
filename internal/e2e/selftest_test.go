package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultSelfTestConfig(t *testing.T) {
	cfg := DefaultSelfTestConfig("/tmp/repo")

	if cfg.RepoPath != "/tmp/repo" {
		t.Errorf("RepoPath = %q, want /tmp/repo", cfg.RepoPath)
	}
	if cfg.MaxIterations != 3 {
		t.Errorf("MaxIterations = %d, want 3", cfg.MaxIterations)
	}
	if cfg.BudgetUSD != 5.0 {
		t.Errorf("BudgetUSD = %f, want 5.0", cfg.BudgetUSD)
	}
	if !cfg.UseSnapshot {
		t.Error("UseSnapshot = false, want true")
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := SelfTestConfig{RepoPath: "/tmp/repo"}
	cfg.applyDefaults()

	if cfg.MaxIterations != 3 {
		t.Errorf("MaxIterations = %d, want 3", cfg.MaxIterations)
	}
	if cfg.BudgetUSD != 5.0 {
		t.Errorf("BudgetUSD = %f, want 5.0", cfg.BudgetUSD)
	}
}

func TestApplyDefaultsPreservesExplicitValues(t *testing.T) {
	cfg := SelfTestConfig{
		RepoPath:      "/tmp/repo",
		MaxIterations: 10,
		BudgetUSD:     20.0,
	}
	cfg.applyDefaults()

	if cfg.MaxIterations != 10 {
		t.Errorf("MaxIterations = %d, want 10", cfg.MaxIterations)
	}
	if cfg.BudgetUSD != 20.0 {
		t.Errorf("BudgetUSD = %f, want 20.0", cfg.BudgetUSD)
	}
}

func TestPrepareNoSnapshot(t *testing.T) {
	// Create a temp dir with a fake binary so we can test Prepare
	// without actually compiling.
	tmpDir := t.TempDir()

	// Create a fake binary file for hashing.
	fakeBin := filepath.Join(tmpDir, "ralphglasses")
	if err := os.WriteFile(fakeBin, []byte("fake-binary-content"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Init a git repo so tagging works.
	initGitRepo(t, tmpDir)

	cfg := SelfTestConfig{
		RepoPath:    tmpDir,
		BinaryPath:  fakeBin,
		UseSnapshot: false,
	}

	ctx := context.Background()
	runner, err := Prepare(ctx, cfg)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}

	if runner.BinaryPath != fakeBin {
		t.Errorf("BinaryPath = %q, want %q", runner.BinaryPath, fakeBin)
	}
	if runner.BinaryHash == "" {
		t.Error("BinaryHash is empty, want a SHA256 hex string")
	}
	if len(runner.BinaryHash) != 64 {
		t.Errorf("BinaryHash length = %d, want 64 hex chars", len(runner.BinaryHash))
	}
	if runner.PreparedAt.IsZero() {
		t.Error("PreparedAt is zero")
	}
	if runner.Config.MaxIterations != 3 {
		t.Errorf("Config.MaxIterations = %d, want 3 (default)", runner.Config.MaxIterations)
	}
	if runner.Config.BudgetUSD != 5.0 {
		t.Errorf("Config.BudgetUSD = %f, want 5.0 (default)", runner.Config.BudgetUSD)
	}
}

func TestPrepareEmptyRepoPath(t *testing.T) {
	cfg := SelfTestConfig{}
	ctx := context.Background()
	_, err := Prepare(ctx, cfg)
	if err == nil {
		t.Fatal("Prepare() with empty RepoPath should return error")
	}
}

func TestPrepareNoSnapshotFallbackBinaryPath(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	cfg := SelfTestConfig{
		RepoPath:    tmpDir,
		UseSnapshot: false,
		// No BinaryPath set — should fall back to <RepoPath>/ralphglasses
	}

	ctx := context.Background()
	runner, err := Prepare(ctx, cfg)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}

	want := filepath.Join(tmpDir, "ralphglasses")
	if runner.BinaryPath != want {
		t.Errorf("BinaryPath = %q, want %q", runner.BinaryPath, want)
	}
	// Hash will be empty since the file doesn't exist, but that's OK
	// for UseSnapshot=false.
	if runner.BinaryHash != "" {
		t.Errorf("BinaryHash = %q, want empty (file doesn't exist)", runner.BinaryHash)
	}
}

func TestSelfTestRunnerFieldInit(t *testing.T) {
	now := time.Now()
	runner := &SelfTestRunner{
		Config: SelfTestConfig{
			RepoPath:      "/tmp/repo",
			MaxIterations: 5,
			BudgetUSD:     10.0,
			AllowedPaths:  []string{"/tmp/repo/internal"},
			ForbiddenPaths: []string{"/tmp/repo/.git"},
		},
		BinaryHash: "abc123",
		BinaryPath: "/tmp/repo/bin/test",
		PreparedAt: now,
		GitTag:     "selftest-12345",
	}

	if runner.Config.RepoPath != "/tmp/repo" {
		t.Errorf("Config.RepoPath = %q", runner.Config.RepoPath)
	}
	if runner.Config.MaxIterations != 5 {
		t.Errorf("Config.MaxIterations = %d", runner.Config.MaxIterations)
	}
	if runner.BinaryHash != "abc123" {
		t.Errorf("BinaryHash = %q", runner.BinaryHash)
	}
	if runner.BinaryPath != "/tmp/repo/bin/test" {
		t.Errorf("BinaryPath = %q", runner.BinaryPath)
	}
	if runner.PreparedAt != now {
		t.Errorf("PreparedAt mismatch")
	}
	if runner.GitTag != "selftest-12345" {
		t.Errorf("GitTag = %q", runner.GitTag)
	}
	if len(runner.Config.AllowedPaths) != 1 {
		t.Errorf("AllowedPaths len = %d", len(runner.Config.AllowedPaths))
	}
	if len(runner.Config.ForbiddenPaths) != 1 {
		t.Errorf("ForbiddenPaths len = %d", len(runner.Config.ForbiddenPaths))
	}
}

func TestSelfTestResultFields(t *testing.T) {
	result := &SelfTestResult{
		Iterations:   3,
		TotalCostUSD: 2.50,
		Duration:     5 * time.Second,
		BinaryHash:   "deadbeef",
		Observations: []map[string]any{
			{"iteration": 0, "exit_code": 0},
			{"iteration": 1, "exit_code": 0},
			{"iteration": 2, "exit_code": 1, "error": "budget exceeded"},
		},
	}

	if result.Iterations != 3 {
		t.Errorf("Iterations = %d", result.Iterations)
	}
	if result.TotalCostUSD != 2.50 {
		t.Errorf("TotalCostUSD = %f", result.TotalCostUSD)
	}
	if len(result.Observations) != 3 {
		t.Errorf("Observations len = %d", len(result.Observations))
	}
}

func TestJoinPathList(t *testing.T) {
	tests := []struct {
		input []string
		want  string
	}{
		{[]string{"/a", "/b", "/c"}, "/a:/b:/c"},
		{[]string{"/single"}, "/single"},
	}
	for _, tt := range tests {
		got := joinPathList(tt.input)
		if got != tt.want {
			t.Errorf("joinPathList(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestHashFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "testfile")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	hash, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile error: %v", err)
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}

	// Verify determinism.
	hash2, err := hashFile(path)
	if err != nil {
		t.Fatalf("hashFile error: %v", err)
	}
	if hash != hash2 {
		t.Error("hashFile not deterministic")
	}
}

func TestHashFileNotFound(t *testing.T) {
	_, err := hashFile("/nonexistent/file")
	if err == nil {
		t.Error("hashFile should error for nonexistent file")
	}
}

func TestDryRunReturnsZeroIterations(t *testing.T) {
	tmpDir := t.TempDir()
	initGitRepo(t, tmpDir)

	// Create a fake binary so Prepare succeeds with UseSnapshot=false.
	fakeBin := filepath.Join(tmpDir, "ralphglasses")
	if err := os.WriteFile(fakeBin, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := SelfTestConfig{
		RepoPath:    tmpDir,
		BinaryPath:  fakeBin,
		UseSnapshot: false,
		DryRun:      true,
	}

	ctx := context.Background()
	runner, err := Prepare(ctx, cfg)
	if err != nil {
		t.Fatalf("Prepare() error: %v", err)
	}

	result, err := runner.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if result.Iterations != 0 {
		t.Errorf("Iterations = %d, want 0 for dry run", result.Iterations)
	}
	if len(result.Observations) != 1 {
		t.Fatalf("Observations len = %d, want 1", len(result.Observations))
	}
	if result.Observations[0]["status"] != "dry_run" {
		t.Errorf("status = %v, want dry_run", result.Observations[0]["status"])
	}
}

func TestBudgetOverrideApplied(t *testing.T) {
	cfg := SelfTestConfig{
		RepoPath:       "/tmp/repo",
		BudgetUSD:      5.0,
		BudgetOverride: 1.5,
	}
	cfg.applyDefaults()

	if cfg.BudgetUSD != 1.5 {
		t.Errorf("BudgetUSD = %f, want 1.5 (from BudgetOverride)", cfg.BudgetUSD)
	}
}

func TestBudgetOverrideZeroNoEffect(t *testing.T) {
	cfg := SelfTestConfig{
		RepoPath:       "/tmp/repo",
		BudgetUSD:      10.0,
		BudgetOverride: 0,
	}
	cfg.applyDefaults()

	if cfg.BudgetUSD != 10.0 {
		t.Errorf("BudgetUSD = %f, want 10.0 (BudgetOverride=0 should not change it)", cfg.BudgetUSD)
	}
}

// initGitRepo creates a minimal git repo with one commit for tagging tests.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	env := []string{
		"HOME=" + dir,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		"PATH=" + os.Getenv("PATH"),
	}
	cmds := [][]string{
		{"git", "init"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
}
