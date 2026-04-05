package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// writeConfig is a helper that writes a Config as JSON to the given path.
func writeConfig(t *testing.T, path string, cfg Config) {
	t.Helper()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestWatcher_DetectsFileChange(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	// Write initial config.
	writeConfig(t, cfgPath, Config{DefaultProvider: "claude", MaxWorkers: 2})

	w := NewWatcher(cfgPath)
	w.debounce = 50 * time.Millisecond // speed up for tests

	var got atomic.Value
	ready := make(chan struct{}, 1)
	w.OnChange(func(cfg Config) {
		got.Store(cfg)
		select {
		case ready <- struct{}{}:
		default:
		}
	})

	ctx := t.Context()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Verify initial load.
	initial := w.Current()
	if initial.DefaultProvider != "claude" {
		t.Errorf("initial DefaultProvider = %q, want %q", initial.DefaultProvider, "claude")
	}

	// Modify the config file.
	writeConfig(t, cfgPath, Config{DefaultProvider: "gemini", MaxWorkers: 8})

	// Wait for callback.
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for change callback")
	}

	newCfg, ok := got.Load().(Config)
	if !ok {
		t.Fatal("callback was not called")
	}
	if newCfg.DefaultProvider != "gemini" {
		t.Errorf("callback DefaultProvider = %q, want %q", newCfg.DefaultProvider, "gemini")
	}
	if newCfg.MaxWorkers != 8 {
		t.Errorf("callback MaxWorkers = %d, want 8", newCfg.MaxWorkers)
	}

	// Current() should also reflect the new config.
	cur := w.Current()
	if cur.DefaultProvider != "gemini" {
		t.Errorf("Current() DefaultProvider = %q, want %q", cur.DefaultProvider, "gemini")
	}
}

func TestWatcher_Debounce(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	writeConfig(t, cfgPath, Config{MaxWorkers: 1})

	w := NewWatcher(cfgPath)
	w.debounce = 200 * time.Millisecond

	var callCount atomic.Int32
	var lastCfg atomic.Value
	done := make(chan struct{}, 1)

	w.OnChange(func(cfg Config) {
		callCount.Add(1)
		lastCfg.Store(cfg)
		select {
		case done <- struct{}{}:
		default:
		}
	})

	ctx := t.Context()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Rapid successive writes within debounce window.
	for i := range 5 {
		writeConfig(t, cfgPath, Config{MaxWorkers: i + 10})
		time.Sleep(20 * time.Millisecond) // well within 200ms debounce
	}

	// Wait for the debounced callback.
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for debounced callback")
	}

	// Give a little extra time to ensure no additional callbacks fire.
	time.Sleep(300 * time.Millisecond)

	count := callCount.Load()
	if count != 1 {
		t.Errorf("expected 1 debounced callback, got %d", count)
	}

	// The last write should be the one that took effect (MaxWorkers=14).
	cfg, ok := lastCfg.Load().(Config)
	if !ok {
		t.Fatal("callback was not called")
	}
	if cfg.MaxWorkers != 14 {
		t.Errorf("debounced MaxWorkers = %d, want 14", cfg.MaxWorkers)
	}
}

func TestWatcher_InvalidConfigKeepsOld(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	writeConfig(t, cfgPath, Config{DefaultProvider: "claude", MaxWorkers: 4})

	w := NewWatcher(cfgPath)
	w.debounce = 50 * time.Millisecond

	var callbackCalled atomic.Bool
	w.OnChange(func(cfg Config) {
		callbackCalled.Store(true)
	})

	ctx := t.Context()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Verify initial load.
	initial := w.Current()
	if initial.DefaultProvider != "claude" {
		t.Fatalf("initial DefaultProvider = %q, want %q", initial.DefaultProvider, "claude")
	}

	// Write invalid JSON.
	if err := os.WriteFile(cfgPath, []byte("{invalid json!!!"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for the debounce period to pass and the reload to happen.
	time.Sleep(500 * time.Millisecond)

	// The old config should be preserved.
	cur := w.Current()
	if cur.DefaultProvider != "claude" {
		t.Errorf("after invalid write, DefaultProvider = %q, want %q (old)", cur.DefaultProvider, "claude")
	}
	if cur.MaxWorkers != 4 {
		t.Errorf("after invalid write, MaxWorkers = %d, want 4 (old)", cur.MaxWorkers)
	}

	// Callback should NOT have been called for invalid config.
	if callbackCalled.Load() {
		t.Error("callback should not fire for invalid config")
	}
}

func TestWatcher_CallbackReceivesNewConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	writeConfig(t, cfgPath, Config{DefaultProvider: "claude"})

	w := NewWatcher(cfgPath)
	w.debounce = 50 * time.Millisecond

	var received []Config
	var mu sync.Mutex
	ready := make(chan struct{}, 5)

	w.OnChange(func(cfg Config) {
		mu.Lock()
		received = append(received, cfg)
		mu.Unlock()
		select {
		case ready <- struct{}{}:
		default:
		}
	})

	ctx := t.Context()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Write first change.
	writeConfig(t, cfgPath, Config{DefaultProvider: "gemini", MaxWorkers: 3})
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for first callback")
	}

	// Write second change (after debounce settles).
	time.Sleep(100 * time.Millisecond)
	writeConfig(t, cfgPath, Config{DefaultProvider: "codex", MaxWorkers: 7})
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for second callback")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(received) < 2 {
		t.Fatalf("expected at least 2 callbacks, got %d", len(received))
	}

	if received[0].DefaultProvider != "gemini" {
		t.Errorf("first callback DefaultProvider = %q, want %q", received[0].DefaultProvider, "gemini")
	}
	if received[1].DefaultProvider != "codex" {
		t.Errorf("second callback DefaultProvider = %q, want %q", received[1].DefaultProvider, "codex")
	}
}

func TestWatcher_StopStopsWatching(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	writeConfig(t, cfgPath, Config{DefaultProvider: "claude"})

	w := NewWatcher(cfgPath)
	w.debounce = 50 * time.Millisecond

	var callbackCalled atomic.Bool
	w.OnChange(func(cfg Config) {
		callbackCalled.Store(true)
	})

	ctx := t.Context()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Stop the watcher.
	w.Stop()

	// Write a change after stop.
	writeConfig(t, cfgPath, Config{DefaultProvider: "gemini"})

	// Wait enough time for a callback to fire if the watcher were still running.
	time.Sleep(300 * time.Millisecond)

	if callbackCalled.Load() {
		t.Error("callback should not fire after Stop()")
	}

	// Double-stop should not panic.
	w.Stop()
}

func TestWatcher_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	writeConfig(t, cfgPath, Config{DefaultProvider: "claude"})

	w := NewWatcher(cfgPath)
	w.debounce = 50 * time.Millisecond

	var callbackCalled atomic.Bool
	w.OnChange(func(cfg Config) {
		callbackCalled.Store(true)
	})

	ctx, cancel := context.WithCancel(context.Background())

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Cancel the context.
	cancel()

	// Give the loop time to exit.
	time.Sleep(100 * time.Millisecond)

	// Write a change after cancellation.
	writeConfig(t, cfgPath, Config{DefaultProvider: "gemini"})
	time.Sleep(200 * time.Millisecond)

	if callbackCalled.Load() {
		t.Error("callback should not fire after context cancellation")
	}
}

func TestWatcher_MultipleCallbacks(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	writeConfig(t, cfgPath, Config{DefaultProvider: "claude"})

	w := NewWatcher(cfgPath)
	w.debounce = 50 * time.Millisecond

	var count1, count2 atomic.Int32
	ready := make(chan struct{}, 2)

	w.OnChange(func(cfg Config) {
		count1.Add(1)
		select {
		case ready <- struct{}{}:
		default:
		}
	})
	w.OnChange(func(cfg Config) {
		count2.Add(1)
		select {
		case ready <- struct{}{}:
		default:
		}
	})

	ctx := t.Context()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	writeConfig(t, cfgPath, Config{DefaultProvider: "gemini"})

	// Wait for both callbacks.
	for range 2 {
		select {
		case <-ready:
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for callbacks")
		}
	}

	if count1.Load() != 1 {
		t.Errorf("callback 1 called %d times, want 1", count1.Load())
	}
	if count2.Load() != 1 {
		t.Errorf("callback 2 called %d times, want 1", count2.Load())
	}
}

func TestWatcher_MissingFileAtStart(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	// Do NOT create the file.
	w := NewWatcher(cfgPath)
	w.debounce = 50 * time.Millisecond

	// Should start with zero-value config.
	cur := w.Current()
	if cur.DefaultProvider != "" {
		t.Errorf("expected empty DefaultProvider for missing file, got %q", cur.DefaultProvider)
	}

	ready := make(chan struct{}, 1)
	w.OnChange(func(cfg Config) {
		select {
		case ready <- struct{}{}:
		default:
		}
	})

	ctx := t.Context()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer w.Stop()

	// Now create the file.
	writeConfig(t, cfgPath, Config{DefaultProvider: "gemini"})

	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for callback after file creation")
	}

	cur = w.Current()
	if cur.DefaultProvider != "gemini" {
		t.Errorf("after file creation, DefaultProvider = %q, want %q", cur.DefaultProvider, "gemini")
	}
}

func TestDirOf(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/user/config.json", "/home/user"},
		{"/tmp/a", "/tmp"},
		{"config.json", "."},
	}
	for _, tt := range tests {
		got := dirOf(tt.input)
		if got != tt.want {
			t.Errorf("dirOf(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBaseOf(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/home/user/config.json", "config.json"},
		{"/tmp/a", "a"},
		{"config.json", "config.json"},
	}
	for _, tt := range tests {
		got := baseOf(tt.input)
		if got != tt.want {
			t.Errorf("baseOf(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
