package marathon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Checkpoint captures marathon state at a point in time for resumability.
type Checkpoint struct {
	Timestamp       time.Time                `json:"timestamp"`
	CyclesCompleted int                      `json:"cycles_completed"`
	SpentUSD        float64                  `json:"spent_usd"`
	SupervisorState session.SupervisorState  `json:"supervisor_state"`
}

// SaveCheckpoint writes a checkpoint to the given directory.
// The filename includes a timestamp for ordering.
func SaveCheckpoint(dir string, cp *Checkpoint) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create checkpoint dir: %w", err)
	}

	if cp.Timestamp.IsZero() {
		cp.Timestamp = time.Now()
	}

	filename := fmt.Sprintf("cp-%s.json", cp.Timestamp.Format("20060102-150405.000"))
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(cp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write checkpoint: %w", err)
	}
	return nil
}

// LoadLatestCheckpoint reads the most recent checkpoint from the directory.
// Returns an error if no checkpoints exist.
func LoadLatestCheckpoint(dir string) (*Checkpoint, error) {
	cps, err := ListCheckpoints(dir)
	if err != nil {
		return nil, err
	}
	if len(cps) == 0 {
		return nil, fmt.Errorf("no checkpoints found in %s", dir)
	}
	return cps[len(cps)-1], nil
}

// ListCheckpoints returns all checkpoints in the directory, sorted by timestamp
// ascending (oldest first).
func ListCheckpoints(dir string) ([]*Checkpoint, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read checkpoint dir: %w", err)
	}

	var checkpoints []*Checkpoint
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "cp-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue // skip unreadable files
		}
		var cp Checkpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			continue // skip malformed files
		}
		checkpoints = append(checkpoints, &cp)
	}

	sort.Slice(checkpoints, func(i, j int) bool {
		return checkpoints[i].Timestamp.Before(checkpoints[j].Timestamp)
	})

	return checkpoints, nil
}
