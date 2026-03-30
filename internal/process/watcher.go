package process

import (
	"fmt"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
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
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return WatcherErrorMsg{Err: fmt.Errorf("create watcher: %w", err)}
		}
		return watchWithWatcher(repoPaths, watcher)
	}
}

// watchWithWatcher runs the watch loop using the provided watcher. Extracted to
// allow injection of a pre-built watcher in tests.
func watchWithWatcher(repoPaths []string, watcher *fsnotify.Watcher) tea.Msg {
	var addErrors []error
	for _, rp := range repoPaths {
		ralphDir := filepath.Join(rp, ".ralph")
		if err := watcher.Add(ralphDir); err != nil {
			addErrors = append(addErrors, fmt.Errorf("watch %s: %w", ralphDir, err))
		}
		// Also watch the repo root for .ralphrc config changes.
		if err := watcher.Add(rp); err != nil {
			addErrors = append(addErrors, fmt.Errorf("watch %s: %w", rp, err))
		}
	}
	// If ALL watches failed, report error and let TUI fall back to polling
	if len(addErrors) == len(repoPaths)*2 && len(repoPaths) > 0 {
		_ = watcher.Close()
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
			return WatcherErrorMsg{Err: fmt.Errorf("fsnotify: %w", watchErr)}
		case <-time.After(2 * time.Second):
			_ = watcher.Close()
			return WatcherErrorMsg{Err: fmt.Errorf("watcher idle timeout: falling back to polling")}
		}
	}
}
