package observability

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/finops"
	"github.com/hairglasses-studio/ralphglasses/internal/tracing"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const llmTracerName = "ralphglasses/llm"

type LLMCallInfo struct {
	Operation    string
	Provider     string
	System       string
	Model        string
	BaseURL      string
	ResponseID   string
	MaxTokens    int
	InputTokens  int64
	OutputTokens int64
	CostUSD      float64
	Attributes   []attribute.KeyValue
}

func StartLLMCallSpan(ctx context.Context, info LLMCallInfo) (context.Context, trace.Span, time.Time) {
	tracer := otel.Tracer(llmTracerName)
	attrs := []attribute.KeyValue{
		attribute.String(tracing.AttrGenAISystem, strings.TrimSpace(info.System)),
		attribute.String(tracing.AttrGenAIProvider, strings.TrimSpace(info.Provider)),
		attribute.String(tracing.AttrGenAIModel, strings.TrimSpace(info.Model)),
	}
	if info.MaxTokens > 0 {
		attrs = append(attrs, attribute.Int(tracing.AttrGenAIMaxTokens, info.MaxTokens))
	}
	if repo := tracing.RepoFromContext(ctx); repo != "" {
		attrs = append(attrs, attribute.String(tracing.AttrGenAIRepoName, repo))
	}
	if tool := tracing.ToolNameFromContext(ctx); tool != "" {
		attrs = append(attrs, attribute.String("mcp.tool.name", tool))
	}
	if info.BaseURL != "" {
		attrs = append(attrs, attribute.String("gen_ai.request.base_url", info.BaseURL))
	}
	attrs = append(attrs, info.Attributes...)

	ctx, span := tracer.Start(ctx, "llm."+strings.TrimSpace(info.Operation),
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attrs...),
	)
	return ctx, span, time.Now()
}

func FinishLLMCallSpan(span trace.Span, started time.Time, info LLMCallInfo, err error) {
	if span == nil {
		return
	}

	span.SetAttributes(
		attribute.Int64(tracing.AttrGenAILatencyMs, time.Since(started).Milliseconds()),
	)

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
		span.End()
		return
	}

	attrs := []attribute.KeyValue{}
	if info.ResponseID != "" {
		attrs = append(attrs, attribute.String("gen_ai.response.id", info.ResponseID))
	}
	if info.InputTokens > 0 {
		attrs = append(attrs, attribute.Int64(tracing.AttrGenAIInputTokens, info.InputTokens))
	}
	if info.OutputTokens > 0 {
		attrs = append(attrs, attribute.Int64(tracing.AttrGenAIOutputTokens, info.OutputTokens))
	}
	totalTokens := info.InputTokens + info.OutputTokens
	if totalTokens > 0 {
		attrs = append(attrs, attribute.Int64(tracing.AttrGenAITotalTokens, totalTokens))
	}
	if info.CostUSD > 0 {
		attrs = append(attrs, attribute.Float64(tracing.AttrGenAICostUSD, info.CostUSD))
	}
	span.SetAttributes(attrs...)
	span.SetStatus(codes.Ok, "")
	span.End()
}

func EstimateLLMCostUSD(system, model string, inputTokens, outputTokens int64) float64 {
	if strings.EqualFold(strings.TrimSpace(system), "ollama") {
		return 0
	}
	return finops.ModelCost(model, inputTokens, outputTokens)
}

func ResolveGenAISystem(baseURL, fallback string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err == nil {
			host := strings.ToLower(parsed.Hostname())
			if host == "127.0.0.1" || host == "localhost" || host == "::1" {
				return "ollama"
			}
		}
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "unknown"
	}
	return fallback
}
