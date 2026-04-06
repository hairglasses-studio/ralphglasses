package session

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hairglasses-studio/mcpkit/a2a"
)

func TestA2AProviderConstants(t *testing.T) {
	t.Parallel()
	if ProviderA2A != "a2a" {
		t.Fatalf("ProviderA2A = %q, want %q", ProviderA2A, "a2a")
	}
	if got := ProviderDefaults(ProviderA2A); got != "a2a-remote" {
		t.Errorf("ProviderDefaults(a2a) = %q, want %q", got, "a2a-remote")
	}
	if got := providerEnvVar(ProviderA2A); got != "A2A_AGENT_URL" {
		t.Errorf("providerEnvVar(a2a) = %q, want %q", got, "A2A_AGENT_URL")
	}
}

func TestA2AValidateProvider(t *testing.T) {
	t.Parallel()
	// A2A is HTTP-based; ValidateProvider should always succeed
	// regardless of PATH contents.
	if err := ValidateProvider(ProviderA2A); err != nil {
		t.Errorf("ValidateProvider(a2a) returned error: %v", err)
	}
}

func TestA2AValidateProviderEnv(t *testing.T) {
	t.Parallel()
	// A2A doesn't require env vars — URL is passed at launch time.
	if err := ValidateProviderEnv(ProviderA2A); err != nil {
		t.Errorf("ValidateProviderEnv(a2a) returned error: %v", err)
	}
}

func TestA2ABuildCmdForProvider_ReturnsError(t *testing.T) {
	t.Parallel()
	// A2A should not use buildCmdForProvider — it's HTTP-based.
	_, err := buildCmdForProvider(t.Context(), LaunchOptions{
		Provider: ProviderA2A,
		RepoPath: "/tmp/repo",
		Prompt:   "test",
	})
	if err == nil {
		t.Fatal("expected error from buildCmdForProvider for A2A, got nil")
	}
	if !strings.Contains(err.Error(), "launchA2A") {
		t.Errorf("error should mention launchA2A, got: %s", err)
	}
}

func TestA2AUnsupportedOptionsWarnings(t *testing.T) {
	t.Parallel()
	warnings := UnsupportedOptionsWarnings(ProviderA2A, LaunchOptions{
		SystemPrompt: "test",
		MaxBudgetUSD: 10.0,
		Agent:        "test-agent",
		MaxTurns:     5,
		AllowedTools: []string{"tool1"},
		Worktree:     "main",
		Resume:       "session-123",
	})
	if len(warnings) != 7 {
		t.Errorf("expected 7 warnings for A2A, got %d: %v", len(warnings), warnings)
	}
	for _, w := range warnings {
		if !strings.Contains(w, "a2a") {
			t.Errorf("warning should mention a2a: %s", w)
		}
	}
}

func TestA2ACostRate(t *testing.T) {
	t.Parallel()
	rate, ok := getProviderCostRate(ProviderA2A)
	if !ok {
		t.Fatal("ProviderA2A not found in ProviderCostRates")
	}
	if rate.InputPer1M <= 0 || rate.OutputPer1M <= 0 {
		t.Errorf("A2A cost rate should be positive, got input=%f output=%f",
			rate.InputPer1M, rate.OutputPer1M)
	}
}

func TestNormalizeA2AEvent_TaskStatus(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		input     string
		wantType  string
		wantError bool
	}{
		{
			name:     "completed task",
			input:    `{"state":"completed","id":"task-1","content":"Done!"}`,
			wantType: "result",
		},
		{
			name:      "failed task",
			input:     `{"state":"failed","id":"task-2","error":"timeout"}`,
			wantType:  "result",
			wantError: true,
		},
		{
			name:     "working task",
			input:    `{"state":"working","id":"task-3","content":"Processing..."}`,
			wantType: "assistant",
		},
		{
			name:     "submitted task",
			input:    `{"state":"submitted","id":"task-4"}`,
			wantType: "system",
		},
		{
			name:     "assistant message",
			input:    `{"type":"assistant","content":"Hello from A2A agent"}`,
			wantType: "assistant",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			event, err := normalizeA2AEvent([]byte(tt.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if event.Type != tt.wantType {
				t.Errorf("event.Type = %q, want %q", event.Type, tt.wantType)
			}
			if tt.wantError && !event.IsError {
				t.Error("expected IsError=true")
			}
		})
	}
}

func TestNormalizeA2AEvent_ViaDispatcher(t *testing.T) {
	t.Parallel()
	line := []byte(`{"state":"completed","id":"task-123","content":"Result text"}`)
	event, err := normalizeEvent(ProviderA2A, line)
	if err != nil {
		t.Fatalf("normalizeEvent(a2a) error: %v", err)
	}
	if event.Type != "result" {
		t.Errorf("event.Type = %q, want result", event.Type)
	}
	if event.SessionID != "task-123" {
		t.Errorf("event.SessionID = %q, want task-123", event.SessionID)
	}
}

func TestNormalizeA2AEvent_CostExtraction(t *testing.T) {
	t.Parallel()
	line := []byte(`{"type":"assistant","content":"done","cost_usd":0.05}`)
	event, err := normalizeA2AEvent(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.CostUSD != 0.05 {
		t.Errorf("event.CostUSD = %f, want 0.05", event.CostUSD)
	}
	if event.CostSource != "structured" {
		t.Errorf("event.CostSource = %q, want structured", event.CostSource)
	}
}

func TestNormalizeA2AEvent_FallbackText(t *testing.T) {
	t.Parallel()
	line := []byte("plain text output from agent")
	event, err := normalizeA2AEvent(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.Type != "assistant" {
		t.Errorf("event.Type = %q, want assistant", event.Type)
	}
	if !strings.Contains(event.Content, "plain text output") {
		t.Errorf("event.Content = %q, expected to contain plain text", event.Content)
	}
}

func TestExtractA2AResult(t *testing.T) {
	t.Parallel()

	t.Run("from artifacts", func(t *testing.T) {
		task := &a2a.Task{
			ID:    "task-1",
			State: a2a.TaskCompleted,
			Artifacts: []a2a.Artifact{
				{Parts: []a2a.Part{a2a.TextPart("artifact result")}},
			},
		}
		result := extractA2AResult(task)
		if result != "artifact result" {
			t.Errorf("extractA2AResult = %q, want 'artifact result'", result)
		}
	})

	t.Run("from last agent message", func(t *testing.T) {
		task := &a2a.Task{
			ID:    "task-2",
			State: a2a.TaskCompleted,
			Messages: []a2a.Message{
				{Role: "user", Parts: []a2a.Part{a2a.TextPart("hello")}},
				{Role: "agent", Parts: []a2a.Part{a2a.TextPart("agent response")}},
			},
		}
		result := extractA2AResult(task)
		if result != "agent response" {
			t.Errorf("extractA2AResult = %q, want 'agent response'", result)
		}
	})

	t.Run("empty", func(t *testing.T) {
		task := &a2a.Task{ID: "task-3", State: a2a.TaskCompleted}
		result := extractA2AResult(task)
		if result != "" {
			t.Errorf("extractA2AResult = %q, want empty", result)
		}
	})
}

func TestExtractA2AError(t *testing.T) {
	t.Parallel()

	t.Run("from agent message", func(t *testing.T) {
		task := &a2a.Task{
			ID:    "task-1",
			State: a2a.TaskFailed,
			Messages: []a2a.Message{
				{Role: "user", Parts: []a2a.Part{a2a.TextPart("do something")}},
				{Role: "agent", Parts: []a2a.Part{a2a.TextPart("something went wrong")}},
			},
		}
		errMsg := extractA2AError(task)
		if errMsg != "something went wrong" {
			t.Errorf("extractA2AError = %q, want 'something went wrong'", errMsg)
		}
	})

	t.Run("no agent message", func(t *testing.T) {
		task := &a2a.Task{ID: "task-2", State: a2a.TaskFailed}
		errMsg := extractA2AError(task)
		if !strings.Contains(errMsg, "task-2") {
			t.Errorf("extractA2AError should mention task ID, got: %s", errMsg)
		}
	})
}

func TestA2AMessageToStreamEvent(t *testing.T) {
	t.Parallel()
	task := &a2a.Task{ID: "task-1", State: a2a.TaskWorking}

	t.Run("agent message", func(t *testing.T) {
		event := a2aMessageToStreamEvent("agent", "hello from agent", task)
		if event.Type != "assistant" {
			t.Errorf("Type = %q, want assistant", event.Type)
		}
		if event.SessionID != "task-1" {
			t.Errorf("SessionID = %q, want task-1", event.SessionID)
		}
		if event.Content != "hello from agent" {
			t.Errorf("Content = %q, want 'hello from agent'", event.Content)
		}
	})

	t.Run("user message", func(t *testing.T) {
		event := a2aMessageToStreamEvent("user", "input text", task)
		if event.Type != "system" {
			t.Errorf("Type = %q, want system", event.Type)
		}
	})
}

func TestRepoNameFromPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want string
	}{
		{"/home/hg/hairglasses-studio/ralphglasses", "ralphglasses"},
		{"/tmp/repo", "repo"},
		{"/tmp/repo/", "repo"},
		{"", ""},
		{"single", "single"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			t.Parallel()
			got := repoNameFromPath(tt.path)
			if got != tt.want {
				t.Errorf("repoNameFromPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestA2AAgentRegistry(t *testing.T) {
	t.Parallel()
	reg := NewA2AAgentRegistry()

	// Empty registry.
	if cards := reg.List(); len(cards) != 0 {
		t.Errorf("expected empty registry, got %d cards", len(cards))
	}

	// Manually populate (Discover requires a live HTTP server).
	reg.mu.Lock()
	reg.agents["http://example.com"] = &a2a.AgentCard{
		Name: "test-agent",
		URL:  "http://example.com",
	}
	reg.mu.Unlock()

	cards := reg.List()
	if len(cards) != 1 {
		t.Fatalf("expected 1 card, got %d", len(cards))
	}
	if cards[0].Name != "test-agent" {
		t.Errorf("card name = %q, want test-agent", cards[0].Name)
	}

	// Remove.
	reg.Remove("http://example.com")
	if cards := reg.List(); len(cards) != 0 {
		t.Errorf("expected empty after remove, got %d", len(cards))
	}
}

func TestNormalizeA2AEvent_EmptyLine(t *testing.T) {
	t.Parallel()
	_, err := normalizeEvent(ProviderA2A, []byte{})
	if err == nil {
		t.Error("expected error for empty line")
	}
}

func TestNormalizeA2AEvent_TokenCostEstimation(t *testing.T) {
	t.Parallel()
	raw := map[string]any{
		"type":    "assistant",
		"content": "result",
		"usage": map[string]any{
			"input_tokens":  float64(1000),
			"output_tokens": float64(500),
		},
	}
	line, _ := json.Marshal(raw)
	event, err := normalizeA2AEvent(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if event.CostUSD <= 0 {
		t.Errorf("expected positive estimated cost, got %f", event.CostUSD)
	}
	if event.CostSource != "estimated" {
		t.Errorf("CostSource = %q, want estimated", event.CostSource)
	}
}

func TestNormalizeProviderCost_A2A(t *testing.T) {
	t.Parallel()
	n := NormalizeProviderCost(ProviderA2A, 0.50, 0, 0)
	if n.Provider != ProviderA2A {
		t.Errorf("Provider = %q, want a2a", n.Provider)
	}
	if n.RawCostUSD != 0.50 {
		t.Errorf("RawCostUSD = %f, want 0.50", n.RawCostUSD)
	}
	if n.NormalizedUSD <= 0 {
		t.Errorf("NormalizedUSD should be positive, got %f", n.NormalizedUSD)
	}
}
