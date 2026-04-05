package repofiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateIntegrity_AllPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)
	_ = os.WriteFile(filepath.Join(dir, ".ralphrc"), []byte("X=1\n"), 0644)

	if err := ValidateIntegrity(dir); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateIntegrity_AllMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	err := ValidateIntegrity(dir)
	if err == nil {
		t.Fatal("expected error for missing files")
	}
	for _, p := range RequiredPaths {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error should mention %q: %v", p, err)
		}
	}
}

func TestValidateIntegrity_PartialMissing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)
	// .ralphrc is missing

	err := ValidateIntegrity(dir)
	if err == nil {
		t.Fatal("expected error for missing .ralphrc")
	}
	if !strings.Contains(err.Error(), ".ralphrc") {
		t.Errorf("error should mention .ralphrc: %v", err)
	}
}

func TestOptimize_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	result, err := Optimize(dir, OptimizeOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if result.ProjectType != "unknown" {
		t.Errorf("ProjectType = %q, want unknown", result.ProjectType)
	}
	if len(result.Issues) == 0 {
		t.Error("expected issues for empty dir")
	}
}

func TestOptimize_FocusPlan(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	result, err := Optimize(dir, OptimizeOptions{Focus: "plan"})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	// Should only have plan-related issues
	for _, issue := range result.Issues {
		if issue.File == ".ralphrc" || issue.File == ".ralph/PROMPT.md" {
			// plan focus should not check prompt or config directly (only allowed_tools does .ralphrc)
			if issue.File == ".ralph/PROMPT.md" {
				t.Errorf("focus=plan should not report %s issues", issue.File)
			}
		}
	}
}

func TestOptimize_FocusPrompt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	result, err := Optimize(dir, OptimizeOptions{Focus: "prompt"})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	// Should have prompt issue but not plan
	for _, issue := range result.Issues {
		if issue.File == ".ralph/fix_plan.md" {
			t.Errorf("focus=prompt should not report fix_plan.md issues")
		}
	}
}

func TestOptimize_FullConfig(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644)

	rcContent := `PROJECT_NAME="test"
PROJECT_TYPE="go"
RALPH_SESSION_BUDGET=100
CB_NO_PROGRESS_THRESHOLD=4
CB_SAME_ERROR_THRESHOLD=5
CB_COOLDOWN_MINUTES=15
FAST_MODE_ENABLED=true
QG_GO_BUILD=true
QG_GO_VET=true
QG_GO_TEST=true
ALLOWED_TOOLS="Bash(go build *),Bash(go test *),Bash(go vet *)"
`
	_ = os.WriteFile(filepath.Join(dir, ".ralphrc"), []byte(rcContent), 0644)
	_ = os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)

	promptContent := `# Instructions
go project with RALPH_STATUS
Protected Files section: DO NOT MODIFY
fix_plan reference here
`
	_ = os.WriteFile(filepath.Join(dir, ".ralph", "PROMPT.md"), []byte(promptContent), 0644)

	planContent := `# Plan
roadmap reference
- [ ] task one
- [x] done task
`
	_ = os.WriteFile(filepath.Join(dir, ".ralph", "fix_plan.md"), []byte(planContent), 0644)

	result, err := Optimize(dir, OptimizeOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	for _, issue := range result.Issues {
		if issue.Severity == "error" {
			t.Errorf("unexpected error issue with full config: %s - %s", issue.File, issue.Issue)
		}
	}
}

func TestOptimize_MismatchedProjectType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0644)
	_ = os.WriteFile(filepath.Join(dir, ".ralphrc"), []byte("PROJECT_TYPE=\"node\"\n"), 0644)

	result, err := Optimize(dir, OptimizeOptions{Focus: "config"})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	found := false
	for _, issue := range result.Issues {
		if strings.Contains(issue.Issue, "doesn't match detected type") {
			found = true
		}
	}
	if !found {
		t.Error("expected project type mismatch warning")
	}
}

func TestReadKVFile_MissingFile(t *testing.T) {
	t.Parallel()
	values := readKVFile("/nonexistent/path/file")
	if len(values) != 0 {
		t.Errorf("expected empty map for missing file, got %d entries", len(values))
	}
}

func TestReadKVFile_EmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.rc")
	_ = os.WriteFile(path, []byte(""), 0644)

	values := readKVFile(path)
	if len(values) != 0 {
		t.Errorf("expected empty map for empty file, got %d entries", len(values))
	}
}

func TestReadKVFile_CommentsOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "comments.rc")
	_ = os.WriteFile(path, []byte("# comment 1\n# comment 2\n\n"), 0644)

	values := readKVFile(path)
	if len(values) != 0 {
		t.Errorf("expected empty map for comments-only file, got %d entries", len(values))
	}
}

func TestReadKVFile_NoEqualsLine(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.rc")
	_ = os.WriteFile(path, []byte("noequals\nKEY=val\n"), 0644)

	values := readKVFile(path)
	if len(values) != 1 {
		t.Errorf("expected 1 entry, got %d", len(values))
	}
	if values["KEY"] != "val" {
		t.Errorf("KEY = %q, want val", values["KEY"])
	}
}

func TestDetectProjectType_Unknown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got := detectProjectType(dir)
	if got != "unknown" {
		t.Errorf("detectProjectType(empty) = %q, want unknown", got)
	}
}

func TestDetectProjectType_AllTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		file string
		want string
	}{
		{"go.mod", "go"},
		{"package.json", "node"},
		{"Cargo.toml", "rust"},
		{"pyproject.toml", "python"},
		{"requirements.txt", "python"},
		{"pom.xml", "java"},
		{"build.gradle", "java"},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			dir := t.TempDir()
			_ = os.WriteFile(filepath.Join(dir, tt.file), []byte(""), 0644)
			got := detectProjectType(dir)
			if got != tt.want {
				t.Errorf("detectProjectType(%s) = %q, want %q", tt.file, got, tt.want)
			}
		})
	}
}

func TestReadClaudeMD_Exists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	content := "# My Project\nSome info.\n"
	_ = os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte(content), 0644)

	got := ReadClaudeMD(dir)
	if got != content {
		t.Errorf("ReadClaudeMD = %q, want %q", got, content)
	}
}

func TestReadClaudeMD_Missing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got := ReadClaudeMD(dir)
	if got != "" {
		t.Errorf("ReadClaudeMD(missing) = %q, want empty", got)
	}
}

func TestReadRoadmap_Exists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	lines := "line1\nline2\nline3\nline4\nline5\n"
	_ = os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte(lines), 0644)

	got := ReadRoadmap(dir, 3)
	if strings.Count(got, "\n") != 2 { // 3 lines joined = 2 newlines
		t.Errorf("ReadRoadmap(3) returned wrong line count: %q", got)
	}
	if !strings.HasPrefix(got, "line1") {
		t.Errorf("ReadRoadmap should start with first line: %q", got)
	}
}

func TestReadRoadmap_Missing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got := ReadRoadmap(dir, 10)
	if got != "" {
		t.Errorf("ReadRoadmap(missing) = %q, want empty", got)
	}
}

func TestReadRoadmap_MoreLinesThanFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "ROADMAP.md"), []byte("one\ntwo\n"), 0644)

	got := ReadRoadmap(dir, 100)
	if !strings.Contains(got, "one") || !strings.Contains(got, "two") {
		t.Errorf("ReadRoadmap should contain all lines: %q", got)
	}
}

func TestBuildCommands_AllTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		projectType string
		wantBuild   string
		wantTest    string
	}{
		{"go", "go build ./...", "go test ./..."},
		{"node", "npm run build", "npm test"},
		{"rust", "cargo build", "cargo test"},
		{"python", "python -m py_compile", "pytest"},
		{"unknown", "make build", "make test"},
	}
	for _, tt := range tests {
		t.Run(tt.projectType, func(t *testing.T) {
			b, tst, _ := buildCommands(tt.projectType)
			if b != tt.wantBuild {
				t.Errorf("build(%s) = %q, want %q", tt.projectType, b, tt.wantBuild)
			}
			if tst != tt.wantTest {
				t.Errorf("test(%s) = %q, want %q", tt.projectType, tst, tt.wantTest)
			}
		})
	}
}

func TestRunCommand_AllTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		projectType string
		want        string
	}{
		{"go", "go run ."},
		{"node", "npm start"},
		{"rust", "cargo run"},
		{"python", "python -m main"},
		{"unknown", "make run"},
	}
	for _, tt := range tests {
		t.Run(tt.projectType, func(t *testing.T) {
			got := runCommand(tt.projectType)
			if got != tt.want {
				t.Errorf("runCommand(%s) = %q, want %q", tt.projectType, got, tt.want)
			}
		})
	}
}

func TestQualityGateKeys(t *testing.T) {
	t.Parallel()
	tests := []struct {
		projectType string
		wantLen     int
	}{
		{"go", 3},
		{"node", 2},
		{"python", 2},
	}
	for _, tt := range tests {
		t.Run(tt.projectType, func(t *testing.T) {
			keys := qualityGateKeys(tt.projectType)
			if len(keys) != tt.wantLen {
				t.Errorf("qualityGateKeys(%s) len = %d, want %d", tt.projectType, len(keys), tt.wantLen)
			}
		})
	}
}

func TestRelPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		base string
		full string
		want string
	}{
		{"/a/b", "/a/b/c/d", "c/d"},
		{"/a/b", "/a/b", "."},
	}
	for _, tt := range tests {
		t.Run(tt.full, func(t *testing.T) {
			got := relPath(tt.base, tt.full)
			if got != tt.want {
				t.Errorf("relPath(%q, %q) = %q, want %q", tt.base, tt.full, got, tt.want)
			}
		})
	}
}

func TestOptimize_FixPlanAllDone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, ".ralph"), 0755)
	_ = os.WriteFile(filepath.Join(dir, ".ralph", "fix_plan.md"), []byte("- [x] done\n"), 0644)

	result, err := Optimize(dir, OptimizeOptions{Focus: "plan"})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	foundNoTasks := false
	foundAllDone := false
	for _, issue := range result.Issues {
		if strings.Contains(issue.Issue, "No open tasks") {
			foundNoTasks = true
		}
		if strings.Contains(issue.Issue, "All tasks completed") {
			foundAllDone = true
		}
	}
	if !foundNoTasks {
		t.Error("expected 'No open tasks' issue")
	}
	if !foundAllDone {
		t.Error("expected 'All tasks completed' issue")
	}
}

func TestScaffold_CustomProjectNameAndType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	result, err := Scaffold(dir, ScaffoldOptions{
		ProjectName: "myproject",
		ProjectType: "rust",
	})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if len(result.Created) != 6 {
		t.Errorf("expected 6 created, got %d", len(result.Created))
	}

	data, _ := os.ReadFile(filepath.Join(dir, ".ralphrc"))
	if !strings.Contains(string(data), "PROJECT_TYPE=\"rust\"") {
		t.Error("expected rust project type in .ralphrc")
	}
	if !strings.Contains(string(data), "PROJECT_NAME=\"myproject\"") {
		t.Error("expected myproject name in .ralphrc")
	}
	if !strings.Contains(string(data), "cargo build") {
		t.Error("expected cargo build in .ralphrc")
	}
}
