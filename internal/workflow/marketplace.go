// E4.2: Workflow Marketplace — community workflow registry fetched from a
// GitHub-hosted YAML index. Supports search, install, and validation.
package workflow

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

// WorkflowEntry describes a community workflow available for installation.
type WorkflowEntry struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description"`
	Version     string   `yaml:"version" json:"version"`
	Author      string   `yaml:"author" json:"author"`
	URL         string   `yaml:"url" json:"url"`         // download URL
	Checksum    string   `yaml:"checksum" json:"checksum"` // SHA-256 of the artifact
	Tags        []string `yaml:"tags" json:"tags"`
	Inputs      []string `yaml:"inputs,omitempty" json:"inputs,omitempty"` // required input params
	Steps       int      `yaml:"steps" json:"steps"`                       // number of workflow steps
}

// WorkflowMarketplace manages discovery and installation of community workflows.
type WorkflowMarketplace struct {
	mu         sync.RWMutex
	indexURL   string
	cache      []WorkflowEntry
	cacheAt    time.Time
	cacheTTL   time.Duration
	client     *http.Client
	installDir string
}

// NewWorkflowMarketplace creates a marketplace client.
// indexURL is the URL of the YAML workflow index (e.g., a raw GitHub file).
// installDir is where workflows are installed (e.g., ~/.config/ralphglasses/workflows/).
func NewWorkflowMarketplace(indexURL, installDir string) *WorkflowMarketplace {
	return &WorkflowMarketplace{
		indexURL:   indexURL,
		installDir: installDir,
		cacheTTL:   1 * time.Hour,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

// Search finds workflows matching a query string (searches name, description, tags).
func (m *WorkflowMarketplace) Search(query string) ([]WorkflowEntry, error) {
	entries, err := m.fetchIndex()
	if err != nil {
		return nil, err
	}

	if query == "" {
		return entries, nil
	}

	queryLower := strings.ToLower(query)
	var matches []WorkflowEntry

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

// Install downloads and installs a workflow by name.
// Verifies the SHA-256 checksum and validates the YAML is a valid workflow
// definition before writing to disk.
func (m *WorkflowMarketplace) Install(name string) error {
	entries, err := m.fetchIndex()
	if err != nil {
		return err
	}

	var entry *WorkflowEntry
	for i := range entries {
		if entries[i].Name == name {
			entry = &entries[i]
			break
		}
	}
	if entry == nil {
		return fmt.Errorf("workflow %q not found in marketplace", name)
	}

	// Download the artifact.
	resp, err := m.client.Get(entry.URL)
	if err != nil {
		return fmt.Errorf("download %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", name, resp.StatusCode)
	}

	// Read body and verify checksum.
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

	// Validate the downloaded content is a valid workflow YAML.
	if _, err := ParseBytes(body); err != nil {
		return fmt.Errorf("validate workflow %s: %w", name, err)
	}

	// Write to install directory.
	if err := os.MkdirAll(m.installDir, 0755); err != nil {
		return fmt.Errorf("create install dir: %w", err)
	}

	destPath := filepath.Join(m.installDir, name+".yaml")
	if err := os.WriteFile(destPath, body, 0644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}

	slog.Info("marketplace: installed workflow", "name", name, "version", entry.Version, "path", destPath)
	return nil
}

// Installed returns the list of installed workflow names.
func (m *WorkflowMarketplace) Installed() []string {
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

// fetchIndex retrieves and caches the workflow index.
func (m *WorkflowMarketplace) fetchIndex() ([]WorkflowEntry, error) {
	m.mu.RLock()
	if len(m.cache) > 0 && time.Since(m.cacheAt) < m.cacheTTL {
		result := make([]WorkflowEntry, len(m.cache))
		copy(result, m.cache)
		m.mu.RUnlock()
		return result, nil
	}
	m.mu.RUnlock()

	resp, err := m.client.Get(m.indexURL)
	if err != nil {
		return nil, fmt.Errorf("fetch workflow index: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("workflow index HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read workflow index: %w", err)
	}

	var entries []WorkflowEntry
	if err := yaml.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parse workflow index: %w", err)
	}

	m.mu.Lock()
	m.cache = entries
	m.cacheAt = time.Now()
	m.mu.Unlock()

	return entries, nil
}
