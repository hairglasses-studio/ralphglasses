package process

import (
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

const (
	statusWatchTimeout      = 2 * time.Second
	statusWatchPollInterval = 100 * time.Millisecond
)

// FileChangedMsg is sent when a watched status file changes.
type FileChangedMsg struct {
	RepoPath string
}

// WatcherErrorMsg is sent when the fsnotify watcher encounters an error.
type WatcherErrorMsg struct {
	Err error
}

// WatchStatusFiles watches .ralph/ directories for status file changes
// and emits Bubble Tea messages. On watcher creation or runtime errors,
// returns WatcherErrorMsg so the TUI can fall back to polling.
func WatchStatusFiles(repoPaths []string) tea.Cmd {
	return func() tea.Msg {
		validRepoPaths, err := validStatusWatchRepoPaths(repoPaths)
		if err != nil {
			return WatcherErrorMsg{Err: err}
		}

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			if shouldFallbackToPolling(err) && len(validRepoPaths) > 0 {
				return watchStatusFilesByPolling(validRepoPaths)
			}
			return WatcherErrorMsg{Err: fmt.Errorf("create watcher: %w", err)}
		}
		return watchWithWatcher(validRepoPaths, watcher)
	}
}

// watchWithWatcher runs the watch loop using the provided watcher. Extracted to
// allow injection of a pre-built watcher in tests.
func watchWithWatcher(repoPaths []string, watcher *fsnotify.Watcher) tea.Msg {
	var addErrors []error
	attemptedWatches := 0
	for _, rp := range repoPaths {
		ralphDir := filepath.Join(rp, ".ralph")
		attemptedWatches++
		if err := watcher.Add(ralphDir); err != nil {
			addErrors = append(addErrors, fmt.Errorf("watch %s: %w", ralphDir, err))
		}
		// Also watch the repo root for .ralphrc config changes.
		attemptedWatches++
		if err := watcher.Add(rp); err != nil {
			addErrors = append(addErrors, fmt.Errorf("watch %s: %w", rp, err))
		}
	}
	// If ALL watches failed, report error and let TUI fall back to polling
	if attemptedWatches > 0 && len(addErrors) == attemptedWatches {
		_ = watcher.Close()
		if shouldFallbackErrors(addErrors) {
			return watchStatusFilesByPolling(repoPaths)
		}
		return WatcherErrorMsg{Err: fmt.Errorf("all watches failed: %w", addErrors[0])}
	}

	// ralphFiles are files inside .ralph/ — repo path is two levels up.
	// rcFiles are at the repo root — repo path is one level up.
	ralphFiles := map[string]bool{
		"status.json":            true,
		".circuit_breaker_state": true,
		"progress.json":          true,
	}

	// Block until an event arrives or timeout.
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return WatcherErrorMsg{Err: fmt.Errorf("watcher events channel closed")}
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				base := filepath.Base(event.Name)
				if ralphFiles[base] {
					repoPath := filepath.Dir(filepath.Dir(event.Name))
					_ = watcher.Close()
					return FileChangedMsg{RepoPath: repoPath}
				}
				if base == ".ralphrc" {
					repoPath := filepath.Dir(event.Name)
					_ = watcher.Close()
					return FileChangedMsg{RepoPath: repoPath}
				}
			}
		case watchErr, ok := <-watcher.Errors:
			_ = watcher.Close()
			if !ok {
				return WatcherErrorMsg{Err: fmt.Errorf("watcher errors channel closed")}
			}
			if shouldFallbackToPolling(watchErr) {
				return watchStatusFilesByPolling(repoPaths)
			}
			return WatcherErrorMsg{Err: fmt.Errorf("fsnotify: %w", watchErr)}
		case <-time.After(statusWatchTimeout):
			_ = watcher.Close()
			return WatcherErrorMsg{Err: fmt.Errorf("watcher idle timeout: falling back to polling")}
		}
	}
}

type fileWatchState struct {
	exists  bool
	modTime time.Time
	size    int64
	digest  uint64
}

type repoWatchState struct {
	status         fileWatchState
	circuitBreaker fileWatchState
	progress       fileWatchState
	config         fileWatchState
}

func validStatusWatchRepoPaths(repoPaths []string) ([]string, error) {
	if len(repoPaths) == 0 {
		return nil, nil
	}

	valid := make([]string, 0, len(repoPaths))
	var watchErrors []error
	for _, repoPath := range repoPaths {
		info, err := os.Stat(repoPath)
		if err != nil {
			watchErrors = append(watchErrors, fmt.Errorf("watch %s: %w", filepath.Join(repoPath, ".ralph"), err))
			continue
		}
		if !info.IsDir() {
			watchErrors = append(watchErrors, fmt.Errorf("watch %s: not a directory", repoPath))
			continue
		}
		valid = append(valid, repoPath)
	}

	if len(valid) == 0 && len(watchErrors) > 0 {
		return nil, fmt.Errorf("all watches failed: %w", watchErrors[0])
	}

	return valid, nil
}

func watchStatusFilesByPolling(repoPaths []string) tea.Msg {
	if len(repoPaths) == 0 {
		return WatcherErrorMsg{Err: fmt.Errorf("watcher idle timeout: falling back to polling")}
	}

	last := make(map[string]repoWatchState, len(repoPaths))
	for _, repoPath := range repoPaths {
		last[repoPath] = currentRepoWatchState(repoPath)
	}

	ticker := time.NewTicker(statusWatchPollInterval)
	defer ticker.Stop()

	timeout := time.NewTimer(statusWatchTimeout)
	defer timeout.Stop()

	for {
		select {
		case <-ticker.C:
			for _, repoPath := range repoPaths {
				next := currentRepoWatchState(repoPath)
				if next != last[repoPath] {
					return FileChangedMsg{RepoPath: repoPath}
				}
			}
		case <-timeout.C:
			return WatcherErrorMsg{Err: fmt.Errorf("watcher idle timeout: falling back to polling")}
		}
	}
}

func currentRepoWatchState(repoPath string) repoWatchState {
	return repoWatchState{
		status:         currentFileWatchState(filepath.Join(repoPath, ".ralph", "status.json")),
		circuitBreaker: currentFileWatchState(filepath.Join(repoPath, ".ralph", ".circuit_breaker_state")),
		progress:       currentFileWatchState(filepath.Join(repoPath, ".ralph", "progress.json")),
		config:         currentFileWatchState(filepath.Join(repoPath, ".ralphrc")),
	}
}

func currentFileWatchState(path string) fileWatchState {
	info, err := os.Stat(path)
	if err != nil {
		return fileWatchState{}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fileWatchState{
			exists:  true,
			modTime: info.ModTime(),
			size:    info.Size(),
		}
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write(data)
	return fileWatchState{
		exists:  true,
		modTime: info.ModTime(),
		size:    info.Size(),
		digest:  hasher.Sum64(),
	}
}

func shouldFallbackErrors(errs []error) bool {
	for _, err := range errs {
		if shouldFallbackToPolling(err) {
			return true
		}
	}
	return false
}

func shouldFallbackToPolling(err error) bool {
	return errors.Is(err, syscall.EMFILE) ||
		errors.Is(err, syscall.ENFILE) ||
		errors.Is(err, syscall.ENOSPC) ||
		strings.Contains(strings.ToLower(err.Error()), "too many open files")
}
