package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// errPlugin is a fakePlugin whose Init and Shutdown can return errors.
type errPlugin struct {
	name        string
	version     string
	initErr     error
	shutdownErr error
}

func (e *errPlugin) Name() string                              { return e.name }
func (e *errPlugin) Version() string                           { return e.version }
func (e *errPlugin) Init(_ context.Context, _ PluginHost) error { return e.initErr }
func (e *errPlugin) Shutdown() error                           { return e.shutdownErr }

// ── Enable / Disable / GetStatus ─────────────────────────────────────────────

func TestRegistry_EnableDisable_HappyPath(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&fakePlugin{name: "ep", version: "1.0"})

	// Plugin starts as StatusLoaded — activate it first.
	ctx := context.Background()
	if err := r.InitAll(ctx, nil); err != nil {
		t.Fatalf("InitAll: %v", err)
	}

	// Disable active plugin.
	if err := r.Disable("ep"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	status, ok := r.GetStatus("ep")
	if !ok {
		t.Fatal("GetStatus returned ok=false after Disable")
	}
	if status != StatusDisabled {
		t.Errorf("status after Disable = %q, want %q", status, StatusDisabled)
	}

	// Re-enable it.
	if err := r.Enable("ep"); err != nil {
		t.Fatalf("Enable: %v", err)
	}
	status, ok = r.GetStatus("ep")
	if !ok {
		t.Fatal("GetStatus returned ok=false after Enable")
	}
	if status != StatusActive {
		t.Errorf("status after Enable = %q, want %q", status, StatusActive)
	}
}

func TestRegistry_Disable_NotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	err := r.Disable("ghost")
	if err == nil {
		t.Fatal("expected error when disabling nonexistent plugin, got nil")
	}
}

func TestRegistry_Disable_NotActive(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&fakePlugin{name: "loaded-only", version: "1.0"})
	// Plugin is in StatusLoaded (not StatusActive) — Disable should fail.

	err := r.Disable("loaded-only")
	if err == nil {
		t.Fatal("expected error when disabling a non-active plugin, got nil")
	}
}

func TestRegistry_Enable_NotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	err := r.Enable("ghost")
	if err == nil {
		t.Fatal("expected error when enabling nonexistent plugin, got nil")
	}
}

func TestRegistry_Enable_NotDisabled(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&fakePlugin{name: "active", version: "1.0"})
	r.InitAll(context.Background(), nil) //nolint:errcheck

	// Plugin is StatusActive — Enable should fail because it is not disabled.
	err := r.Enable("active")
	if err == nil {
		t.Fatal("expected error when enabling a non-disabled plugin, got nil")
	}
}

func TestRegistry_GetStatus_NotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	_, ok := r.GetStatus("missing")
	if ok {
		t.Error("GetStatus returned ok=true for nonexistent plugin")
	}
}

func TestRegistry_GetStatus_Found(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&fakePlugin{name: "checkme", version: "1.0"})

	status, ok := r.GetStatus("checkme")
	if !ok {
		t.Fatal("GetStatus returned ok=false for registered plugin")
	}
	if status != StatusLoaded {
		t.Errorf("status = %q, want %q", status, StatusLoaded)
	}
}

// ── InitAll error path ────────────────────────────────────────────────────────

func TestRegistry_InitAll_RecordsFailureAndContinues(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	initErr := errors.New("init boom")
	r.Register(&errPlugin{name: "fail1", version: "1.0", initErr: initErr})
	r.Register(&fakePlugin{name: "ok1", version: "1.0"})

	err := r.InitAll(context.Background(), nil)
	if err == nil {
		t.Fatal("InitAll expected to return error for failing plugin, got nil")
	}

	// fail1 should be StatusFailed.
	status, _ := r.GetStatus("fail1")
	if status != StatusFailed {
		t.Errorf("fail1 status = %q, want %q", status, StatusFailed)
	}

	// ok1 should still be StatusActive.
	status, _ = r.GetStatus("ok1")
	if status != StatusActive {
		t.Errorf("ok1 status = %q, want %q", status, StatusActive)
	}
}

// ── ShutdownAll error path ────────────────────────────────────────────────────

func TestRegistry_ShutdownAll_ReturnsFirstError(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	shutErr := errors.New("shutdown boom")
	r.Register(&errPlugin{name: "sd1", version: "1.0", shutdownErr: shutErr})
	r.Register(&fakePlugin{name: "sd2", version: "1.0"})

	// Init both to put them in StatusActive.
	if err := r.InitAll(context.Background(), nil); err != nil {
		t.Fatalf("InitAll: %v", err)
	}

	err := r.ShutdownAll()
	if err == nil {
		t.Fatal("ShutdownAll expected to return error, got nil")
	}
}

func TestRegistry_ShutdownAll_SkipsNonActive(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	// Plugin with a shutdown error but it is in StatusLoaded (not Active).
	shutErr := errors.New("should not fire")
	r.Register(&errPlugin{name: "loaded", version: "1.0", shutdownErr: shutErr})

	// Don't call InitAll — plugin stays StatusLoaded.
	if err := r.ShutdownAll(); err != nil {
		t.Errorf("ShutdownAll on non-active plugin returned error: %v", err)
	}
}

// ── HandleSIGHUP ─────────────────────────────────────────────────────────────

func TestRegistry_HandleSIGHUP_ExitsOnContextCancel(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	r.HandleSIGHUP(ctx)

	// Cancel context and give the goroutine time to exit.
	cancel()
	time.Sleep(50 * time.Millisecond)
	// No assertion needed — test passes if it does not block or panic.
}

func TestRegistry_HandleSIGHUP_TriggersReload(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	r := NewRegistry()
	r.AddPluginDir(dir)

	// Register callback to detect reload.
	reloaded := make(chan struct{}, 1)
	r.OnReload(func(added, _ []string) {
		if len(added) > 0 {
			select {
			case reloaded <- struct{}{}:
			default:
			}
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	r.HandleSIGHUP(ctx)

	// Write a plugin manifest so Reload detects a new plugin.
	pluginDir := filepath.Join(dir, "sighup-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := PluginManifest{Name: "sighup-plugin", Version: "1.0", Protocol: "builtin"}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Send SIGHUP to ourselves.
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess: %v", err)
	}
	if err := proc.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("Signal SIGHUP: %v", err)
	}

	select {
	case <-reloaded:
		// success
	case <-ctx.Done():
		t.Fatal("SIGHUP reload not triggered within timeout")
	}
}

// ── LoadFromDir ───────────────────────────────────────────────────────────────

func TestLoadFromDir_NonexistentDir(t *testing.T) {
	t.Parallel()

	plugins, err := LoadFromDir("/nonexistent/path/for/loadfromdir")
	if err != nil {
		t.Fatalf("LoadFromDir nonexistent dir returned error: %v", err)
	}
	if plugins != nil {
		t.Errorf("LoadFromDir nonexistent dir returned %v, want nil", plugins)
	}
}

func TestLoadFromDir_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	plugins, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir empty dir: %v", err)
	}
	if plugins != nil {
		t.Errorf("LoadFromDir empty dir returned %v, want nil", plugins)
	}
}

func TestLoadFromDir_WithManifest(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	pluginDir := filepath.Join(dir, "lfd-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	m := PluginManifest{Name: "lfd-plugin", Version: "1.0", Protocol: "builtin"}
	data, _ := json.Marshal(m)
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// LoadFromDir returns nil plugins (builtin instantiation is caller's job),
	// but must not return an error.
	plugins, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir with manifest: %v", err)
	}
	// Per the implementation comment: returns nil, callers register builtins.
	if plugins != nil {
		t.Errorf("LoadFromDir returned %v, want nil (caller-side registration)", plugins)
	}
}
