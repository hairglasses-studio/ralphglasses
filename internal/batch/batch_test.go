package batch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// ---------------------------------------------------------------------------
// Integration tests: Claude batch API (httptest)
// ---------------------------------------------------------------------------

func TestClaudeSubmit_HTTPRoundTrip(t *testing.T) {
	var capturedReq *http.Request
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		resp := claudeBatchResponse{
			ID:               "batch-abc",
			Type:             "message_batch",
			ProcessingStatus: "in_progress",
			RequestCounts:    claudeCounts{Processing: 2},
			CreatedAt:        time.Now(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, err := NewClient(ProviderClaude, "sk-ant-test-key", WithBaseURL(srv.URL))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	ctx := context.Background()
	status, err := c.Submit(ctx, []Request{
		{ID: "r1", UserPrompt: "Hello", MaxTokens: 100},
		{ID: "r2", UserPrompt: "World", SystemPrompt: "Be brief"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Verify HTTP method and path.
	if capturedReq.Method != "POST" {
		t.Errorf("method = %s, want POST", capturedReq.Method)
	}
	if !strings.HasSuffix(capturedReq.URL.Path, "/v1/messages/batches") {
		t.Errorf("path = %s, want suffix /v1/messages/batches", capturedReq.URL.Path)
	}

	// Verify Claude auth headers (x-api-key, not Bearer).
	if got := capturedReq.Header.Get("x-api-key"); got != "sk-ant-test-key" {
		t.Errorf("x-api-key = %q, want %q", got, "sk-ant-test-key")
	}
	if got := capturedReq.Header.Get("anthropic-version"); got != claudeAPIVersion {
		t.Errorf("anthropic-version = %q, want %q", got, claudeAPIVersion)
	}
	if got := capturedReq.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	// Verify body shape.
	var body claudeBatchRequest
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body.Requests) != 2 {
		t.Fatalf("len(requests) = %d, want 2", len(body.Requests))
	}
	if body.Requests[0].CustomID != "r1" {
		t.Errorf("requests[0].custom_id = %s, want r1", body.Requests[0].CustomID)
	}
	if body.Requests[1].Params.System != "Be brief" {
		t.Errorf("requests[1].system = %q, want %q", body.Requests[1].Params.System, "Be brief")
	}

	// Verify returned status.
	if status.ID != "batch-abc" {
		t.Errorf("status.ID = %s, want batch-abc", status.ID)
	}
	if status.Provider != ProviderClaude {
		t.Errorf("status.Provider = %s, want claude", status.Provider)
	}
}

func TestClaudePoll_HTTPRoundTrip(t *testing.T) {
	var capturedReq *http.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		resp := claudeBatchResponse{
			ID:               "batch-poll-1",
			ProcessingStatus: "ended",
			RequestCounts:    claudeCounts{Succeeded: 3, Errored: 1},
			CreatedAt:        time.Now(),
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, _ := NewClient(ProviderClaude, "sk-ant-poll", WithBaseURL(srv.URL))
	status, err := c.Poll(context.Background(), "batch-poll-1")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}

	if capturedReq.Method != "GET" {
		t.Errorf("method = %s, want GET", capturedReq.Method)
	}
	if !strings.HasSuffix(capturedReq.URL.Path, "/v1/messages/batches/batch-poll-1") {
		t.Errorf("path = %s, want suffix /v1/messages/batches/batch-poll-1", capturedReq.URL.Path)
	}
	if status.Status != "completed" {
		t.Errorf("status = %s, want completed", status.Status)
	}
	if status.Completed != 3 {
		t.Errorf("completed = %d, want 3", status.Completed)
	}
	if status.Failed != 1 {
		t.Errorf("failed = %d, want 1", status.Failed)
	}
}

func TestClaudeResults_HTTPRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify path includes /results suffix.
		if !strings.HasSuffix(r.URL.Path, "/results") {
			t.Errorf("path = %s, want suffix /results", r.URL.Path)
		}

		// Return JSONL (one object per line).
		line1 := claudeResultLine{
			CustomID: "r1",
			Result: claudeResultDetail{
				Type: "succeeded",
				Message: &claudeResultMsg{
					Content: []claudeContentBlock{{Type: "text", Text: "Hello there!"}},
					Usage:   claudeUsage{InputTokens: 10, OutputTokens: 5},
				},
			},
		}
		line2 := claudeResultLine{
			CustomID: "r2",
			Result: claudeResultDetail{
				Type:  "errored",
				Error: &claudeResultError{Type: "api_error", Message: "overloaded"},
			},
		}
		enc := json.NewEncoder(w)
		enc.Encode(line1)
		enc.Encode(line2)
	}))
	defer srv.Close()

	c, _ := NewClient(ProviderClaude, "sk-ant-res", WithBaseURL(srv.URL))
	results, err := c.Results(context.Background(), "batch-res-1")
	if err != nil {
		t.Fatalf("Results: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Content != "Hello there!" {
		t.Errorf("results[0].Content = %q, want %q", results[0].Content, "Hello there!")
	}
	if results[0].InputTokens != 10 {
		t.Errorf("results[0].InputTokens = %d, want 10", results[0].InputTokens)
	}
	if results[1].Error != "overloaded" {
		t.Errorf("results[1].Error = %q, want %q", results[1].Error, "overloaded")
	}
}

// ---------------------------------------------------------------------------
// Integration tests: OpenAI batch API (httptest)
// ---------------------------------------------------------------------------

func TestOpenAISubmit_HTTPRoundTrip(t *testing.T) {
	var fileUploadAuth string
	var batchCreateAuth string
	var batchCreateBody []byte

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch {
		case r.URL.Path == "/v1/files" && r.Method == "POST":
			// File upload endpoint.
			fileUploadAuth = r.Header.Get("Authorization")
			json.NewEncoder(w).Encode(openaiFileUploadResponse{ID: "file-upload-123"})

		case r.URL.Path == "/v1/batches" && r.Method == "POST":
			// Batch create endpoint.
			batchCreateAuth = r.Header.Get("Authorization")
			batchCreateBody, _ = io.ReadAll(r.Body)
			resp := openaiBatchResponse{
				ID:            "batch-oai-1",
				Status:        "validating",
				InputFileID:   "file-upload-123",
				RequestCounts: &openaiRequestCounts{Total: 1},
				CreatedAt:     time.Now().Unix(),
			}
			json.NewEncoder(w).Encode(resp)

		default:
			http.Error(w, "unexpected path: "+r.URL.Path, 404)
		}
	}))
	defer srv.Close()

	c, _ := NewClient(ProviderOpenAI, "sk-oai-test", WithBaseURL(srv.URL))
	status, err := c.Submit(context.Background(), []Request{
		{ID: "oai-r1", UserPrompt: "Hello", SystemPrompt: "You help", MaxTokens: 200},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Verify Bearer auth (not x-api-key).
	wantAuth := "Bearer sk-oai-test"
	if fileUploadAuth != wantAuth {
		t.Errorf("file upload auth = %q, want %q", fileUploadAuth, wantAuth)
	}
	if batchCreateAuth != wantAuth {
		t.Errorf("batch create auth = %q, want %q", batchCreateAuth, wantAuth)
	}

	// Verify batch create body shape.
	var createReq openaiBatchCreateRequest
	if err := json.Unmarshal(batchCreateBody, &createReq); err != nil {
		t.Fatalf("unmarshal batch create: %v", err)
	}
	if createReq.InputFileID != "file-upload-123" {
		t.Errorf("input_file_id = %s, want file-upload-123", createReq.InputFileID)
	}
	if createReq.Endpoint != "/v1/chat/completions" {
		t.Errorf("endpoint = %s, want /v1/chat/completions", createReq.Endpoint)
	}
	if createReq.CompletionWindow != "24h" {
		t.Errorf("completion_window = %s, want 24h", createReq.CompletionWindow)
	}

	if status.ID != "batch-oai-1" {
		t.Errorf("status.ID = %s, want batch-oai-1", status.ID)
	}
	if status.Provider != ProviderOpenAI {
		t.Errorf("status.Provider = %s, want openai", status.Provider)
	}
	if status.Status != "processing" {
		t.Errorf("status.Status = %s, want processing", status.Status)
	}
}

func TestOpenAIPoll_HTTPRoundTrip(t *testing.T) {
	var capturedReq *http.Request

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		completedAt := time.Now().Unix()
		resp := openaiBatchResponse{
			ID:            "batch-oai-poll",
			Status:        "completed",
			RequestCounts: &openaiRequestCounts{Total: 5, Completed: 4, Failed: 1},
			CreatedAt:     time.Now().Add(-1 * time.Hour).Unix(),
			CompletedAt:   &completedAt,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, _ := NewClient(ProviderOpenAI, "sk-oai-poll", WithBaseURL(srv.URL))
	status, err := c.Poll(context.Background(), "batch-oai-poll")
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}

	if capturedReq.Method != "GET" {
		t.Errorf("method = %s, want GET", capturedReq.Method)
	}
	if !strings.HasSuffix(capturedReq.URL.Path, "/v1/batches/batch-oai-poll") {
		t.Errorf("path = %s, want suffix /v1/batches/batch-oai-poll", capturedReq.URL.Path)
	}
	if capturedReq.Header.Get("Authorization") != "Bearer sk-oai-poll" {
		t.Errorf("auth = %q, want Bearer sk-oai-poll", capturedReq.Header.Get("Authorization"))
	}
	if status.Status != "completed" {
		t.Errorf("status = %s, want completed", status.Status)
	}
	if status.Completed != 4 {
		t.Errorf("completed = %d, want 4", status.Completed)
	}
	if status.CompletedAt == nil {
		t.Error("CompletedAt should not be nil")
	}
}

func TestOpenAIResults_HTTPRoundTrip(t *testing.T) {
	completedAt := time.Now().Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/batches/batch-oai-res":
			// Poll + batch detail: return completed with output file ID.
			resp := openaiBatchResponse{
				ID:            "batch-oai-res",
				Status:        "completed",
				OutputFileID:  "file-out-999",
				RequestCounts: &openaiRequestCounts{Total: 1, Completed: 1},
				CreatedAt:     time.Now().Unix(),
				CompletedAt:   &completedAt,
			}
			json.NewEncoder(w).Encode(resp)

		case r.URL.Path == "/v1/files/file-out-999/content":
			// Download results as JSONL.
			line := openaiResultLine{
				ID:       "resp-1",
				CustomID: "oai-r1",
				Response: &openaiResultBody{
					StatusCode: 200,
					Body: openaiChatResponse{
						Choices: []openaiChoice{{Message: openaiMessage{Role: "assistant", Content: "Hi!"}}},
						Usage:   &openaiUsage{PromptTokens: 8, CompletionTokens: 3},
					},
				},
			}
			json.NewEncoder(w).Encode(line)

		default:
			http.Error(w, "unexpected: "+r.URL.Path, 404)
		}
	}))
	defer srv.Close()

	c, _ := NewClient(ProviderOpenAI, "sk-oai-res", WithBaseURL(srv.URL))
	results, err := c.Results(context.Background(), "batch-oai-res")
	if err != nil {
		t.Fatalf("Results: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Content != "Hi!" {
		t.Errorf("results[0].Content = %q, want %q", results[0].Content, "Hi!")
	}
	if results[0].InputTokens != 8 {
		t.Errorf("InputTokens = %d, want 8", results[0].InputTokens)
	}
	if results[0].OutputTokens != 3 {
		t.Errorf("OutputTokens = %d, want 3", results[0].OutputTokens)
	}
}

// ---------------------------------------------------------------------------
// Integration tests: Gemini batch API (httptest)
// ---------------------------------------------------------------------------

func TestGeminiSubmit_HTTPRoundTrip(t *testing.T) {
	var capturedReq *http.Request
	var capturedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}

		resp := geminiBatchResponse{
			Responses: []geminiInlineResponse{
				{
					Candidates: []geminiCandidate{
						{Content: geminiContent{Parts: []geminiPart{{Text: "Gemini says hi"}}}},
					},
					UsageMetadata: &geminiUsageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 3},
				},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, _ := NewClient(ProviderGemini, "gemini-api-key", WithBaseURL(srv.URL))
	status, err := c.Submit(context.Background(), []Request{
		{ID: "g1", UserPrompt: "Hello Gemini", MaxTokens: 50},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Verify method and path (includes model and API key as query param).
	if capturedReq.Method != "POST" {
		t.Errorf("method = %s, want POST", capturedReq.Method)
	}
	if !strings.Contains(capturedReq.URL.Path, ":batchGenerateContent") {
		t.Errorf("path = %s, want to contain :batchGenerateContent", capturedReq.URL.Path)
	}

	// Gemini uses query param for auth, not header.
	qKey := capturedReq.URL.Query().Get("key")
	if qKey != "gemini-api-key" {
		t.Errorf("query key = %q, want %q", qKey, "gemini-api-key")
	}
	// Verify no Authorization header (Gemini uses query param).
	if auth := capturedReq.Header.Get("Authorization"); auth != "" {
		t.Errorf("unexpected Authorization header: %q", auth)
	}

	// Verify body shape.
	var body geminiBatchRequest
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(body.Requests) != 1 {
		t.Fatalf("len(requests) = %d, want 1", len(body.Requests))
	}
	if body.Requests[0].Contents[0].Parts[0].Text != "Hello Gemini" {
		t.Errorf("user prompt = %q, want %q", body.Requests[0].Contents[0].Parts[0].Text, "Hello Gemini")
	}
	if body.Requests[0].GenerationConfig == nil || body.Requests[0].GenerationConfig.MaxOutputTokens != 50 {
		t.Error("expected generationConfig.maxOutputTokens = 50")
	}

	// Gemini returns completed inline.
	if status.Status != "completed" {
		t.Errorf("status = %s, want completed", status.Status)
	}
	if status.Completed != 1 {
		t.Errorf("completed = %d, want 1", status.Completed)
	}
	if status.Provider != ProviderGemini {
		t.Errorf("provider = %s, want gemini", status.Provider)
	}

	// Verify we can retrieve results via the stored batch ID.
	results, err := c.Results(context.Background(), status.ID)
	if err != nil {
		t.Fatalf("Results: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Content != "Gemini says hi" {
		t.Errorf("results[0].Content = %q, want %q", results[0].Content, "Gemini says hi")
	}
	if results[0].InputTokens != 5 {
		t.Errorf("InputTokens = %d, want 5", results[0].InputTokens)
	}
}

func TestGeminiPoll_ReturnsStoredResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiBatchResponse{
			Responses: []geminiInlineResponse{
				{Candidates: []geminiCandidate{{Content: geminiContent{Parts: []geminiPart{{Text: "a"}}}}}},
				{Candidates: []geminiCandidate{{Content: geminiContent{Parts: []geminiPart{{Text: "b"}}}}}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c, _ := NewClient(ProviderGemini, "gk", WithBaseURL(srv.URL))
	status, err := c.Submit(context.Background(), []Request{
		{ID: "g1", UserPrompt: "a"},
		{ID: "g2", UserPrompt: "b"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	pollStatus, err := c.Poll(context.Background(), status.ID)
	if err != nil {
		t.Fatalf("Poll: %v", err)
	}
	if pollStatus.Status != "completed" {
		t.Errorf("poll status = %s, want completed", pollStatus.Status)
	}
	if pollStatus.Total != 2 {
		t.Errorf("poll total = %d, want 2", pollStatus.Total)
	}
}

func TestGeminiPoll_UnknownBatch(t *testing.T) {
	c, _ := NewClient(ProviderGemini, "gk")
	_, err := c.Poll(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown batch ID")
	}
}

// ---------------------------------------------------------------------------
// Cross-provider auth header verification
// ---------------------------------------------------------------------------

func TestAuthHeaders_PerProvider(t *testing.T) {
	tests := []struct {
		provider  Provider
		apiKey    string
		wantKey   string // header name to check
		wantValue string // expected header value
		noBearer  bool   // if true, Authorization header should be absent
	}{
		{ProviderClaude, "sk-ant-123", "x-api-key", "sk-ant-123", true},
		{ProviderOpenAI, "sk-oai-456", "Authorization", "Bearer sk-oai-456", false},
	}

	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			var captured http.Header
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				captured = r.Header.Clone()
				// Return minimal valid response to avoid errors.
				switch tt.provider {
				case ProviderClaude:
					json.NewEncoder(w).Encode(claudeBatchResponse{
						ID: "b1", ProcessingStatus: "ended",
						CreatedAt: time.Now(),
					})
				case ProviderOpenAI:
					json.NewEncoder(w).Encode(openaiBatchResponse{
						ID: "b1", Status: "completed",
						CreatedAt: time.Now().Unix(),
					})
				}
			}))
			defer srv.Close()

			c, _ := NewClient(tt.provider, tt.apiKey, WithBaseURL(srv.URL))
			_, _ = c.Poll(context.Background(), "b1")

			got := captured.Get(tt.wantKey)
			if got != tt.wantValue {
				t.Errorf("%s header %s = %q, want %q", tt.provider, tt.wantKey, got, tt.wantValue)
			}

			if tt.noBearer {
				if auth := captured.Get("Authorization"); auth != "" {
					t.Errorf("%s should not have Authorization header, got %q", tt.provider, auth)
				}
			}
		})
	}

	// Gemini: verify auth via query param, not header.
	t.Run("gemini", func(t *testing.T) {
		var capturedURL string
		var capturedAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedURL = r.URL.String()
			capturedAuth = r.Header.Get("Authorization")
			json.NewEncoder(w).Encode(geminiBatchResponse{
				Responses: []geminiInlineResponse{
					{Candidates: []geminiCandidate{{Content: geminiContent{Parts: []geminiPart{{Text: "ok"}}}}}},
				},
			})
		}))
		defer srv.Close()

		c, _ := NewClient(ProviderGemini, "gemini-key-789", WithBaseURL(srv.URL))
		_, _ = c.Submit(context.Background(), []Request{{ID: "x", UserPrompt: "test"}})

		if !strings.Contains(capturedURL, "key=gemini-key-789") {
			t.Errorf("Gemini URL should contain key=gemini-key-789, got %s", capturedURL)
		}
		if capturedAuth != "" {
			t.Errorf("Gemini should not have Authorization header, got %q", capturedAuth)
		}
	})
}

// Suppress unused import warnings.
var _ = fmt.Sprintf
var _ = context.Background

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
