package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PruneLoopRuns scans persisted loop run JSON files in the manager's state
// directory (stateDir/loops/) and removes those matching the given status
// filter whose UpdatedAt is older than the threshold. In dry-run mode it
// returns the count of prunable files without deleting anything.
func PruneLoopRuns(loopDir string, olderThan time.Duration, statuses []string, dryRun bool) (pruned int, err error) {
	if loopDir == "" {
		return 0, nil
	}

	entries, err := os.ReadDir(loopDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	statusSet := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		statusSet[strings.TrimSpace(strings.ToLower(s))] = true
	}

	cutoff := time.Now().Add(-olderThan)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(loopDir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}

		var run loopRunPruneView
		if err := json.Unmarshal(data, &run); err != nil {
			continue
		}

		if !statusSet[strings.ToLower(run.Status)] {
			continue
		}

		// Use UpdatedAt for age check; fall back to CreatedAt if UpdatedAt is zero.
		ts := run.UpdatedAt
		if ts.IsZero() {
			ts = run.CreatedAt
		}
		if ts.IsZero() || ts.After(cutoff) {
			continue
		}

		pruned++
		if !dryRun {
			if rmErr := os.Remove(path); rmErr != nil {
				return pruned, rmErr
			}
		}
	}

	return pruned, nil
}

// PruneLoopRunsFiltered is like PruneLoopRuns but additionally filters by repo name.
func PruneLoopRunsFiltered(loopDir string, olderThan time.Duration, statuses []string, repoFilter string, dryRun bool) (pruned int, err error) {
	if loopDir == "" {
		return 0, nil
	}

	entries, err := os.ReadDir(loopDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	statusSet := make(map[string]bool, len(statuses))
	for _, s := range statuses {
		statusSet[strings.TrimSpace(strings.ToLower(s))] = true
	}

	cutoff := time.Now().Add(-olderThan)
	repoLower := strings.ToLower(strings.TrimSpace(repoFilter))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		path := filepath.Join(loopDir, entry.Name())
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			continue
		}

		var run loopRunPruneView
		if err := json.Unmarshal(data, &run); err != nil {
			continue
		}

		if repoLower != "" && strings.ToLower(run.RepoName) != repoLower {
			continue
		}

		if !statusSet[strings.ToLower(run.Status)] {
			continue
		}

		ts := run.UpdatedAt
		if ts.IsZero() {
			ts = run.CreatedAt
		}
		if ts.IsZero() || ts.After(cutoff) {
			continue
		}

		pruned++
		if !dryRun {
			if rmErr := os.Remove(path); rmErr != nil {
				return pruned, rmErr
			}
		}
	}

	return pruned, nil
}

// loopRunPruneView is a lightweight view of LoopRun used for pruning decisions,
// avoiding full deserialization of iterations and profile data.
type loopRunPruneView struct {
	ID        string    `json:"id"`
	RepoName  string    `json:"repo_name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
