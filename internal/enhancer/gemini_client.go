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

// GeminiClient calls the Google AI Gemini API to improve prompts using a meta-prompt.
type GeminiClient struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client
	CacheName  string // optional: cached content name from CreateCachedContent
}

// NewGeminiClient creates a Gemini client from config. Returns nil if no API key is available.
func NewGeminiClient(cfg LLMConfig) *GeminiClient {
	apiKey := os.Getenv(cfg.APIKeyEnv)
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		return nil
	}

	model := cfg.Model
	if model == "" {
		model = "gemini-2.5-pro"
	}

	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com"
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &GeminiClient{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// Provider returns the provider name.
func (c *GeminiClient) Provider() ProviderName { return ProviderGemini }

// geminiRequest is the Gemini generateContent request body.
type geminiRequest struct {
	Contents          []geminiContent        `json:"contents"`
	SystemInstruction *geminiContent         `json:"systemInstruction,omitempty"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
	CachedContent     string                 `json:"cachedContent,omitempty"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens int             `json:"maxOutputTokens"`
	ThinkingConfig  *geminiThinking `json:"thinkingConfig,omitempty"`
}

// geminiThinking controls the thinking budget for Gemini models.
// ThinkingBudget: 0 = disabled (no thinking), -1 = dynamic (model decides), N>0 = token count.
type geminiThinking struct {
	ThinkingBudget int `json:"thinkingBudget"`
}

type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
	Error      *geminiError      `json:"error,omitempty"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

// Improve sends the prompt to Gemini with a meta-prompt and returns the improved version.
func (c *GeminiClient) Improve(ctx context.Context, prompt string, opts ImproveOptions) (*ImproveResult, error) {
	systemPrompt := MetaPromptFor(ProviderGemini, opts.ThinkingEnabled)

	userContent := prompt
	if opts.Feedback != "" {
		userContent += "\n\n[Additional guidance: " + opts.Feedback + "]"
	}

	// Set thinking budget based on opts.ThinkingEnabled:
	// enabled -> dynamic (-1, let model decide), disabled -> 0 (no thinking, saves tokens).
	var thinking *geminiThinking
	if opts.ThinkingEnabled {
		thinking = &geminiThinking{ThinkingBudget: -1}
	} else {
		thinking = &geminiThinking{ThinkingBudget: 0}
	}

	reqBody := geminiRequest{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: userContent}}},
		},
		GenerationConfig: geminiGenerationConfig{
			MaxOutputTokens: 4096,
			ThinkingConfig:  thinking,
		},
	}

	// If we have a cached content reference, use it instead of sending the system prompt inline.
	// The cached content already contains the system instruction.
	if c.CacheName != "" {
		reqBody.CachedContent = c.CacheName
	} else {
		reqBody.SystemInstruction = &geminiContent{
			Parts: []geminiPart{{Text: systemPrompt}},
		}
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.BaseURL, c.Model, c.APIKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

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

	var apiResp geminiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("api error: %s: %s", apiResp.Error.Status, apiResp.Error.Message)
	}

	// Extract text from first candidate's parts
	var enhanced strings.Builder
	if len(apiResp.Candidates) > 0 {
		for _, part := range apiResp.Candidates[0].Content.Parts {
			enhanced.WriteString(part.Text)
		}
	}

	return &ImproveResult{
		Enhanced:     strings.TrimSpace(enhanced.String()),
		TaskType:     string(opts.TaskType),
		Improvements: []string{"LLM-powered improvement via Gemini API"},
	}, nil
}

// geminiCachedContentRequest is the request body for creating cached content.
type geminiCachedContentRequest struct {
	Model    string          `json:"model"`
	Contents []geminiContent `json:"contents"`
	TTL      string          `json:"ttl"`
}

// geminiCachedContentResponse is the response from the cached content API.
type geminiCachedContentResponse struct {
	Name  string       `json:"name"`
	Error *geminiError `json:"error,omitempty"`
}

// CreateCachedContent creates a cached version of the system prompt via the Gemini
// context caching API. Returns the cache name (e.g. "cachedContents/abc123") that
// can be stored in c.CacheName for use in subsequent Improve calls.
// The cache has a default TTL of 1 hour. Callers should call this once and reuse
// the cache name across multiple requests to get the 90% read discount.
func (c *GeminiClient) CreateCachedContent(ctx context.Context, systemPrompt string) (string, error) {
	reqBody := geminiCachedContentRequest{
		Model: "models/" + c.Model,
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: systemPrompt}}},
		},
		TTL: "3600s",
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal cache request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/cachedContents?key=%s", c.BaseURL, c.APIKey)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("create cache request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("cache api call: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read cache response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("cache api error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var cacheResp geminiCachedContentResponse
	if err := json.Unmarshal(respBody, &cacheResp); err != nil {
		return "", fmt.Errorf("unmarshal cache response: %w", err)
	}

	if cacheResp.Error != nil {
		return "", fmt.Errorf("cache api error: %s: %s", cacheResp.Error.Status, cacheResp.Error.Message)
	}

	if cacheResp.Name == "" {
		return "", fmt.Errorf("cache api returned empty name")
	}

	return cacheResp.Name, nil
}
