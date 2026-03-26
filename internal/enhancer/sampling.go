package enhancer

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// ProviderSampling identifies the MCP Sampling provider.
const ProviderSampling ProviderName = "sampling"

// ModeSampling is an enhancement mode that uses MCP Sampling.
const ModeSampling EnhanceMode = "sampling"

// SamplingClient abstracts the MCP Sampling CreateMessage call.
// In production this is backed by server.MCPServer.RequestSampling;
// in tests it can be a simple mock.
type SamplingClient interface {
	CreateMessage(ctx context.Context, request mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error)
}

// SamplingEngine implements PromptImprover using MCP Sampling.
// Instead of calling a provider API directly, it asks the host client
// (e.g., Claude Code) to perform the LLM completion via the MCP
// sampling/createMessage method.
type SamplingEngine struct {
	Sampler SamplingClient
}

// NewSamplingEngine creates a SamplingEngine from the given client.
// Returns nil if sampler is nil.
func NewSamplingEngine(sampler SamplingClient) *SamplingEngine {
	if sampler == nil {
		return nil
	}
	return &SamplingEngine{Sampler: sampler}
}

// Provider returns ProviderSampling.
func (e *SamplingEngine) Provider() ProviderName { return ProviderSampling }

// Improve sends the prompt to the host client via MCP Sampling for enhancement.
func (e *SamplingEngine) Improve(ctx context.Context, prompt string, opts ImproveOptions) (*ImproveResult, error) {
	systemPrompt := MetaPromptFor(ProviderClaude, opts.ThinkingEnabled)

	userContent := prompt
	if opts.Feedback != "" {
		userContent += "\n\n[Additional guidance: " + opts.Feedback + "]"
	}

	req := mcp.CreateMessageRequest{
		CreateMessageParams: mcp.CreateMessageParams{
			Messages: []mcp.SamplingMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.TextContent{Type: "text", Text: userContent},
				},
			},
			SystemPrompt: systemPrompt,
			MaxTokens:    4096,
		},
	}

	result, err := e.Sampler.CreateMessage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("sampling createMessage: %w", err)
	}

	text := extractSamplingText(result)
	if text == "" {
		return nil, fmt.Errorf("sampling returned empty response")
	}

	return &ImproveResult{
		Enhanced:     strings.TrimSpace(text),
		TaskType:     string(opts.TaskType),
		Improvements: []string{"LLM-powered improvement via MCP Sampling (client-side completion)"},
	}, nil
}

// extractSamplingText pulls the text string out of a CreateMessageResult.
// The Content field is typed as `any` and may be a TextContent struct,
// a map from JSON unmarshalling, or a string.
func extractSamplingText(result *mcp.CreateMessageResult) string {
	if result == nil {
		return ""
	}

	switch c := result.Content.(type) {
	case mcp.TextContent:
		return c.Text
	case map[string]interface{}:
		if t, ok := c["text"].(string); ok {
			return t
		}
	case string:
		return c
	}
	return ""
}

// SamplingScore produces a quality ScoreReport for a prompt using the local
// scoring pipeline. MCP Sampling does not provide a dedicated scoring endpoint,
// so we reuse the deterministic scorer. This keeps the SamplingEngine focused on
// the Improve path where Sampling adds real value.
func SamplingScore(text string, taskType TaskType, targetProvider ProviderName) *ScoreReport {
	ar := Analyze(text)
	lints := Lint(text)
	return Score(text, taskType, lints, &ar, targetProvider)
}
