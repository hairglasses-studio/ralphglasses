package styles

import (
	"fmt"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

// ThemeChangedMsg is sent to the BubbleTea program when the watched theme file
// changes on disk and the new theme has been applied to package-level styles.
type ThemeChangedMsg struct {
	Theme *Theme
	Path  string
}

// ThemeWatcherErrorMsg is sent when the theme watcher encounters an error.
type ThemeWatcherErrorMsg struct {
	Err error
}

// ThemeWatcher watches a theme YAML file for changes and applies new themes
// at runtime without restarting the TUI. It debounces rapid writes (e.g.,
// editors that write+rename) and only fires after the file is stable.
type ThemeWatcher struct {
	path     string
	watcher  *fsnotify.Watcher
	program  *tea.Program
	debounce time.Duration

	mu      sync.Mutex
	closed  bool
	closeCh chan struct{}
}

// ThemeWatcherOption configures a ThemeWatcher.
type ThemeWatcherOption func(*ThemeWatcher)

// WithDebounce sets the debounce duration for file change events.
// Defaults to 100ms if not set.
func WithDebounce(d time.Duration) ThemeWatcherOption {
	return func(tw *ThemeWatcher) {
		tw.debounce = d
	}
}

// NewThemeWatcher creates a ThemeWatcher that watches the given YAML file path
// and sends ThemeChangedMsg to the provided BubbleTea program when the theme
// changes. The watcher debounces rapid writes; the default debounce is 100ms.
func NewThemeWatcher(path string, program *tea.Program, opts ...ThemeWatcherOption) (*ThemeWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	tw := &ThemeWatcher{
		path:     path,
		watcher:  watcher,
		program:  program,
		debounce: 100 * time.Millisecond,
		closeCh:  make(chan struct{}),
	}
	for _, opt := range opts {
		opt(tw)
	}

	if err := watcher.Add(path); err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("watch %s: %w", path, err)
	}

	go tw.loop()
	return tw, nil
}

// newThemeWatcherWithFSNotify is an internal constructor that accepts a
// pre-built fsnotify.Watcher, used for testing.
func newThemeWatcherWithFSNotify(path string, watcher *fsnotify.Watcher, program *tea.Program, opts ...ThemeWatcherOption) *ThemeWatcher {
	tw := &ThemeWatcher{
		path:     path,
		watcher:  watcher,
		program:  program,
		debounce: 100 * time.Millisecond,
		closeCh:  make(chan struct{}),
	}
	for _, opt := range opts {
		opt(tw)
	}
	go tw.loop()
	return tw
}

// Close stops the watcher and releases resources. It is safe to call multiple
// times.
func (tw *ThemeWatcher) Close() error {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.closed {
		return nil
	}
	tw.closed = true
	close(tw.closeCh)
	return tw.watcher.Close()
}

// loop is the main event loop that listens for fsnotify events and debounces
// them before loading and applying the new theme.
func (tw *ThemeWatcher) loop() {
	var timer *time.Timer

	for {
		select {
		case <-tw.closeCh:
			if timer != nil {
				timer.Stop()
			}
			return

		case event, ok := <-tw.watcher.Events:
			if !ok {
				tw.sendError(fmt.Errorf("watcher events channel closed"))
				return
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				// Reset the debounce timer on each write event.
				if timer != nil {
					timer.Stop()
				}
				timer = time.AfterFunc(tw.debounce, func() {
					tw.reload()
				})
			}

		case err, ok := <-tw.watcher.Errors:
			if !ok {
				tw.sendError(fmt.Errorf("watcher errors channel closed"))
				return
			}
			tw.sendError(fmt.Errorf("fsnotify: %w", err))
			return
		}
	}
}

// reload loads the theme file, applies it, and sends a ThemeChangedMsg to
// the BubbleTea program.
func (tw *ThemeWatcher) reload() {
	theme, err := LoadTheme(tw.path)
	if err != nil {
		tw.sendError(fmt.Errorf("load theme %s: %w", tw.path, err))
		return
	}
	ApplyTheme(theme)
	if tw.program != nil {
		tw.program.Send(ThemeChangedMsg{Theme: theme, Path: tw.path})
	}
}

// sendError sends a ThemeWatcherErrorMsg to the BubbleTea program.
func (tw *ThemeWatcher) sendError(err error) {
	if tw.program != nil {
		tw.program.Send(ThemeWatcherErrorMsg{Err: err})
	}
}

// WatchThemeFile returns a tea.Cmd that creates a ThemeWatcher and blocks until
// the watched file changes, returning a ThemeChangedMsg. This is a one-shot
// command suitable for use in BubbleTea's Init or Update cycle; the caller
// should re-issue the command after handling the message to keep watching.
func WatchThemeFile(path string) tea.Cmd {
	return func() tea.Msg {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			return ThemeWatcherErrorMsg{Err: fmt.Errorf("create watcher: %w", err)}
		}

		if err := watcher.Add(path); err != nil {
			_ = watcher.Close()
			return ThemeWatcherErrorMsg{Err: fmt.Errorf("watch %s: %w", path, err)}
		}

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return ThemeWatcherErrorMsg{Err: fmt.Errorf("watcher events channel closed")}
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					_ = watcher.Close()
					theme, err := LoadTheme(path)
					if err != nil {
						return ThemeWatcherErrorMsg{Err: fmt.Errorf("load theme: %w", err)}
					}
					ApplyTheme(theme)
					return ThemeChangedMsg{Theme: theme, Path: path}
				}

			case watchErr, ok := <-watcher.Errors:
				_ = watcher.Close()
				if !ok {
					return ThemeWatcherErrorMsg{Err: fmt.Errorf("watcher errors channel closed")}
				}
				return ThemeWatcherErrorMsg{Err: fmt.Errorf("fsnotify: %w", watchErr)}
			}
		}
	}
}
