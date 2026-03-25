package enhancer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenAIClient calls the OpenAI Responses API to improve prompts using a meta-prompt.
type OpenAIClient struct {
	APIKey       string
	Model        string
	BaseURL      string
	HTTPClient   *http.Client
	UseWebSocket bool // When true, prefer WebSocket transport for multi-turn tool chains.
}

// NewOpenAIClient creates an OpenAI client from config. Returns nil if no API key is available.
func NewOpenAIClient(cfg LLMConfig) *OpenAIClient {
	apiKey := os.Getenv(cfg.APIKeyEnv)
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		return nil
	}

	model := cfg.Model
	if model == "" {
		model = "o3"
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &OpenAIClient{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Provider returns the provider name.
func (c *OpenAIClient) Provider() ProviderName { return ProviderOpenAI }

// responsesRequest is the OpenAI Responses API request body.
type responsesRequest struct {
	Model          string           `json:"model"`
	Instructions   string           `json:"instructions"`
	Input          string           `json:"input"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	Reasoning      *reasoningConfig `json:"reasoning,omitempty"`
}

// reasoningConfig controls the reasoning effort for the Responses API.
type reasoningConfig struct {
	Effort string `json:"effort"` // "none", "low", "medium", "high"
}

// responsesResponse is the OpenAI Responses API response body.
type responsesResponse struct {
	ID     string           `json:"id"`
	Output []responseOutput `json:"output"`
	Error  *openaiError     `json:"error,omitempty"`
	Usage  *responseUsage   `json:"usage,omitempty"`
}

// responseOutput is a single output item from the Responses API.
type responseOutput struct {
	Type    string          `json:"type"`
	Content []outputContent `json:"content"`
}

// outputContent is a content block within a response output.
type outputContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// responseUsage tracks token usage from the Responses API.
type responseUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type openaiError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// reasoningEffort returns the reasoning effort level based on task type.
// Simple/routine tasks use "low", complex tasks use "medium".
func reasoningEffort(taskType TaskType) string {
	switch taskType {
	case TaskTypeTroubleshooting, TaskTypeWorkflow:
		return "low"
	case TaskTypeCode, TaskTypeCreative, TaskTypeAnalysis:
		return "medium"
	default:
		return "medium"
	}
}

// Improve sends the prompt to OpenAI with a meta-prompt and returns the improved version.
func (c *OpenAIClient) Improve(ctx context.Context, prompt string, opts ImproveOptions) (*ImproveResult, error) {
	instructions := MetaPromptFor(ProviderOpenAI, opts.ThinkingEnabled)

	userContent := prompt
	if opts.Feedback != "" {
		userContent += "\n\n[Additional guidance: " + opts.Feedback + "]"
	}

	effort := reasoningEffort(opts.TaskType)

	reqBody := responsesRequest{
		Model:          c.Model,
		Instructions:   instructions,
		Input:          userContent,
		MaxOutputTokens: 4096,
		Reasoning:      &reasoningConfig{Effort: effort},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/responses", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var apiResp responsesResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("api error: %s: %s", apiResp.Error.Type, apiResp.Error.Message)
	}

	enhanced := extractResponseText(apiResp.Output)

	return &ImproveResult{
		Enhanced:     strings.TrimSpace(enhanced),
		TaskType:     string(opts.TaskType),
		Improvements: []string{"LLM-powered improvement via OpenAI Responses API"},
	}, nil
}

// extractResponseText extracts the text content from Responses API output.
func extractResponseText(outputs []responseOutput) string {
	for _, output := range outputs {
		if output.Type != "message" {
			continue
		}
		for _, content := range output.Content {
			if content.Type == "output_text" {
				return content.Text
			}
		}
	}
	return ""
}
