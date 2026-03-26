package batch

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpenAISubmit_EmptyRequests(t *testing.T) {
	c := newOpenAIClient("key")
	_, err := c.Submit(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for empty requests")
	}
}

func TestOpenAISubmit_ExceedsMaxBatchSize(t *testing.T) {
	c := newOpenAIClient("key")
	reqs := make([]Request, openaiMaxBatchSize+1)
	_, err := c.Submit(context.Background(), reqs)
	if err == nil {
		t.Fatal("expected error for exceeding max batch size")
	}
}

func TestOpenAISubmit_DefaultsApplied(t *testing.T) {
	// Track what the file upload receives
	var uploadedData []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/files":
			// Parse multipart to get the JSONL content
			r.ParseMultipartForm(10 << 20)
			file, _, err := r.FormFile("file")
			if err != nil {
				t.Fatalf("FormFile: %v", err)
			}
			defer file.Close()
			buf := make([]byte, 10000)
			n, _ := file.Read(buf)
			uploadedData = buf[:n]

			purpose := r.FormValue("purpose")
			if purpose != "batch" {
				t.Errorf("purpose = %q, want batch", purpose)
			}
			json.NewEncoder(w).Encode(openaiFileUploadResponse{ID: "file-1"})

		case r.URL.Path == "/v1/batches":
			json.NewEncoder(w).Encode(openaiBatchResponse{
				ID: "b1", Status: "validating", CreatedAt: time.Now().Unix(),
			})
		}
	}))
	defer srv.Close()

	c := newOpenAIClient("key", WithBaseURL(srv.URL))
	_, err := c.Submit(context.Background(), []Request{
		{ID: "r1", UserPrompt: "hello"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	// Verify the JSONL line has default model and max_tokens
	var line openaiJSONLRequest
	if err := json.Unmarshal(uploadedData, &line); err != nil {
		t.Fatalf("unmarshal JSONL: %v", err)
	}
	if line.Body.Model != openaiDefaultModel {
		t.Errorf("model = %s, want %s", line.Body.Model, openaiDefaultModel)
	}
	if line.Body.MaxTokens != 4096 {
		t.Errorf("max_tokens = %d, want 4096", line.Body.MaxTokens)
	}
	if line.Method != "POST" {
		t.Errorf("method = %s, want POST", line.Method)
	}
	if line.URL != "/v1/chat/completions" {
		t.Errorf("url = %s, want /v1/chat/completions", line.URL)
	}
}

func TestOpenAISubmit_WithSystemPrompt(t *testing.T) {
	var uploadedData []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/files":
			r.ParseMultipartForm(10 << 20)
			file, _, _ := r.FormFile("file")
			defer file.Close()
			buf := make([]byte, 10000)
			n, _ := file.Read(buf)
			uploadedData = buf[:n]
			json.NewEncoder(w).Encode(openaiFileUploadResponse{ID: "file-1"})
		case r.URL.Path == "/v1/batches":
			json.NewEncoder(w).Encode(openaiBatchResponse{
				ID: "b1", Status: "validating", CreatedAt: time.Now().Unix(),
			})
		}
	}))
	defer srv.Close()

	c := newOpenAIClient("key", WithBaseURL(srv.URL))
	_, err := c.Submit(context.Background(), []Request{
		{ID: "r1", UserPrompt: "hello", SystemPrompt: "be helpful"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	var line openaiJSONLRequest
	json.Unmarshal(uploadedData, &line)
	if len(line.Body.Messages) != 2 {
		t.Fatalf("messages count = %d, want 2", len(line.Body.Messages))
	}
	if line.Body.Messages[0].Role != "system" {
		t.Errorf("first message role = %s, want system", line.Body.Messages[0].Role)
	}
	if line.Body.Messages[1].Role != "user" {
		t.Errorf("second message role = %s, want user", line.Body.Messages[1].Role)
	}
}

func TestOpenAIUploadFile_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("bad file"))
	}))
	defer srv.Close()

	c := newOpenAIClient("key", WithBaseURL(srv.URL))
	_, err := c.Submit(context.Background(), []Request{{ID: "r1", UserPrompt: "hi"}})
	if err == nil {
		t.Fatal("expected error for upload failure")
	}
}

func TestOpenAICancel_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %s, want POST", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := newOpenAIClient("key", WithBaseURL(srv.URL))
	err := c.Cancel(context.Background(), "batch-1")
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
}

func TestOpenAICancel_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte("conflict"))
	}))
	defer srv.Close()

	c := newOpenAIClient("key", WithBaseURL(srv.URL))
	err := c.Cancel(context.Background(), "batch-1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpenAIToBatchStatus_AllStates(t *testing.T) {
	tests := []struct {
		apiStatus  string
		wantStatus string
	}{
		{"validating", "processing"},
		{"in_progress", "processing"},
		{"finalizing", "processing"},
		{"completed", "completed"},
		{"failed", "failed"},
		{"expired", "expired"},
		{"cancelling", "failed"},
		{"cancelled", "failed"},
		{"unknown_state", "pending"}, // default
	}

	o := &openaiClient{}
	for _, tt := range tests {
		t.Run(tt.apiStatus, func(t *testing.T) {
			resp := &openaiBatchResponse{
				ID:        "b1",
				Status:    tt.apiStatus,
				CreatedAt: time.Now().Unix(),
			}
			status := o.toBatchStatus(resp)
			if status.Status != tt.wantStatus {
				t.Errorf("toBatchStatus(%q) = %q, want %q", tt.apiStatus, status.Status, tt.wantStatus)
			}
		})
	}
}

func TestOpenAIToBatchStatus_WithCounts(t *testing.T) {
	o := &openaiClient{}
	completedAt := time.Now().Unix()
	resp := &openaiBatchResponse{
		ID:            "b1",
		Status:        "completed",
		RequestCounts: &openaiRequestCounts{Total: 10, Completed: 8, Failed: 2},
		CreatedAt:     time.Now().Add(-time.Hour).Unix(),
		CompletedAt:   &completedAt,
	}
	status := o.toBatchStatus(resp)
	if status.Total != 10 {
		t.Errorf("Total = %d, want 10", status.Total)
	}
	if status.Completed != 8 {
		t.Errorf("Completed = %d, want 8", status.Completed)
	}
	if status.Failed != 2 {
		t.Errorf("Failed = %d, want 2", status.Failed)
	}
	if status.CompletedAt == nil {
		t.Fatal("CompletedAt should not be nil")
	}
}

func TestOpenAIToBatchStatus_NilCounts(t *testing.T) {
	o := &openaiClient{}
	resp := &openaiBatchResponse{
		ID:        "b1",
		Status:    "validating",
		CreatedAt: time.Now().Unix(),
	}
	status := o.toBatchStatus(resp)
	if status.Total != 0 {
		t.Errorf("Total = %d, want 0 when counts nil", status.Total)
	}
	if status.CompletedAt != nil {
		t.Errorf("CompletedAt should be nil when not set")
	}
}

func TestOpenAIDownloadResults_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return 2 JSONL lines
		line1, _ := json.Marshal(openaiResultLine{
			ID: "resp-1", CustomID: "r1",
			Response: &openaiResultBody{
				StatusCode: 200,
				Body: openaiChatResponse{
					Choices: []openaiChoice{{Message: openaiMessage{Role: "assistant", Content: "answer1"}}},
					Usage:   &openaiUsage{PromptTokens: 10, CompletionTokens: 5},
				},
			},
		})
		line2, _ := json.Marshal(openaiResultLine{
			ID: "resp-2", CustomID: "r2",
			Error: &openaiResultError{Code: "rate_limit", Message: "too fast"},
		})
		w.Write(line1)
		w.Write([]byte("\n"))
		w.Write(line2)
	}))
	defer srv.Close()

	c := newOpenAIClient("key", WithBaseURL(srv.URL))
	results, err := c.downloadResults(context.Background(), "file-out-1")
	if err != nil {
		t.Fatalf("downloadResults: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Content != "answer1" {
		t.Errorf("results[0].Content = %q, want answer1", results[0].Content)
	}
	if results[0].InputTokens != 10 {
		t.Errorf("InputTokens = %d, want 10", results[0].InputTokens)
	}
	if results[0].OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", results[0].OutputTokens)
	}
	if results[1].Error != "too fast" {
		t.Errorf("results[1].Error = %q, want too fast", results[1].Error)
	}
}

func TestOpenAIDownloadResults_EmptyLines(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		line, _ := json.Marshal(openaiResultLine{ID: "r1", CustomID: "c1"})
		w.Write([]byte("\n"))
		w.Write(line)
		w.Write([]byte("\n\n"))
	}))
	defer srv.Close()

	c := newOpenAIClient("key", WithBaseURL(srv.URL))
	results, err := c.downloadResults(context.Background(), "file-1")
	if err != nil {
		t.Fatalf("downloadResults: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("len(results) = %d, want 1 (empty lines skipped)", len(results))
	}
}

func TestOpenAIDownloadResults_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not valid json\n"))
	}))
	defer srv.Close()

	c := newOpenAIClient("key", WithBaseURL(srv.URL))
	_, err := c.downloadResults(context.Background(), "file-1")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestOpenAIDownloadResults_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	}))
	defer srv.Close()

	c := newOpenAIClient("key", WithBaseURL(srv.URL))
	_, err := c.downloadResults(context.Background(), "file-1")
	if err == nil {
		t.Fatal("expected error for API error")
	}
}

func TestOpenAISetHeaders(t *testing.T) {
	o := &openaiClient{apiKey: "sk-test"}
	req, _ := http.NewRequest("GET", "http://example.com", nil)
	o.setHeaders(req)

	if got := req.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Errorf("Authorization = %q, want Bearer sk-test", got)
	}
	if got := req.Header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
}

func TestOpenAIProvider(t *testing.T) {
	o := &openaiClient{}
	if o.Provider() != ProviderOpenAI {
		t.Errorf("Provider() = %s, want openai", o.Provider())
	}
}

func TestNewOpenAIClient_Defaults(t *testing.T) {
	c := newOpenAIClient("key")
	if c.baseURL != openaiDefaultBaseURL {
		t.Errorf("baseURL = %s, want %s", c.baseURL, openaiDefaultBaseURL)
	}
	if c.model != openaiDefaultModel {
		t.Errorf("model = %s, want %s", c.model, openaiDefaultModel)
	}
}

func TestOpenAIDownloadResults_NoUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		line, _ := json.Marshal(openaiResultLine{
			ID: "r1", CustomID: "c1",
			Response: &openaiResultBody{
				StatusCode: 200,
				Body: openaiChatResponse{
					Choices: []openaiChoice{{Message: openaiMessage{Content: "hi"}}},
					// Usage is nil
				},
			},
		})
		w.Write(line)
	}))
	defer srv.Close()

	c := newOpenAIClient("key", WithBaseURL(srv.URL))
	results, err := c.downloadResults(context.Background(), "f1")
	if err != nil {
		t.Fatalf("downloadResults: %v", err)
	}
	if results[0].InputTokens != 0 || results[0].OutputTokens != 0 {
		t.Errorf("expected zero tokens when usage is nil")
	}
}
