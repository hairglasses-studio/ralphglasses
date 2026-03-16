package process

import (
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/fsnotify/fsnotify"
)

// FileChangedMsg is sent when a watched status file changes.
type FileChangedMsg struct {
	RepoPath string
}

// WatchStatusFiles watches .ralph/ directories for status file changes
// and emits Bubble Tea messages.
func WatchStatusFiles(repoPaths []string) tea.Cmd {
	return func() tea.Msg {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return nil
		}

		for _, rp := range repoPaths {
			ralphDir := filepath.Join(rp, ".ralph")
			_ = watcher.Add(ralphDir)
		}

		// Block until an event arrives.
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return nil
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					base := filepath.Base(event.Name)
					if base == "status.json" || base == ".circuit_breaker_state" || base == "progress.json" {
						repoPath := filepath.Dir(filepath.Dir(event.Name))
						_ = watcher.Close()
						return FileChangedMsg{RepoPath: repoPath}
					}
				}
			case _, ok := <-watcher.Errors:
				if !ok {
					return nil
				}
			}
		}
	}
}
