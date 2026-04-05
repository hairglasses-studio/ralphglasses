package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/clients"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// Module implements the ToolModule interface for memory tools
type Module struct{}

// Name returns the module name
func (m *Module) Name() string {
	return "memory"
}

// Description returns a brief description of the module
func (m *Module) Description() string {
	return "Session memory and learning tools (Azure SRE Agent pattern)"
}

// Tools returns all tool definitions in this module
func (m *Module) Tools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		GatewayToolDefinition(),
		// Remember - Save team knowledge
		{
			Tool: mcp.NewTool("webb_memory_remember",
				mcp.WithDescription("Save team knowledge to memory. Use #remember pattern: '#remember Team owns headspace-v2 in prod'. Creates searchable memories for future investigations."),
				mcp.WithString("content",
					mcp.Required(),
					mcp.Description("The knowledge to remember (e.g., 'Team owns headspace-v2 in prod')"),
				),
				mcp.WithString("category",
					mcp.Description("Memory category: team, service, workflow, standard, general (default: general)"),
				),
				mcp.WithString("added_by",
					mcp.Description("Who added this memory (default: current user)"),
				),
				mcp.WithString("tags",
					mcp.Description("Comma-separated tags for this memory"),
				),
			),
			Handler:  handleRemember,
			Category: "memory",
			Tags:     []string{"memory", "learning", "team-knowledge"},
			UseCases: []string{"Save team knowledge for future reference"},
			IsWrite:  true,
		},
		// Forget - Remove memory
		{
			Tool: mcp.NewTool("webb_memory_forget",
				mcp.WithDescription("Remove a memory by ID or semantic match. Use when knowledge becomes outdated."),
				mcp.WithString("id_or_description",
					mcp.Required(),
					mcp.Description("Memory ID or description to match semantically"),
				),
			),
			Handler:  handleForget,
			Category: "memory",
			Tags:     []string{"memory", "learning", "team-knowledge"},
			UseCases: []string{"Remove outdated team knowledge"},
			IsWrite:  true,
		},
		// Search - Query all memory components
		{
			Tool: mcp.NewTool("webb_memory_search",
				mcp.WithDescription("Search across all memory components (team memories + session insights). Uses semantic and keyword matching."),
				mcp.WithString("query",
					mcp.Required(),
					mcp.Description("Search query (natural language or keywords)"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum results per type (default: 10)"),
				),
			),
			Handler:  handleSearch,
			Category: "memory",
			Tags:     []string{"memory", "search", "semantic"},
			UseCases: []string{"Find relevant team knowledge and past insights"},
			IsWrite:  false,
		},
		// List Memories
		{
			Tool: mcp.NewTool("webb_memory_list",
				mcp.WithDescription("List user memories by category. Shows team knowledge stored via #remember."),
				mcp.WithString("category",
					mcp.Description("Filter by category: team, service, workflow, standard, general (empty for all)"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum memories to return (default: 50)"),
				),
			),
			Handler:  handleListMemories,
			Category: "memory",
			Tags:     []string{"memory", "list", "team-knowledge"},
			UseCases: []string{"Browse team knowledge"},
			IsWrite:  false,
		},
		// Generate Insight
		{
			Tool: mcp.NewTool("webb_session_insight_generate",
				mcp.WithDescription("Generate an insight from a completed investigation session. Extracts symptoms, resolution steps, root cause, and pitfalls automatically."),
				mcp.WithString("session_id",
					mcp.Required(),
					mcp.Description("Session ID to generate insight from"),
				),
				mcp.WithBoolean("save",
					mcp.Description("Save the insight to memory (default: true)"),
				),
			),
			Handler:  handleGenerateInsight,
			Category: "memory",
			Tags:     []string{"memory", "insights", "learning"},
			UseCases: []string{"Extract learnings from completed investigations"},
			IsWrite:  true,
		},
		// List Insights
		{
			Tool: mcp.NewTool("webb_session_insight_list",
				mcp.WithDescription("List past session insights filtered by service, customer, or cluster. Use to find learnings from similar past incidents."),
				mcp.WithString("service",
					mcp.Description("Filter by service name"),
				),
				mcp.WithString("customer",
					mcp.Description("Filter by customer"),
				),
				mcp.WithString("cluster",
					mcp.Description("Filter by cluster"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum insights to return (default: 20)"),
				),
			),
			Handler:  handleListInsights,
			Category: "memory",
			Tags:     []string{"memory", "insights", "history"},
			UseCases: []string{"Browse past investigation insights"},
			IsWrite:  false,
		},
		// Load Context
		{
			Tool: mcp.NewTool("webb_session_context_load",
				mcp.WithDescription("Load relevant memories and insights for a new investigation. Provides suggested steps and pitfalls to avoid based on past incidents."),
				mcp.WithString("symptoms",
					mcp.Description("Comma-separated symptoms observed (e.g., 'high latency, connection timeouts')"),
				),
				mcp.WithString("cluster",
					mcp.Description("Cluster being investigated"),
				),
				mcp.WithString("customer",
					mcp.Description("Customer being investigated"),
				),
			),
			Handler:  handleContextLoad,
			Category: "memory",
			Tags:     []string{"memory", "context", "auto-retrieval"},
			UseCases: []string{"Auto-load relevant context for investigations"},
			IsWrite:  false,
		},
	}
}

// Register the module
func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// Handler implementations

func handleRemember(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	content, err := req.RequireString("content")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("content is required")), nil
	}

	category := req.GetString("category", "general")
	addedBy := req.GetString("added_by", "webb")
	tagsStr := req.GetString("tags", "")

	// Clean #remember prefix if present
	content = strings.TrimPrefix(content, "#remember ")
	content = strings.TrimPrefix(content, "#remember")
	content = strings.TrimSpace(content)

	// Parse tags
	var tags []string
	if tagsStr != "" {
		for _, t := range strings.Split(tagsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}

	client := clients.GetSessionMemoryClient()
	memory, err := client.Remember(content, category, addedBy, tags)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to remember: %v", err)), nil
	}

	return tools.TextResult(fmt.Sprintf("Remembered: %s\n\nID: %s\nCategory: %s\n\nUse `webb_memory_search` to find this later.",
		memory.Content, memory.ID, memory.Category)), nil
}

func handleForget(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idOrDesc, err := req.RequireString("id_or_description")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("id_or_description is required")), nil
	}

	client := clients.GetSessionMemoryClient()
	err = client.Forget(idOrDesc)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to forget: %v", err)), nil
	}

	return tools.TextResult(fmt.Sprintf("Forgotten: %s", idOrDesc)), nil
}

func handleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := req.RequireString("query")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("query is required")), nil
	}

	limit := req.GetInt("limit", 10)

	client := clients.GetSessionMemoryClient()
	result, err := client.Search(query, limit)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("search failed: %v", err)), nil
	}

	return tools.TextResult(clients.FormatSearchResult(result)), nil
}

func handleListMemories(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	category := req.GetString("category", "")
	limit := req.GetInt("limit", 50)

	client := clients.GetSessionMemoryClient()
	memories, err := client.ListMemories(category, limit)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to list memories: %v", err)), nil
	}

	return tools.TextResult(clients.FormatMemoryList(memories)), nil
}

func handleGenerateInsight(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID, err := req.RequireString("session_id")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("session_id is required")), nil
	}

	save := req.GetBool("save", true)

	// Load the session
	sessionClient, err := clients.NewSessionClient()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to create session client: %v", err)), nil
	}

	session, err := sessionClient.LoadSession(sessionID)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load session: %v", err)), nil
	}

	// Generate insight
	memoryClient := clients.GetSessionMemoryClient()
	insight, err := memoryClient.GenerateInsight(session)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to generate insight: %v", err)), nil
	}

	// Save if requested
	if save {
		if err := memoryClient.SaveInsight(insight); err != nil {
			return tools.ErrorResult(fmt.Errorf("insight generated but save failed: %v", err)), nil
		}
	}

	// Format output
	output := clients.FormatInsightList([]*clients.SessionInsight{insight})
	if save {
		output += "\n*Insight saved to memory.*"
	} else {
		output += "\n*Insight not saved (preview only).*"
	}

	return tools.TextResult(output), nil
}

func handleListInsights(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	service := req.GetString("service", "")
	customer := req.GetString("customer", "")
	cluster := req.GetString("cluster", "")
	limit := req.GetInt("limit", 20)

	client := clients.GetSessionMemoryClient()
	insights, err := client.ListInsights(service, customer, cluster, limit)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to list insights: %v", err)), nil
	}

	return tools.TextResult(clients.FormatInsightList(insights)), nil
}

func handleContextLoad(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symptomsStr := req.GetString("symptoms", "")
	cluster := req.GetString("cluster", "")
	customer := req.GetString("customer", "")

	// Parse symptoms
	var symptoms []string
	if symptomsStr != "" {
		for _, s := range strings.Split(symptomsStr, ",") {
			s = strings.TrimSpace(s)
			if s != "" {
				symptoms = append(symptoms, s)
			}
		}
	}

	if len(symptoms) == 0 && cluster == "" && customer == "" {
		return tools.ErrorResult(fmt.Errorf("at least one of symptoms, cluster, or customer is required")), nil
	}

	client := clients.GetSessionMemoryClient()
	context, err := client.LoadRelevantContext(symptoms, cluster, customer)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load context: %v", err)), nil
	}

	return tools.TextResult(clients.FormatRelevantContext(context)), nil
}
