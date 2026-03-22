package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

func makeMCPReq(args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Arguments: args,
		},
	}
}

func getResultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content is not TextContent: %T", result.Content[0])
	}
	return tc.Text
}

func TestMCP_HandleAnalyze(t *testing.T) {
	t.Run("valid prompt", func(t *testing.T) {
		req := makeMCPReq(map[string]any{"prompt": "fix this"})
		result, err := mcpHandleAnalyze(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Error("expected success, got error")
		}
		text := getResultText(t, result)
		if !strings.Contains(text, "score") {
			t.Error("result should contain score")
		}
		if !strings.Contains(text, "suggestions") {
			t.Error("result should contain suggestions")
		}
		if !strings.Contains(text, "score_report") {
			t.Error("result should contain score_report")
		}

		var parsed enhancer.AnalyzeResult
		if err := json.Unmarshal([]byte(text), &parsed); err != nil {
			t.Errorf("result is not valid JSON: %v", err)
		}
	})

	t.Run("empty prompt", func(t *testing.T) {
		req := makeMCPReq(map[string]any{"prompt": ""})
		result, err := mcpHandleAnalyze(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected error for empty prompt")
		}
	})

	t.Run("with task type", func(t *testing.T) {
		req := makeMCPReq(map[string]any{"prompt": "review this code for bugs and security issues", "task_type": "code"})
		result, err := mcpHandleAnalyze(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := getResultText(t, result)
		var parsed enhancer.AnalyzeResult
		if err := json.Unmarshal([]byte(text), &parsed); err != nil {
			t.Errorf("result is not valid JSON: %v", err)
		}
	})
}

func TestMCP_HandleEnhance(t *testing.T) {
	t.Run("valid prompt", func(t *testing.T) {
		req := makeMCPReq(map[string]any{"prompt": "fix this bug in the sorting function"})
		result, err := mcpHandleEnhance(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Error("expected success, got error")
		}
		text := getResultText(t, result)
		if !strings.Contains(text, "enhanced") {
			t.Error("result should contain enhanced field")
		}
		if !strings.Contains(text, "stages_run") {
			t.Error("result should contain stages_run field")
		}

		var parsed enhancer.EnhanceResult
		if err := json.Unmarshal([]byte(text), &parsed); err != nil {
			t.Errorf("result is not valid JSON: %v", err)
		}
		if parsed.Enhanced == "" {
			t.Error("enhanced prompt should not be empty")
		}
	})

	t.Run("empty prompt", func(t *testing.T) {
		req := makeMCPReq(map[string]any{"prompt": ""})
		result, err := mcpHandleEnhance(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected error for empty prompt")
		}
	})

	t.Run("with task type override", func(t *testing.T) {
		req := makeMCPReq(map[string]any{"prompt": "write a haiku about testing", "task_type": "creative"})
		result, err := mcpHandleEnhance(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := getResultText(t, result)
		var parsed enhancer.EnhanceResult
		if err := json.Unmarshal([]byte(text), &parsed); err != nil {
			t.Errorf("result is not valid JSON: %v", err)
		}
		if parsed.TaskType != "creative" {
			t.Errorf("expected task type creative, got %s", parsed.TaskType)
		}
	})
}

func TestMCP_HandleLint(t *testing.T) {
	t.Run("clean prompt", func(t *testing.T) {
		req := makeMCPReq(map[string]any{"prompt": "Return exactly 5 user records as JSON, sorted by creation date."})
		result, err := mcpHandleLint(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Error("expected success, got error")
		}
	})

	t.Run("dirty prompt", func(t *testing.T) {
		req := makeMCPReq(map[string]any{"prompt": "CRITICAL: You MUST follow this rule. NEVER ignore it."})
		result, err := mcpHandleLint(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := getResultText(t, result)
		if !strings.Contains(text, "overtrigger-phrase") {
			t.Error("should detect overtrigger phrase")
		}
	})

	t.Run("empty prompt", func(t *testing.T) {
		req := makeMCPReq(map[string]any{"prompt": ""})
		result, err := mcpHandleLint(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Error("expected error for empty prompt")
		}
	})

	t.Run("cache check included", func(t *testing.T) {
		req := makeMCPReq(map[string]any{"prompt": "<constraints>Be thorough.</constraints>\n<role>You are an expert.</role>\n<instructions>Do the thing.</instructions>"})
		result, err := mcpHandleLint(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := getResultText(t, result)
		if !strings.Contains(text, "cache") && !strings.Contains(text, "No issues") {
			t.Error("should include cache check results or report no issues")
		}
	})
}
