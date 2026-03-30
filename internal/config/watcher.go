package config

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches a config file for changes and notifies registered
// subscribers when the config is updated. It validates new config before
// applying and ignores rapid successive writes via a 500ms debounce.
type Watcher struct {
	path    string
	current atomic.Pointer[Config]

	mu        sync.Mutex
	callbacks []func(Config)

	watcher  *fsnotify.Watcher
	stopOnce sync.Once
	done     chan struct{}

	// debounce controls how long to wait after a write before reloading.
	// Exported for testing; defaults to 500ms.
	debounce time.Duration

	// newWatcher is an fsnotify constructor, overridable for testing.
	newWatcher func() (*fsnotify.Watcher, error)
}

// NewWatcher creates a config file watcher for the given path.
// It loads the current config immediately. If the file does not exist,
// the watcher starts with a zero-value Config and will pick up the file
// once it is created.
func NewWatcher(path string) *Watcher {
	w := &Watcher{
		path:       path,
		done:       make(chan struct{}),
		debounce:   500 * time.Millisecond,
		newWatcher: fsnotify.NewWatcher,
	}

	// Load initial config (best-effort).
	cfg, err := Load(path)
	if err != nil {
		slog.Warn("config watcher: initial load failed, using defaults", "path", path, "error", err)
		cfg = &Config{}
	}
	w.current.Store(cfg)

	return w
}

// Current returns the most recently loaded valid config.
func (w *Watcher) Current() Config {
	if p := w.current.Load(); p != nil {
		return *p
	}
	return Config{}
}

// OnChange registers a callback that fires when the config changes.
// The callback receives a copy of the new config. Callbacks are invoked
// synchronously in order of registration; keep them fast to avoid blocking
// the watcher loop.
func (w *Watcher) OnChange(fn func(Config)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, fn)
}

// Start begins watching the config file for changes. It blocks until
// the context is cancelled or Stop is called. Returns nil on clean
// shutdown.
func (w *Watcher) Start(ctx context.Context) error {
	fw, err := w.newWatcher()
	if err != nil {
		return err
	}
	w.watcher = fw

	// Watch the directory containing the config file so we catch
	// create events (editors often write to a temp file then rename).
	dir := dirOf(w.path)
	if err := fw.Add(dir); err != nil {
		_ = fw.Close()
		return err
	}

	go w.loop(ctx)

	return nil
}

// Stop stops the watcher. Safe to call multiple times.
func (w *Watcher) Stop() {
	w.stopOnce.Do(func() {
		if w.watcher != nil {
			_ = w.watcher.Close()
		}
		close(w.done)
	})
}

// loop is the main event loop. It debounces rapid writes and reloads
// the config after the debounce period.
func (w *Watcher) loop(ctx context.Context) {
	var debounceTimer *time.Timer
	var debounceC <-chan time.Time

	base := baseOf(w.path)

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case <-w.done:
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			// Only react to writes/creates/renames for our config file.
			if baseOf(event.Name) != base {
				continue
			}
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) {
				continue
			}
			// Reset debounce timer.
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.NewTimer(w.debounce)
			debounceC = debounceTimer.C

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("config watcher: fsnotify error", "error", err)

		case <-debounceC:
			debounceC = nil
			debounceTimer = nil
			w.reload()
		}
	}
}

// reload reads the config file, validates it, and notifies subscribers
// if the config changed. Invalid configs are logged but not applied.
func (w *Watcher) reload() {
	data, err := os.ReadFile(w.path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			slog.Warn("config watcher: read failed", "path", w.path, "error", err)
		}
		return
	}

	var newCfg Config
	if err := json.Unmarshal(data, &newCfg); err != nil {
		slog.Warn("config watcher: invalid JSON, keeping old config", "path", w.path, "error", err)
		return
	}

	if warnings := ValidateConfig(&newCfg); len(warnings) > 0 {
		for _, warn := range warnings {
			slog.Warn("config watcher: validation warning", "warning", warn)
		}
		// Validation warnings are non-fatal; we still apply the config.
		// Only JSON parse errors prevent application.
	}

	w.current.Store(&newCfg)

	// Notify subscribers.
	w.mu.Lock()
	cbs := make([]func(Config), len(w.callbacks))
	copy(cbs, w.callbacks)
	w.mu.Unlock()

	for _, fn := range cbs {
		fn(newCfg)
	}
}

// dirOf returns the directory portion of a path.
func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return "."
}

// baseOf returns the base name of a path.
func baseOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return path
}
