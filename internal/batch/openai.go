package batch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const (
	openaiDefaultBaseURL = "https://api.openai.com"
	openaiDefaultModel   = "gpt-4o"
	openaiMaxBatchSize   = 50000
)

// openaiClient implements Client for the OpenAI Batch API.
type openaiClient struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

func newOpenAIClient(apiKey string, opts ...Option) *openaiClient {
	cfg := applyOpts(opts)
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = openaiDefaultBaseURL
	}
	model := cfg.Model
	if model == "" {
		model = openaiDefaultModel
	}
	return &openaiClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: cfg.httpClient(),
	}
}

// OpenAI batch API types.

type openaiJSONLRequest struct {
	CustomID string              `json:"custom_id"`
	Method   string              `json:"method"`
	URL      string              `json:"url"`
	Body     openaiChatRequest   `json:"body"`
}

type openaiChatRequest struct {
	Model     string           `json:"model"`
	Messages  []openaiMessage  `json:"messages"`
	MaxTokens int              `json:"max_tokens,omitempty"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiFileUploadResponse struct {
	ID string `json:"id"`
}

type openaiBatchCreateRequest struct {
	InputFileID      string `json:"input_file_id"`
	Endpoint         string `json:"endpoint"`
	CompletionWindow string `json:"completion_window"`
}

type openaiBatchResponse struct {
	ID               string              `json:"id"`
	Status           string              `json:"status"`
	InputFileID      string              `json:"input_file_id"`
	OutputFileID     string              `json:"output_file_id,omitempty"`
	ErrorFileID      string              `json:"error_file_id,omitempty"`
	RequestCounts    *openaiRequestCounts `json:"request_counts,omitempty"`
	CreatedAt        int64               `json:"created_at"`
	CompletedAt      *int64              `json:"completed_at,omitempty"`
}

type openaiRequestCounts struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

type openaiResultLine struct {
	ID       string              `json:"id"`
	CustomID string              `json:"custom_id"`
	Response *openaiResultBody   `json:"response,omitempty"`
	Error    *openaiResultError  `json:"error,omitempty"`
}

type openaiResultBody struct {
	StatusCode int                  `json:"status_code"`
	Body       openaiChatResponse   `json:"body"`
}

type openaiChatResponse struct {
	Choices []openaiChoice `json:"choices"`
	Usage   *openaiUsage   `json:"usage,omitempty"`
}

type openaiChoice struct {
	Message openaiMessage `json:"message"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type openaiResultError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (o *openaiClient) Provider() Provider { return ProviderOpenAI }

func (o *openaiClient) Submit(ctx context.Context, requests []Request) (*BatchStatus, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("batch: no requests provided")
	}
	if len(requests) > openaiMaxBatchSize {
		return nil, fmt.Errorf("batch: %d requests exceeds OpenAI max of %d", len(requests), openaiMaxBatchSize)
	}

	// Step 1: Build JSONL content.
	var jsonlBuf bytes.Buffer
	encoder := json.NewEncoder(&jsonlBuf)
	for _, r := range requests {
		model := r.Model
		if model == "" {
			model = o.model
		}
		maxTokens := r.MaxTokens
		if maxTokens <= 0 {
			maxTokens = 4096
		}

		var msgs []openaiMessage
		if r.SystemPrompt != "" {
			msgs = append(msgs, openaiMessage{Role: "system", Content: r.SystemPrompt})
		}
		msgs = append(msgs, openaiMessage{Role: "user", Content: r.UserPrompt})

		line := openaiJSONLRequest{
			CustomID: r.ID,
			Method:   "POST",
			URL:      "/v1/chat/completions",
			Body: openaiChatRequest{
				Model:     model,
				Messages:  msgs,
				MaxTokens: maxTokens,
			},
		}
		if err := encoder.Encode(line); err != nil {
			return nil, fmt.Errorf("batch: encode jsonl line: %w", err)
		}
	}

	// Step 2: Upload the JSONL file.
	fileID, err := o.uploadFile(ctx, jsonlBuf.Bytes())
	if err != nil {
		return nil, err
	}

	// Step 3: Create the batch.
	createReq := openaiBatchCreateRequest{
		InputFileID:      fileID,
		Endpoint:         "/v1/chat/completions",
		CompletionWindow: "24h",
	}
	body, err := json.Marshal(createReq)
	if err != nil {
		return nil, fmt.Errorf("batch: marshal batch create: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/v1/batches", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("batch: create request: %w", err)
	}
	o.setHeaders(req)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("batch: api call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("batch: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("batch: api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var batchResp openaiBatchResponse
	if err := json.Unmarshal(respBody, &batchResp); err != nil {
		return nil, fmt.Errorf("batch: unmarshal response: %w", err)
	}

	return o.toBatchStatus(&batchResp), nil
}

func (o *openaiClient) Poll(ctx context.Context, batchID string) (*BatchStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/v1/batches/"+batchID, nil)
	if err != nil {
		return nil, fmt.Errorf("batch: create request: %w", err)
	}
	o.setHeaders(req)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("batch: api call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("batch: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("batch: api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var batchResp openaiBatchResponse
	if err := json.Unmarshal(respBody, &batchResp); err != nil {
		return nil, fmt.Errorf("batch: unmarshal response: %w", err)
	}

	return o.toBatchStatus(&batchResp), nil
}

func (o *openaiClient) Results(ctx context.Context, batchID string) ([]Result, error) {
	// First poll to get the output file ID.
	status, err := o.Poll(ctx, batchID)
	if err != nil {
		return nil, err
	}
	if status.Status != "completed" {
		return nil, fmt.Errorf("batch: batch %s is not complete (status: %s)", batchID, status.Status)
	}

	// Get the batch details for the output file ID.
	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/v1/batches/"+batchID, nil)
	if err != nil {
		return nil, fmt.Errorf("batch: create request: %w", err)
	}
	o.setHeaders(req)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("batch: api call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("batch: read response: %w", err)
	}

	var batchResp openaiBatchResponse
	if err := json.Unmarshal(respBody, &batchResp); err != nil {
		return nil, fmt.Errorf("batch: unmarshal response: %w", err)
	}

	if batchResp.OutputFileID == "" {
		return nil, fmt.Errorf("batch: no output file for batch %s", batchID)
	}

	// Download the output file.
	return o.downloadResults(ctx, batchResp.OutputFileID)
}

func (o *openaiClient) Cancel(ctx context.Context, batchID string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/v1/batches/"+batchID+"/cancel", nil)
	if err != nil {
		return fmt.Errorf("batch: create request: %w", err)
	}
	o.setHeaders(req)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("batch: api call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("batch: api error (status %d): %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (o *openaiClient) uploadFile(ctx context.Context, jsonlData []byte) (string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if err := w.WriteField("purpose", "batch"); err != nil {
		return "", fmt.Errorf("batch: write purpose field: %w", err)
	}

	fw, err := w.CreateFormFile("file", "batch_input.jsonl")
	if err != nil {
		return "", fmt.Errorf("batch: create form file: %w", err)
	}
	if _, err := fw.Write(jsonlData); err != nil {
		return "", fmt.Errorf("batch: write file data: %w", err)
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/v1/files", &buf)
	if err != nil {
		return "", fmt.Errorf("batch: create upload request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("batch: upload file: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("batch: read upload response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("batch: upload error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var fileResp openaiFileUploadResponse
	if err := json.Unmarshal(respBody, &fileResp); err != nil {
		return "", fmt.Errorf("batch: unmarshal upload response: %w", err)
	}
	return fileResp.ID, nil
}

func (o *openaiClient) downloadResults(ctx context.Context, fileID string) ([]Result, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/v1/files/"+fileID+"/content", nil)
	if err != nil {
		return nil, fmt.Errorf("batch: create download request: %w", err)
	}
	o.setHeaders(req)

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("batch: download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("batch: download error (status %d): %s", resp.StatusCode, string(respBody))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("batch: read download: %w", err)
	}

	var results []Result
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if line == "" {
			continue
		}
		var rl openaiResultLine
		if err := json.Unmarshal([]byte(line), &rl); err != nil {
			return nil, fmt.Errorf("batch: unmarshal result line: %w", err)
		}

		r := Result{
			ID:        rl.ID,
			RequestID: rl.CustomID,
		}
		if rl.Error != nil {
			r.Error = rl.Error.Message
		}
		if rl.Response != nil && len(rl.Response.Body.Choices) > 0 {
			r.Content = rl.Response.Body.Choices[0].Message.Content
			if rl.Response.Body.Usage != nil {
				r.InputTokens = rl.Response.Body.Usage.PromptTokens
				r.OutputTokens = rl.Response.Body.Usage.CompletionTokens
			}
		}
		results = append(results, r)
	}

	return results, nil
}

func (o *openaiClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
}

func (o *openaiClient) toBatchStatus(resp *openaiBatchResponse) *BatchStatus {
	status := "pending"
	switch resp.Status {
	case "validating", "in_progress", "finalizing":
		status = "processing"
	case "completed":
		status = "completed"
	case "failed":
		status = "failed"
	case "expired":
		status = "expired"
	case "cancelling", "cancelled":
		status = "failed"
	}

	bs := &BatchStatus{
		ID:        resp.ID,
		Provider:  ProviderOpenAI,
		Status:    status,
		CreatedAt: time.Unix(resp.CreatedAt, 0),
	}

	if resp.RequestCounts != nil {
		bs.Total = resp.RequestCounts.Total
		bs.Completed = resp.RequestCounts.Completed
		bs.Failed = resp.RequestCounts.Failed
	}

	if resp.CompletedAt != nil {
		t := time.Unix(*resp.CompletedAt, 0)
		bs.CompletedAt = &t
	}

	return bs
}
