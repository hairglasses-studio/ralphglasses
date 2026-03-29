package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// cyclesDir returns the canonical directory for cycle JSON files.
func cyclesDir(repoPath string) string {
	return filepath.Join(repoPath, ".ralph", "cycles")
}

// SaveCycle writes a CycleRun to .ralph/cycles/{cycle_id}.json.
func SaveCycle(repoPath string, cycle *CycleRun) error {
	dir := cyclesDir(repoPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create cycles dir: %w", err)
	}

	data, err := json.MarshalIndent(cycle, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal cycle: %w", err)
	}

	path := filepath.Join(dir, cycle.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// LoadCycle reads a CycleRun from .ralph/cycles/{cycleID}.json.
func LoadCycle(repoPath, cycleID string) (*CycleRun, error) {
	path := filepath.Join(cyclesDir(repoPath), cycleID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read cycle: %w", err)
	}

	var cycle CycleRun
	if err := json.Unmarshal(data, &cycle); err != nil {
		return nil, fmt.Errorf("unmarshal cycle: %w", err)
	}
	return &cycle, nil
}

// ListCycles returns all cycles in .ralph/cycles/, sorted by UpdatedAt descending.
func ListCycles(repoPath string) ([]*CycleRun, error) {
	dir := cyclesDir(repoPath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list cycles: %w", err)
	}

	var cycles []*CycleRun
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var c CycleRun
		if err := json.Unmarshal(data, &c); err != nil {
			continue
		}
		cycles = append(cycles, &c)
	}

	sort.Slice(cycles, func(i, j int) bool {
		return cycles[i].UpdatedAt.After(cycles[j].UpdatedAt)
	})
	return cycles, nil
}

// ActiveCycle returns the first non-complete, non-failed cycle, or nil if none.
func ActiveCycle(repoPath string) (*CycleRun, error) {
	cycles, err := ListCycles(repoPath)
	if err != nil {
		return nil, err
	}
	for _, c := range cycles {
		if c.Phase != CycleComplete && c.Phase != CycleFailed {
			return c, nil
		}
	}
	return nil, nil
}
