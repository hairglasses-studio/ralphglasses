// Package themekit provides theme synchronization across TUI components.
//
// ThemeSync watches a theme file for changes, debounces rapid writes,
// and applies the resolved theme to all registered components. It is
// safe for concurrent use.
package themekit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
	"gopkg.in/yaml.v3"
)

// ThemeApplier is implemented by TUI components that need to react to
// theme changes. ApplyTheme is called under the ThemeSync read-lock,
// so implementations must not call back into ThemeSync.
type ThemeApplier interface {
	ApplyTheme(styles.Theme) error
}

// ThemeSync manages theme state and distributes theme updates to all
// registered components. It watches a YAML theme file for changes,
// debounces rapid writes (100ms default), and applies the new theme
// atomically to every registered applier.
type ThemeSync struct {
	themePath string
	debounce  time.Duration

	mu       sync.RWMutex
	current  styles.Theme
	appliers map[string]ThemeApplier

	// newWatcher is an fsnotify constructor, overridable for testing.
	newWatcher func() (*fsnotify.Watcher, error)
}

// NewThemeSync creates a ThemeSync that watches the given theme file path.
// It loads the current theme immediately. If the file does not exist or
// cannot be parsed, the error is returned.
func NewThemeSync(themePath string) (*ThemeSync, error) {
	theme, err := styles.LoadTheme(themePath)
	if err != nil {
		return nil, fmt.Errorf("load theme %s: %w", themePath, err)
	}

	return &ThemeSync{
		themePath:  themePath,
		debounce:   100 * time.Millisecond,
		current:    *theme,
		appliers:   make(map[string]ThemeApplier),
		newWatcher: fsnotify.NewWatcher,
	}, nil
}

// Register adds a named component that will receive theme updates.
// Returns an error if a component with the same name is already registered.
func (ts *ThemeSync) Register(name string, applier ThemeApplier) error {
	if name == "" {
		return errors.New("themekit: component name must not be empty")
	}
	if applier == nil {
		return errors.New("themekit: applier must not be nil")
	}

	ts.mu.Lock()
	defer ts.mu.Unlock()

	if _, exists := ts.appliers[name]; exists {
		return fmt.Errorf("themekit: component %q already registered", name)
	}
	ts.appliers[name] = applier
	return nil
}

// Unregister removes a named component from theme updates.
func (ts *ThemeSync) Unregister(name string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.appliers, name)
}

// Apply sets the given theme as current and distributes it to all
// registered appliers. It also updates the package-level styles.
// The first error encountered is returned; remaining appliers are
// still called.
func (ts *ThemeSync) Apply(theme styles.Theme) error {
	ts.mu.Lock()
	ts.current = theme
	appliers := make(map[string]ThemeApplier, len(ts.appliers))
	maps.Copy(appliers, ts.appliers)
	ts.mu.Unlock()

	// Update package-level styles.
	styles.ApplyTheme(&theme)

	var firstErr error
	for name, a := range appliers {
		if err := a.ApplyTheme(theme); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("themekit: apply to %q: %w", name, err)
		}
	}
	return firstErr
}

// Current returns a copy of the current theme.
func (ts *ThemeSync) Current() styles.Theme {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.current
}

// Watch watches the theme file for changes and applies updates until
// the context is cancelled. It blocks until the context is done.
func (ts *ThemeSync) Watch(ctx context.Context) error {
	watcher, err := ts.newWatcher()
	if err != nil {
		return fmt.Errorf("themekit: create watcher: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(ts.themePath); err != nil {
		return fmt.Errorf("themekit: watch %s: %w", ts.themePath, err)
	}

	var debounceTimer *time.Timer
	var debounceC <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return errors.New("themekit: watcher events channel closed")
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				if debounceTimer != nil {
					debounceTimer.Stop()
				}
				debounceTimer = time.NewTimer(ts.debounce)
				debounceC = debounceTimer.C
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return errors.New("themekit: watcher errors channel closed")
			}
			return fmt.Errorf("themekit: fsnotify: %w", err)

		case <-debounceC:
			debounceC = nil
			debounceTimer = nil
			ts.reload()
		}
	}
}

// reload reads the theme file and applies it.
func (ts *ThemeSync) reload() {
	theme, err := styles.LoadTheme(ts.themePath)
	if err != nil {
		return
	}
	_ = ts.Apply(*theme)
}

// Export serializes the current theme in the requested format.
// Supported formats: "json", "yaml".
func (ts *ThemeSync) Export(format string) ([]byte, error) {
	ts.mu.RLock()
	theme := ts.current
	ts.mu.RUnlock()

	switch format {
	case "json":
		return json.MarshalIndent(theme, "", "  ")
	case "yaml", "yml":
		return yaml.Marshal(theme)
	default:
		return nil, fmt.Errorf("themekit: unsupported export format %q (supported: json, yaml)", format)
	}
}

// Import deserializes a theme from the given data in the specified format,
// sets it as current, and applies it to all registered components.
// Supported formats: "json", "yaml".
func (ts *ThemeSync) Import(data []byte, format string) error {
	var theme styles.Theme

	switch format {
	case "json":
		if err := json.Unmarshal(data, &theme); err != nil {
			return fmt.Errorf("themekit: import json: %w", err)
		}
	case "yaml", "yml":
		if err := yaml.Unmarshal(data, &theme); err != nil {
			return fmt.Errorf("themekit: import yaml: %w", err)
		}
	default:
		return fmt.Errorf("themekit: unsupported import format %q (supported: json, yaml)", format)
	}

	return ts.Apply(theme)
}
