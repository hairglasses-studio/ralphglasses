package enhancer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAIClient_Improve(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-openai-key" {
			t.Errorf("expected bearer auth, got %q", r.Header.Get("Authorization"))
		}

		var req openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Model != "o3" {
			t.Errorf("expected model o3, got %s", req.Model)
		}
		if len(req.Messages) != 2 {
			t.Errorf("expected 2 messages (system+user), got %d", len(req.Messages))
		}
		if req.MaxCompletionTokens != 4096 {
			t.Errorf("expected max_completion_tokens 4096, got %d", req.MaxCompletionTokens)
		}

		resp := openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "OpenAI improved prompt"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &OpenAIClient{
		APIKey:     "test-openai-key",
		Model:      "o3",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	result, err := client.Improve(context.Background(), "fix the bug", ImproveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Enhanced != "OpenAI improved prompt" {
		t.Errorf("unexpected enhanced: %q", result.Enhanced)
	}
}

func TestOpenAIClient_Provider(t *testing.T) {
	t.Parallel()
	c := &OpenAIClient{}
	if c.Provider() != ProviderOpenAI {
		t.Errorf("expected provider %q, got %q", ProviderOpenAI, c.Provider())
	}
}

func TestOpenAIClient_ImproveWithFeedback(t *testing.T) {
	t.Parallel()
	var receivedUserContent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openaiRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		for _, msg := range req.Messages {
			if msg.Role == "user" {
				receivedUserContent = msg.Content
			}
		}
		resp := openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "improved"}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &OpenAIClient{
		APIKey:     "test-key",
		Model:      "o3",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	_, err := client.Improve(context.Background(), "fix the bug", ImproveOptions{
		Feedback: "focus on error handling",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, receivedUserContent, "fix the bug")
	assertContains(t, receivedUserContent, "focus on error handling")
}

func TestOpenAIClient_APIError(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"type":"rate_limit_error","message":"rate limited"}}`))
	}))
	defer server.Close()

	client := &OpenAIClient{
		APIKey:     "test-key",
		Model:      "o3",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	_, err := client.Improve(context.Background(), "test", ImproveOptions{})
	if err == nil {
		t.Error("expected error for 429 response")
	}
	assertContains(t, err.Error(), "429")
}

func TestNewOpenAIClient_NoAPIKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	client := NewOpenAIClient(LLMConfig{
		APIKeyEnv: "NONEXISTENT_KEY_FOR_TESTING_12345",
	})
	if client != nil {
		t.Error("expected nil client when API key is missing")
	}
}
