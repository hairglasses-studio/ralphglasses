package repofiles

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHealthScore_WellStructured(t *testing.T) {
	t.Parallel()
	dir := setupWellStructuredRepo(t)

	hs, err := ScoreRepo(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScoreRepo: %v", err)
	}

	// A well-structured repo should score reasonably high
	if hs.Overall < 0.5 {
		t.Errorf("well-structured repo overall = %.2f, want >= 0.5", hs.Overall)
	}

	// Should have all 6 dimensions
	expectedDims := []string{"config", "git", "tests", "docs", "deps", "structure"}
	for _, dim := range expectedDims {
		if _, ok := hs.Dimensions[dim]; !ok {
			t.Errorf("missing dimension %q", dim)
		}
	}

	// Config should score high with full .ralphrc
	if d := hs.Dimensions["config"]; d.Score < 0.6 {
		t.Errorf("config score = %.2f, want >= 0.6; details: %s", d.Score, d.Details)
	}

	// Docs should score 1.0 with README and CLAUDE.md
	if d := hs.Dimensions["docs"]; d.Score < 1.0 {
		t.Errorf("docs score = %.2f, want 1.0; details: %s", d.Score, d.Details)
	}

	// Structure should score high with standard Go layout
	if d := hs.Dimensions["structure"]; d.Score < 0.5 {
		t.Errorf("structure score = %.2f, want >= 0.5; details: %s", d.Score, d.Details)
	}

	// Timestamp should be set
	if hs.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestHealthScore_MissingRalphRC(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	// No .ralphrc at all
	hs, err := ScoreRepo(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScoreRepo: %v", err)
	}

	// Config dimension should be 0
	if d := hs.Dimensions["config"]; d.Score != 0.0 {
		t.Errorf("config score without .ralphrc = %.2f, want 0.0", d.Score)
	}

	// Should have an error-level issue about missing .ralphrc
	found := false
	for _, issue := range hs.Issues {
		if issue.Dimension == "config" && issue.Severity == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error-level config issue for missing .ralphrc")
	}
}

func TestHealthScore_DirtyGitState(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Create an untracked file to dirty the working tree
	_ = os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("dirty"), 0644)

	hs, err := ScoreRepo(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScoreRepo: %v", err)
	}

	// Git score should be penalized
	gitDim := hs.Dimensions["git"]
	if gitDim.Score >= 1.0 {
		t.Errorf("dirty git state should not score 1.0, got %.2f", gitDim.Score)
	}

	// Should have an issue about untracked files
	found := false
	for _, issue := range hs.Issues {
		if issue.Dimension == "git" && issue.Message != "" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected git issue for dirty working tree")
	}
}

func TestHealthScore_OverallIsWeightedAverage(t *testing.T) {
	t.Parallel()
	dir := setupWellStructuredRepo(t)

	hs, err := ScoreRepo(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScoreRepo: %v", err)
	}

	// Manually compute weighted average
	var expected float64
	for _, dim := range hs.Dimensions {
		expected += dim.Score * dim.Weight
	}

	if math.Abs(hs.Overall-expected) > 0.001 {
		t.Errorf("overall = %.4f, expected weighted average = %.4f", hs.Overall, expected)
	}
}

func TestHealthScore_IssuesHaveFixSuggestions(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Minimal repo to generate issues
	hs, err := ScoreRepo(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScoreRepo: %v", err)
	}

	if len(hs.Issues) == 0 {
		t.Fatal("expected at least some issues for a minimal repo")
	}

	for _, issue := range hs.Issues {
		if issue.Fix == "" {
			t.Errorf("issue %q (dimension=%s) has no fix suggestion", issue.Message, issue.Dimension)
		}
		if issue.Severity == "" {
			t.Errorf("issue %q has no severity", issue.Message)
		}
		if issue.Dimension == "" {
			t.Errorf("issue %q has no dimension", issue.Message)
		}
	}
}

func TestHealthScore_DimensionWeights(t *testing.T) {
	t.Parallel()

	// Verify weights sum to 1.0
	var total float64
	for _, w := range dimensionWeights {
		total += w
	}
	if math.Abs(total-1.0) > 0.001 {
		t.Errorf("dimension weights sum to %.3f, want 1.0", total)
	}

	// Verify all expected dimensions have weights
	expected := []string{"config", "git", "tests", "docs", "deps", "structure"}
	for _, dim := range expected {
		if _, ok := dimensionWeights[dim]; !ok {
			t.Errorf("missing weight for dimension %q", dim)
		}
	}
}

func TestHealthScore_SpecificWeightValues(t *testing.T) {
	t.Parallel()

	wantWeights := map[string]float64{
		"config":    0.3,
		"git":       0.2,
		"tests":     0.2,
		"docs":      0.1,
		"deps":      0.1,
		"structure": 0.1,
	}

	for dim, want := range wantWeights {
		got := dimensionWeights[dim]
		if math.Abs(got-want) > 0.001 {
			t.Errorf("weight[%s] = %.2f, want %.2f", dim, got, want)
		}
	}
}

func TestHealthScore_InvalidPath(t *testing.T) {
	t.Parallel()

	_, err := ScoreRepo(context.Background(), "/nonexistent/path/for/testing")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

func TestHealthScore_FileNotDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	_ = os.WriteFile(f, []byte("not a dir"), 0644)

	_, err := ScoreRepo(context.Background(), f)
	if err == nil {
		t.Error("expected error when path is a file, not directory")
	}
}

func TestHealthScore_IssuesNeverNil(t *testing.T) {
	t.Parallel()
	dir := setupWellStructuredRepo(t)

	hs, err := ScoreRepo(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScoreRepo: %v", err)
	}

	if hs.Issues == nil {
		t.Error("Issues should never be nil (should be empty slice)")
	}
}

func TestHealthScore_ScoresInRange(t *testing.T) {
	t.Parallel()
	dir := setupWellStructuredRepo(t)

	hs, err := ScoreRepo(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScoreRepo: %v", err)
	}

	if hs.Overall < 0.0 || hs.Overall > 1.0 {
		t.Errorf("overall score %.2f out of [0.0, 1.0] range", hs.Overall)
	}

	for name, dim := range hs.Dimensions {
		if dim.Score < 0.0 || dim.Score > 1.0 {
			t.Errorf("dimension %s score %.2f out of [0.0, 1.0] range", name, dim.Score)
		}
	}
}

func TestHealthScore_DocsWithReadmeOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Only README, no CLAUDE.md
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0644)

	hs, err := ScoreRepo(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScoreRepo: %v", err)
	}

	docsDim := hs.Dimensions["docs"]
	if docsDim.Score != 0.5 {
		t.Errorf("docs with README only = %.2f, want 0.5", docsDim.Score)
	}
}

func TestHealthScore_StructureNoGitignore(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Go project with cmd/ but no .gitignore
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n\ngo 1.22\n"), 0644)
	_ = os.MkdirAll(filepath.Join(dir, "cmd"), 0755)
	_ = os.MkdirAll(filepath.Join(dir, "internal"), 0755)

	hs, err := ScoreRepo(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScoreRepo: %v", err)
	}

	// Should have issue about missing .gitignore
	found := false
	for _, issue := range hs.Issues {
		if issue.Dimension == "structure" && issue.Message == "no .gitignore file" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected .gitignore issue")
	}

	structDim := hs.Dimensions["structure"]
	// 2/3 checks pass (cmd + internal exist, .gitignore missing)
	if math.Abs(structDim.Score-2.0/3.0) > 0.01 {
		t.Errorf("structure score = %.4f, want ~0.667", structDim.Score)
	}
}

func TestHealthScore_NotGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	hs, err := ScoreRepo(context.Background(), dir)
	if err != nil {
		t.Fatalf("ScoreRepo: %v", err)
	}

	gitDim := hs.Dimensions["git"]
	if gitDim.Score != 0.0 {
		t.Errorf("non-git repo git score = %.2f, want 0.0", gitDim.Score)
	}

	found := false
	for _, issue := range hs.Issues {
		if issue.Dimension == "git" && issue.Severity == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected error-level git issue for non-git repo")
	}
}

// --- test helpers ---

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init setup (%v): %v\n%s", args, err, out)
		}
	}

	// Create an initial commit so HEAD exists
	readme := filepath.Join(dir, ".gitkeep")
	_ = os.WriteFile(readme, []byte(""), 0644)
	addCmd := exec.Command("git", "add", ".")
	addCmd.Dir = dir
	if out, err := addCmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	commitCmd := exec.Command("git", "commit", "-m", "init")
	commitCmd.Dir = dir
	if out, err := commitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func setupWellStructuredRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Go project indicators
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testproject\n\ngo 1.22\n"), 0644)

	// Standard Go layout
	_ = os.MkdirAll(filepath.Join(dir, "cmd", "app"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "cmd", "app", "main.go"), []byte("package main\nfunc main() {}\n"), 0644)
	_ = os.MkdirAll(filepath.Join(dir, "internal", "core"), 0755)
	_ = os.WriteFile(filepath.Join(dir, "internal", "core", "core.go"), []byte("package core\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "internal", "core", "core_test.go"), []byte("package core\nimport \"testing\"\nfunc TestPlaceholder(t *testing.T) {}\n"), 0644)

	// Documentation
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test Project\nA test project.\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("# Claude Instructions\nTest project.\n"), 0644)

	// .gitignore
	_ = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.exe\n*.o\n"), 0644)

	// Full .ralphrc
	rcContent := `PROJECT_NAME="testproject"
PROJECT_TYPE="go"
PRIMARY_MODEL="sonnet"
RALPH_SESSION_BUDGET=100
CB_NO_PROGRESS_THRESHOLD=4
CB_SAME_ERROR_THRESHOLD=5
CB_COOLDOWN_MINUTES=15
ALLOWED_TOOLS="Bash(go build *),Bash(go test *),Bash(go vet *)"
QG_GO_BUILD=true
QG_GO_VET=true
QG_GO_TEST=true
FAST_MODE_ENABLED=true
`
	_ = os.WriteFile(filepath.Join(dir, ".ralphrc"), []byte(rcContent), 0644)

	// Commit everything
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = dir
	_ = cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "setup")
	cmd.Dir = dir
	_ = cmd.Run()

	return dir
}
