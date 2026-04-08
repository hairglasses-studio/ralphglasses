package mcpserver

import (
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver/descriptions"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildDocsGroup() ToolGroup {
	return ToolGroup{
		Name:        "docs",
		Description: "Docs repo integration: search research, check existing, write findings, push changes, meta-roadmap coordination, cross-repo roadmap management",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_docs_search",
				mcp.WithDescription(descriptions.DescRalphglassesDocsSearch),
				mcp.WithString("query", mcp.Required(), mcp.Description("Search query (regex supported)")),
				mcp.WithString("domain", mcp.Description("Filter to research domain: mcp, agents, orchestration, cost-optimization, go-ecosystem, terminal, competitive")),
				mcp.WithNumber("limit", mcp.Description("Max results (default: 20)")),
			), s.handleDocsSearch},
			{mcp.NewTool("ralphglasses_docs_check_existing",
				mcp.WithDescription(descriptions.DescRalphglassesDocsCheckExisting),
				mcp.WithString("topic", mcp.Required(), mcp.Description("Topic to check (e.g., 'MCP tool design', 'cascade routing')")),
			), s.handleDocsCheckExisting},
			{mcp.NewTool("ralphglasses_docs_write_finding",
				mcp.WithDescription(descriptions.DescRalphglassesDocsWriteFinding),
				mcp.WithString("domain", mcp.Required(), mcp.Description("Research domain: mcp, agents, orchestration, cost-optimization, go-ecosystem, terminal, competitive")),
				mcp.WithString("filename", mcp.Required(), mcp.Description("Filename (kebab-case.md, e.g., 'fleet-metrics-scaling.md')")),
				mcp.WithString("content", mcp.Required(), mcp.Description("Markdown content to write")),
			), s.handleDocsWriteFinding},
			{mcp.NewTool("ralphglasses_docs_push",
				mcp.WithDescription(descriptions.DescRalphglassesDocsPush),
			), s.handleDocsPush},
			{mcp.NewTool("ralphglasses_meta_roadmap_status",
				mcp.WithDescription(descriptions.DescRalphglassesMetaRoadmapStatus),
			), s.handleMetaRoadmapStatus},
			{mcp.NewTool("ralphglasses_meta_roadmap_next_task",
				mcp.WithDescription(descriptions.DescRalphglassesMetaRoadmapNextTask),
				mcp.WithString("phase", mcp.Description("Filter to phase containing this string (e.g., 'Wave 1', 'Security')")),
			), s.handleMetaRoadmapNextTask},
			{mcp.NewTool("ralphglasses_roadmap_cross_repo",
				mcp.WithDescription(descriptions.DescRalphglassesRoadmapCrossRepo),
				mcp.WithNumber("limit", mcp.Description("Max repos to return (default: 10)")),
			), s.handleRoadmapCrossRepo},
			{mcp.NewTool("ralphglasses_roadmap_assign_loop",
				mcp.WithDescription(descriptions.DescRalphglassesRoadmapAssignLoop),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repository name")),
				mcp.WithString("task", mcp.Required(), mcp.Description("Task description from roadmap")),
				mcp.WithString("provider", mcp.Description("Provider: claude (default), gemini, codex")),
				mcp.WithNumber("budget_usd", mcp.Description("Budget in USD (default: 3.0)")),
			), s.handleRoadmapAssignLoop},
		},
	}
}
