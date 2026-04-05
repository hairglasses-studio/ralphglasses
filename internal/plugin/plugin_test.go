package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// fakePlugin implements Plugin for testing.
type fakePlugin struct {
	name    string
	version string
	onEvent func(ctx context.Context, event Event) error
}

func (f *fakePlugin) Name() string                               { return f.name }
func (f *fakePlugin) Version() string                            { return f.version }
func (f *fakePlugin) Init(_ context.Context, _ PluginHost) error { return nil }
func (f *fakePlugin) Shutdown() error                            { return nil }
func (f *fakePlugin) OnEvent(ctx context.Context, event Event) error {
	if f.onEvent != nil {
		return f.onEvent(ctx, event)
	}
	return nil
}

func TestNewRegistry(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry returned nil")
	}
	if got := len(r.List()); got != 0 {
		t.Errorf("new registry has %d plugins, want 0", got)
	}
}

func TestRegistryRegisterAndList(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p1 := &fakePlugin{name: "p1", version: "1.0"}
	p2 := &fakePlugin{name: "p2", version: "2.0"}

	r.Register(p1)
	r.Register(p2)

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("List() returned %d plugins, want 2", len(list))
	}
	if list[0].Name != "p1" || list[1].Name != "p2" {
		t.Errorf("List() = [%s, %s], want [p1, p2]", list[0].Name, list[1].Name)
	}
}

func TestRegistryListReturnsSnapshot(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&fakePlugin{name: "a", version: "1.0"})

	snapshot := r.List()
	// Mutating the snapshot should not affect the registry.
	snapshot[0].Name = "mutated"

	list := r.List()
	if list[0].Name == "mutated" {
		t.Error("List() returned a reference to internal slice, not a copy")
	}
}

func TestRegistryDispatch(t *testing.T) {
	t.Parallel()

	var received []Event
	var mu sync.Mutex

	p := &fakePlugin{
		name:    "collector",
		version: "1.0",
		onEvent: func(_ context.Context, event Event) error {
			mu.Lock()
			received = append(received, event)
			mu.Unlock()
			return nil
		},
	}

	r := NewRegistry()
	r.Register(p)

	evt := Event{Type: "session.start", Repo: "/tmp/repo", Payload: map[string]any{"id": "s1"}}
	r.Dispatch(context.Background(), evt)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("plugin received %d events, want 1", len(received))
	}
	if received[0].Type != "session.start" || received[0].Repo != "/tmp/repo" {
		t.Errorf("received event = %+v, want type=session.start repo=/tmp/repo", received[0])
	}
}

func TestRegistryDispatchContinuesOnError(t *testing.T) {
	t.Parallel()

	var called bool
	errPlugin := &fakePlugin{
		name:    "failing",
		version: "1.0",
		onEvent: func(_ context.Context, _ Event) error {
			return errors.New("boom")
		},
	}
	okPlugin := &fakePlugin{
		name:    "ok",
		version: "1.0",
		onEvent: func(_ context.Context, _ Event) error {
			called = true
			return nil
		},
	}

	r := NewRegistry()
	r.Register(errPlugin)
	r.Register(okPlugin)

	r.Dispatch(context.Background(), Event{Type: "test"})

	if !called {
		t.Error("second plugin was not called after first plugin returned error")
	}
}

func TestRegistryDispatchFunc(t *testing.T) {
	t.Parallel()

	var received Event
	p := &fakePlugin{
		name:    "df",
		version: "1.0",
		onEvent: func(_ context.Context, e Event) error {
			received = e
			return nil
		},
	}

	r := NewRegistry()
	r.Register(p)

	fn := r.DispatchFunc()
	fn(context.Background(), "loop.done", "/tmp/r", map[string]any{"count": 5})

	if received.Type != "loop.done" || received.Repo != "/tmp/r" {
		t.Errorf("DispatchFunc event = %+v, want type=loop.done repo=/tmp/r", received)
	}
}

func TestRegistryString(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&fakePlugin{name: "x", version: "1.0"})

	got := r.String()
	want := "plugin.Registry{count: 1, grpc: 0, tools: 0}"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	var wg sync.WaitGroup
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			r.Register(&fakePlugin{name: fmt.Sprintf("c-%d", n), version: "1.0"})
			r.Dispatch(context.Background(), Event{Type: "test"})
			r.List()
		}(i)
	}
	wg.Wait()

	if got := len(r.List()); got != 50 {
		t.Errorf("after concurrent registration, got %d plugins, want 50", got)
	}
}

func TestLoadDirNonExistent(t *testing.T) {
	t.Parallel()
	plugins, err := LoadDir("/nonexistent/path/that/does/not/exist")
	if err != nil {
		t.Fatalf("LoadDir on non-existent dir returned error: %v", err)
	}
	if plugins != nil {
		t.Errorf("LoadDir on non-existent dir returned %v, want nil", plugins)
	}
}

func TestLoadDirEmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	plugins, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir on empty dir returned error: %v", err)
	}
	if plugins != nil {
		t.Errorf("LoadDir on empty dir returned %v, want nil", plugins)
	}
}

func TestLoadDirIgnoresNonSoFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create non-.so files
	for _, name := range []string{"readme.md", "config.json", "plugin.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	plugins, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	if plugins != nil {
		t.Errorf("LoadDir returned %v, want nil (no .so files)", plugins)
	}
}

func TestLoadDirWithSoFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create .so files (stub loading returns nil)
	for _, name := range []string{"myplugin.so", "other.so"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Also a non-.so file
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Current implementation logs but returns nil (stub)
	plugins, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	if plugins != nil {
		t.Errorf("LoadDir returned %v, want nil (stub implementation)", plugins)
	}
}

func TestLoadDirSkipsSubdirectories(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a subdirectory named with .so extension
	if err := os.Mkdir(filepath.Join(dir, "subdir.so"), 0o755); err != nil {
		t.Fatal(err)
	}

	plugins, err := LoadDir(dir)
	if err != nil {
		t.Fatalf("LoadDir returned error: %v", err)
	}
	if plugins != nil {
		t.Errorf("LoadDir returned %v, want nil (directories should be skipped)", plugins)
	}
}
