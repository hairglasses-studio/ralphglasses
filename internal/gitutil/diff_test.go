package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initTestRepo creates a temp git repo with one committed file.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command %v failed: %v\n%s", args, err, out)
		}
	}

	// Create and commit a file
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "hello.txt"},
		{"git", "commit", "-m", "init"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git command %v failed: %v\n%s", args, err, out)
		}
	}

	return dir
}

func TestGitDiffPaths(t *testing.T) {
	dir := initTestRepo(t)

	// Modify the file
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	paths, err := GitDiffPaths(dir)
	if err != nil {
		t.Fatalf("GitDiffPaths: %v", err)
	}
	if len(paths) != 1 || paths[0] != "hello.txt" {
		t.Fatalf("expected [hello.txt], got %v", paths)
	}
}

func TestGitDiffPaths_Clean(t *testing.T) {
	dir := initTestRepo(t)

	paths, err := GitDiffPaths(dir)
	if err != nil {
		t.Fatalf("GitDiffPaths: %v", err)
	}
	if len(paths) != 0 {
		t.Fatalf("expected empty diff, got %v", paths)
	}
}

func TestGitDiffStats(t *testing.T) {
	dir := initTestRepo(t)

	// Modify the file
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello world\nextra line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, added, removed := GitDiffStats(dir)
	if files != 1 {
		t.Errorf("expected 1 file changed, got %d", files)
	}
	if added == 0 && removed == 0 {
		t.Error("expected non-zero added or removed lines")
	}
}

func TestGitDiffStats_Clean(t *testing.T) {
	dir := initTestRepo(t)

	files, added, removed := GitDiffStats(dir)
	if files != 0 || added != 0 || removed != 0 {
		t.Errorf("expected all zeros for clean repo, got files=%d added=%d removed=%d", files, added, removed)
	}
}

func TestParseLines(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want int
	}{
		{"empty", []byte(""), 0},
		{"whitespace", []byte("  \n  "), 0},
		{"single", []byte("hello.txt\n"), 1},
		{"multiple", []byte("a.go\nb.go\nc.go\n"), 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseLines(tt.in)
			if len(got) != tt.want {
				t.Errorf("ParseLines(%q) = %d lines, want %d", tt.in, len(got), tt.want)
			}
		})
	}
}
