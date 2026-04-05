package enhancer

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// mockSamplingClient implements SamplingClient for testing.
type mockSamplingClient struct {
	result *mcp.CreateMessageResult
	err    error
	called bool
	req    mcp.CreateMessageRequest
}

func (m *mockSamplingClient) CreateMessage(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
	m.called = true
	m.req = request
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func newMockSamplingResult(text string) *mcp.CreateMessageResult {
	return &mcp.CreateMessageResult{
		SamplingMessage: mcp.SamplingMessage{
			Role:    mcp.RoleAssistant,
			Content: mcp.TextContent{Type: "text", Text: text},
		},
		Model:      "claude-sonnet-4-6",
		StopReason: "endTurn",
	}
}

func TestSamplingEngine_Improve(t *testing.T) {
	t.Parallel()

	mock := &mockSamplingClient{
		result: newMockSamplingResult("You are an expert. Enhanced prompt here."),
	}
	engine := NewSamplingEngine(mock)

	result, err := engine.Improve(context.Background(), "fix the bug", ImproveOptions{
		TaskType: TaskTypeCode,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !mock.called {
		t.Error("expected CreateMessage to be called")
	}
	if result.Enhanced != "You are an expert. Enhanced prompt here." {
		t.Errorf("unexpected enhanced: %q", result.Enhanced)
	}
	if result.TaskType != string(TaskTypeCode) {
		t.Errorf("expected task type %q, got %q", TaskTypeCode, result.TaskType)
	}
	assertContains(t, result.Improvements[0], "MCP Sampling")
}

func TestSamplingEngine_Improve_WithFeedback(t *testing.T) {
	t.Parallel()

	mock := &mockSamplingClient{
		result: newMockSamplingResult("improved with feedback"),
	}
	engine := NewSamplingEngine(mock)

	_, err := engine.Improve(context.Background(), "fix the bug", ImproveOptions{
		Feedback: "focus on error handling",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the feedback was included in the message
	msg := mock.req.Messages[0]
	tc, ok := msg.Content.(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", msg.Content)
	}
	assertContains(t, tc.Text, "focus on error handling")
}

func TestSamplingEngine_Improve_Error(t *testing.T) {
	t.Parallel()

	mock := &mockSamplingClient{
		err: errors.New("sampling unavailable"),
	}
	engine := NewSamplingEngine(mock)

	_, err := engine.Improve(context.Background(), "fix the bug", ImproveOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertContains(t, err.Error(), "sampling createMessage")
}

func TestSamplingEngine_Improve_EmptyResponse(t *testing.T) {
	t.Parallel()

	mock := &mockSamplingClient{
		result: newMockSamplingResult(""),
	}
	engine := NewSamplingEngine(mock)

	_, err := engine.Improve(context.Background(), "fix the bug", ImproveOptions{})
	if err == nil {
		t.Fatal("expected error for empty response, got nil")
	}
	assertContains(t, err.Error(), "empty response")
}

func TestSamplingEngine_Provider(t *testing.T) {
	t.Parallel()
	engine := NewSamplingEngine(&mockSamplingClient{})
	if engine.Provider() != ProviderSampling {
		t.Errorf("expected provider %q, got %q", ProviderSampling, engine.Provider())
	}
}

func TestNewSamplingEngine_NilSampler(t *testing.T) {
	t.Parallel()
	engine := NewSamplingEngine(nil)
	if engine != nil {
		t.Error("expected nil engine when sampler is nil")
	}
}

func TestSamplingEngine_Score(t *testing.T) {
	t.Parallel()

	report := SamplingScore("fix the bug in the authentication module", TaskTypeCode, ProviderClaude)
	if report == nil {
		t.Fatal("expected non-nil score report")
	}
	if report.Overall < 0 || report.Overall > 100 {
		t.Errorf("overall score out of range: %d", report.Overall)
	}
	if len(report.Dimensions) == 0 {
		t.Error("expected at least one dimension")
	}
}

func TestSamplingEngine_ImproveSystemPrompt(t *testing.T) {
	t.Parallel()

	mock := &mockSamplingClient{
		result: newMockSamplingResult("improved"),
	}
	engine := NewSamplingEngine(mock)

	_, err := engine.Improve(context.Background(), "test prompt", ImproveOptions{
		ThinkingEnabled: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// When thinking is enabled, the system prompt should include the thinking addendum
	assertContains(t, mock.req.SystemPrompt, "Thinking mode addendum")
}

func TestSamplingEngine_DefaultSystemPromptTargetsOpenAI(t *testing.T) {
	t.Parallel()

	mock := &mockSamplingClient{
		result: newMockSamplingResult("improved"),
	}
	engine := NewSamplingEngine(mock)

	_, err := engine.Improve(context.Background(), "test prompt", ImproveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertContains(t, mock.req.SystemPrompt, "optimized for OpenAI models")
}

func TestSamplingEngine_FallbackInHybrid(t *testing.T) {
	t.Parallel()

	mock := &mockSamplingClient{
		result: newMockSamplingResult("sampling-enhanced prompt"),
	}
	samplingEngine := NewSamplingEngine(mock)

	// Create a HybridEngine backed by sampling (no API keys needed)
	engine := &HybridEngine{
		Client: samplingEngine,
		CB:     NewCircuitBreaker(),
		Cache:  NewPromptCache(),
		Cfg:    LLMConfig{Enabled: true, Provider: "sampling"},
	}

	result := EnhanceHybrid(context.Background(), "fix the bug", "", Config{}, engine, ModeAuto, "")

	if result.Source != "llm" {
		t.Errorf("expected source 'llm', got %q", result.Source)
	}
	if result.Enhanced != "sampling-enhanced prompt" {
		t.Errorf("unexpected enhanced: %q", result.Enhanced)
	}
	if !mock.called {
		t.Error("expected sampling client to be called")
	}
}

func TestExtractSamplingText_MapContent(t *testing.T) {
	t.Parallel()

	result := &mcp.CreateMessageResult{
		SamplingMessage: mcp.SamplingMessage{
			Role: mcp.RoleAssistant,
			Content: map[string]interface{}{
				"type": "text",
				"text": "from map",
			},
		},
	}
	got := extractSamplingText(result)
	if got != "from map" {
		t.Errorf("expected 'from map', got %q", got)
	}
}

func TestExtractSamplingText_StringContent(t *testing.T) {
	t.Parallel()

	result := &mcp.CreateMessageResult{
		SamplingMessage: mcp.SamplingMessage{
			Role:    mcp.RoleAssistant,
			Content: "plain string",
		},
	}
	got := extractSamplingText(result)
	if got != "plain string" {
		t.Errorf("expected 'plain string', got %q", got)
	}
}

func TestExtractSamplingText_Nil(t *testing.T) {
	t.Parallel()
	got := extractSamplingText(nil)
	if got != "" {
		t.Errorf("expected empty string for nil result, got %q", got)
	}
}
