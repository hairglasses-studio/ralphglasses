package mcpserver

import "github.com/mark3labs/mcp-go/mcp"

func (s *Server) buildPromptDJGroup() ToolGroup {
	return ToolGroup{
		Name:        "promptdj",
		Description: "Prompt DJ: quality-aware prompt routing to optimal providers",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_promptdj_route",
				mcp.WithDescription("Route a prompt to the best provider based on quality, task type, and domain. Does NOT launch a session."),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt text to route")),
				mcp.WithString("repo", mcp.Description("Repository name")),
				mcp.WithString("task_type", mcp.Description("Override: code, analysis, troubleshooting, creative, workflow, general")),
				mcp.WithNumber("score", mcp.Description("Pre-computed quality score 0-100")),
			), s.handlePromptDJRoute},
			{mcp.NewTool("ralphglasses_promptdj_dispatch",
				mcp.WithDescription("Route AND launch a session. Optionally enhances prompt if quality is low."),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt to route and dispatch")),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repository path")),
				mcp.WithString("task_type", mcp.Description("Override task type")),
				mcp.WithString("enhance", mcp.Description("Enhancement: none, local, auto (default: auto)")),
				mcp.WithNumber("budget_usd", mcp.Description("Max session budget USD")),
				mcp.WithBoolean("dry_run", mcp.Description("Preview routing without launching")),
			), s.handlePromptDJDispatch},
			{mcp.NewTool("ralphglasses_promptdj_feedback",
				mcp.WithDescription("Record outcome feedback to improve routing over time."),
				mcp.WithString("decision_id", mcp.Required(), mcp.Description("Decision ID from route/dispatch")),
				mcp.WithBoolean("success", mcp.Required(), mcp.Description("Whether session succeeded")),
				mcp.WithNumber("cost_usd", mcp.Description("Actual cost USD")),
				mcp.WithNumber("turns", mcp.Description("Actual turn count")),
				mcp.WithString("notes", mcp.Description("Outcome notes")),
				mcp.WithString("correct_provider", mcp.Description("Correct provider if DJ chose wrong")),
			), s.handlePromptDJFeedback},
			{mcp.NewTool("ralphglasses_promptdj_similar",
				mcp.WithDescription("Find similar high-quality prompts from the registry for few-shot context injection. Uses BM25-lite keyword similarity, Jaccard tag overlap, and MMR diversity re-ranking."),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt to find similar examples for")),
				mcp.WithString("repo", mcp.Description("Repository context for relevance boosting")),
			), s.handlePromptDJSimilar},
			{mcp.NewTool("ralphglasses_promptdj_suggest",
				mcp.WithDescription("Get routing-aware improvement suggestions for a prompt. Shows where it would route, quality score, and actionable suggestions by category (quality, structure, cost, provider_fit)."),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt to analyze")),
				mcp.WithString("repo", mcp.Description("Repository context")),
			), s.handlePromptDJSuggest},
		},
	}
}
