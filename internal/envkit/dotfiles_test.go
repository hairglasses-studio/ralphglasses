package envkit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotRestoreRoundTrip(t *testing.T) {
	// Create a temp dir with some managed config files
	srcDir := t.TempDir()

	// Write test config files
	files := map[string]string{
		".config/starship.toml":    "format = \"$directory\"\n",
		".config/ghostty/config":   "font-family = MonaspiceNe Nerd Font\n",
		".config/bat/config":       "--theme=Catppuccin Mocha\n",
	}

	for rel, content := range files {
		abs := filepath.Join(srcDir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// Snapshot from srcDir
	snap, err := SnapshotDir(srcDir)
	if err != nil {
		t.Fatalf("SnapshotDir: %v", err)
	}

	if len(snap.Files) != len(files) {
		t.Errorf("expected %d files, got %d", len(files), len(snap.Files))
	}

	// Verify snapshot contents
	for rel, want := range files {
		got, ok := snap.Files[rel]
		if !ok {
			t.Errorf("snapshot missing %s", rel)
			continue
		}
		if got != want {
			t.Errorf("snapshot %s: got %q, want %q", rel, got, want)
		}
	}

	// Restore to a different dir
	dstDir := t.TempDir()
	if err := RestoreDir(snap, dstDir); err != nil {
		t.Fatalf("RestoreDir: %v", err)
	}

	// Verify restored files
	for rel, want := range files {
		abs := filepath.Join(dstDir, rel)
		data, err := os.ReadFile(abs)
		if err != nil {
			t.Errorf("read restored %s: %v", rel, err)
			continue
		}
		if string(data) != want {
			t.Errorf("restored %s: got %q, want %q", rel, string(data), want)
		}
	}
}

func TestSnapshotEmptyDir(t *testing.T) {
	dir := t.TempDir()
	snap, err := SnapshotDir(dir)
	if err != nil {
		t.Fatalf("SnapshotDir on empty dir: %v", err)
	}
	if len(snap.Files) != 0 {
		t.Errorf("expected 0 files, got %d", len(snap.Files))
	}
}

func TestSnapshotSummary(t *testing.T) {
	snap := &DotfileSnapshot{
		Files: map[string]string{
			".config/starship.toml": "content",
		},
	}

	summary := SnapshotSummary(snap)
	if summary == "" {
		t.Error("summary should not be empty")
	}

	emptSnap := &DotfileSnapshot{Files: map[string]string{}}
	emptySummary := SnapshotSummary(emptSnap)
	if emptySummary == "" {
		t.Error("empty summary should not be empty string")
	}
}
