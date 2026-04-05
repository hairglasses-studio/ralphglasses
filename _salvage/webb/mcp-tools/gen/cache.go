package gen

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CacheFile stores checksums for a module's source files
type CacheFile struct {
	Version    string            `json:"version"`
	Checksums  map[string]string `json:"checksums"`
	OutputHash string            `json:"output_hash"`
}

const cacheVersion = "1"

// ModuleCache manages build caching for a module
type ModuleCache struct {
	moduleDir string
	cacheFile string
}

// NewModuleCache creates a cache manager for a module directory
func NewModuleCache(moduleDir string) *ModuleCache {
	return &ModuleCache{
		moduleDir: moduleDir,
		cacheFile: filepath.Join(moduleDir, ".toolgen.cache"),
	}
}

// NeedsRegeneration checks if the module needs to be regenerated
// Returns true if:
// - Cache file doesn't exist
// - Source files have changed
// - Generated file doesn't exist or has been modified
func (c *ModuleCache) NeedsRegeneration() (bool, string, error) {
	// Check if generated file exists
	genFile := filepath.Join(c.moduleDir, "tools_gen.go")
	if _, err := os.Stat(genFile); os.IsNotExist(err) {
		return true, "tools_gen.go doesn't exist", nil
	}

	// Check if cache file exists
	cache, err := c.loadCache()
	if err != nil {
		return true, "cache file not found or invalid", nil
	}

	// Check version
	if cache.Version != cacheVersion {
		return true, "cache version mismatch", nil
	}

	// Get current source checksums
	currentChecksums, err := c.computeSourceChecksums()
	if err != nil {
		return true, fmt.Sprintf("error computing checksums: %v", err), nil
	}

	// Compare checksums
	if !checksumMapsEqual(cache.Checksums, currentChecksums) {
		return true, "source files changed", nil
	}

	// Check if output file matches cached hash
	outputHash, err := fileChecksum(genFile)
	if err != nil {
		return true, "error reading generated file", nil
	}
	if outputHash != cache.OutputHash {
		return true, "tools_gen.go was modified", nil
	}

	return false, "up to date", nil
}

// UpdateCache updates the cache after successful generation
func (c *ModuleCache) UpdateCache() error {
	checksums, err := c.computeSourceChecksums()
	if err != nil {
		return fmt.Errorf("failed to compute source checksums: %w", err)
	}

	genFile := filepath.Join(c.moduleDir, "tools_gen.go")
	outputHash, err := fileChecksum(genFile)
	if err != nil {
		return fmt.Errorf("failed to compute output checksum: %w", err)
	}

	cache := CacheFile{
		Version:    cacheVersion,
		Checksums:  checksums,
		OutputHash: outputHash,
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	if err := os.WriteFile(c.cacheFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// computeSourceChecksums computes checksums for all source files
func (c *ModuleCache) computeSourceChecksums() (map[string]string, error) {
	checksums := make(map[string]string)

	// Files to track: tools.yaml and all .go files except generated ones
	patterns := []string{"tools.yaml", "handlers.go", "*.go"}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(c.moduleDir, pattern))
		if err != nil {
			continue
		}

		for _, path := range matches {
			// Skip generated files
			base := filepath.Base(path)
			if strings.HasSuffix(base, "_gen.go") || base == ".toolgen.cache" {
				continue
			}

			hash, err := fileChecksum(path)
			if err != nil {
				return nil, fmt.Errorf("failed to checksum %s: %w", path, err)
			}
			checksums[base] = hash
		}
	}

	return checksums, nil
}

// loadCache loads the cache file
func (c *ModuleCache) loadCache() (*CacheFile, error) {
	data, err := os.ReadFile(c.cacheFile)
	if err != nil {
		return nil, err
	}

	var cache CacheFile
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, err
	}

	return &cache, nil
}

// fileChecksum computes SHA256 checksum of a file
func fileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// checksumMapsEqual compares two checksum maps
func checksumMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// FindModulesWithYAML finds all module directories containing tools.yaml
func FindModulesWithYAML(rootDir string) ([]string, error) {
	var modules []string

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == "tools.yaml" && !info.IsDir() {
			modules = append(modules, filepath.Dir(path))
		}
		return nil
	})

	if err != nil {
		return nil, err
	}

	sort.Strings(modules)
	return modules, nil
}

// GenerateStatus represents the status of a module's generation
type GenerateStatus struct {
	Module       string
	NeedsRegen   bool
	Reason       string
	Error        error
	Regenerated  bool
}

// CheckAllModules checks all modules for regeneration needs
func CheckAllModules(rootDir string) ([]GenerateStatus, error) {
	modules, err := FindModulesWithYAML(rootDir)
	if err != nil {
		return nil, err
	}

	var statuses []GenerateStatus
	for _, moduleDir := range modules {
		cache := NewModuleCache(moduleDir)
		needsRegen, reason, err := cache.NeedsRegeneration()

		status := GenerateStatus{
			Module:     filepath.Base(moduleDir),
			NeedsRegen: needsRegen,
			Reason:     reason,
			Error:      err,
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}
