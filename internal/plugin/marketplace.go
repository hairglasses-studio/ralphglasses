// E4.1: Plugin Marketplace — community plugin registry fetched from a
// GitHub-hosted YAML index. Supports search, install, and update operations.
package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// MarketplaceEntry describes a community plugin available for installation.
type MarketplaceEntry struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Version     string   `yaml:"version" json:"version"`
	Author      string   `yaml:"author" json:"author"`
	URL         string   `yaml:"url" json:"url"`         // download URL
	Type        string   `yaml:"type" json:"type"`       // "provider", "tool", "strategy", "workflow", "theme"
	Checksum    string   `yaml:"checksum" json:"checksum"` // SHA-256 of the artifact
	Tags        []string `yaml:"tags" json:"tags"`
	License     string   `yaml:"license,omitempty" json:"license,omitempty"`
	Homepage    string   `yaml:"homepage,omitempty" json:"homepage,omitempty"`
}

// Marketplace manages discovery and installation of community plugins.
type Marketplace struct {
	mu        sync.RWMutex
	indexURL  string
	cache     []MarketplaceEntry
	cacheAt   time.Time
	cacheTTL  time.Duration
	client    *http.Client
	installDir string
}

// NewMarketplace creates a marketplace client.
// indexURL is the URL of the YAML plugin index (e.g., a raw GitHub file).
// installDir is where plugins are installed (e.g., ~/.config/ralphglasses/plugins/).
func NewMarketplace(indexURL, installDir string) *Marketplace {
	return &Marketplace{
		indexURL:   indexURL,
		installDir: installDir,
		cacheTTL:   1 * time.Hour,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

// Search finds plugins matching a query string (searches name, description, tags).
func (m *Marketplace) Search(query string) ([]MarketplaceEntry, error) {
	entries, err := m.fetchIndex()
	if err != nil {
		return nil, err
	}

	if query == "" {
		return entries, nil
	}

	queryLower := strings.ToLower(query)
	var matches []MarketplaceEntry

	for _, e := range entries {
		score := 0
		if strings.Contains(strings.ToLower(e.Name), queryLower) {
			score += 3
		}
		if strings.Contains(strings.ToLower(e.Description), queryLower) {
			score += 1
		}
		for _, tag := range e.Tags {
			if strings.Contains(strings.ToLower(tag), queryLower) {
				score += 2
			}
		}
		if score > 0 {
			matches = append(matches, e)
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Name < matches[j].Name
	})

	return matches, nil
}

// Install downloads and installs a plugin by name.
// Verifies the SHA-256 checksum before installation.
func (m *Marketplace) Install(name string) error {
	entries, err := m.fetchIndex()
	if err != nil {
		return err
	}

	var entry *MarketplaceEntry
	for i := range entries {
		if entries[i].Name == name {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("plugin %q not found in marketplace", name)
	}

	// Download the artifact
	resp, err := m.client.Get(entry.URL)
	if err != nil {
		return fmt.Errorf("download %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", name, resp.StatusCode)
	}

	// Read body and verify checksum
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}

	if entry.Checksum != "" {
		hash := sha256.Sum256(body)
		actual := hex.EncodeToString(hash[:])
		if actual != entry.Checksum {
			return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", name, entry.Checksum, actual)
		}
	}

	// Write to install directory
	if err := os.MkdirAll(m.installDir, 0755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}

	destPath := filepath.Join(m.installDir, name+".yaml")
	if err := os.WriteFile(destPath, body, 0644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}

	slog.Info("marketplace: installed plugin", "name", name, "version", entry.Version, "path", destPath)
	return nil
}

// Installed returns the list of installed plugin names.
func (m *Marketplace) Installed() []string {
	entries, err := os.ReadDir(m.installDir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
		}
	}
	return names
}

// fetchIndex retrieves and caches the plugin index.
func (m *Marketplace) fetchIndex() ([]MarketplaceEntry, error) {
	m.mu.RLock()
	if len(m.cache) > 0 && time.Since(m.cacheAt) < m.cacheTTL {
		result := make([]MarketplaceEntry, len(m.cache))
		copy(result, m.cache)
		m.mu.RUnlock()
		return result, nil
	}
	m.mu.RUnlock()

	resp, err := m.client.Get(m.indexURL)
	if err != nil {
		return nil, fmt.Errorf("fetch marketplace index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("marketplace index HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read marketplace index: %w", err)
	}

	var entries []MarketplaceEntry
	if err := yaml.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parse marketplace index: %w", err)
	}

	m.mu.Lock()
	m.cache = entries
	m.cacheAt = time.Now()
	m.mu.Unlock()

	return entries, nil
}
