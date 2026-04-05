package batch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	geminiDefaultBaseURL = "https://generativelanguage.googleapis.com"
	geminiDefaultModel   = "gemini-2.5-flash"
)

// geminiClient implements Client for the Gemini batch API.
type geminiClient struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client

	mu      sync.RWMutex
	results map[string][]Result
}

func newGeminiClient(apiKey string, opts ...Option) *geminiClient {
	cfg := applyOpts(opts)
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = geminiDefaultBaseURL
	}
	model := cfg.Model
	if model == "" {
		model = geminiDefaultModel
	}
	return &geminiClient{
		apiKey:     apiKey,
		baseURL:    baseURL,
		model:      model,
		httpClient: cfg.httpClient(),
	}
}

// Gemini batch API request/response types.

type geminiBatchRequest struct {
	Requests []geminiInlineRequest `json:"requests"`
}

type geminiInlineRequest struct {
	Contents          []geminiContent      `json:"contents"`
	SystemInstruction *geminiContent       `json:"systemInstruction,omitempty"`
	GenerationConfig  *geminiGenerationCfg `json:"generationConfig,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationCfg struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

type geminiBatchResponse struct {
	Responses []geminiInlineResponse `json:"responses"`
}

type geminiInlineResponse struct {
	Candidates    []geminiCandidate    `json:"candidates"`
	UsageMetadata *geminiUsageMetadata `json:"usageMetadata,omitempty"`
	Error         *geminiError         `json:"error,omitempty"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Provider returns ProviderGemini.
func (g *geminiClient) Provider() Provider { return ProviderGemini }

// Submit sends a batch of requests to the Gemini API using inline batch prediction.
func (g *geminiClient) Submit(ctx context.Context, requests []Request) (*BatchStatus, error) {
	if len(requests) == 0 {
		return nil, fmt.Errorf("batch: no requests provided")
	}

	model := g.model
	if len(requests) > 0 && requests[0].Model != "" {
		model = requests[0].Model
	}

	inlineReqs := make([]geminiInlineRequest, len(requests))
	for i, r := range requests {
		ir := geminiInlineRequest{
			Contents: []geminiContent{
				{Parts: []geminiPart{{Text: r.UserPrompt}}, Role: "user"},
			},
		}
		if r.SystemPrompt != "" {
			ir.SystemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: r.SystemPrompt}},
			}
		}
		if r.MaxTokens > 0 {
			ir.GenerationConfig = &geminiGenerationCfg{MaxOutputTokens: r.MaxTokens}
		}
		inlineReqs[i] = ir
	}

	body, err := json.Marshal(geminiBatchRequest{Requests: inlineReqs})
	if err != nil {
		return nil, fmt.Errorf("batch: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:batchGenerateContent?key=%s", g.baseURL, model, g.apiKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("batch: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
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

	// Gemini returns results inline for batchGenerateContent.
	// Store them keyed by request IDs for later retrieval.
	var batchResp geminiBatchResponse
	if err := json.Unmarshal(respBody, &batchResp); err != nil {
		return nil, fmt.Errorf("batch: unmarshal response: %w", err)
	}

	// Generate a synthetic batch ID since Gemini returns results inline.
	batchID := fmt.Sprintf("gemini-batch-%d", time.Now().UnixNano())

	// Store the raw response in the client for later retrieval.
	g.storeResults(batchID, requests, &batchResp)

	completed := 0
	failed := 0
	for _, r := range batchResp.Responses {
		if r.Error != nil {
			failed++
		} else {
			completed++
		}
	}

	now := time.Now()
	return &BatchStatus{
		ID:          batchID,
		Provider:    ProviderGemini,
		Status:      "completed",
		Total:       len(requests),
		Completed:   completed,
		Failed:      failed,
		CreatedAt:   now,
		CompletedAt: &now,
	}, nil
}

// Poll returns the status of a Gemini batch. Since Gemini inline batches complete synchronously,
// this always returns completed if the batch ID is known.
func (g *geminiClient) Poll(_ context.Context, batchID string) (*BatchStatus, error) {
	// Gemini batch results are returned inline, so polling always returns completed
	// if we have the results, or not found otherwise.
	g.mu.RLock()
	defer g.mu.RUnlock()

	stored, ok := g.results[batchID]
	if !ok {
		return nil, fmt.Errorf("batch: unknown batch ID %s", batchID)
	}

	return &BatchStatus{
		ID:        batchID,
		Provider:  ProviderGemini,
		Status:    "completed",
		Total:     len(stored),
		Completed: len(stored),
		CreatedAt: time.Now(),
	}, nil
}

// Results retrieves the stored results for a completed Gemini batch.
func (g *geminiClient) Results(_ context.Context, batchID string) ([]Result, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	stored, ok := g.results[batchID]
	if !ok {
		return nil, fmt.Errorf("batch: unknown batch ID %s", batchID)
	}
	return stored, nil
}

// Cancel removes stored results for a Gemini batch. Since inline batches complete immediately,
// this is effectively a cleanup operation.
func (g *geminiClient) Cancel(_ context.Context, batchID string) error {
	// Gemini inline batches complete immediately; cancel is a no-op.
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.results, batchID)
	return nil
}

func (g *geminiClient) storeResults(batchID string, requests []Request, resp *geminiBatchResponse) {
	results := make([]Result, len(resp.Responses))
	for i, r := range resp.Responses {
		res := Result{}
		if i < len(requests) {
			res.RequestID = requests[i].ID
		}
		if r.Error != nil {
			res.Error = r.Error.Message
		} else if len(r.Candidates) > 0 {
			var text strings.Builder
			for _, p := range r.Candidates[0].Content.Parts {
				text.WriteString(p.Text)
			}
			res.Content = text.String()
		}
		if r.UsageMetadata != nil {
			res.InputTokens = r.UsageMetadata.PromptTokenCount
			res.OutputTokens = r.UsageMetadata.CandidatesTokenCount
		}
		results[i] = res
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	if g.results == nil {
		g.results = make(map[string][]Result)
	}
	g.results[batchID] = results
}
