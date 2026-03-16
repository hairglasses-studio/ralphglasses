package discovery

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

// makeRepo creates a fake repo directory with optional .ralph/ dir and .ralphrc file.
func makeRepo(t *testing.T, root, name string, withRalphDir, withRC bool) string {
	t.Helper()
	repoPath := filepath.Join(root, name)
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		t.Fatal(err)
	}
	if withRalphDir {
		ralphDir := filepath.Join(repoPath, ".ralph")
		if err := os.MkdirAll(ralphDir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if withRC {
		rc := filepath.Join(repoPath, ".ralphrc")
		if err := os.WriteFile(rc, []byte("MODEL=sonnet\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return repoPath
}

func TestScan_FindsReposWithRalphDir(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "repo-a", true, false)
	makeRepo(t, root, "repo-b", true, false)

	repos, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(repos))
	}

	// Verify sorted by name
	if repos[0].Name != "repo-a" {
		t.Errorf("first repo = %q, want %q", repos[0].Name, "repo-a")
	}
	if repos[1].Name != "repo-b" {
		t.Errorf("second repo = %q, want %q", repos[1].Name, "repo-b")
	}
}

func TestScan_FindsReposWithRCOnly(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "rc-only", false, true)

	repos, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if !repos[0].HasRC {
		t.Error("expected HasRC to be true")
	}
	if repos[0].HasRalph {
		t.Error("expected HasRalph to be false")
	}
}

func TestScan_SkipsNonRalphDirs(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "ralph-repo", true, true)

	// Create a regular directory without .ralph or .ralphrc
	normalDir := filepath.Join(root, "normal-project")
	if err := os.MkdirAll(normalDir, 0755); err != nil {
		t.Fatal(err)
	}

	repos, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo (skipping normal), got %d", len(repos))
	}
	if repos[0].Name != "ralph-repo" {
		t.Errorf("repo name = %q, want %q", repos[0].Name, "ralph-repo")
	}
}

func TestScan_EmptyDirectory(t *testing.T) {
	root := t.TempDir()

	repos, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestScan_NonexistentRoot(t *testing.T) {
	_, err := Scan("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent root")
	}
}

func TestScan_SkipsFiles(t *testing.T) {
	root := t.TempDir()

	// Create a file (not directory) at root level
	if err := os.WriteFile(filepath.Join(root, "readme.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	makeRepo(t, root, "real-repo", true, false)

	repos, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo (skipping file), got %d", len(repos))
	}
}

func TestScan_SortedAlphabetically(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "zebra", true, false)
	makeRepo(t, root, "alpha", true, false)
	makeRepo(t, root, "middle", true, false)

	repos, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(repos) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(repos))
	}

	names := []string{repos[0].Name, repos[1].Name, repos[2].Name}
	expected := []string{"alpha", "middle", "zebra"}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("repos[%d].Name = %q, want %q", i, names[i], want)
		}
	}
}

func TestScan_RefreshesStatus(t *testing.T) {
	root := t.TempDir()
	repoPath := makeRepo(t, root, "with-status", true, true)

	// Write a status.json
	status := model.LoopStatus{
		LoopCount: 42,
		Status:    "running",
	}
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoPath, ".ralph", "status.json"), data, 0644); err != nil {
		t.Fatal(err)
	}

	repos, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}

	r := repos[0]
	if r.Status == nil {
		t.Fatal("Status should not be nil after scan with status.json")
	}
	if r.Status.LoopCount != 42 {
		t.Errorf("LoopCount = %d, want 42", r.Status.LoopCount)
	}
	if r.Config == nil {
		t.Fatal("Config should not be nil for repo with .ralphrc")
	}
}

func TestScan_HasRalphAndHasRC(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "both", true, true)
	makeRepo(t, root, "dir-only", true, false)
	makeRepo(t, root, "rc-only", false, true)

	repos, err := Scan(root)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(repos) != 3 {
		t.Fatalf("expected 3 repos, got %d", len(repos))
	}

	// Find each by name
	byName := make(map[string]*model.Repo)
	for _, r := range repos {
		byName[r.Name] = r
	}

	if r := byName["both"]; !r.HasRalph || !r.HasRC {
		t.Errorf("both: HasRalph=%v HasRC=%v, want true/true", r.HasRalph, r.HasRC)
	}
	if r := byName["dir-only"]; !r.HasRalph || r.HasRC {
		t.Errorf("dir-only: HasRalph=%v HasRC=%v, want true/false", r.HasRalph, r.HasRC)
	}
	if r := byName["rc-only"]; r.HasRalph || !r.HasRC {
		t.Errorf("rc-only: HasRalph=%v HasRC=%v, want false/true", r.HasRalph, r.HasRC)
	}
}
