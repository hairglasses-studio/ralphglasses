package session

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrateJSONToStore(t *testing.T) {
	// Create a temp dir with some JSON session files.
	jsonDir := filepath.Join(t.TempDir(), "sessions")
	if err := os.MkdirAll(jsonDir, 0755); err != nil {
		t.Fatal(err)
	}

	sess1 := &Session{
		ID:           "migrate-1",
		Provider:     ProviderClaude,
		RepoPath:     "/repos/alpha",
		RepoName:     "alpha",
		Status:       StatusCompleted,
		Prompt:       "fix the tests",
		SpentUSD:     2.50,
		LaunchedAt:   time.Now().Add(-1 * time.Hour),
		LastActivity: time.Now(),
	}
	sess2 := &Session{
		ID:           "migrate-2",
		Provider:     ProviderGemini,
		RepoPath:     "/repos/beta",
		Status:       StatusRunning,
		Prompt:       "add docs",
		SpentUSD:     0.80,
		LaunchedAt:   time.Now().Add(-30 * time.Minute),
		LastActivity: time.Now(),
	}

	for _, s := range []*Session{sess1, sess2} {
		data, _ := json.Marshal(s)
		if err := os.WriteFile(filepath.Join(jsonDir, s.ID+".json"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Also write a non-JSON file that should be skipped.
	if err := os.WriteFile(filepath.Join(jsonDir, "notes.txt"), []byte("ignore me"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	t.Run("MemoryStore", func(t *testing.T) {
		store := NewMemoryStore()
		imported, err := MigrateJSONToStore(ctx, jsonDir, store)
		if err != nil {
			t.Fatalf("MigrateJSONToStore: %v", err)
		}
		if imported != 2 {
			t.Errorf("imported = %d, want 2", imported)
		}

		got, err := store.GetSession(ctx, "migrate-1")
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if got.SpentUSD != 2.50 {
			t.Errorf("SpentUSD = %f, want 2.50", got.SpentUSD)
		}
	})

	t.Run("SQLiteStore", func(t *testing.T) {
		dbPath := filepath.Join(t.TempDir(), "migrate.db")
		store, err := NewSQLiteStore(dbPath)
		if err != nil {
			t.Fatalf("NewSQLiteStore: %v", err)
		}
		defer store.Close()

		imported, err := MigrateJSONToStore(ctx, jsonDir, store)
		if err != nil {
			t.Fatalf("MigrateJSONToStore: %v", err)
		}
		if imported != 2 {
			t.Errorf("imported = %d, want 2", imported)
		}

		// Verify RepoName was set for sess2 (which had no RepoName in JSON).
		got, err := store.GetSession(ctx, "migrate-2")
		if err != nil {
			t.Fatalf("GetSession: %v", err)
		}
		if got.RepoName != "beta" {
			t.Errorf("RepoName = %q, want %q", got.RepoName, "beta")
		}
		if got.Provider != ProviderGemini {
			t.Errorf("Provider = %q, want %q", got.Provider, ProviderGemini)
		}
	})

	t.Run("Idempotent", func(t *testing.T) {
		store := NewMemoryStore()
		_, _ = MigrateJSONToStore(ctx, jsonDir, store)
		imported, err := MigrateJSONToStore(ctx, jsonDir, store)
		if err != nil {
			t.Fatalf("MigrateJSONToStore: %v", err)
		}
		if imported != 0 {
			t.Errorf("second migration imported = %d, want 0", imported)
		}
	})

	t.Run("MissingDir", func(t *testing.T) {
		store := NewMemoryStore()
		imported, err := MigrateJSONToStore(ctx, "/nonexistent/path", store)
		if err != nil {
			t.Fatalf("expected nil error for missing dir, got: %v", err)
		}
		if imported != 0 {
			t.Errorf("imported = %d, want 0", imported)
		}
	})
}
