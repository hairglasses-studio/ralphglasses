package session

import (
	"testing"
	"time"
)

func TestContextStore_RegisterAndDeregister(t *testing.T) {
	cs := NewContextStore(t.TempDir())

	cs.Register(ContextEntry{
		SessionID: "s1",
		RepoPath:  "/repo/a",
		RepoName:  "a",
		Provider:  ProviderClaude,
		TaskDesc:  "fix bugs",
	})

	entries := cs.ActiveForRepo("/repo/a")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].SessionID != "s1" {
		t.Fatal("wrong session ID")
	}

	cs.Deregister("s1")
	entries = cs.ActiveForRepo("/repo/a")
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries after deregister, got %d", len(entries))
	}
}

func TestContextStore_FileConflicts(t *testing.T) {
	cs := NewContextStore(t.TempDir())

	cs.Register(ContextEntry{
		SessionID:   "s1",
		RepoPath:    "/repo/a",
		ActiveFiles: []string{"main.go", "handler.go"},
	})

	cs.Register(ContextEntry{
		SessionID:   "s2",
		RepoPath:    "/repo/a",
		ActiveFiles: []string{"types.go", "util.go"},
	})

	// No conflicts with different files
	conflicts := cs.FileConflicts("/repo/a", []string{"new_file.go"}, "s3")
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflicts, got %d", len(conflicts))
	}

	// Conflict with s1's main.go
	conflicts = cs.FileConflicts("/repo/a", []string{"main.go", "types.go"}, "s3")
	if len(conflicts) != 2 {
		t.Fatalf("expected 2 conflicts, got %d", len(conflicts))
	}
	if conflicts["main.go"] != "s1" {
		t.Fatalf("expected conflict with s1, got %s", conflicts["main.go"])
	}
	if conflicts["types.go"] != "s2" {
		t.Fatalf("expected conflict with s2, got %s", conflicts["types.go"])
	}

	// Exclude self — no conflict
	conflicts = cs.FileConflicts("/repo/a", []string{"main.go"}, "s1")
	if len(conflicts) != 0 {
		t.Fatalf("expected no conflict when excluding self, got %d", len(conflicts))
	}
}

func TestContextStore_Cleanup(t *testing.T) {
	cs := NewContextStore(t.TempDir())

	cs.Register(ContextEntry{
		SessionID: "old",
		RepoPath:  "/repo/a",
	})

	// Artificially age the entry
	cs.mu.Lock()
	cs.entries["old"].UpdatedAt = time.Now().Add(-2 * time.Hour)
	cs.mu.Unlock()

	cs.Register(ContextEntry{
		SessionID: "new",
		RepoPath:  "/repo/a",
	})

	removed := cs.Cleanup(time.Hour)
	if removed != 1 {
		t.Fatalf("expected 1 removed, got %d", removed)
	}

	all := cs.All()
	if len(all) != 1 || all[0].SessionID != "new" {
		t.Fatal("expected only 'new' entry to survive cleanup")
	}
}

func TestContextStore_UpdateFiles(t *testing.T) {
	cs := NewContextStore(t.TempDir())

	cs.Register(ContextEntry{
		SessionID:   "s1",
		RepoPath:    "/repo/a",
		ActiveFiles: []string{"main.go"},
	})

	cs.UpdateFiles("s1", []string{"main.go", "handler.go", "types.go"})

	entries := cs.ActiveForRepo("/repo/a")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if len(entries[0].ActiveFiles) != 3 {
		t.Errorf("expected 3 active files, got %d", len(entries[0].ActiveFiles))
	}

	// UpdateFiles on nonexistent session is a no-op
	cs.UpdateFiles("nonexistent", []string{"foo.go"})
}

func TestContextStore_Persistence(t *testing.T) {
	dir := t.TempDir()

	// Write
	cs1 := NewContextStore(dir)
	cs1.Register(ContextEntry{
		SessionID:   "s1",
		RepoPath:    "/repo/a",
		ActiveFiles: []string{"main.go"},
	})

	// Load
	cs2 := NewContextStore(dir)
	entries := cs2.ActiveForRepo("/repo/a")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after reload, got %d", len(entries))
	}
	if entries[0].SessionID != "s1" {
		t.Fatal("wrong session ID after reload")
	}
}
