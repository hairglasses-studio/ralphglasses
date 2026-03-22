package awesome

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const awesomeDir = ".ralph/awesome"

// StorePath returns the awesome storage directory for a repo.
func StorePath(repoPath string) string {
	return filepath.Join(repoPath, awesomeDir)
}

// SaveIndex writes the index to disk and rotates the previous one.
func SaveIndex(repoPath string, idx *Index) error {
	dir := StorePath(repoPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	indexPath := filepath.Join(dir, "index.json")
	prevPath := filepath.Join(dir, "index.prev.json")

	// Rotate: current → prev
	if _, err := os.Stat(indexPath); err == nil {
		os.Rename(indexPath, prevPath)
	}

	return writeJSON(indexPath, idx)
}

// LoadIndex reads the current index from disk.
func LoadIndex(repoPath string) (*Index, error) {
	path := filepath.Join(StorePath(repoPath), "index.json")
	var idx Index
	if err := readJSON(path, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// LoadPrevIndex reads the previous index from disk.
func LoadPrevIndex(repoPath string) (*Index, error) {
	path := filepath.Join(StorePath(repoPath), "index.prev.json")
	var idx Index
	if err := readJSON(path, &idx); err != nil {
		return nil, err
	}
	return &idx, nil
}

// SaveAnalysis writes the analysis to disk.
func SaveAnalysis(repoPath string, a *Analysis) error {
	dir := StorePath(repoPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return writeJSON(filepath.Join(dir, "analysis.json"), a)
}

// LoadAnalysis reads the analysis from disk.
func LoadAnalysis(repoPath string) (*Analysis, error) {
	path := filepath.Join(StorePath(repoPath), "analysis.json")
	var a Analysis
	if err := readJSON(path, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

// SaveReport writes the report as markdown.
func SaveReport(repoPath string, content string) error {
	dir := StorePath(repoPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(content), 0644)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
