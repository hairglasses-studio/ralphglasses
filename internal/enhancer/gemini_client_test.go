package enhancer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGeminiClient_Improve(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Query().Get("key") != "test-gemini-key" {
			t.Errorf("expected api key in query param, got %q", r.URL.Query().Get("key"))
		}

		var req geminiRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if len(req.Contents) != 1 {
			t.Errorf("expected 1 content block, got %d", len(req.Contents))
		}
		if req.SystemInstruction == nil {
			t.Error("expected system instruction")
		}

		resp := geminiResponse{
			Candidates: []geminiCandidate{
				{Content: geminiContent{Parts: []geminiPart{{Text: "Gemini improved prompt"}}}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &GeminiClient{
		APIKey:     "test-gemini-key",
		Model:      "gemini-2.5-pro",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	result, err := client.Improve(context.Background(), "fix the bug", ImproveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Enhanced != "Gemini improved prompt" {
		t.Errorf("unexpected enhanced: %q", result.Enhanced)
	}
}

func TestGeminiClient_Provider(t *testing.T) {
	c := &GeminiClient{}
	if c.Provider() != ProviderGemini {
		t.Errorf("expected provider %q, got %q", ProviderGemini, c.Provider())
	}
}

func TestGeminiClient_ImproveWithFeedback(t *testing.T) {
	var receivedContent string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiRequest
		json.NewDecoder(r.Body).Decode(&req)
		if len(req.Contents) > 0 && len(req.Contents[0].Parts) > 0 {
			receivedContent = req.Contents[0].Parts[0].Text
		}
		resp := geminiResponse{
			Candidates: []geminiCandidate{
				{Content: geminiContent{Parts: []geminiPart{{Text: "improved"}}}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &GeminiClient{
		APIKey:     "test-key",
		Model:      "gemini-2.5-pro",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	_, err := client.Improve(context.Background(), "fix the bug", ImproveOptions{
		Feedback: "focus on error handling",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertContains(t, receivedContent, "fix the bug")
	assertContains(t, receivedContent, "focus on error handling")
}

func TestGeminiClient_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte(`{"error":{"code":429,"message":"rate limited","status":"RESOURCE_EXHAUSTED"}}`))
	}))
	defer server.Close()

	client := &GeminiClient{
		APIKey:     "test-key",
		Model:      "gemini-2.5-pro",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	_, err := client.Improve(context.Background(), "test", ImproveOptions{})
	if err == nil {
		t.Error("expected error for 429 response")
	}
	assertContains(t, err.Error(), "429")
}

func TestNewGeminiClient_NoAPIKey(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	client := NewGeminiClient(LLMConfig{
		APIKeyEnv: "NONEXISTENT_KEY_FOR_TESTING_12345",
	})
	if client != nil {
		t.Error("expected nil client when API key is missing")
	}
}

func TestGeminiClient_MultipleParts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiResponse{
			Candidates: []geminiCandidate{
				{Content: geminiContent{Parts: []geminiPart{
					{Text: "Part one. "},
					{Text: "Part two."},
				}}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &GeminiClient{
		APIKey:     "test-key",
		Model:      "gemini-2.5-pro",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	result, err := client.Improve(context.Background(), "test", ImproveOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Enhanced != "Part one. Part two." {
		t.Errorf("expected concatenated parts, got %q", result.Enhanced)
	}
}
