package enhancer

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/hairglasses-studio/ralphglasses/internal/observability"
)

// LLMClient calls the Claude Messages API to improve prompts using the official Anthropic Go SDK.
type LLMClient struct {
	APIKey     string
	Model      string
	BaseURL    string
	HTTPClient *http.Client

	// sdk is the underlying Anthropic SDK client.
	sdk *anthropic.Client

	// effortLevel controls the output effort parameter ("low", "medium", "high", "max").
	effortLevel string

	// cacheControl enables prompt caching on the system message.
	cacheControl bool

	// displayThinking controls whether thinking tokens are shown (default: omitted for fleet).
	displayThinking bool
}

// NewLLMClient creates a client from config. Returns nil if no API key is available.
func NewLLMClient(cfg LLMConfig) *LLMClient {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}

	apiKey := os.Getenv(cfg.APIKeyEnv)
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return nil
	}

	model := cfg.Model
	if model == "" {
		model = "claude-sonnet-4-6"
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	effortLevel := cfg.EffortLevel
	if effortLevel == "" {
		effortLevel = "medium"
	}

	cacheControl := cfg.CacheControl
	// Default to true if not explicitly set (zero value is false, so we check the config)
	if !cfg.cacheControlSet {
		cacheControl = true
	}

	httpClient := &http.Client{Timeout: timeout}

	// Build SDK client options
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL),
		option.WithHTTPClient(httpClient),
	}

	sdkClient := anthropic.NewClient(opts...)

	return &LLMClient{
		APIKey:          apiKey,
		Model:           model,
		BaseURL:         baseURL,
		HTTPClient:      httpClient,
		sdk:             &sdkClient,
		effortLevel:     effortLevel,
		cacheControl:    cacheControl,
		displayThinking: cfg.DisplayThinking,
	}
}

// Provider returns the provider name.
func (c *LLMClient) Provider() ProviderName { return ProviderClaude }

// ImproveOptions configures the LLM improvement request.
type ImproveOptions struct {
	ThinkingEnabled bool
	TaskType        TaskType
	Feedback        string       // optional targeted hints
	Provider        ProviderName // for cache key disambiguation
}

// ImproveResult holds the LLM-improved prompt and metadata.
type ImproveResult struct {
	Enhanced     string   `json:"enhanced"`
	TaskType     string   `json:"task_type"`
	Improvements []string `json:"improvements"`
}

// Improve sends the prompt to Claude with the meta-prompt and returns the improved version.
func (c *LLMClient) Improve(ctx context.Context, prompt string, opts ImproveOptions) (*ImproveResult, error) {
	systemPrompt := MetaPromptFor(ProviderClaude, opts.ThinkingEnabled)

	userContent := prompt
	if opts.Feedback != "" {
		userContent += "\n\n[Additional guidance: " + opts.Feedback + "]"
	}

	// Build system message
	sysBlock := anthropic.TextBlockParam{
		Text: systemPrompt,
	}

	// Build request params
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(c.Model),
		MaxTokens: 4096,
		System:    []anthropic.TextBlockParam{sysBlock},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userContent)),
		},
	}

	// Enable prompt caching via top-level cache_control (auto-applies to last cacheable block)
	if c.cacheControl {
		params.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}

	// Set effort level via output config
	if c.effortLevel != "" {
		params.OutputConfig = anthropic.OutputConfigParam{
			Effort: anthropic.OutputConfigEffort(c.effortLevel),
		}
	}

	// Configure adaptive thinking when enabled
	if opts.ThinkingEnabled {
		display := anthropic.ThinkingConfigAdaptiveDisplayOmitted
		if c.displayThinking {
			display = anthropic.ThinkingConfigAdaptiveDisplaySummarized
		}
		params.Thinking = anthropic.ThinkingConfigParamUnion{
			OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{
				Display: display,
			},
		}
	}

	call := observability.LLMCallInfo{
		Operation: "prompt_improver.improve",
		Provider:  string(c.Provider()),
		System:    observability.ResolveGenAISystem(c.BaseURL, "anthropic"),
		Model:     c.Model,
		BaseURL:   c.BaseURL,
		MaxTokens: 4096,
	}
	ctx, span, started := observability.StartLLMCallSpan(ctx, call)

	resp, err := c.sdk.Messages.New(ctx, params)
	defer func() {
		observability.FinishLLMCallSpan(span, started, call, err)
	}()
	if err != nil {
		return nil, fmt.Errorf("api call: %w", err)
	}
	call.ResponseID = resp.ID
	call.InputTokens = int64(resp.Usage.InputTokens)
	call.OutputTokens = int64(resp.Usage.OutputTokens)
	call.CostUSD = observability.EstimateLLMCostUSD(call.System, c.Model, call.InputTokens, call.OutputTokens)

	// Extract text from content blocks
	var enhanced strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			enhanced.WriteString(block.Text)
		}
	}

	result := &ImproveResult{
		Enhanced:     strings.TrimSpace(enhanced.String()),
		TaskType:     string(opts.TaskType),
		Improvements: []string{"LLM-powered improvement via Claude Messages API (SDK)"},
	}

	return result, nil
}
