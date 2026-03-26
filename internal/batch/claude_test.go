package batch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClaudeSubmit_EmptyRequests(t *testing.T) {
	c := newClaudeClient("key")
	_, err := c.Submit(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for empty requests")
	}
}

func TestClaudeSubmit_ExceedsMaxBatchSize(t *testing.T) {
	c := newClaudeClient("key")
	reqs := make([]Request, claudeMaxBatchSize+1)
	_, err := c.Submit(context.Background(), reqs)
	if err == nil {
		t.Fatal("expected error for exceeding max batch size")
	}
}

func TestClaudeSubmit_DefaultMaxTokens(t *testing.T) {
	var body claudeBatchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(claudeBatchResponse{
			ID: "b1", ProcessingStatus: "in_progress", CreatedAt: time.Now(),
		})
	}))
	defer srv.Close()

	c := newClaudeClient("key", WithBaseURL(srv.URL))
	_, err := c.Submit(context.Background(), []Request{
		{ID: "r1", UserPrompt: "hello"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if body.Requests[0].Params.MaxTokens != 4096 {
		t.Errorf("default MaxTokens = %d, want 4096", body.Requests[0].Params.MaxTokens)
	}
}

func TestClaudeSubmit_UsesRequestModel(t *testing.T) {
	var body claudeBatchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&body)
		json.NewEncoder(w).Encode(claudeBatchResponse{
			ID: "b1", ProcessingStatus: "in_progress", CreatedAt: time.Now(),
		})
	}))
	defer srv.Close()

	c := newClaudeClient("key", WithBaseURL(srv.URL))
	_, err := c.Submit(context.Background(), []Request{
		{ID: "r1", UserPrompt: "hi", Model: "claude-opus-4-6"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if body.Requests[0].Params.Model != "claude-opus-4-6" {
		t.Errorf("model = %s, want claude-opus-4-6", body.Requests[0].Params.Model)
	}
}

func TestClaudeSubmit_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer srv.Close()

	c := newClaudeClient("key", WithBaseURL(srv.URL))
	_, err := c.Submit(context.Background(), []Request{{ID: "r1", UserPrompt: "hi"}})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestClaudePoll_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("server error"))
	}))
	defer srv.Close()

	c := newClaudeClient("key", WithBaseURL(srv.URL))
	_, err := c.Poll(context.Background(), "batch-1")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestClaudeResults_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	c := newClaudeClient("key", WithBaseURL(srv.URL))
	_, err := c.Results(context.Background(), "batch-1")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestClaudeCancel_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Cancel method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newClaudeClient("key", WithBaseURL(srv.URL))
	err := c.Cancel(context.Background(), "batch-cancel-1")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
}

func TestClaudeCancel_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("already cancelled"))
	}))
	defer srv.Close()

	c := newClaudeClient("key", WithBaseURL(srv.URL))
	err := c.Cancel(context.Background(), "batch-1")
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestClaudeToBatchStatus_AllStates(t *testing.T) {
	tests := []struct {
		processing string
		wantStatus string
	}{
		{"in_progress", "processing"},
		{"ended", "completed"},
		{"canceling", "processing"},
		{"something_else", "processing"}, // default case
	}

	c := &claudeClient{}
	for _, tt := range tests {
		t.Run(tt.processing, func(t *testing.T) {
			resp := &claudeBatchResponse{
				ID:               "b1",
				ProcessingStatus: tt.processing,
				RequestCounts: claudeCounts{
					Succeeded: 5,
					Errored:   1,
					Canceled:  2,
					Expired:   1,
				},
				CreatedAt: time.Now(),
			}
			status := c.toBatchStatus(resp, 10)
			if status.Status != tt.wantStatus {
				t.Errorf("toBatchStatus(%q) = %q, want %q", tt.processing, status.Status, tt.wantStatus)
			}
			if status.Provider != ProviderClaude {
				t.Errorf("Provider = %s, want claude", status.Provider)
			}
			if status.Completed != 5 {
				t.Errorf("Completed = %d, want 5", status.Completed)
			}
			// Failed = Errored + Canceled + Expired = 1+2+1 = 4
			if status.Failed != 4 {
				t.Errorf("Failed = %d, want 4", status.Failed)
			}
		})
	}
}

func TestClaudeToBatchStatus_EndedAt(t *testing.T) {
	c := &claudeClient{}
	now := time.Now()
	resp := &claudeBatchResponse{
		ID:               "b1",
		ProcessingStatus: "ended",
		CreatedAt:        now.Add(-time.Hour),
		EndedAt:          &now,
	}
	status := c.toBatchStatus(resp, 1)
	if status.CompletedAt == nil {
		t.Fatal("CompletedAt should not be nil when EndedAt is set")
	}
	if !status.CompletedAt.Equal(now) {
		t.Errorf("CompletedAt = %v, want %v", status.CompletedAt, now)
	}
}

func TestClaudeResults_MultipleTextBlocks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		line := claudeResultLine{
			CustomID: "r1",
			Result: claudeResultDetail{
				Type: "succeeded",
				Message: &claudeResultMsg{
					Content: []claudeContentBlock{
						{Type: "text", Text: "part1 "},
						{Type: "image", Text: "ignored"},
						{Type: "text", Text: "part2"},
					},
					Usage: claudeUsage{InputTokens: 10, OutputTokens: 8},
				},
			},
		}
		json.NewEncoder(w).Encode(line)
	}))
	defer srv.Close()

	c := newClaudeClient("key", WithBaseURL(srv.URL))
	results, err := c.Results(context.Background(), "batch-1")
	if err != nil {
		t.Fatalf("Results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	// Only text blocks concatenated
	if results[0].Content != "part1 part2" {
		t.Errorf("Content = %q, want %q", results[0].Content, "part1 part2")
	}
}

func TestClaudeSetHeaders(t *testing.T) {
	c := &claudeClient{apiKey: "test-key-123"}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	c.setHeaders(req)

	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := req.Header.Get("x-api-key"); got != "test-key-123" {
		t.Errorf("x-api-key = %q, want test-key-123", got)
	}
	if got := req.Header.Get("anthropic-version"); got != claudeAPIVersion {
		t.Errorf("anthropic-version = %q, want %q", got, claudeAPIVersion)
	}
}

func TestClaudeProvider(t *testing.T) {
	c := &claudeClient{}
	if c.Provider() != ProviderClaude {
		t.Errorf("Provider() = %s, want claude", c.Provider())
	}
}

func TestNewClaudeClient_Defaults(t *testing.T) {
	c := newClaudeClient("key")
	if c.baseURL != claudeDefaultBaseURL {
		t.Errorf("baseURL = %s, want %s", c.baseURL, claudeDefaultBaseURL)
	}
	if c.model != claudeDefaultModel {
		t.Errorf("model = %s, want %s", c.model, claudeDefaultModel)
	}
	if c.apiKey != "key" {
		t.Errorf("apiKey = %s, want key", c.apiKey)
	}
}
