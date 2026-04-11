package plugin

import (
	"context"
	"errors"
	"testing"
)

// --- Mock implementations ---

type mockProviderPlugin struct {
	fakePlugin
	providerName string
	completeFunc func(ctx context.Context, prompt string, opts map[string]any) (string, error)
}

func (m *mockProviderPlugin) ProviderName() string { return m.providerName }
func (m *mockProviderPlugin) Complete(ctx context.Context, prompt string, opts map[string]any) (string, error) {
	if m.completeFunc != nil {
		return m.completeFunc(ctx, prompt, opts)
	}
	return "mock-response", nil
}

type mockToolPlugin struct {
	fakePlugin
	tools      []ToolDef
	handleFunc func(ctx context.Context, name string, args map[string]any) (any, error)
}

func (m *mockToolPlugin) Tools() []ToolDef { return m.tools }
func (m *mockToolPlugin) HandleToolCall(ctx context.Context, name string, args map[string]any) (any, error) {
	if m.handleFunc != nil {
		return m.handleFunc(ctx, name, args)
	}
	return nil, nil
}

type mockStrategyPlugin struct {
	fakePlugin
	planFunc func(ctx context.Context, pc PlanContext, tasks []Task) ([]Task, error)
}

func (m *mockStrategyPlugin) Plan(ctx context.Context, pc PlanContext, tasks []Task) ([]Task, error) {
	if m.planFunc != nil {
		return m.planFunc(ctx, pc, tasks)
	}
	return tasks, nil
}

// multiPlugin implements all three typed interfaces plus the base Plugin.
type multiPlugin struct {
	fakePlugin
	providerName string
}

func (m *multiPlugin) ProviderName() string { return m.providerName }
func (m *multiPlugin) Complete(_ context.Context, prompt string, _ map[string]any) (string, error) {
	return "multi:" + prompt, nil
}
func (m *multiPlugin) Tools() []ToolDef {
	return []ToolDef{{Name: "multi_tool", Description: "from multi"}}
}
func (m *multiPlugin) HandleToolCall(_ context.Context, name string, _ map[string]any) (any, error) {
	return map[string]string{"tool": name}, nil
}
func (m *multiPlugin) Plan(_ context.Context, _ PlanContext, tasks []Task) ([]Task, error) {
	return tasks, nil
}

// --- Interface satisfaction (compile-time) ---

var (
	_ Plugin         = (*mockProviderPlugin)(nil)
	_ ProviderPlugin = (*mockProviderPlugin)(nil)

	_ Plugin     = (*mockToolPlugin)(nil)
	_ ToolPlugin = (*mockToolPlugin)(nil)

	_ Plugin         = (*mockStrategyPlugin)(nil)
	_ StrategyPlugin = (*mockStrategyPlugin)(nil)

	_ ProviderPlugin = (*multiPlugin)(nil)
	_ ToolPlugin     = (*multiPlugin)(nil)
	_ StrategyPlugin = (*multiPlugin)(nil)
)

// --- ProviderPlugin tests ---

func TestProviderPlugin_Complete(t *testing.T) {
	t.Parallel()

	p := &mockProviderPlugin{
		fakePlugin:   fakePlugin{name: "vllm", version: "0.1"},
		providerName: "vllm-local",
		completeFunc: func(_ context.Context, prompt string, opts map[string]any) (string, error) {
			model, _ := opts["model"].(string)
			return "reply from " + model + ": " + prompt, nil
		},
	}

	if p.ProviderName() != "vllm-local" {
		t.Errorf("ProviderName() = %q, want %q", p.ProviderName(), "vllm-local")
	}

	result, err := p.Complete(context.Background(), "hello", map[string]any{"model": "llama3"})
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}
	want := "reply from llama3: hello"
	if result != want {
		t.Errorf("Complete() = %q, want %q", result, want)
	}
}

func TestProviderPlugin_CompleteError(t *testing.T) {
	t.Parallel()

	p := &mockProviderPlugin{
		fakePlugin:   fakePlugin{name: "broken", version: "0.1"},
		providerName: "broken",
		completeFunc: func(_ context.Context, _ string, _ map[string]any) (string, error) {
			return "", errors.New("model not found")
		},
	}

	_, err := p.Complete(context.Background(), "test", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "model not found" {
		t.Errorf("error = %q, want %q", err.Error(), "model not found")
	}
}

// --- ToolPlugin tests ---

func TestToolPlugin_ToolsAndHandle(t *testing.T) {
	t.Parallel()

	p := &mockToolPlugin{
		fakePlugin: fakePlugin{name: "custom-tools", version: "1.0"},
		tools: []ToolDef{
			{Name: "greet", Description: "Says hello", InputSchema: map[string]any{"type": "object"}},
			{Name: "add", Description: "Adds numbers"},
		},
		handleFunc: func(_ context.Context, name string, args map[string]any) (any, error) {
			switch name {
			case "greet":
				who, _ := args["name"].(string)
				return map[string]string{"message": "hello " + who}, nil
			case "add":
				a, _ := args["a"].(float64)
				b, _ := args["b"].(float64)
				return a + b, nil
			default:
				return nil, errors.New("unknown tool: " + name)
			}
		},
	}

	tools := p.Tools()
	if len(tools) != 2 {
		t.Fatalf("Tools() returned %d, want 2", len(tools))
	}
	if tools[0].Name != "greet" || tools[1].Name != "add" {
		t.Errorf("Tools() names = [%s, %s], want [greet, add]", tools[0].Name, tools[1].Name)
	}

	result, err := p.HandleToolCall(context.Background(), "greet", map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("HandleToolCall error: %v", err)
	}
	m, ok := result.(map[string]string)
	if !ok {
		t.Fatalf("result type = %T, want map[string]string", result)
	}
	if m["message"] != "hello world" {
		t.Errorf("result message = %q, want %q", m["message"], "hello world")
	}

	numResult, err := p.HandleToolCall(context.Background(), "add", map[string]any{"a": 2.0, "b": 3.0})
	if err != nil {
		t.Fatalf("HandleToolCall error: %v", err)
	}
	if numResult != 5.0 {
		t.Errorf("add result = %v, want 5.0", numResult)
	}
}

func TestToolPlugin_HandleUnknownTool(t *testing.T) {
	t.Parallel()

	p := &mockToolPlugin{
		fakePlugin: fakePlugin{name: "strict", version: "1.0"},
		handleFunc: func(_ context.Context, name string, _ map[string]any) (any, error) {
			return nil, errors.New("unknown tool: " + name)
		},
	}

	_, err := p.HandleToolCall(context.Background(), "nonexistent", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

// --- StrategyPlugin tests ---

func TestStrategyPlugin_Plan(t *testing.T) {
	t.Parallel()

	// Strategy that reverses task order (simple reordering).
	p := &mockStrategyPlugin{
		fakePlugin: fakePlugin{name: "reverse-strategy", version: "1.0"},
		planFunc: func(_ context.Context, _ PlanContext, tasks []Task) ([]Task, error) {
			out := make([]Task, len(tasks))
			for i, t := range tasks {
				out[len(tasks)-1-i] = t
			}
			return out, nil
		},
	}

	tasks := []Task{
		{ID: "1", Description: "first", Priority: 1},
		{ID: "2", Description: "second", Priority: 2},
		{ID: "3", Description: "third", Priority: 3},
	}

	pc := PlanContext{Repo: "/tmp/repo", Provider: "claude"}
	result, err := p.Plan(context.Background(), pc, tasks)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("Plan returned %d tasks, want 3", len(result))
	}
	if result[0].ID != "3" || result[1].ID != "2" || result[2].ID != "1" {
		t.Errorf("Plan order = [%s, %s, %s], want [3, 2, 1]",
			result[0].ID, result[1].ID, result[2].ID)
	}
}

func TestStrategyPlugin_PlanFilter(t *testing.T) {
	t.Parallel()

	// Strategy that filters to high-priority tasks only.
	p := &mockStrategyPlugin{
		fakePlugin: fakePlugin{name: "priority-filter", version: "1.0"},
		planFunc: func(_ context.Context, _ PlanContext, tasks []Task) ([]Task, error) {
			var out []Task
			for _, t := range tasks {
				if t.Priority >= 5 {
					out = append(out, t)
				}
			}
			return out, nil
		},
	}

	tasks := []Task{
		{ID: "low", Priority: 1},
		{ID: "high", Priority: 10},
		{ID: "mid", Priority: 3},
	}

	result, err := p.Plan(context.Background(), PlanContext{}, tasks)
	if err != nil {
		t.Fatalf("Plan error: %v", err)
	}
	if len(result) != 1 || result[0].ID != "high" {
		t.Errorf("Plan result = %v, want single task with ID=high", result)
	}
}

func TestStrategyPlugin_PlanError(t *testing.T) {
	t.Parallel()

	p := &mockStrategyPlugin{
		fakePlugin: fakePlugin{name: "failing-strategy", version: "1.0"},
		planFunc: func(_ context.Context, _ PlanContext, _ []Task) ([]Task, error) {
			return nil, errors.New("planning failed")
		},
	}

	_, err := p.Plan(context.Background(), PlanContext{}, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// --- Type assertion helper tests ---

func TestAsProvider(t *testing.T) {
	t.Parallel()

	provider := &mockProviderPlugin{
		fakePlugin:   fakePlugin{name: "p", version: "1.0"},
		providerName: "test",
	}
	plain := &fakePlugin{name: "plain", version: "1.0"}

	pp, ok := AsProvider(provider)
	if !ok || pp == nil {
		t.Error("AsProvider failed for ProviderPlugin implementation")
	}
	if pp.ProviderName() != "test" {
		t.Errorf("ProviderName() = %q, want %q", pp.ProviderName(), "test")
	}

	pp2, ok2 := AsProvider(plain)
	if ok2 || pp2 != nil {
		t.Error("AsProvider should return nil, false for plain Plugin")
	}
}

func TestAsTool(t *testing.T) {
	t.Parallel()

	tool := &mockToolPlugin{
		fakePlugin: fakePlugin{name: "t", version: "1.0"},
		tools:      []ToolDef{{Name: "x"}},
	}
	plain := &fakePlugin{name: "plain", version: "1.0"}

	tp, ok := AsTool(tool)
	if !ok || tp == nil {
		t.Error("AsTool failed for ToolPlugin implementation")
	}
	if len(tp.Tools()) != 1 {
		t.Errorf("Tools() len = %d, want 1", len(tp.Tools()))
	}

	tp2, ok2 := AsTool(plain)
	if ok2 || tp2 != nil {
		t.Error("AsTool should return nil, false for plain Plugin")
	}
}

func TestAsStrategy(t *testing.T) {
	t.Parallel()

	strategy := &mockStrategyPlugin{
		fakePlugin: fakePlugin{name: "s", version: "1.0"},
	}
	plain := &fakePlugin{name: "plain", version: "1.0"}

	sp, ok := AsStrategy(strategy)
	if !ok || sp == nil {
		t.Error("AsStrategy failed for StrategyPlugin implementation")
	}

	sp2, ok2 := AsStrategy(plain)
	if ok2 || sp2 != nil {
		t.Error("AsStrategy should return nil, false for plain Plugin")
	}
}

func TestAsGRPC(t *testing.T) {
	t.Parallel()

	grpc := &fakeGRPCPlugin{
		name:         "g",
		version:      "1.0",
		capabilities: []string{"tool_a"},
	}
	plain := &fakePlugin{name: "plain", version: "1.0"}

	gp, ok := AsGRPC(grpc)
	if !ok || gp == nil {
		t.Error("AsGRPC failed for GRPCPlugin implementation")
	}

	gp2, ok2 := AsGRPC(plain)
	if ok2 || gp2 != nil {
		t.Error("AsGRPC should return nil, false for plain Plugin")
	}
}

// --- Multi-interface plugin tests ---

func TestMultiPlugin_AllInterfaces(t *testing.T) {
	t.Parallel()

	mp := &multiPlugin{
		fakePlugin:   fakePlugin{name: "multi", version: "2.0"},
		providerName: "multi-provider",
	}

	// Register as a plain Plugin.
	r := NewRegistry()
	r.Register(mp)

	// Should be discoverable via all three assertion helpers.
	registered, found := r.Get("multi")
	if !found {
		t.Fatal("Get(\"multi\") not found after Register")
	}

	pp, ok := AsProvider(registered)
	if !ok {
		t.Fatal("multi-plugin should satisfy ProviderPlugin")
	}
	result, err := pp.Complete(context.Background(), "test", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result != "multi:test" {
		t.Errorf("Complete() = %q, want %q", result, "multi:test")
	}

	tp, ok := AsTool(registered)
	if !ok {
		t.Fatal("multi-plugin should satisfy ToolPlugin")
	}
	if len(tp.Tools()) != 1 || tp.Tools()[0].Name != "multi_tool" {
		t.Errorf("Tools() = %v, want single tool named multi_tool", tp.Tools())
	}

	sp, ok := AsStrategy(registered)
	if !ok {
		t.Fatal("multi-plugin should satisfy StrategyPlugin")
	}
	tasks := []Task{{ID: "a"}}
	planned, err := sp.Plan(context.Background(), PlanContext{}, tasks)
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 1 || planned[0].ID != "a" {
		t.Errorf("Plan() = %v, want passthrough", planned)
	}
}

func TestProviderPlugin_ContextCancellation(t *testing.T) {
	t.Parallel()

	p := &mockProviderPlugin{
		fakePlugin:   fakePlugin{name: "ctx-test", version: "1.0"},
		providerName: "ctx-test",
		completeFunc: func(ctx context.Context, _ string, _ map[string]any) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			return "ok", nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := p.Complete(ctx, "test", nil)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestToolDef_Fields(t *testing.T) {
	t.Parallel()

	td := ToolDef{
		Name:        "search",
		Description: "Search the codebase",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
	}

	if td.Name != "search" {
		t.Errorf("Name = %q, want %q", td.Name, "search")
	}
	if td.Description != "Search the codebase" {
		t.Errorf("Description = %q, want %q", td.Description, "Search the codebase")
	}
	if td.InputSchema["type"] != "object" {
		t.Errorf("InputSchema type = %v, want object", td.InputSchema["type"])
	}
}

func TestTask_Fields(t *testing.T) {
	t.Parallel()

	task := Task{
		ID:          "task-1",
		Description: "Fix the bug",
		Priority:    5,
		Metadata:    map[string]any{"assignee": "alice"},
	}

	if task.ID != "task-1" {
		t.Errorf("ID = %q, want %q", task.ID, "task-1")
	}
	if task.Priority != 5 {
		t.Errorf("Priority = %d, want 5", task.Priority)
	}
}

func TestPlanContext_Fields(t *testing.T) {
	t.Parallel()

	pc := PlanContext{
		Repo:     "/tmp/myrepo",
		Provider: "claude",
		History:  []Event{{Type: "start", Repo: "/tmp/myrepo"}},
		Extra:    map[string]any{"budget": 100},
	}

	if pc.Repo != "/tmp/myrepo" {
		t.Errorf("Repo = %q, want %q", pc.Repo, "/tmp/myrepo")
	}
	if pc.Provider != "claude" {
		t.Errorf("Provider = %q, want %q", pc.Provider, "claude")
	}
	if len(pc.History) != 1 {
		t.Errorf("History len = %d, want 1", len(pc.History))
	}
}
