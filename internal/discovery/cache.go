package discovery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/appdir"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

// DefaultCacheDir returns the default scan cache directory.
func DefaultCacheDir() string {
	if legacy := legacyCacheDir(); legacy != "" {
		return legacy
	}
	return appdir.CacheDir("ralphglasses")
}

func legacyCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	legacyDir := filepath.Join(home, ".ralphglasses")
	if _, err := os.Stat(filepath.Join(legacyDir, "scan-cache.json")); err == nil {
		return legacyDir
	}
	return ""
}

// cacheEntry is the on-disk representation of a single cached repo.
type cacheEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	HasRalph bool   `json:"has_ralph"`
	HasRC    bool   `json:"has_rc"`
}

// cacheFile is the JSON structure persisted to scan-cache.json.
type cacheFile struct {
	Root      string       `json:"root"`
	ScannedAt time.Time    `json:"scanned_at"`
	Repos     []cacheEntry `json:"repos"`
}

// ScanCache provides filesystem-aware caching for scan results. It stores
// results in a JSON file and checks filesystem mtimes to decide if the cache
// is still fresh.
type ScanCache struct {
	mu       sync.Mutex
	cacheDir string
	// In-memory copy of the last loaded cache, keyed by root path.
	mem map[string]*cacheFile
}

// NewScanCache creates a ScanCache that persists to the given directory.
// If cacheDir is empty, DefaultCacheDir() is used.
func NewScanCache(cacheDir string) *ScanCache {
	if cacheDir == "" {
		cacheDir = DefaultCacheDir()
	}
	return &ScanCache{
		cacheDir: cacheDir,
		mem:      make(map[string]*cacheFile),
	}
}

// cachePath returns the path to the cache JSON file.
func (sc *ScanCache) cachePath() string {
	return filepath.Join(sc.cacheDir, "scan-cache.json")
}

// CachedScan returns cached scan results if they are fresh enough. A cache
// entry is considered stale if:
//   - It is older than maxAge
//   - Any .ralph/ directory under root has an mtime newer than the cache
//
// When the cache is stale or missing, a fresh ParallelScan is performed with
// a default worker count of 4, and the results are persisted to disk.
func (sc *ScanCache) CachedScan(ctx context.Context, root string, maxAge time.Duration) ([]*model.Repo, error) {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Try to load the on-disk cache if we don't have it in memory.
	if _, ok := sc.mem[root]; !ok {
		if cf, err := sc.loadFromDisk(); err == nil && cf.Root == root {
			sc.mem[root] = cf
		}
	}

	if cf, ok := sc.mem[root]; ok {
		if sc.isFresh(cf, maxAge) {
			return sc.hydrate(ctx, cf), nil
		}
	}

	// Cache miss or stale: perform a fresh scan.
	repos, err := ParallelScan(ctx, root, 4)
	if err != nil {
		return nil, err
	}

	cf := sc.toCache(root, repos)
	sc.mem[root] = cf
	_ = sc.persistToDisk(cf)

	return repos, nil
}

// isFresh returns true if the cache entry is within maxAge and no .ralph/
// directory under root has been modified since the cache was created.
func (sc *ScanCache) isFresh(cf *cacheFile, maxAge time.Duration) bool {
	if time.Since(cf.ScannedAt) > maxAge {
		return false
	}

	// Check if any .ralph/ dir was modified after the cache was built.
	entries, err := os.ReadDir(cf.Root)
	if err != nil {
		return false // can't verify, treat as stale
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		ralphDir := filepath.Join(cf.Root, e.Name(), ".ralph")
		info, err := os.Stat(ralphDir)
		if err != nil {
			continue // no .ralph/ dir — fine
		}
		if info.ModTime().After(cf.ScannedAt) {
			return false
		}
	}

	return true
}

// hydrate converts cached entries back into model.Repo pointers, calling
// RefreshRepo to pick up current status data.
func (sc *ScanCache) hydrate(ctx context.Context, cf *cacheFile) []*model.Repo {
	repos := make([]*model.Repo, 0, len(cf.Repos))
	for _, ce := range cf.Repos {
		r := &model.Repo{
			Name:     ce.Name,
			Path:     ce.Path,
			HasRalph: ce.HasRalph,
			HasRC:    ce.HasRC,
		}
		_ = model.RefreshRepo(ctx, r)
		repos = append(repos, r)
	}
	return repos
}

// toCache converts scan results into a cacheFile for persistence.
func (sc *ScanCache) toCache(root string, repos []*model.Repo) *cacheFile {
	entries := make([]cacheEntry, 0, len(repos))
	for _, r := range repos {
		entries = append(entries, cacheEntry{
			Name:     r.Name,
			Path:     r.Path,
			HasRalph: r.HasRalph,
			HasRC:    r.HasRC,
		})
	}
	return &cacheFile{
		Root:      root,
		ScannedAt: time.Now(),
		Repos:     entries,
	}
}

// loadFromDisk reads the cache file from disk.
func (sc *ScanCache) loadFromDisk() (*cacheFile, error) {
	data, err := os.ReadFile(sc.cachePath())
	if err != nil {
		return nil, err
	}
	var cf cacheFile
	if err := json.Unmarshal(data, &cf); err != nil {
		return nil, err
	}
	return &cf, nil
}

// persistToDisk writes the cache to disk, creating the cache directory if
// needed.
func (sc *ScanCache) persistToDisk(cf *cacheFile) error {
	if err := os.MkdirAll(sc.cacheDir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cf, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sc.cachePath(), data, 0644)
}
