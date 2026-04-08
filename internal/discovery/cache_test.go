package discovery

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCachedScan_HitReturnsCachedResults(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeRepo(t, root, "cached-repo", true, false)

	cacheDir := t.TempDir()
	sc := NewScanCache(cacheDir)

	// First call: cache miss, triggers scan.
	repos1, err := sc.CachedScan(context.Background(), root, 5*time.Minute)
	if err != nil {
		t.Fatalf("first CachedScan: %v", err)
	}
	if len(repos1) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos1))
	}

	// Verify cache file was written.
	if _, err := os.Stat(sc.cachePath()); err != nil {
		t.Fatalf("cache file not written: %v", err)
	}

	// Second call: should hit cache (within maxAge, no modifications).
	repos2, err := sc.CachedScan(context.Background(), root, 5*time.Minute)
	if err != nil {
		t.Fatalf("second CachedScan: %v", err)
	}
	if len(repos2) != 1 {
		t.Fatalf("expected 1 repo from cache, got %d", len(repos2))
	}
	if repos2[0].Name != "cached-repo" {
		t.Errorf("expected cached-repo, got %q", repos2[0].Name)
	}
}

func TestCachedScan_MissOnExpired(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeRepo(t, root, "old-repo", true, false)

	cacheDir := t.TempDir()
	sc := NewScanCache(cacheDir)

	// Prime the cache.
	_, err := sc.CachedScan(context.Background(), root, 5*time.Minute)
	if err != nil {
		t.Fatalf("prime CachedScan: %v", err)
	}

	// Add a new repo.
	makeRepo(t, root, "new-repo", true, false)

	// Use a very short maxAge so the cache is immediately stale.
	repos, err := sc.CachedScan(context.Background(), root, 0)
	if err != nil {
		t.Fatalf("CachedScan with maxAge=0: %v", err)
	}

	if len(repos) != 2 {
		t.Fatalf("expected 2 repos after cache expiry, got %d", len(repos))
	}
}

func TestCachedScan_InvalidatedByModifiedRalphDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	repoPath := makeRepo(t, root, "live-repo", true, false)

	cacheDir := t.TempDir()
	sc := NewScanCache(cacheDir)

	// Prime the cache.
	repos1, err := sc.CachedScan(context.Background(), root, 5*time.Minute)
	if err != nil {
		t.Fatalf("prime: %v", err)
	}
	if len(repos1) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos1))
	}

	// Touch the .ralph/ dir to make it newer than the cache.
	// Sleep briefly to ensure the mtime is strictly newer.
	time.Sleep(50 * time.Millisecond)
	ralphDir := filepath.Join(repoPath, ".ralph")
	now := time.Now().Add(time.Second)
	if err := os.Chtimes(ralphDir, now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	// This should detect the modification and rescan.
	repos2, err := sc.CachedScan(context.Background(), root, 5*time.Minute)
	if err != nil {
		t.Fatalf("post-modify CachedScan: %v", err)
	}
	if len(repos2) != 1 {
		t.Fatalf("expected 1 repo after invalidation, got %d", len(repos2))
	}
}

func TestCachedScan_PersistsToDisk(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeRepo(t, root, "persist-repo", true, false)

	cacheDir := t.TempDir()

	// First instance: scan and persist.
	sc1 := NewScanCache(cacheDir)
	repos1, err := sc1.CachedScan(context.Background(), root, 5*time.Minute)
	if err != nil {
		t.Fatalf("first instance: %v", err)
	}
	if len(repos1) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos1))
	}

	// Second instance with empty memory: should load from disk.
	sc2 := NewScanCache(cacheDir)
	repos2, err := sc2.CachedScan(context.Background(), root, 5*time.Minute)
	if err != nil {
		t.Fatalf("second instance: %v", err)
	}
	if len(repos2) != 1 {
		t.Fatalf("expected 1 repo from disk cache, got %d", len(repos2))
	}
	if repos2[0].Name != "persist-repo" {
		t.Errorf("expected persist-repo, got %q", repos2[0].Name)
	}
}

func TestCachedScan_EmptyRoot(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cacheDir := t.TempDir()
	sc := NewScanCache(cacheDir)

	repos, err := sc.CachedScan(context.Background(), root, 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos, got %d", len(repos))
	}
}

func TestCachedScan_NonexistentRoot(t *testing.T) {
	t.Parallel()

	cacheDir := t.TempDir()
	sc := NewScanCache(cacheDir)

	_, err := sc.CachedScan(context.Background(), "/nonexistent/path/xyzzy", 5*time.Minute)
	if err == nil {
		t.Fatal("expected error for nonexistent root")
	}
}

func TestNewScanCache_DefaultCacheDir(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	sc := NewScanCache("")
	if got, want := sc.cacheDir, filepath.Join(xdg, "ralphglasses"); got != want {
		t.Fatalf("cache dir = %q, want %q", got, want)
	}
}

func TestDefaultCacheDir_LegacyFileWins(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", "")
	legacyDir := filepath.Join(home, ".ralphglasses")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "scan-cache.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write legacy cache: %v", err)
	}

	if got, want := DefaultCacheDir(), legacyDir; got != want {
		t.Fatalf("DefaultCacheDir() = %q, want %q", got, want)
	}
}

func TestScanCache_CorruptCacheFile(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	makeRepo(t, root, "repo-a", true, false)

	cacheDir := t.TempDir()
	// Write corrupt data to the cache file.
	os.MkdirAll(cacheDir, 0755)
	os.WriteFile(filepath.Join(cacheDir, "scan-cache.json"), []byte("not json{{{"), 0644)

	sc := NewScanCache(cacheDir)
	repos, err := sc.CachedScan(context.Background(), root, 5*time.Minute)
	if err != nil {
		t.Fatalf("unexpected error with corrupt cache: %v", err)
	}
	// Should still find the repo via fresh scan.
	if len(repos) != 1 {
		t.Errorf("expected 1 repo despite corrupt cache, got %d", len(repos))
	}
}
