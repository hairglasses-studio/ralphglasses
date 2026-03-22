package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

// MCP helpers — match ralphglasses internal/mcpserver patterns.

func mcpArgsMap(req mcp.CallToolRequest) map[string]any {
	if m, ok := req.Params.Arguments.(map[string]any); ok {
		return m
	}
	return nil
}

func mcpGetString(req mcp.CallToolRequest, key string) string {
	m := mcpArgsMap(req)
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func mcpGetBool(req mcp.CallToolRequest, key string) bool {
	m := mcpArgsMap(req)
	if m == nil {
		return false
	}
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func mcpErrResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: msg,
		}},
	}
}

func mcpTextResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: text,
		}},
	}
}

func mcpJSONResult(v any) *mcp.CallToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return mcpErrResult(fmt.Sprintf("json marshal: %v", err))
	}
	return mcpTextResult(string(data))
}

// MCP tool handlers

func mcpHandleAnalyze(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := mcpGetString(req, "prompt")
	if prompt == "" {
		return mcpErrResult("prompt required"), nil
	}
	result := enhancer.Analyze(prompt)
	if tt := enhancer.ValidTaskType(mcpGetString(req, "task_type")); tt != "" {
		result.TaskType = tt
	}
	return mcpJSONResult(result), nil
}

func mcpHandleEnhance(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := mcpGetString(req, "prompt")
	if prompt == "" {
		return mcpErrResult("prompt required"), nil
	}
	tt := enhancer.ValidTaskType(mcpGetString(req, "task_type"))
	result := enhancer.Enhance(prompt, tt)
	return mcpJSONResult(result), nil
}

func mcpHandleLint(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := mcpGetString(req, "prompt")
	if prompt == "" {
		return mcpErrResult("prompt required"), nil
	}
	results := enhancer.Lint(prompt)
	cacheResults := enhancer.VerifyCacheFriendlyOrder(prompt)
	results = append(results, cacheResults...)

	if len(results) == 0 {
		return mcpTextResult("No issues found."), nil
	}
	return mcpJSONResult(results), nil
}

func mcpHandleDiff(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := mcpGetString(req, "prompt")
	if prompt == "" {
		return mcpErrResult("prompt required"), nil
	}
	tt := enhancer.ValidTaskType(mcpGetString(req, "task_type"))
	result := enhancer.Enhance(prompt, tt)

	var sb strings.Builder
	sb.WriteString("--- original\n+++ enhanced\n\n")
	for _, line := range strings.Split(prompt, "\n") {
		fmt.Fprintf(&sb, "- %s\n", line)
	}
	sb.WriteString("\n")
	for _, line := range strings.Split(result.Enhanced, "\n") {
		fmt.Fprintf(&sb, "+ %s\n", line)
	}
	if len(result.Improvements) > 0 {
		fmt.Fprintf(&sb, "\n%d improvements:\n", len(result.Improvements))
		for _, imp := range result.Improvements {
			fmt.Fprintf(&sb, "  • %s\n", imp)
		}
	}
	return mcpTextResult(sb.String()), nil
}

func mcpHandleImprove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := mcpGetString(req, "prompt")
	if prompt == "" {
		return mcpErrResult("prompt required"), nil
	}

	tt := enhancer.ValidTaskType(mcpGetString(req, "task_type"))
	mode := enhancer.ValidMode(mcpGetString(req, "mode"))
	if mode == "" {
		mode = enhancer.ModeAuto
	}

	cfg := enhancer.ResolveConfig(".")
	cfg.LLM.Enabled = true
	if mcpGetBool(req, "thinking_enabled") {
		cfg.LLM.ThinkingEnabled = true
	}

	engine := getOrCreateEngine(cfg.LLM)
	if engine == nil && mode != enhancer.ModeLocal {
		return mcpErrResult("ANTHROPIC_API_KEY not set — cannot use LLM improvement. Use mode=local for deterministic enhancement."), nil
	}

	result := enhancer.EnhanceHybrid(ctx, prompt, tt, cfg, engine, mode)
	return mcpJSONResult(result), nil
}

func mcpHandleCheckClaudeMD(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := mcpGetString(req, "path")
	if path == "" {
		path = "./CLAUDE.md"
	}
	results, err := enhancer.CheckClaudeMD(path)
	if err != nil {
		return mcpErrResult("check failed: " + err.Error()), nil
	}
	if len(results) == 0 {
		return mcpTextResult("CLAUDE.md looks healthy — no issues found."), nil
	}
	return mcpJSONResult(results), nil
}

func mcpHandleListTemplates(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	templates := enhancer.ListTemplates()
	return mcpJSONResult(templates), nil
}

// runMCP starts the standalone MCP stdio server.
func runMCP() {
	cfg := enhancer.ResolveConfig(".")
	if cfg.LLM.Enabled {
		getOrCreateEngine(cfg.LLM)
	}

	srv := server.NewMCPServer(
		"prompt-improver",
		version,
		server.WithToolCapabilities(true),
	)

	srv.AddTool(mcp.NewTool("analyze_prompt",
		mcp.WithDescription("Score a prompt across 10 quality dimensions (0-100) with letter grades and actionable suggestions. Returns specificity, structure, examples, framing, emphasis, format, context placement, injection safety, task-fit, and conciseness scores."),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to analyze")),
		mcp.WithString("task_type", mcp.Description("Task type override: code, creative, analysis, troubleshooting, workflow, or general")),
	), mcpHandleAnalyze)

	srv.AddTool(mcp.NewTool("enhance_prompt",
		mcp.WithDescription("Apply a 13-stage enhancement pipeline to a prompt: specificity, positive reframing, tone normalization, overtrigger rewrite, example wrapping, XML structure, context reordering, format enforcement, quote grounding, self-check, overengineering guard, and preamble suppression."),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to enhance")),
		mcp.WithString("task_type", mcp.Description("Task type override: code, creative, analysis, troubleshooting, workflow, or general")),
	), mcpHandleEnhance)

	srv.AddTool(mcp.NewTool("lint_prompt",
		mcp.WithDescription("Deep lint a prompt for 11 anti-patterns: overtrigger phrases, negative framing, aggressive emphasis, vague quantifiers, unmotivated rules, over-specification, injection risk, thinking-mode redundancy, example quality, compaction readiness, and cache-friendly ordering."),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to lint")),
	), mcpHandleLint)

	srv.AddTool(mcp.NewTool("diff_prompt",
		mcp.WithDescription("Show a unified diff of original vs enhanced prompt. Displays added/removed lines and lists improvements applied by the 13-stage pipeline."),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to diff (original vs enhanced)")),
		mcp.WithString("task_type", mcp.Description("Task type override: code, creative, analysis, troubleshooting, workflow, or general")),
	), mcpHandleDiff)

	srv.AddTool(mcp.NewTool("improve_prompt",
		mcp.WithDescription("LLM-powered prompt improvement using Claude. Analyzes task type, adds domain-specific role, structured output sections, scratchpad, and template variables. Falls back to local 13-stage pipeline if LLM is unavailable. Set mode=local for deterministic-only enhancement."),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to improve")),
		mcp.WithString("task_type", mcp.Description("Task type override: code, creative, analysis, troubleshooting, workflow, or general")),
		mcp.WithBoolean("thinking_enabled", mcp.Description("Add thinking scaffolding to the improved prompt")),
		mcp.WithString("feedback", mcp.Description("Optional targeted improvement hints")),
		mcp.WithString("mode", mcp.Description("Enhancement mode: local, llm, or auto (default: auto)")),
	), mcpHandleImprove)

	srv.AddTool(mcp.NewTool("check_claudemd",
		mcp.WithDescription("Health-check a CLAUDE.md file for common issues: excessive length, inline code blocks, style guide content that belongs in linter config, overtrigger language, aggressive ALL-CAPS, and missing section headers."),
		mcp.WithString("path", mcp.Description("Path to the CLAUDE.md file to check (default: ./CLAUDE.md)")),
	), mcpHandleCheckClaudeMD)

	srv.AddTool(mcp.NewTool("list_templates",
		mcp.WithDescription("List all available prompt templates with their names, descriptions, task types, variables, and usage examples."),
	), mcpHandleListTemplates)

	if err := server.ServeStdio(srv); err != nil {
		if strings.Contains(err.Error(), "EOF") {
			return
		}
		fmt.Fprintf(os.Stderr, "mcp server error: %v\n", err)
		os.Exit(1)
	}
}
