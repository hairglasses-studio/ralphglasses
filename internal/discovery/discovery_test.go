package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestScan_NonExistentDirectory verifies that scanning a non-existent directory
// returns an error (Phase 0.6.1.1).
func TestScan_NonExistentDirectory(t *testing.T) {
	t.Parallel()

	_, err := Scan(context.Background(), "/tmp/nonexistent-dir-that-does-not-exist-12345")
	if err == nil {
		t.Fatal("expected error for non-existent directory, got nil")
	}
}

// TestScan_PermissionDeniedRoot verifies that scanning a directory with
// permission 000 returns an error from os.ReadDir (Phase 0.6.1.1).
func TestScan_PermissionDeniedRoot(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}
	t.Parallel()

	dir := t.TempDir()
	restricted := filepath.Join(dir, "noperm")
	if err := os.MkdirAll(restricted, 0755); err != nil {
		t.Fatal(err)
	}
	// Place a repo inside before locking down.
	if err := os.MkdirAll(filepath.Join(restricted, "repo", ".ralph"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(restricted, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(restricted, 0755) //nolint:errcheck

	_, err := Scan(context.Background(), restricted)
	if err == nil {
		t.Fatal("expected error for permission-denied root, got nil")
	}
}

// TestScan_SymlinkCycleNoHang verifies that a symlink cycle in the scan root
// does not cause the scanner to hang (Phase 0.6.1.1).
func TestScan_SymlinkCycleNoHang(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create mutual symlinks: a -> b, b -> a
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.Symlink(b, a); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
	if err := os.Symlink(a, b); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}

	// Should complete without hanging. Scanner only reads one level deep.
	repos, err := Scan(context.Background(), dir)
	if err != nil {
		t.Fatalf("unexpected error with symlink cycle: %v", err)
	}
	// Neither symlink is a real directory with .ralph, so 0 repos expected.
	if len(repos) != 0 {
		t.Errorf("expected 0 repos with symlink cycle, got %d", len(repos))
	}
}

// TestScan_FileAsRoot verifies that a file path (not directory) returns an error.
func TestScan_FileAsRoot(t *testing.T) {
	t.Parallel()

	f := filepath.Join(t.TempDir(), "afile.txt")
	if err := os.WriteFile(f, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := Scan(context.Background(), f)
	if err == nil {
		t.Fatal("expected error when root is a file, got nil")
	}
}
