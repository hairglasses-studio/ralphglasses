package enhancer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// newTestClient creates an LLMClient pointing at a test server.
func newTestClient(t *testing.T, serverURL string) *LLMClient {
	t.Helper()
	httpClient := &http.Client{Timeout: 5 * time.Second}
	sdkClient := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(serverURL),
		option.WithHTTPClient(httpClient),
	)
	return &LLMClient{
		APIKey:       "test-key",
		Model:        "claude-sonnet-4-6",
		BaseURL:      serverURL,
		HTTPClient:   httpClient,
		sdk:          &sdkClient,
		effortLevel:  "medium",
		cacheControl: true,
	}
}

// messagesRequest is used only for test request body decoding.
type messagesRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    json.RawMessage `json:"system"`
	Messages  []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"messages"`
	Thinking     json.RawMessage `json:"thinking,omitempty"`
	OutputConfig json.RawMessage `json:"output_config,omitempty"`
	CacheControl json.RawMessage `json:"cache_control,omitempty"`
}

// messagesResponse matches the Claude Messages API response shape.
type messagesResponse struct {
	ID      string         `json:"id"`
	Type    string         `json:"type"`
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
	Model   string         `json:"model"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	StopReason string `json:"stop_reason"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

func newMockResponse(text string) messagesResponse {
	return messagesResponse{
		ID:   "msg_test",
		Type: "message",
		Role: "assistant",
		Content: []contentBlock{
			{Type: "text", Text: text},
		},
		Model: "claude-sonnet-4-6",
		Usage: struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		}{InputTokens: 10, OutputTokens: 20},
		StopReason: "end_turn",
	}
}

func TestLLMClient_Improve(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected /v1/messages, got %s", r.URL.Path)
		}
		if r.Header.Get("X-Api-Key") != "test-key" {
			t.Errorf("expected api key 'test-key', got %q", r.Header.Get("X-Api-Key"))
		}

		// Verify request body
		var req messagesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Model != "claude-sonnet-4-6" {
			t.Errorf("expected model claude-sonnet-4-6, got %s", req.Model)
		}
		if len(req.Messages) != 1 {
			t.Error("expected single message")
		}

		// Return mock response
		resp := newMockResponse("You are an expert software engineer.\n\nImproved prompt here.")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	result, err := client.Improve(context.Background(), "fix this bug", ImproveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Enhanced == "" {
		t.Error("expected non-empty enhanced prompt")
	}
	if result.Enhanced != "You are an expert software engineer.\n\nImproved prompt here." {
		t.Errorf("unexpected enhanced: %q", result.Enhanced)
	}
}

func TestLLMClient_ImproveWithThinking(t *testing.T) {
	t.Parallel()
	var receivedSystem string
	var receivedThinking json.RawMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req messagesRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		// Decode system blocks to get the text
		var sysBlocks []struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(req.System, &sysBlocks)
		if len(sysBlocks) > 0 {
			receivedSystem = sysBlocks[0].Text
		}
		receivedThinking = req.Thinking

		w.Header().Set("Content-Type", "application/json")
		resp := newMockResponse("improved")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.Improve(context.Background(), "test", ImproveOptions{ThinkingEnabled: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedSystem == MetaPrompt {
		t.Error("expected thinking meta-prompt, got standard")
	}
	if receivedSystem != MetaPromptWithThinking {
		t.Error("expected MetaPromptWithThinking system prompt")
	}

	// Verify thinking config was sent
	if len(receivedThinking) == 0 {
		t.Error("expected thinking config in request")
	}
	var thinkingConfig struct {
		Type    string `json:"type"`
		Display string `json:"display"`
	}
	if err := json.Unmarshal(receivedThinking, &thinkingConfig); err == nil {
		if thinkingConfig.Type != "adaptive" {
			t.Errorf("expected adaptive thinking, got %q", thinkingConfig.Type)
		}
	}
}

func TestLLMClient_ImproveWithFeedback(t *testing.T) {
	t.Parallel()
	var receivedContent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&raw)

		// Extract the user message content
		var messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		}
		_ = json.Unmarshal(raw["messages"], &messages)
		if len(messages) > 0 && len(messages[0].Content) > 0 {
			receivedContent = messages[0].Content[0].Text
		}

		w.Header().Set("Content-Type", "application/json")
		resp := newMockResponse("improved")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.Improve(context.Background(), "fix the bug", ImproveOptions{
		Feedback: "focus on error handling",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertContains(t, receivedContent, "fix the bug")
	assertContains(t, receivedContent, "focus on error handling")
}

func TestLLMClient_APIError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"rate limited"}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.Improve(context.Background(), "test", ImproveOptions{})
	if err == nil {
		t.Error("expected error for 429 response")
	}
	assertContains(t, err.Error(), "429")
}

func TestLLMClient_ContextCancellation(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // simulate slow response
		resp := newMockResponse("too late")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := client.Improve(ctx, "test", ImproveOptions{})
	if err == nil {
		t.Error("expected error from context cancellation")
	}
}

func TestNewLLMClient_NoAPIKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	client := NewLLMClient(LLMConfig{
		APIKeyEnv: "NONEXISTENT_KEY_FOR_TESTING_12345",
	})
	if client != nil {
		t.Error("expected nil client when API key is missing")
	}
}

func TestLLMClient_CacheControl(t *testing.T) {
	t.Parallel()
	var hasCacheControl bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&raw)

		// Check for top-level cache_control field
		if cc, ok := raw["cache_control"]; ok {
			hasCacheControl = len(cc) > 0 && string(cc) != "null"
		}

		w.Header().Set("Content-Type", "application/json")
		resp := newMockResponse("cached")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	client.cacheControl = true

	_, err := client.Improve(context.Background(), "test", ImproveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hasCacheControl {
		t.Error("expected cache_control in request")
	}
}

func TestLLMClient_EffortLevel(t *testing.T) {
	t.Parallel()
	var receivedEffort string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var raw map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&raw)

		if oc, ok := raw["output_config"]; ok {
			var outputConfig struct {
				Effort string `json:"effort"`
			}
			_ = json.Unmarshal(oc, &outputConfig)
			receivedEffort = outputConfig.Effort
		}

		w.Header().Set("Content-Type", "application/json")
		resp := newMockResponse("improved")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	client.effortLevel = "high"

	_, err := client.Improve(context.Background(), "test", ImproveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedEffort != "high" {
		t.Errorf("expected effort 'high', got %q", receivedEffort)
	}
}
