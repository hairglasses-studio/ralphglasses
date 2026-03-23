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

// OpenAIClient calls the OpenAI Chat Completions API to improve prompts using a meta-prompt.
type OpenAIClient struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
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

// openaiRequest is the OpenAI Chat Completions request body.
type openaiRequest struct {
	Model              string          `json:"model"`
	Messages           []openaiMessage `json:"messages"`
	MaxCompletionTokens int            `json:"max_completion_tokens"`
}

type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Error   *openaiError   `json:"error,omitempty"`
}

type openaiChoice struct {
	Message openaiMessage `json:"message"`
}

type openaiError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Improve sends the prompt to OpenAI with a meta-prompt and returns the improved version.
func (c *OpenAIClient) Improve(ctx context.Context, prompt string, opts ImproveOptions) (*ImproveResult, error) {
	systemPrompt := MetaPromptFor(ProviderOpenAI, opts.ThinkingEnabled)

	userContent := prompt
	if opts.Feedback != "" {
		userContent += "\n\n[Additional guidance: " + opts.Feedback + "]"
	}

	reqBody := openaiRequest{
		Model: c.Model,
		Messages: []openaiMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
		MaxCompletionTokens: 4096,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.BaseURL+"/v1/chat/completions", bytes.NewReader(bodyBytes))
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

	var apiResp openaiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("api error: %s: %s", apiResp.Error.Type, apiResp.Error.Message)
	}

	enhanced := ""
	if len(apiResp.Choices) > 0 {
		enhanced = apiResp.Choices[0].Message.Content
	}

	return &ImproveResult{
		Enhanced:     strings.TrimSpace(enhanced),
		TaskType:     string(opts.TaskType),
		Improvements: []string{"LLM-powered improvement via OpenAI API"},
	}, nil
}
