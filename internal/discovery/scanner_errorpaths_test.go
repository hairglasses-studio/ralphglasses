package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestScan_CancelledDuringIteration cancels the context after ReadDir
// succeeds but before the loop finishes, hitting the ctx.Err() check
// inside the for loop (line 27-28).
func TestScan_CancelledDuringIteration(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create many repos so the cancellation has a chance to fire mid-loop.
	for i := range 20 {
		name := filepath.Join(dir, "repo-"+string(rune('a'+i)))
		if err := os.MkdirAll(filepath.Join(name, ".ralph"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately — the ReadDir will succeed, but the loop iteration
	// should detect the cancelled context.
	cancel()

	_, err := Scan(ctx, dir)
	if err == nil {
		t.Fatal("expected error for cancelled context during iteration")
	}
}

// TestScan_RootIsFile verifies that passing a file (not directory) as root
// returns an error from os.ReadDir.
func TestScan_RootIsFile(t *testing.T) {
	t.Parallel()

	f := filepath.Join(t.TempDir(), "not-a-dir.txt")
	if err := os.WriteFile(f, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Scan(context.Background(), f)
	if err == nil {
		t.Fatal("expected error when root is a file, not a directory")
	}
}

// TestScan_EmptyRoot verifies that an empty root directory returns no repos
// and no error.
func TestScan_EmptyRoot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repos, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

// TestScan_OnlyRCFilePresent tests a repo with just .ralphrc (no .ralph dir)
// to exercise the hasRalph=false,hasRC=true branch.
func TestScan_OnlyRCFilePresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	repoDir := filepath.Join(dir, "rc-repo")
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".ralphrc"), []byte("MODEL=test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	repos, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].HasRalph {
		t.Error("expected HasRalph=false for .ralphrc-only repo")
	}
	if !repos[0].HasRC {
		t.Error("expected HasRC=true")
	}
}
