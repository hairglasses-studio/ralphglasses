package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestScan_PermissionDeniedSubdir(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}
	t.Parallel()

	dir := t.TempDir()
	repoDir := filepath.Join(dir, "restricted")
	if err := os.MkdirAll(filepath.Join(repoDir, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}
	// Make the repo dir unreadable so os.ReadDir inside RefreshRepo may fail.
	// The scanner itself only reads one level deep, so this tests graceful
	// handling of errors during RefreshRepo (which logs but does not fail).
	statusDir := filepath.Join(repoDir, ".ralph", "status")
	if err := os.MkdirAll(statusDir, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(statusDir, 0755) //nolint:errcheck

	repos, err := Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should still discover the repo even if refresh had issues.
	if len(repos) != 1 {
		t.Errorf("expected 1 repo despite permission issues, got %d", len(repos))
	}
}

func TestScan_SymlinkLoop(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	linkPath := filepath.Join(dir, "looplink")
	// Create a symlink pointing back to parent — should not hang.
	if err := os.Symlink(dir, linkPath); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	// Scan walks one level deep, so the symlink is treated as a directory
	// entry. It has no .ralph/ or .ralphrc, so it is skipped.
	repos, err := Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error with symlink loop: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos with symlink loop, got %d", len(repos))
	}
}

func TestScan_SymlinkToValidRepo(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create a real repo directory outside the scan root.
	realRepo := filepath.Join(dir, "real")
	if err := os.MkdirAll(filepath.Join(realRepo, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}

	// Create a scan root with a symlink pointing to the real repo.
	scanRoot := filepath.Join(dir, "scanroot")
	if err := os.MkdirAll(scanRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRepo, filepath.Join(scanRoot, "linked")); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	repos, err := Scan(scanRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Current behavior: scanner uses e.IsDir() from os.ReadDir which returns
	// false for symlinks (it reports the link, not the target). So symlinked
	// repos are NOT discovered. This documents that limitation.
	if len(repos) != 0 {
		t.Errorf("expected 0 repos (symlinks not followed by scanner), got %d", len(repos))
	}
}

func TestScan_DotPrefixedDirs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create a hidden directory with .ralph inside
	hiddenDir := filepath.Join(dir, ".hidden-project")
	if err := os.MkdirAll(filepath.Join(hiddenDir, ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}

	repos, err := Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Hidden directories with .ralph should still be discovered.
	if len(repos) != 1 {
		t.Errorf("expected 1 repo for dot-prefixed dir, got %d", len(repos))
	}
}

func TestScan_ManyRepos(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	count := 50
	for i := 0; i < count; i++ {
		name := filepath.Join(dir, fmt.Sprintf("repo-%03d", i))
		if err := os.MkdirAll(filepath.Join(name, ".ralph"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	repos, err := Scan(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != count {
		t.Errorf("expected %d repos, got %d", count, len(repos))
	}
	// Verify sorted.
	for i := 1; i < len(repos); i++ {
		if repos[i].Name < repos[i-1].Name {
			t.Errorf("repos not sorted: %s before %s", repos[i-1].Name, repos[i].Name)
			break
		}
	}
}
