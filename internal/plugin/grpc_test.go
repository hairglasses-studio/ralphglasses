package plugin

import (
	"context"
	"testing"
)

// fakeGRPCPlugin implements GRPCPlugin for testing.
type fakeGRPCPlugin struct {
	name         string
	version      string
	capabilities []string
	handleFunc   func(ctx context.Context, name string, args map[string]any) (string, error)
	onEvent      func(ctx context.Context, event Event) error
}

func (f *fakeGRPCPlugin) Name() string    { return f.name }
func (f *fakeGRPCPlugin) Version() string { return f.version }
func (f *fakeGRPCPlugin) OnEvent(ctx context.Context, event Event) error {
	if f.onEvent != nil {
		return f.onEvent(ctx, event)
	}
	return nil
}
func (f *fakeGRPCPlugin) HandleToolCall(ctx context.Context, name string, args map[string]any) (string, error) {
	if f.handleFunc != nil {
		return f.handleFunc(ctx, name, args)
	}
	return "", nil
}
func (f *fakeGRPCPlugin) Capabilities() []string { return f.capabilities }

func TestGRPCPlugin_InterfaceSatisfaction(t *testing.T) {
	t.Parallel()

	// Verify that fakeGRPCPlugin satisfies both Plugin and GRPCPlugin.
	var _ Plugin = (*fakeGRPCPlugin)(nil)
	var _ GRPCPlugin = (*fakeGRPCPlugin)(nil)
}

func TestRegistry_RegisterGRPC(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	p := &fakeGRPCPlugin{
		name:         "grpc-test",
		version:      "1.0",
		capabilities: []string{"tool_a", "tool_b"},
	}

	r.RegisterGRPC(p)

	// Should appear in both regular and gRPC lists.
	if got := len(r.List()); got != 1 {
		t.Errorf("List() = %d plugins, want 1", got)
	}
	if got := len(r.ListGRPC()); got != 1 {
		t.Errorf("ListGRPC() = %d plugins, want 1", got)
	}

	tools := r.ListTools()
	if len(tools) != 2 {
		t.Fatalf("ListTools() = %d tools, want 2", len(tools))
	}
}

func TestRegistry_DispatchToolCall(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	p := &fakeGRPCPlugin{
		name:         "echo",
		version:      "1.0",
		capabilities: []string{"echo_tool"},
		handleFunc: func(_ context.Context, name string, args map[string]any) (string, error) {
			return "handled:" + name, nil
		},
	}
	r.RegisterGRPC(p)

	result, err := r.DispatchToolCall(context.Background(), "echo_tool", nil)
	if err != nil {
		t.Fatalf("DispatchToolCall error: %v", err)
	}
	if result != "handled:echo_tool" {
		t.Errorf("DispatchToolCall result = %q, want %q", result, "handled:echo_tool")
	}
}

func TestRegistry_DispatchToolCall_NotFound(t *testing.T) {
	t.Parallel()

	r := NewRegistry()
	_, err := r.DispatchToolCall(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Error("expected error for unknown tool, got nil")
	}
}

func TestRegistry_DispatchToolCall_MultiPlugin(t *testing.T) {
	t.Parallel()

	r := NewRegistry()

	p1 := &fakeGRPCPlugin{
		name:         "plugin-a",
		version:      "1.0",
		capabilities: []string{"tool_x"},
		handleFunc: func(_ context.Context, _ string, _ map[string]any) (string, error) {
			return "from-a", nil
		},
	}
	p2 := &fakeGRPCPlugin{
		name:         "plugin-b",
		version:      "1.0",
		capabilities: []string{"tool_y"},
		handleFunc: func(_ context.Context, _ string, _ map[string]any) (string, error) {
			return "from-b", nil
		},
	}

	r.RegisterGRPC(p1)
	r.RegisterGRPC(p2)

	res1, err := r.DispatchToolCall(context.Background(), "tool_x", nil)
	if err != nil {
		t.Fatal(err)
	}
	res2, err := r.DispatchToolCall(context.Background(), "tool_y", nil)
	if err != nil {
		t.Fatal(err)
	}

	if res1 != "from-a" {
		t.Errorf("tool_x result = %q, want %q", res1, "from-a")
	}
	if res2 != "from-b" {
		t.Errorf("tool_y result = %q, want %q", res2, "from-b")
	}
}

func TestMagicCookieConstants(t *testing.T) {
	t.Parallel()

	if MagicCookieKey != "RALPHGLASSES_PLUGIN" {
		t.Errorf("MagicCookieKey = %q, want %q", MagicCookieKey, "RALPHGLASSES_PLUGIN")
	}
	if MagicCookieValue != "v1" {
		t.Errorf("MagicCookieValue = %q, want %q", MagicCookieValue, "v1")
	}
}
