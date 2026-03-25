package batch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	claudeDefaultBaseURL = "https://api.anthropic.com"
	claudeDefaultModel   = "claude-sonnet-4-6"
	claudeAPIVersion     = "2023-06-01"
	claudeMaxBatchSize   = 10000
)

// claudeClient implements Client for the Anthropic Messages Batches API.
type claudeClient struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

func newClaudeClient(apiKey string, opts ...Option) *claudeClient {
	cfg := applyOpts(opts)
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = claudeDefaultBaseURL
	}
	model := cfg.Model
	if model == "" {
		model = claudeDefaultModel
	}
	return &claudeClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: cfg.httpClient(),
	}
}

// Claude batch API request/response types.

type claudeBatchRequest struct {
	Requests []claudeBatchItem `json:"requests"`
}

type claudeBatchItem struct {
	CustomID string              `json:"custom_id"`
	Params   claudeMessageParam `json:"params"`
}

type claudeMessageParam struct {
	Model     string         `json:"model"`
	MaxTokens int            `json:"max_tokens"`
	System    string         `json:"system,omitempty"`
	Messages  []claudeMsg    `json:"messages"`
}

type claudeMsg struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeBatchResponse struct {
	ID              string     `json:"id"`
	Type            string     `json:"type"`
	ProcessingStatus string   `json:"processing_status"`
	RequestCounts   claudeCounts `json:"request_counts"`
	CreatedAt       time.Time  `json:"created_at"`
	EndedAt         *time.Time `json:"ended_at,omitempty"`
}

type claudeCounts struct {
	Processing int `json:"processing"`
	Succeeded  int `json:"succeeded"`
	Errored    int `json:"errored"`
	Canceled   int `json:"canceled"`
	Expired    int `json:"expired"`
}

type claudeResultLine struct {
	CustomID string             `json:"custom_id"`
	Result   claudeResultDetail `json:"result"`
}

type claudeResultDetail struct {
	Type    string              `json:"type"`
	Message *claudeResultMsg    `json:"message,omitempty"`
	Error   *claudeResultError  `json:"error,omitempty"`
}

type claudeResultMsg struct {
	Content []claudeContentBlock `json:"content"`
	Usage   claudeUsage          `json:"usage"`
}

type claudeContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeResultError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

func (c *claudeClient) Provider() Provider { return ProviderClaude }

func (c *claudeClient) Submit(ctx context.Context, requests []Request) (*BatchStatus, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("batch: no requests provided")
	}
	if len(requests) > claudeMaxBatchSize {
		return nil, fmt.Errorf("batch: %d requests exceeds Claude max of %d", len(requests), claudeMaxBatchSize)
	}

	items := make([]claudeBatchItem, len(requests))
	for i, r := range requests {
		maxTokens := r.MaxTokens
		if maxTokens <= 0 {
			maxTokens = 4096
		}
		model := r.Model
		if model == "" {
			model = c.model
		}
		items[i] = claudeBatchItem{
			CustomID: r.ID,
			Params: claudeMessageParam{
				Model:     model,
				MaxTokens: maxTokens,
				System:    r.SystemPrompt,
				Messages:  []claudeMsg{{Role: "user", Content: r.UserPrompt}},
			},
		}
	}

	body, err := json.Marshal(claudeBatchRequest{Requests: items})
	if err != nil {
		return nil, fmt.Errorf("batch: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages/batches", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("batch: create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

	var batchResp claudeBatchResponse
	if err := json.Unmarshal(respBody, &batchResp); err != nil {
		return nil, fmt.Errorf("batch: unmarshal response: %w", err)
	}

	return c.toBatchStatus(&batchResp, len(requests)), nil
}

func (c *claudeClient) Poll(ctx context.Context, batchID string) (*BatchStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/messages/batches/"+batchID, nil)
	if err != nil {
		return nil, fmt.Errorf("batch: create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

	var batchResp claudeBatchResponse
	if err := json.Unmarshal(respBody, &batchResp); err != nil {
		return nil, fmt.Errorf("batch: unmarshal response: %w", err)
	}

	total := batchResp.RequestCounts.Processing + batchResp.RequestCounts.Succeeded +
		batchResp.RequestCounts.Errored + batchResp.RequestCounts.Canceled + batchResp.RequestCounts.Expired
	return c.toBatchStatus(&batchResp, total), nil
}

func (c *claudeClient) Results(ctx context.Context, batchID string) ([]Result, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/messages/batches/"+batchID+"/results", nil)
	if err != nil {
		return nil, fmt.Errorf("batch: create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("batch: api call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("batch: api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Results are JSONL (one JSON object per line).
	var results []Result
	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		var line claudeResultLine
		if err := decoder.Decode(&line); err != nil {
			return nil, fmt.Errorf("batch: decode result line: %w", err)
		}

		r := Result{
			RequestID: line.CustomID,
		}

		if line.Result.Error != nil {
			r.Error = line.Result.Error.Message
		}
		if line.Result.Message != nil {
			var text string
			for _, block := range line.Result.Message.Content {
				if block.Type == "text" {
					text += block.Text
				}
			}
			r.Content = text
			r.InputTokens = line.Result.Message.Usage.InputTokens
			r.OutputTokens = line.Result.Message.Usage.OutputTokens
		}

		results = append(results, r)
	}

	return results, nil
}

func (c *claudeClient) Cancel(ctx context.Context, batchID string) error {
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages/batches/"+batchID+"/cancel", nil)
	if err != nil {
		return fmt.Errorf("batch: create request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
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

func (c *claudeClient) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", claudeAPIVersion)
}

func (c *claudeClient) toBatchStatus(resp *claudeBatchResponse, total int) *BatchStatus {
	status := "processing"
	switch resp.ProcessingStatus {
	case "ended":
		status = "completed"
	case "canceling":
		status = "processing"
	}

	return &BatchStatus{
		ID:          resp.ID,
		Provider:    ProviderClaude,
		Status:      status,
		Total:       total,
		Completed:   resp.RequestCounts.Succeeded,
		Failed:      resp.RequestCounts.Errored + resp.RequestCounts.Canceled + resp.RequestCounts.Expired,
		CreatedAt:   resp.CreatedAt,
		CompletedAt: resp.EndedAt,
	}
}
