package batch

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		provider Provider
		wantType string
	}{
		{ProviderClaude, "*batch.claudeClient"},
		{ProviderGemini, "*batch.geminiClient"},
		{ProviderOpenAI, "*batch.openaiClient"},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			c, err := NewClient(tt.provider, "test-key")
			if err != nil {
				t.Fatalf("NewClient(%s) error: %v", tt.provider, err)
			}
			if c == nil {
				t.Fatalf("NewClient(%s) returned nil", tt.provider)
			}
			if c.Provider() != tt.provider {
				t.Errorf("Provider() = %s, want %s", c.Provider(), tt.provider)
			}
		})
	}
}

func TestNewClient_InvalidProvider(t *testing.T) {
	_, err := NewClient("invalid", "test-key")
	if err == nil {
		t.Fatal("expected error for invalid provider, got nil")
	}
}

func TestNewClient_WithOptions(t *testing.T) {
	c, err := NewClient(ProviderClaude, "test-key",
		WithBaseURL("https://custom.api.com"),
		WithModel("claude-opus-4-6"),
	)
	if err != nil {
		t.Fatalf("NewClient error: %v", err)
	}

	cc, ok := c.(*claudeClient)
	if !ok {
		t.Fatal("expected *claudeClient")
	}
	if cc.baseURL != "https://custom.api.com" {
		t.Errorf("baseURL = %s, want https://custom.api.com", cc.baseURL)
	}
	if cc.model != "claude-opus-4-6" {
		t.Errorf("model = %s, want claude-opus-4-6", cc.model)
	}
}

func TestRequest_JSON(t *testing.T) {
	r := Request{
		ID:           "req-1",
		Model:        "claude-sonnet-4-6",
		SystemPrompt: "You are helpful.",
		UserPrompt:   "Hello",
		MaxTokens:    1024,
		Metadata:     map[string]string{"task": "test"},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Request
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ID != r.ID {
		t.Errorf("ID = %s, want %s", decoded.ID, r.ID)
	}
	if decoded.Model != r.Model {
		t.Errorf("Model = %s, want %s", decoded.Model, r.Model)
	}
	if decoded.SystemPrompt != r.SystemPrompt {
		t.Errorf("SystemPrompt = %s, want %s", decoded.SystemPrompt, r.SystemPrompt)
	}
	if decoded.UserPrompt != r.UserPrompt {
		t.Errorf("UserPrompt = %s, want %s", decoded.UserPrompt, r.UserPrompt)
	}
	if decoded.MaxTokens != r.MaxTokens {
		t.Errorf("MaxTokens = %d, want %d", decoded.MaxTokens, r.MaxTokens)
	}
	if decoded.Metadata["task"] != "test" {
		t.Errorf("Metadata[task] = %s, want test", decoded.Metadata["task"])
	}
}

func TestRequest_JSON_OmitsEmpty(t *testing.T) {
	r := Request{
		ID:         "req-1",
		UserPrompt: "Hello",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	raw := string(data)
	if containsKey(raw, "system_prompt") {
		t.Error("expected system_prompt to be omitted when empty")
	}
	if containsKey(raw, "metadata") {
		t.Error("expected metadata to be omitted when empty")
	}
}

func TestBatchStatus_JSON(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	completed := now.Add(5 * time.Minute)

	bs := BatchStatus{
		ID:          "batch-123",
		Provider:    ProviderClaude,
		Status:      "completed",
		Total:       100,
		Completed:   95,
		Failed:      5,
		CreatedAt:   now,
		CompletedAt: &completed,
	}

	data, err := json.Marshal(bs)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded BatchStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ID != bs.ID {
		t.Errorf("ID = %s, want %s", decoded.ID, bs.ID)
	}
	if decoded.Provider != bs.Provider {
		t.Errorf("Provider = %s, want %s", decoded.Provider, bs.Provider)
	}
	if decoded.Status != bs.Status {
		t.Errorf("Status = %s, want %s", decoded.Status, bs.Status)
	}
	if decoded.Total != bs.Total {
		t.Errorf("Total = %d, want %d", decoded.Total, bs.Total)
	}
	if decoded.Completed != bs.Completed {
		t.Errorf("Completed = %d, want %d", decoded.Completed, bs.Completed)
	}
	if decoded.Failed != bs.Failed {
		t.Errorf("Failed = %d, want %d", decoded.Failed, bs.Failed)
	}
	if decoded.CompletedAt == nil {
		t.Fatal("CompletedAt is nil")
	}
}

func TestBatchStatus_JSON_OmitsCompletedAt(t *testing.T) {
	bs := BatchStatus{
		ID:        "batch-123",
		Provider:  ProviderOpenAI,
		Status:    "processing",
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(bs)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	if containsKey(string(data), "completed_at") {
		t.Error("expected completed_at to be omitted when nil")
	}
}

func TestResult_JSON(t *testing.T) {
	r := Result{
		ID:           "res-1",
		RequestID:    "req-1",
		Content:      "Hello! How can I help?",
		InputTokens:  10,
		OutputTokens: 20,
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ID != r.ID {
		t.Errorf("ID = %s, want %s", decoded.ID, r.ID)
	}
	if decoded.RequestID != r.RequestID {
		t.Errorf("RequestID = %s, want %s", decoded.RequestID, r.RequestID)
	}
	if decoded.Content != r.Content {
		t.Errorf("Content = %s, want %s", decoded.Content, r.Content)
	}
	if decoded.InputTokens != r.InputTokens {
		t.Errorf("InputTokens = %d, want %d", decoded.InputTokens, r.InputTokens)
	}
	if decoded.OutputTokens != r.OutputTokens {
		t.Errorf("OutputTokens = %d, want %d", decoded.OutputTokens, r.OutputTokens)
	}
}

func TestResult_JSON_WithError(t *testing.T) {
	r := Result{
		ID:        "res-1",
		RequestID: "req-1",
		Error:     "rate limit exceeded",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Result
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Error != r.Error {
		t.Errorf("Error = %s, want %s", decoded.Error, r.Error)
	}
}

func TestResult_JSON_OmitsEmptyError(t *testing.T) {
	r := Result{
		ID:      "res-1",
		Content: "hello",
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	if containsKey(string(data), `"error"`) {
		t.Error("expected error to be omitted when empty")
	}
}

func containsKey(jsonStr, key string) bool {
	return len(jsonStr) > 0 && json.Valid([]byte(jsonStr)) &&
		// Simple check: look for the key in the JSON string
		len(key) > 0 && jsonContains(jsonStr, key)
}

func jsonContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
