package mcpserver

import (
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver/descriptions"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildPromptDJGroup() ToolGroup {
	return ToolGroup{
		Name:        "promptdj",
		Description: "Prompt DJ: quality-aware prompt routing to optimal providers",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_promptdj_route",
				mcp.WithDescription(descriptions.DescRalphglassesPromptdjRoute),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt text to route")),
				mcp.WithString("repo", mcp.Description("Repository name")),
				mcp.WithString("task_type", mcp.Description("Override: code, analysis, troubleshooting, creative, workflow, general")),
				mcp.WithNumber("score", mcp.Description("Pre-computed quality score 0-100")),
			), s.handlePromptDJRoute},
			{mcp.NewTool("ralphglasses_promptdj_dispatch",
				mcp.WithDescription(descriptions.DescRalphglassesPromptdjDispatch),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt to route and dispatch")),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repository path")),
				mcp.WithString("task_type", mcp.Description("Override task type")),
				mcp.WithString("enhance", mcp.Description("Enhancement: none, local, auto (default: auto)")),
				mcp.WithNumber("budget_usd", mcp.Description("Max session budget USD")),
				mcp.WithBoolean("dry_run", mcp.Description("Preview routing without launching")),
			), s.handlePromptDJDispatch},
			{mcp.NewTool("ralphglasses_promptdj_feedback",
				mcp.WithDescription(descriptions.DescRalphglassesPromptdjFeedback),
				mcp.WithString("decision_id", mcp.Required(), mcp.Description("Decision ID from route/dispatch")),
				mcp.WithBoolean("success", mcp.Required(), mcp.Description("Whether session succeeded")),
				mcp.WithNumber("cost_usd", mcp.Description("Actual cost USD")),
				mcp.WithNumber("turns", mcp.Description("Actual turn count")),
				mcp.WithString("notes", mcp.Description("Outcome notes")),
				mcp.WithString("correct_provider", mcp.Description("Correct provider if DJ chose wrong")),
			), s.handlePromptDJFeedback},
			{mcp.NewTool("ralphglasses_promptdj_similar",
				mcp.WithDescription(descriptions.DescRalphglassesPromptdjSimilar),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt to find similar examples for")),
				mcp.WithString("repo", mcp.Description("Repository context for relevance boosting")),
			), s.handlePromptDJSimilar},
			{mcp.NewTool("ralphglasses_promptdj_suggest",
				mcp.WithDescription(descriptions.DescRalphglassesPromptdjSuggest),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt to analyze")),
				mcp.WithString("repo", mcp.Description("Repository context")),
			), s.handlePromptDJSuggest},
			{mcp.NewTool("ralphglasses_promptdj_history",
				mcp.WithDescription(descriptions.DescRalphglassesPromptdjHistory),
				mcp.WithString("repo", mcp.Description("Filter by repo")),
				mcp.WithString("provider", mcp.Description("Filter by provider")),
				mcp.WithString("task_type", mcp.Description("Filter by task type")),
				mcp.WithString("status", mcp.Description("Filter: routed, dispatched, succeeded, failed, all")),
				mcp.WithString("since", mcp.Description("Time window: RFC3339 or duration ('24h', '7d')")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 50)")),
				mcp.WithBoolean("summary", mcp.Description("If true, return aggregate stats instead of individual decisions")),
			), s.handlePromptDJHistory},
		},
	}
}
