package styles

import (
	"errors"
	"fmt"
	"hash/fnv"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/fsnotify/fsnotify"
)

const oneShotThemeReloadDebounce = 100 * time.Millisecond
const defaultThemeWatchPollInterval = 100 * time.Millisecond

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
	path         string
	watcher      *fsnotify.Watcher
	program      *tea.Program
	debounce     time.Duration
	pollInterval time.Duration

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
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("watch %s: %w", path, err)
	}

	tw := &ThemeWatcher{
		path:         path,
		program:      program,
		debounce:     100 * time.Millisecond,
		pollInterval: defaultThemeWatchPollInterval,
		closeCh:      make(chan struct{}),
	}
	for _, opt := range opts {
		opt(tw)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		if shouldFallbackToThemePolling(err) {
			go tw.pollLoop(currentThemeWatchState(path))
			return tw, nil
		}
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	if err := watcher.Add(path); err != nil {
		_ = watcher.Close()
		if shouldFallbackToThemePolling(err) {
			go tw.pollLoop(currentThemeWatchState(path))
			return tw, nil
		}
		return nil, fmt.Errorf("watch %s: %w", path, err)
	}

	tw.watcher = watcher
	go tw.loop()
	return tw, nil
}

// newThemeWatcherWithFSNotify is an internal constructor that accepts a
// pre-built fsnotify.Watcher, used for testing.
func newThemeWatcherWithFSNotify(path string, watcher *fsnotify.Watcher, program *tea.Program, opts ...ThemeWatcherOption) *ThemeWatcher {
	tw := &ThemeWatcher{
		path:         path,
		watcher:      watcher,
		program:      program,
		debounce:     100 * time.Millisecond,
		pollInterval: defaultThemeWatchPollInterval,
		closeCh:      make(chan struct{}),
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
	if tw.watcher == nil {
		return nil
	}
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

func (tw *ThemeWatcher) pollLoop(lastState themeWatchState) {
	interval := tw.pollInterval
	if interval <= 0 {
		interval = defaultThemeWatchPollInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var debounceTimer *time.Timer
	var debounceC <-chan time.Time

	for {
		select {
		case <-tw.closeCh:
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return
		case <-ticker.C:
			nextState := currentThemeWatchState(tw.path)
			if nextState != lastState {
				lastState = nextState
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.NewTimer(tw.debounce)
				debounceC = debounceTimer.C
			}
		case <-debounceC:
			debounceC = nil
			debounceTimer = nil
			tw.reload()
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
		if _, err := os.Stat(path); err != nil {
			return ThemeWatcherErrorMsg{Err: fmt.Errorf("watch %s: %w", path, err)}
		}

		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			if shouldFallbackToThemePolling(err) {
				return watchThemeFileByPolling(path)
			}
			return ThemeWatcherErrorMsg{Err: fmt.Errorf("create watcher: %w", err)}
		}

		if err := watcher.Add(path); err != nil {
			_ = watcher.Close()
			if shouldFallbackToThemePolling(err) {
				return watchThemeFileByPolling(path)
			}
			return ThemeWatcherErrorMsg{Err: fmt.Errorf("watch %s: %w", path, err)}
		}

		var timer *time.Timer
		var timerC <-chan time.Time

		stopTimer := func() {
			if timer == nil {
				return
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}

		defer func() {
			stopTimer()
			_ = watcher.Close()
		}()

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return ThemeWatcherErrorMsg{Err: fmt.Errorf("watcher events channel closed")}
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					stopTimer()
					timer = time.NewTimer(oneShotThemeReloadDebounce)
					timerC = timer.C
				}

			case <-timerC:
				timerC = nil
				theme, err := LoadTheme(path)
				if err != nil {
					return ThemeWatcherErrorMsg{Err: fmt.Errorf("load theme: %w", err)}
				}
				ApplyTheme(theme)
				return ThemeChangedMsg{Theme: theme, Path: path}

			case watchErr, ok := <-watcher.Errors:
				if !ok {
					return ThemeWatcherErrorMsg{Err: fmt.Errorf("watcher errors channel closed")}
				}
				if shouldFallbackToThemePolling(watchErr) {
					return watchThemeFileByPolling(path)
				}
				return ThemeWatcherErrorMsg{Err: fmt.Errorf("fsnotify: %w", watchErr)}
			}
		}
	}
}

type themeWatchState struct {
	exists  bool
	modTime time.Time
	size    int64
	digest  uint64
}

func watchThemeFileByPolling(path string) tea.Msg {
	lastState := currentThemeWatchState(path)
	ticker := time.NewTicker(defaultThemeWatchPollInterval)
	defer ticker.Stop()

	var debounceTimer *time.Timer
	var debounceC <-chan time.Time

	for {
		select {
		case <-ticker.C:
			nextState := currentThemeWatchState(path)
			if nextState != lastState {
				lastState = nextState
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.NewTimer(oneShotThemeReloadDebounce)
				debounceC = debounceTimer.C
			}
		case <-debounceC:
			debounceC = nil
			debounceTimer = nil
			theme, err := LoadTheme(path)
			if err != nil {
				return ThemeWatcherErrorMsg{Err: fmt.Errorf("load theme: %w", err)}
			}
			ApplyTheme(theme)
			return ThemeChangedMsg{Theme: theme, Path: path}
		}
	}
}

func currentThemeWatchState(path string) themeWatchState {
	info, err := os.Stat(path)
	if err != nil {
		return themeWatchState{}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return themeWatchState{
			exists:  true,
			modTime: info.ModTime(),
			size:    info.Size(),
		}
	}

	hasher := fnv.New64a()
	_, _ = hasher.Write(data)
	return themeWatchState{
		exists:  true,
		modTime: info.ModTime(),
		size:    info.Size(),
		digest:  hasher.Sum64(),
	}
}

func shouldFallbackToThemePolling(err error) bool {
	return errors.Is(err, syscall.EMFILE) ||
		errors.Is(err, syscall.ENFILE) ||
		errors.Is(err, syscall.ENOSPC) ||
		strings.Contains(strings.ToLower(err.Error()), "too many open files")
}
