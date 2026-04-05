package mcpserver

import (
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildDocsGroup() ToolGroup {
	return ToolGroup{
		Name:        "docs",
		Description: "Docs repo integration: search research, check existing, write findings, push changes, meta-roadmap coordination, cross-repo roadmap management",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_docs_search",
				mcp.WithDescription("Full-text search across docs/research/ files using ripgrep. Returns matching file paths, line numbers, and previews."),
				mcp.WithString("query", mcp.Required(), mcp.Description("Search query (regex supported)")),
				mcp.WithString("domain", mcp.Description("Filter to research domain: mcp, agents, orchestration, cost-optimization, go-ecosystem, terminal, competitive")),
				mcp.WithNumber("limit", mcp.Description("Max results (default: 20)")),
			), s.handleDocsSearch},
			{mcp.NewTool("ralphglasses_docs_check_existing",
				mcp.WithDescription("Check if research exists on a topic before starting new work. Searches SEARCH-GUIDE.md and all research files. Returns recommendation: read existing or proceed with new research."),
				mcp.WithString("topic", mcp.Required(), mcp.Description("Topic to check (e.g., 'MCP tool design', 'cascade routing')")),
			), s.handleDocsCheckExisting},
			{mcp.NewTool("ralphglasses_docs_write_finding",
				mcp.WithDescription("Write a research finding to docs/research/<domain>/<filename>. Validates domain and creates directory if needed."),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Research domain: mcp, agents, orchestration, cost-optimization, go-ecosystem, terminal, competitive")),
				mcp.WithString("filename", mcp.Required(), mcp.Description("Filename (kebab-case.md, e.g., 'fleet-metrics-scaling.md')")),
				mcp.WithString("content", mcp.Required(), mcp.Description("Markdown content to write")),
			), s.handleDocsWriteFinding},
			{mcp.NewTool("ralphglasses_docs_push",
				mcp.WithDescription("Commit and push all pending changes in the docs repo via push-docs.sh"),
			), s.handleDocsPush},
			{mcp.NewTool("ralphglasses_meta_roadmap_status",
				mcp.WithDescription("Parse docs/strategy/META-ROADMAP.md and return phase count, task totals, completion percentage, and summary"),
			), s.handleMetaRoadmapStatus},
			{mcp.NewTool("ralphglasses_meta_roadmap_next_task",
				mcp.WithDescription("Get the next incomplete task from META-ROADMAP.md, optionally filtered by phase name"),
				mcp.WithString("phase", mcp.Description("Filter to phase containing this string (e.g., 'Wave 1', 'Security')")),
			), s.handleMetaRoadmapNextTask},
			{mcp.NewTool("ralphglasses_roadmap_cross_repo",
				mcp.WithDescription("Compare roadmap progress across all repos using docs/snapshots/roadmaps/. Returns repos sorted by completion (least complete first)."),
				mcp.WithNumber("limit", mcp.Description("Max repos to return (default: 10)")),
			), s.handleRoadmapCrossRepo},
			{mcp.NewTool("ralphglasses_roadmap_assign_loop",
				mcp.WithDescription("Create an R&D loop targeting a specific roadmap task. Returns loop_start parameters for the task."),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
				mcp.WithString("task", mcp.Required(), mcp.Description("Task description from roadmap")),
				mcp.WithString("provider", mcp.Description("Provider: claude (default), gemini, codex")),
				mcp.WithNumber("budget_usd", mcp.Description("Budget in USD (default: 3.0)")),
			), s.handleRoadmapAssignLoop},
		},
	}
}
