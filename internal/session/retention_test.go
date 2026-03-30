package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyRetention_MaxAge(t *testing.T) {
	dir := t.TempDir()

	// Create a file and backdate it
	old := filepath.Join(dir, "old.json")
	if err := os.WriteFile(old, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-48 * time.Hour)
	os.Chtimes(old, oldTime, oldTime)

	// Create a recent file
	recent := filepath.Join(dir, "recent.json")
	os.WriteFile(recent, []byte("{}"), 0644)

	policy := RetentionPolicy{MaxAge: 24 * time.Hour}
	removed, err := ApplyRetention(dir, policy)
	if err != nil {
		t.Fatalf("ApplyRetention: %v", err)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	// Verify old file is gone
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("old file should have been removed")
	}
	// Verify recent file remains
	if _, err := os.Stat(recent); err != nil {
		t.Error("recent file should remain")
	}
}

func TestApplyRetention_MaxFiles(t *testing.T) {
	dir := t.TempDir()

	// Create 5 files with staggered times
	for i := 0; i < 5; i++ {
		f := filepath.Join(dir, filepath.Base(t.Name())+string(rune('a'+i))+".json")
		os.WriteFile(f, []byte("{}"), 0644)
		mt := time.Now().Add(time.Duration(-i) * time.Hour)
		os.Chtimes(f, mt, mt)
	}

	policy := RetentionPolicy{MaxFiles: 3}
	removed, err := ApplyRetention(dir, policy)
	if err != nil {
		t.Fatalf("ApplyRetention: %v", err)
	}
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}

	entries, _ := os.ReadDir(dir)
	if len(entries) != 3 {
		t.Errorf("expected 3 files remaining, got %d", len(entries))
	}
}

func TestApplyRetention_NonexistentDir(t *testing.T) {
	removed, err := ApplyRetention("/nonexistent/path", DefaultRetention)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed, got %d", removed)
	}
}
