package session

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// looksLikeJSON
// ---------------------------------------------------------------------------

func TestLooksLikeJSON(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  bool
	}{
		{`{"key": "value"}`, true},
		{`[1, 2, 3]`, true},
		{`  {"key": "value"}`, true},   // leading whitespace
		{`  [1, 2, 3]`, true},          // leading whitespace
		{`hello world`, false},
		{``, false},
		{`   `, false},
		{`<xml>`, false},
		{`(parens)`, false},
		{"\n\t{\"a\":1}", true}, // newline + tab before brace
	}
	for _, tt := range tests {
		if got := looksLikeJSON(tt.input); got != tt.want {
			t.Errorf("looksLikeJSON(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// hasGitChanges (requires a real git repo)
// ---------------------------------------------------------------------------

func TestHasGitChanges_NoRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if hasGitChanges(dir) {
		t.Error("expected false for non-git directory")
	}
}

func TestHasGitChanges_CleanRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	if hasGitChanges(dir) {
		t.Error("expected false for clean repo with no uncommitted changes")
	}
}

func TestHasGitChanges_DirtyRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Create an uncommitted tracked file change
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("change"), 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "-C", dir, "add", "new.txt")
	cmd.Env = gitEnv(dir)
	if err := cmd.Run(); err != nil {
		t.Fatalf("git add: %v", err)
	}

	if !hasGitChanges(dir) {
		t.Error("expected true for repo with staged changes")
	}
}

// ---------------------------------------------------------------------------
// RestoreLevel (DecisionLog)
// ---------------------------------------------------------------------------

func TestDecisionLog_RestoreLevel(t *testing.T) {
	t.Parallel()
	dl := NewDecisionLog("", LevelObserve)

	if dl.Level() != LevelObserve {
		t.Errorf("initial level = %v, want LevelObserve", dl.Level())
	}

	dl.RestoreLevel(LevelFullAutonomy)
	if dl.Level() != LevelFullAutonomy {
		t.Errorf("after RestoreLevel(3) = %v, want LevelFullAutonomy", dl.Level())
	}

	dl.RestoreLevel(LevelAutoRecover)
	if dl.Level() != LevelAutoRecover {
		t.Errorf("after RestoreLevel(1) = %v, want LevelAutoRecover", dl.Level())
	}
}

func TestDecisionLog_RestoreLevel_DoesNotPersist(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dl := NewDecisionLog(dir, LevelObserve)

	// SetLevel persists to disk
	dl.SetLevel(LevelAutoOptimize)

	// RestoreLevel should NOT persist — only in-memory
	dl.RestoreLevel(LevelFullAutonomy)
	if dl.Level() != LevelFullAutonomy {
		t.Errorf("level = %v, want LevelFullAutonomy", dl.Level())
	}

	// Verify the persisted file still has the SetLevel value
	level, err := LoadAutonomyLevel(dir)
	if err != nil {
		t.Fatalf("LoadAutonomyLevel: %v", err)
	}
	if level != int(LevelAutoOptimize) {
		t.Errorf("persisted level = %d, want %d (RestoreLevel should not persist)", level, LevelAutoOptimize)
	}
}

// ---------------------------------------------------------------------------
// SaveAutonomyLevel / LoadAutonomyLevel
// ---------------------------------------------------------------------------

func TestSaveAndLoadAutonomyLevel(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := SaveAutonomyLevel(dir, 2); err != nil {
		t.Fatalf("SaveAutonomyLevel: %v", err)
	}

	level, err := LoadAutonomyLevel(dir)
	if err != nil {
		t.Fatalf("LoadAutonomyLevel: %v", err)
	}
	if level != 2 {
		t.Errorf("level = %d, want 2", level)
	}
}

func TestLoadAutonomyLevel_NotFound(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	level, err := LoadAutonomyLevel(dir)
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if level != 0 {
		t.Errorf("level = %d, want 0 (default)", level)
	}
}

func TestSaveAutonomyLevel_EmptyDir(t *testing.T) {
	t.Parallel()
	err := SaveAutonomyLevel("", 1)
	if err == nil {
		t.Error("expected error for empty dir")
	}
}

func TestLoadAutonomyLevel_CorruptFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "autonomy.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadAutonomyLevel(dir)
	if err == nil {
		t.Error("expected error for corrupt JSON")
	}
}

func TestSaveAutonomyLevel_Overwrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	if err := SaveAutonomyLevel(dir, 1); err != nil {
		t.Fatal(err)
	}
	if err := SaveAutonomyLevel(dir, 3); err != nil {
		t.Fatal(err)
	}

	level, err := LoadAutonomyLevel(dir)
	if err != nil {
		t.Fatal(err)
	}
	if level != 3 {
		t.Errorf("level = %d, want 3 after overwrite", level)
	}
}

func TestSaveAutonomyLevel_CreatesSubdir(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "nested", "path")

	if err := SaveAutonomyLevel(dir, 2); err != nil {
		t.Fatalf("SaveAutonomyLevel with nested dir: %v", err)
	}

	level, err := LoadAutonomyLevel(dir)
	if err != nil {
		t.Fatal(err)
	}
	if level != 2 {
		t.Errorf("level = %d, want 2", level)
	}
}

// ---------------------------------------------------------------------------
// FeedbackAnalyzer.IsEmpty / Reset
// ---------------------------------------------------------------------------

func TestFeedbackAnalyzer_IsEmpty(t *testing.T) {
	t.Parallel()
	fa := NewFeedbackAnalyzer("", 5)

	if !fa.IsEmpty() {
		t.Error("expected IsEmpty() = true for fresh analyzer")
	}

	fa.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "add tests", SpentUSD: 0.1, TurnCount: 5, ExitReason: "completed"},
	})

	if fa.IsEmpty() {
		t.Error("expected IsEmpty() = false after Ingest")
	}
}

func TestFeedbackAnalyzer_Reset(t *testing.T) {
	t.Parallel()
	fa := NewFeedbackAnalyzer("", 5)

	fa.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 0.5, TurnCount: 10, ExitReason: "completed"},
	})

	if fa.IsEmpty() {
		t.Fatal("expected non-empty after ingest")
	}

	fa.Reset()

	if !fa.IsEmpty() {
		t.Error("expected IsEmpty() = true after Reset()")
	}

	profiles := fa.AllPromptProfiles()
	if len(profiles) != 0 {
		t.Errorf("expected 0 prompt profiles after Reset, got %d", len(profiles))
	}
}

func TestFeedbackAnalyzer_ResetThenIngest(t *testing.T) {
	t.Parallel()
	fa := NewFeedbackAnalyzer("", 1)

	fa.Ingest([]JournalEntry{
		{Provider: "claude", TaskFocus: "fix bug", SpentUSD: 0.5, TurnCount: 10, ExitReason: "completed"},
	})
	fa.Reset()
	fa.Ingest([]JournalEntry{
		{Provider: "gemini", TaskFocus: "add feature", SpentUSD: 0.3, TurnCount: 5, ExitReason: "completed"},
	})

	if fa.IsEmpty() {
		t.Error("expected non-empty after re-ingest")
	}
}

// ---------------------------------------------------------------------------
// ClassifyTask (exported wrapper)
// ---------------------------------------------------------------------------

func TestClassifyTask_Exported(t *testing.T) {
	t.Parallel()
	tests := []struct {
		focus string
		want  string
	}{
		{"fix broken login", "bug_fix"},
		{"add unit tests", "test"},
		{"implement new search feature", "feature"},
		{"random unrelated prompt", "general"},
		{"", "general"},
	}
	for _, tt := range tests {
		got := ClassifyTask(tt.focus)
		if got != tt.want {
			t.Errorf("ClassifyTask(%q) = %q, want %q", tt.focus, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// PruneLoopRunsFiltered
// ---------------------------------------------------------------------------

func writeLoopRunFileWithRepo(t *testing.T, dir, id, status, repoName string, updatedAt time.Time) {
	t.Helper()
	run := loopRunPruneView{
		ID:        id,
		RepoName:  repoName,
		Status:    status,
		CreatedAt: updatedAt.Add(-time.Hour),
		UpdatedAt: updatedAt,
	}
	data, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestPruneLoopRunsFiltered_RepoFilter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	old := time.Now().Add(-96 * time.Hour)
	writeLoopRunFileWithRepo(t, dir, "repo-a-1", "pending", "repo-a", old)
	writeLoopRunFileWithRepo(t, dir, "repo-a-2", "pending", "repo-a", old)
	writeLoopRunFileWithRepo(t, dir, "repo-b-1", "pending", "repo-b", old)

	pruned, err := PruneLoopRunsFiltered(dir, 72*time.Hour, []string{"pending"}, "repo-a", false)
	if err != nil {
		t.Fatalf("PruneLoopRunsFiltered: %v", err)
	}
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2", pruned)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("remaining files = %d, want 1", len(entries))
	}
}

func TestPruneLoopRunsFiltered_EmptyRepoFilter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	old := time.Now().Add(-96 * time.Hour)
	writeLoopRunFileWithRepo(t, dir, "any-1", "pending", "repo-x", old)
	writeLoopRunFileWithRepo(t, dir, "any-2", "pending", "repo-y", old)

	pruned, err := PruneLoopRunsFiltered(dir, 72*time.Hour, []string{"pending"}, "", false)
	if err != nil {
		t.Fatalf("PruneLoopRunsFiltered: %v", err)
	}
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2 (empty filter matches all)", pruned)
	}
}

func TestPruneLoopRunsFiltered_DryRun(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	old := time.Now().Add(-96 * time.Hour)
	writeLoopRunFileWithRepo(t, dir, "dry-1", "pending", "repo-a", old)

	pruned, err := PruneLoopRunsFiltered(dir, 72*time.Hour, []string{"pending"}, "repo-a", true)
	if err != nil {
		t.Fatalf("PruneLoopRunsFiltered: %v", err)
	}
	if pruned != 1 {
		t.Errorf("pruned = %d, want 1", pruned)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("dry-run should not delete files, got %d remaining", len(entries))
	}
}

func TestPruneLoopRunsFiltered_EmptyDir(t *testing.T) {
	t.Parallel()
	pruned, err := PruneLoopRunsFiltered("", 72*time.Hour, []string{"pending"}, "repo", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0", pruned)
	}
}

func TestPruneLoopRunsFiltered_NonexistentDir(t *testing.T) {
	t.Parallel()
	pruned, err := PruneLoopRunsFiltered("/nonexistent/path", 72*time.Hour, []string{"pending"}, "repo", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0", pruned)
	}
}

func TestPruneLoopRunsFiltered_CaseInsensitiveRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	old := time.Now().Add(-96 * time.Hour)
	writeLoopRunFileWithRepo(t, dir, "mixed-1", "pending", "MyRepo", old)

	pruned, err := PruneLoopRunsFiltered(dir, 72*time.Hour, []string{"pending"}, "myrepo", false)
	if err != nil {
		t.Fatalf("PruneLoopRunsFiltered: %v", err)
	}
	if pruned != 1 {
		t.Errorf("pruned = %d, want 1 (case-insensitive match)", pruned)
	}
}

func TestPruneLoopRunsFiltered_StatusFilter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	old := time.Now().Add(-96 * time.Hour)
	writeLoopRunFileWithRepo(t, dir, "p1", "pending", "repo", old)
	writeLoopRunFileWithRepo(t, dir, "f1", "failed", "repo", old)
	writeLoopRunFileWithRepo(t, dir, "r1", "running", "repo", old)

	pruned, err := PruneLoopRunsFiltered(dir, 72*time.Hour, []string{"pending", "failed"}, "repo", false)
	if err != nil {
		t.Fatalf("PruneLoopRunsFiltered: %v", err)
	}
	if pruned != 2 {
		t.Errorf("pruned = %d, want 2", pruned)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("remaining = %d, want 1 (running survives)", len(entries))
	}
}

func TestPruneLoopRunsFiltered_RecentNotPruned(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	recent := time.Now().Add(-1 * time.Hour)
	writeLoopRunFileWithRepo(t, dir, "recent-1", "pending", "repo", recent)

	pruned, err := PruneLoopRunsFiltered(dir, 72*time.Hour, []string{"pending"}, "repo", false)
	if err != nil {
		t.Fatalf("PruneLoopRunsFiltered: %v", err)
	}
	if pruned != 0 {
		t.Errorf("pruned = %d, want 0 (too recent)", pruned)
	}
}
