package observability

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/hairglasses-studio/mcpkit/finops"
	"github.com/hairglasses-studio/ralphglasses/internal/tracing"
	"github.com/henomis/langfuse-go/model"
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
	endTime := time.Now()

	span.SetAttributes(
		attribute.Int64(tracing.AttrGenAILatencyMs, endTime.Sub(started).Milliseconds()),
	)

	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
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
	if err == nil {
		span.SetStatus(codes.Ok, "")
	}
	span.End()

	// Log to Langfuse if enabled
	if langfuseClient != nil {
		traceID := span.SpanContext().TraceID().String()
		spanID := span.SpanContext().SpanID().String()
		
		lfTrace := &model.Trace{
			ID:        traceID,
			Name:      info.Operation,
			Timestamp: &started,
		}
		langfuseClient.Trace(lfTrace)

		lfGen := &model.Generation{
			ID:        spanID,
			TraceID:   traceID,
			Name:      info.Operation,
			StartTime: &started,
			EndTime:   &endTime,
			Model:     info.Model,
		}
		
		metadata := map[string]interface{}{
			"system":   info.System,
			"provider": info.Provider,
			"base_url": info.BaseURL,
		}
		if info.ResponseID != "" {
			metadata["response_id"] = info.ResponseID
		}
		if info.CostUSD > 0 {
			metadata["cost_usd"] = info.CostUSD
		}
		lfGen.Metadata = metadata

		if info.InputTokens > 0 || info.OutputTokens > 0 {
			lfGen.Usage = model.Usage{
				Input:  int(info.InputTokens),
				Output: int(info.OutputTokens),
				Total:  int(info.InputTokens + info.OutputTokens),
			}
		}

		if err != nil {
			lfGen.Level = model.ObservationLevelError
			lfGen.StatusMessage = err.Error()
		} else {
			lfGen.Level = model.ObservationLevelDefault
		}

		langfuseClient.Generation(lfGen, nil)
	}
}

func EstimateLLMCostUSD(system, model string, inputTokens, outputTokens int64) float64 {
	return finops.ModelCost(model, inputTokens, outputTokens)
}

func ResolveGenAISystem(baseURL, fallback string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL != "" {
		parsed, err := url.Parse(baseURL)
		if err == nil {
			_ = strings.ToLower(parsed.Hostname())
		}
	}
	fallback = strings.TrimSpace(fallback)
	if fallback == "" {
		return "unknown"
	}
	return fallback
}
