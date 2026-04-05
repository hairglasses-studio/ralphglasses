package plugin

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

func TestRegistry_RegisterDuplicate(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p := &fakePlugin{name: "dup", version: "1.0"}
	if err := r.Register(p); err != nil {
		t.Fatalf("first Register: %v", err)
	}

	err := r.Register(&fakePlugin{name: "dup", version: "2.0"})
	if err == nil {
		t.Fatal("expected error on duplicate registration, got nil")
	}
	if got := err.Error(); got != `plugin "dup" already registered` {
		t.Errorf("error = %q, want %q", got, `plugin "dup" already registered`)
	}

	// Registry should still have exactly one entry.
	if n := len(r.List()); n != 1 {
		t.Errorf("List() length = %d, want 1", n)
	}
}

func TestRegistry_GetNonexistent(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	p, ok := r.Get("nonexistent")
	if ok {
		t.Error("Get returned ok=true for nonexistent plugin")
	}
	if p != nil {
		t.Error("Get returned non-nil plugin for nonexistent name")
	}
}

func TestRegistry_GetExisting(t *testing.T) {
	t.Parallel()
	r := NewRegistry()
	r.Register(&fakePlugin{name: "exists", version: "1.0"})

	p, ok := r.Get("exists")
	if !ok {
		t.Fatal("Get returned ok=false for existing plugin")
	}
	if p.Name() != "exists" {
		t.Errorf("Get().Name() = %q, want %q", p.Name(), "exists")
	}
}

func TestRegistry_ListEmpty(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	list := r.List()
	if list == nil {
		t.Fatal("List() on empty registry returned nil, want empty slice")
	}
	if len(list) != 0 {
		t.Errorf("List() length = %d, want 0", len(list))
	}
}

func TestRegistry_ListMultiple(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	names := []string{"alpha", "beta", "gamma", "delta"}
	for _, n := range names {
		r.Register(&fakePlugin{name: n, version: "1.0"})
	}

	list := r.List()
	if len(list) != len(names) {
		t.Fatalf("List() length = %d, want %d", len(list), len(names))
	}

	// Verify registration order is preserved.
	for i, info := range list {
		if info.Name != names[i] {
			t.Errorf("List()[%d].Name = %q, want %q", i, info.Name, names[i])
		}
		if info.Status != StatusLoaded {
			t.Errorf("List()[%d].Status = %q, want %q", i, info.Status, StatusLoaded)
		}
	}
}

func TestRegistry_ConcurrentRegister(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	const n = 100
	var wg sync.WaitGroup
	errs := make(chan error, n)

	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := r.Register(&fakePlugin{
				name:    fmt.Sprintf("concurrent-%d", idx),
				version: "1.0",
			})
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("unexpected registration error: %v", err)
	}

	list := r.List()
	if len(list) != n {
		t.Errorf("List() length = %d, want %d", len(list), n)
	}
}

func TestRegistry_DispatchToolCallNotFound(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	_, err := r.DispatchToolCall(context.Background(), "nonexistent-tool", nil)
	if err == nil {
		t.Fatal("expected error for unregistered tool, got nil")
	}
}

func TestRegistry_PluginsSnapshot(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	r.Register(&fakePlugin{name: "snap1", version: "1.0"})
	r.Register(&fakePlugin{name: "snap2", version: "2.0"})

	plugins := r.Plugins()
	if len(plugins) != 2 {
		t.Fatalf("Plugins() length = %d, want 2", len(plugins))
	}

	// Mutating the returned slice should not affect the registry.
	plugins[0] = nil
	fresh := r.Plugins()
	if fresh[0] == nil {
		t.Error("Plugins() returned a reference to internal slice, not a copy")
	}
}

func TestRegistry_ListToolsEmpty(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	tools := r.ListTools()
	if len(tools) != 0 {
		t.Errorf("ListTools() length = %d, want 0", len(tools))
	}
}

func TestRegistry_ListGRPCEmpty(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	grpcs := r.ListGRPC()
	if len(grpcs) != 0 {
		t.Errorf("ListGRPC() length = %d, want 0", len(grpcs))
	}
}

func TestRegistry_InitAllEmpty(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	err := r.InitAll(context.Background(), nil)
	if err != nil {
		t.Errorf("InitAll on empty registry: %v", err)
	}
}

func TestRegistry_ShutdownAllEmpty(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	err := r.ShutdownAll()
	if err != nil {
		t.Errorf("ShutdownAll on empty registry: %v", err)
	}
}
