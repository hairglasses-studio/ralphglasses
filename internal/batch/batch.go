package batch

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Provider identifies the batch API provider.
type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderGemini Provider = "gemini"
	ProviderOpenAI Provider = "openai"
)

// Request represents a single item in a batch.
type Request struct {
	ID           string            `json:"id"`
	Model        string            `json:"model"`
	SystemPrompt string            `json:"system_prompt,omitempty"`
	UserPrompt   string            `json:"user_prompt"`
	MaxTokens    int               `json:"max_tokens,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Result represents the output of a single batch item.
type Result struct {
	ID           string `json:"id"`
	RequestID    string `json:"request_id"`
	Content      string `json:"content"`
	Error        string `json:"error,omitempty"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
}

// BatchStatus represents the current state of a batch job.
type BatchStatus struct {
	ID          string     `json:"id"`
	Provider    Provider   `json:"provider"`
	Status      string     `json:"status"` // pending, processing, completed, failed, expired
	Total       int        `json:"total"`
	Completed   int        `json:"completed"`
	Failed      int        `json:"failed"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// Client is the interface for batch processing across providers.
type Client interface {
	// Submit creates a new batch job with the given requests.
	Submit(ctx context.Context, requests []Request) (*BatchStatus, error)

	// Poll checks the current status of a batch job.
	Poll(ctx context.Context, batchID string) (*BatchStatus, error)

	// Results retrieves completed results for a batch job.
	// Returns error if batch is not yet complete.
	Results(ctx context.Context, batchID string) ([]Result, error)

	// Cancel attempts to cancel a pending/processing batch.
	Cancel(ctx context.Context, batchID string) error

	// Provider returns which provider this client targets.
	Provider() Provider
}

// NewClient creates a batch client for the given provider.
func NewClient(provider Provider, apiKey string, opts ...Option) (Client, error) {
	switch provider {
	case ProviderClaude:
		return newClaudeClient(apiKey, opts...), nil
	case ProviderGemini:
		return newGeminiClient(apiKey, opts...), nil
	case ProviderOpenAI, "codex":
		return newOpenAIClient(apiKey, opts...), nil
	default:
		return nil, fmt.Errorf("unsupported batch provider: %s", provider)
	}
}

// Option configures a batch client.
type Option func(*clientConfig)

type clientConfig struct {
	BaseURL    string
	HTTPClient *http.Client
	Model      string
}

func applyOpts(opts []Option) *clientConfig {
	cfg := &clientConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

func (c *clientConfig) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

// WithBaseURL sets a custom base URL for the API.
func WithBaseURL(url string) Option {
	return func(c *clientConfig) { c.BaseURL = url }
}

// WithModel sets the default model for batch requests.
func WithModel(model string) Option {
	return func(c *clientConfig) { c.Model = model }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *clientConfig) { c.HTTPClient = client }
}
