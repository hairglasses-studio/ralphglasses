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
		if r.URL.Path != "/v1/responses" {
			t.Errorf("expected /v1/responses, got %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-openai-key" {
			t.Errorf("expected bearer auth, got %q", r.Header.Get("Authorization"))
		}

		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Model != "o3" {
			t.Errorf("expected model o3, got %s", req.Model)
		}
		if req.Instructions == "" {
			t.Error("expected non-empty instructions (system prompt)")
		}
		if req.Input == "" {
			t.Error("expected non-empty input")
		}
		if req.MaxOutputTokens != 4096 {
			t.Errorf("expected max_output_tokens 4096, got %d", req.MaxOutputTokens)
		}
		if req.Reasoning == nil {
			t.Fatal("expected reasoning config to be set")
		}
		if req.Reasoning.Effort != "medium" {
			t.Errorf("expected reasoning effort 'medium' for default task, got %q", req.Reasoning.Effort)
		}

		resp := responsesResponse{
			ID: "resp_123",
			Output: []responseOutput{
				{
					Type: "message",
					Content: []outputContent{
						{Type: "output_text", Text: "OpenAI improved prompt"},
					},
				},
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
	var receivedInput string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req responsesRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		receivedInput = req.Input

		resp := responsesResponse{
			ID: "resp_456",
			Output: []responseOutput{
				{
					Type: "message",
					Content: []outputContent{
						{Type: "output_text", Text: "improved"},
					},
				},
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
	assertContains(t, receivedInput, "fix the bug")
	assertContains(t, receivedInput, "focus on error handling")
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


func TestOpenAIClient_ReasoningEffortByTaskType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		taskType TaskType
		want     string
	}{
		{"troubleshooting uses low", TaskTypeTroubleshooting, "low"},
		{"workflow uses low", TaskTypeWorkflow, "low"},
		{"code uses medium", TaskTypeCode, "medium"},
		{"creative uses medium", TaskTypeCreative, "medium"},
		{"analysis uses medium", TaskTypeAnalysis, "medium"},
		{"general uses medium", TaskTypeGeneral, "medium"},
		{"empty uses medium", "", "medium"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var receivedEffort string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req responsesRequest
				_ = json.NewDecoder(r.Body).Decode(&req)
				if req.Reasoning != nil {
					receivedEffort = req.Reasoning.Effort
				}
				resp := responsesResponse{
					ID: "resp_test",
					Output: []responseOutput{
						{Type: "message", Content: []outputContent{{Type: "output_text", Text: "ok"}}},
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

			_, err := client.Improve(context.Background(), "test", ImproveOptions{TaskType: tt.taskType})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if receivedEffort != tt.want {
				t.Errorf("expected effort %q, got %q", tt.want, receivedEffort)
			}
		})
	}
}

func TestOpenAIClient_PreviousResponseID(t *testing.T) {
	t.Parallel()

	var firstReqPrevID, secondReqPrevID string
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req responsesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		callCount++
		if callCount == 1 {
			firstReqPrevID = req.PreviousResponseID
		} else {
			secondReqPrevID = req.PreviousResponseID
		}

		resp := responsesResponse{
			ID: "resp_call_" + string(rune('0'+callCount)),
			Output: []responseOutput{
				{Type: "message", Content: []outputContent{{Type: "output_text", Text: "improved"}}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := &OpenAIClient{
		APIKey:     "test-key",
		Model:      "o3",
		BaseURL:    server.URL,
		HTTPClient: &http.Client{Timeout: 5 * time.Second},
	}

	// First call: no previous response ID should be sent.
	_, err := client.Improve(context.Background(), "first prompt", ImproveOptions{})
	if err != nil {
		t.Fatalf("first call: unexpected error: %v", err)
	}
	if firstReqPrevID != "" {
		t.Errorf("first call: expected empty previous_response_id, got %q", firstReqPrevID)
	}
	if client.LastResponseID != "resp_call_1" {
		t.Errorf("expected LastResponseID to be 'resp_call_1', got %q", client.LastResponseID)
	}

	// Second call: previous response ID should be included.
	_, err = client.Improve(context.Background(), "second prompt", ImproveOptions{})
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if secondReqPrevID != "resp_call_1" {
		t.Errorf("second call: expected previous_response_id 'resp_call_1', got %q", secondReqPrevID)
	}
	if client.LastResponseID != "resp_call_2" {
		t.Errorf("expected LastResponseID to be 'resp_call_2', got %q", client.LastResponseID)
	}
}

func TestOpenAIClient_PreviousResponseID_Serialization(t *testing.T) {
	t.Parallel()

	// Verify the field serializes correctly to JSON.
	req := responsesRequest{
		Model:              "o3",
		Instructions:       "test",
		Input:              "test",
		PreviousResponseID: "resp_abc123",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if raw["previous_response_id"] != "resp_abc123" {
		t.Errorf("expected previous_response_id in JSON, got %v", raw["previous_response_id"])
	}

	// Verify omitempty: empty string should not appear in JSON.
	req.PreviousResponseID = ""
	data, err = json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}
	var raw2 map[string]any
	if err := json.Unmarshal(data, &raw2); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if _, exists := raw2["previous_response_id"]; exists {
		t.Error("expected previous_response_id to be omitted when empty")
	}
}

func TestExtractResponseText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output []responseOutput
		want   string
	}{
		{
			name:   "empty output",
			output: nil,
			want:   "",
		},
		{
			name: "single message",
			output: []responseOutput{
				{Type: "message", Content: []outputContent{{Type: "output_text", Text: "hello"}}},
			},
			want: "hello",
		},
		{
			name: "skips non-message types",
			output: []responseOutput{
				{Type: "reasoning", Content: []outputContent{{Type: "thinking", Text: "thinking..."}}},
				{Type: "message", Content: []outputContent{{Type: "output_text", Text: "result"}}},
			},
			want: "result",
		},
		{
			name: "skips non-output_text content",
			output: []responseOutput{
				{Type: "message", Content: []outputContent{
					{Type: "refusal", Text: "refused"},
					{Type: "output_text", Text: "actual"},
				}},
			},
			want: "actual",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractResponseText(tt.output)
			if got != tt.want {
				t.Errorf("extractResponseText() = %q, want %q", got, tt.want)
			}
		})
	}
}
