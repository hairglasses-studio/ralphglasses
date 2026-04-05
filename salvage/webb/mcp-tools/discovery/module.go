package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
	"github.com/hairglasses-studio/webb/internal/mcp/tools/common"
)

// Cached tool data for performance optimization
var (
	cachedToolList     []tools.ToolDefinition
	cachedToolMap      map[string]tools.ToolDefinition
	cachedSortedTools  []tools.ToolDefinition // sorted by category then name
	cacheOnce          sync.Once
)

// ToolExample provides usage examples for a tool
type ToolExample struct {
	Description string
	Code        string
	Notes       string
}

// toolExamples maps tool names to their usage examples
var toolExamples = map[string][]ToolExample{
	"webb_cluster_health_full": {
		{Description: "Basic health check", Code: `webb_cluster_health_full(context="headspace-v2")`, Notes: "Returns health score (0-100) with pod, deployment, event, queue, and alert status"},
		{Description: "Without queue/alert checks", Code: `webb_cluster_health_full(context="staging", include_queues=false, include_alerts=false)`, Notes: "Faster check focusing on K8s resources only"},
	},
	"webb_k8s_pods": {
		{Description: "List all pods", Code: `webb_k8s_pods(context="headspace-v2", namespace="acme")`, Notes: "Shows pod status, restarts, and age"},
		{Description: "Filter by label", Code: `webb_k8s_pods(context="staging", namespace="acme", label="app=api")`, Notes: "Filter pods by label selector"},
		{Description: "Unhealthy only", Code: `webb_k8s_pods(context="headspace-v2", format="compact")`, Notes: "Returns only unhealthy pods"},
	},
	"webb_k8s_logs": {
		{Description: "Get pod logs", Code: `webb_k8s_logs(context="headspace-v2", namespace="acme", pod="api-abc123", tail=100)`, Notes: "Returns last 100 lines"},
		{Description: "Specific container", Code: `webb_k8s_logs(context="staging", namespace="acme", pod="runners-xyz", container="runner")`, Notes: "For multi-container pods"},
	},
	"webb_k8s_pod_diagnostic": {
		{Description: "Full pod diagnostic", Code: `webb_k8s_pod_diagnostic(context="headspace-v2", namespace="acme", pod="runners")`, Notes: "Supports partial pod name matching"},
	},
	"webb_pylon_search": {
		{Description: "Search by customer", Code: `webb_pylon_search(query="customer:headspace")`, Notes: "Filter by customer name"},
		{Description: "Open issues with keyword", Code: `webb_pylon_search(query="state:open migration")`, Notes: "Combine state filter with text search"},
	},
	"webb_pylon_get": {
		{Description: "By issue number", Code: `webb_pylon_get(issue_id="#669")`, Notes: "Accepts issue number with # prefix"},
		{Description: "By UUID", Code: `webb_pylon_get(issue_id="abc123-uuid")`, Notes: "Full Pylon issue UUID"},
	},
	"webb_ticket_summary": {
		{Description: "All tickets", Code: `webb_ticket_summary()`, Notes: "Combines Pylon, incident.io, and Shortcut"},
		{Description: "Filter by customer", Code: `webb_ticket_summary(customer="verizon")`, Notes: "Filter to specific customer"},
	},
	"webb_oncall_dashboard": {
		{Description: "Full dashboard", Code: `webb_oncall_dashboard()`, Notes: "Replaces 8+ separate tool calls"},
		{Description: "My queue only", Code: `webb_oncall_dashboard(user="mitch")`, Notes: "Filter to your assigned tickets"},
	},
	"webb_slack_search": {
		{Description: "Search messages", Code: `webb_slack_search(query="redis connection in:#platform-discussions")`, Notes: "Use Slack search syntax"},
		{Description: "From user", Code: `webb_slack_search(query="from:@mitch migration")`, Notes: "Filter by sender"},
	},
	"webb_slack_history": {
		{Description: "Channel history", Code: `webb_slack_history(channel="C08K653F0G2", limit=50)`, Notes: "Use channel ID for reliability"},
	},
	"webb_slack_thread": {
		{Description: "Get thread replies", Code: `webb_slack_thread(channel="C08K653F0G2", thread_ts="1234567890.123456")`, Notes: "Get thread_ts from message"},
		{Description: "Last N replies", Code: `webb_slack_thread(channel="C08K653F0G2", thread_ts="1234567890.123456", limit=10)`, Notes: "Returns last 10 messages"},
	},
	"webb_tool_discover": {
		{Description: "Browse all tools", Code: `webb_tool_discover(detail_level="names")`, Notes: "Minimal tokens (~500)"},
		{Description: "With required params", Code: `webb_tool_discover(detail_level="signatures")`, Notes: "Shows name(required_params) (~800 tokens)"},
		{Description: "Filter by category", Code: `webb_tool_discover(category="kubernetes", detail_level="descriptions")`, Notes: "Browse K8s tools"},
		{Description: "Search by keyword", Code: `webb_tool_discover(search="health", detail_level="descriptions")`, Notes: "Find health-related tools"},
	},
	"webb_tool_schema": {
		{Description: "Get tool schema", Code: `webb_tool_schema(tool_names="webb_k8s_pods,webb_k8s_logs")`, Notes: "Load full schemas on-demand"},
	},
	"webb_ask": {
		{Description: "Natural language query", Code: `webb_ask(question="what is the health of headspace cluster?")`, Notes: "Auto-routes to appropriate tool"},
		{Description: "Customer tickets", Code: `webb_ask(question="any open tickets for verizon?")`, Notes: "Extracts customer context"},
	},
	"webb_database_health_full": {
		{Description: "Full database health", Code: `webb_database_health_full()`, Notes: "Checks Postgres + ClickHouse + connections"},
		{Description: "Long queries only", Code: `webb_database_health_full(include_long_queries=true, long_query_threshold=60)`, Notes: "Focus on queries >60s"},
	},
	"webb_version_audit": {
		{Description: "Audit versions", Code: `webb_version_audit(context="headspace-v2", expected_version="v1.778.2")`, Notes: "Verify deployed versions"},
		{Description: "All components", Code: `webb_version_audit(context="staging")`, Notes: "Check all component versions"},
	},
	"webb_redis_health": {
		{Description: "Redis health check", Code: `webb_redis_health(context="headspace-v2")`, Notes: "Checks pod, NetworkPolicy, secrets"},
	},
	"webb_queue_health_full": {
		{Description: "RabbitMQ health", Code: `webb_queue_health_full()`, Notes: "Wait times, DLQs, consumers, stuck jobs"},
		{Description: "Detailed breakdown", Code: `webb_queue_health_full(detailed=true)`, Notes: "Per-queue breakdown"},
	},
	"webb_grafana_silence_create": {
		{Description: "Silence inactive cluster alerts", Code: `webb_grafana_silence_create(alertname="Inactive Cluster", cluster="mirakl", comment="Cluster decommissioned", duration="7d", confirm=true)`, Notes: "Silences Inactive Cluster alerts for mirakl for 7 days"},
		{Description: "Silence with custom matchers", Code: `webb_grafana_silence_create(matchers="origin_prometheus=staging,severity=warning", comment="Maintenance window", duration="4h", confirm=true)`, Notes: "Use matchers for custom label combinations"},
		{Description: "Dry-run preview (default)", Code: `webb_grafana_silence_create(alertname="Failing Pod", cluster="dev-eks", comment="Testing silence")`, Notes: "Omit confirm=true to preview matchers without creating"},
	},
	"webb_grafana_alerts": {
		{Description: "All firing alerts", Code: `webb_grafana_alerts()`, Notes: "Lists all currently firing alerts"},
		{Description: "Filter by cluster", Code: `webb_grafana_alerts(cluster="headspace")`, Notes: "Filter alerts to specific cluster"},
	},
}

// initCache initializes the cached tool data
func initCache() {
	cacheOnce.Do(func() {
		registry := tools.GetRegistry()
		cachedToolList = registry.GetAllToolDefinitions()

		// Build lookup map
		cachedToolMap = make(map[string]tools.ToolDefinition, len(cachedToolList))
		for _, td := range cachedToolList {
			cachedToolMap[td.Tool.Name] = td
		}

		// Build sorted list (excluding discovery tools)
		cachedSortedTools = make([]tools.ToolDefinition, 0, len(cachedToolList))
		for _, td := range cachedToolList {
			if td.Category != "discovery" {
				cachedSortedTools = append(cachedSortedTools, td)
			}
		}
		sort.Slice(cachedSortedTools, func(i, j int) bool {
			if cachedSortedTools[i].Category != cachedSortedTools[j].Category {
				return cachedSortedTools[i].Category < cachedSortedTools[j].Category
			}
			return cachedSortedTools[i].Tool.Name < cachedSortedTools[j].Tool.Name
		})
	})
}

// getCachedTools returns the cached sorted tool list
func getCachedTools() []tools.ToolDefinition {
	initCache()
	return cachedSortedTools
}

// getCachedToolMap returns the cached tool lookup map
func getCachedToolMap() map[string]tools.ToolDefinition {
	initCache()
	return cachedToolMap
}

// truncateDescription truncates a description to a maximum length.
func truncateDescription(desc string, maxLen int) string {
	if len(desc) <= maxLen {
		return desc
	}
	if maxLen <= 3 {
		return desc[:maxLen]
	}
	return desc[:maxLen-3] + "..."
}

// Module implements the ToolModule interface for progressive tool discovery.
// This module provides on-demand tool schema loading to reduce token usage.
type Module struct{}

func (m *Module) Name() string {
	return "discovery"
}

func (m *Module) Description() string {
	return "Progressive tool discovery for reduced token usage. Browse and load tool schemas on-demand."
}

func (m *Module) Tools() []tools.ToolDefinition {
	baseTools := []tools.ToolDefinition{
		// Gateway tool (consolidates 31 discovery tools)
		GatewayToolDefinition(),

		{
			Tool: mcp.NewTool("webb_tool_discover",
				mcp.WithDescription("Browse tools with detail level: 'names' (~500 tokens), 'signatures' (~800), 'descriptions' (~2000), or 'full' (complete schemas)."),
				mcp.WithString("detail_level",
					mcp.Description("Detail level: 'names' (names only), 'signatures' (names + required params), 'descriptions' (names + descriptions), 'full' (complete schemas). Default: descriptions"),
				),
				mcp.WithString("category",
					mcp.Description("Filter by category: kubernetes, aws, slack, database, tickets, consolidated, operations, collaboration, devtools, presentations, discovery"),
				),
				mcp.WithString("search",
					mcp.Description("Search tools by name or description keyword"),
				),
				mcp.WithString("format",
					mcp.Description("Output format: 'markdown' (default, human-readable), 'compact' (minimal text), 'json' (structured data)"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum tools to return (default: 50, max: 200)"),
				),
				mcp.WithNumber("offset",
					mcp.Description("Offset for pagination (default: 0)"),
				),
			),
			Handler:     handleToolDiscover,
			Category:    "discovery",
			Subcategory: "browse",
			Tags:        []string{"discovery", "browse", "efficiency"},
			UseCases:    []string{"Find tools without loading full schemas", "Browse tool catalog", "Reduce context tokens"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_tool_schema",
				mcp.WithDescription("Get complete schema for specific tool(s) on-demand. Use after webb_tool_discover to load only needed schemas."),
				mcp.WithString("tool_names",
					mcp.Required(),
					mcp.Description("Comma-separated tool names to get schemas for (e.g., 'webb_k8s_pods,webb_cluster_health_full')"),
				),
				mcp.WithBoolean("required_only",
					mcp.Description("Only show required parameters (20-30% token savings for complex tools)"),
				),
				mcp.WithString("format",
					mcp.Description("Output format: 'markdown' (default), 'compact' (minimal), 'json' (structured)"),
				),
			),
			Handler:     handleToolSchema,
			Category:    "discovery",
			Subcategory: "schema",
			Tags:        []string{"discovery", "schema", "efficiency"},
			UseCases:    []string{"Load tool schema on-demand", "Get parameter details for specific tools"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_tool_stats",
				mcp.WithDescription("Get tool registry statistics: total tools, tools per category, and estimated token savings from progressive discovery."),
			),
			Handler:     handleToolStats,
			Category:    "discovery",
			Subcategory: "stats",
			Tags:        []string{"discovery", "stats", "efficiency"},
			UseCases:    []string{"Understand tool distribution", "Estimate token savings"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_tool_help",
				mcp.WithDescription("Get usage examples and detailed help for a tool. Use to learn how to use tools effectively."),
				mcp.WithString("tool_name",
					mcp.Required(),
					mcp.Description("Tool name to get help for (e.g., 'webb_k8s_pods')"),
				),
			),
			Handler:     handleToolHelp,
			Category:    "discovery",
			Subcategory: "help",
			Tags:        []string{"discovery", "help", "examples"},
			UseCases:    []string{"Learn tool usage", "Get examples", "Understand parameters"},
			Complexity:  tools.ComplexitySimple,
		},
	}

	// Add Tool Grouping tools (v7.00)
	baseTools = append(baseTools, ToolGroupingTools()...)

	// Add MCP Registry Management tools (v6.5)
	baseTools = append(baseTools, MCPRegistryTools()...)

	// Add MCP Publish tools (v127) - registry publication, .well-known
	baseTools = append(baseTools, MCPPublishTools()...)

	// Add MCP Ecosystem tools (v6.60) - discover, install, catalog
	baseTools = append(baseTools, MCPEcosystemTools()...)

	// Add MCP Interoperability tools (v13.5) - compliance, registry status, capabilities
	baseTools = append(baseTools, MCPInteropTools()...)

	// Add Scoring tools (v32.5) - quality scoring, benchmarks, trends
	baseTools = append(baseTools, ScoringTools()...)

	// Add Deprecation tools (v38.0) - list deprecated tools
	baseTools = append(baseTools, tools.ToolDefinition{
		Tool: mcp.NewTool("webb_tool_deprecated",
			mcp.WithDescription("List all deprecated tools and their successors. Use to identify tools that need migration."),
		),
		Handler:     handleToolDeprecated,
		Category:    "discovery",
		Subcategory: "lifecycle",
		Tags:        []string{"discovery", "deprecated", "migration", "lifecycle"},
		UseCases:    []string{"Find deprecated tools", "Plan tool migrations", "Maintain compatibility"},
		Complexity:  tools.ComplexitySimple,
	})

	// Configuration tools (scoring, aliases)
	baseTools = append(baseTools, ConfigurationTools()...)

	return baseTools
}

// DetailLevel controls how much information is returned
type DetailLevel string

const (
	DetailNames        DetailLevel = "names"        // ~500 tokens for all tools
	DetailSignatures   DetailLevel = "signatures"   // ~800 tokens - name + required params
	DetailDescriptions DetailLevel = "descriptions" // ~2000 tokens for all tools
	DetailFull         DetailLevel = "full"         // Full schemas, use sparingly
)

// ToolSummary is a minimal representation of a tool
type ToolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	// v128.0: Safety scoring
	SafetyGrade string `json:"safety_grade,omitempty"`
	SafetyScore int    `json:"safety_score,omitempty"`
}

// handleToolDiscover returns tools with configurable detail level
func handleToolDiscover(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	detailStr := req.GetString("detail_level", "descriptions")
	category := req.GetString("category", "")
	search := req.GetString("search", "")
	format := req.GetString("format", "markdown")
	limit := req.GetInt("limit", 50)
	offset := req.GetInt("offset", 0)

	// Validate and cap limits
	if limit > 200 {
		limit = 200
	}
	if limit < 1 {
		limit = 50
	}

	// Validate format
	if format != "markdown" && format != "compact" && format != "json" {
		format = "markdown"
	}

	detail := DetailLevel(detailStr)
	if detail != DetailNames && detail != DetailSignatures && detail != DetailDescriptions && detail != DetailFull {
		detail = DetailDescriptions
	}

	// Use cached, pre-sorted tool list (already excludes discovery tools)
	allTools := getCachedTools()

	// Filter tools (only if category or search specified)
	var filtered []tools.ToolDefinition
	if category == "" && search == "" {
		// No filtering needed, use cached list directly
		filtered = allTools
	} else {
		// Apply filters
		for _, td := range allTools {
			// Category filter
			if category != "" && !strings.EqualFold(td.Category, category) {
				continue
			}

			// Search filter
			if search != "" {
				searchLower := strings.ToLower(search)
				nameMatch := strings.Contains(strings.ToLower(td.Tool.Name), searchLower)
				descMatch := strings.Contains(strings.ToLower(td.Tool.Description), searchLower)
				if !nameMatch && !descMatch {
					continue
				}
			}

			filtered = append(filtered, td)
		}
	}

	// Note: filtered is already sorted (inherited from cached list)

	// Apply pagination
	total := len(filtered)
	if offset >= total {
		offset = 0
	}
	end := offset + limit
	if end > total {
		end = total
	}
	paginated := filtered[offset:end]

	// Handle JSON format separately (structured output)
	if format == "json" {
		type ToolJSON struct {
			Name        string   `json:"name"`
			Description string   `json:"description,omitempty"`
			Category    string   `json:"category"`
			Required    []string `json:"required,omitempty"`
			// v128.0: Safety scoring fields
			SafetyGrade string `json:"safety_grade,omitempty"`
			SafetyScore int    `json:"safety_score,omitempty"`
		}
		type DiscoverResult struct {
			Tools       []ToolJSON `json:"tools"`
			Total       int        `json:"total"`
			Offset      int        `json:"offset"`
			Limit       int        `json:"limit"`
			DetailLevel string     `json:"detail_level"`
		}
		result := DiscoverResult{
			Total:       total,
			Offset:      offset,
			Limit:       limit,
			DetailLevel: string(detail),
			Tools:       make([]ToolJSON, 0, len(paginated)),
		}
		for _, td := range paginated {
			tj := ToolJSON{
				Name:        td.Tool.Name,
				Category:    td.Category,
				SafetyGrade: td.SafetyGrade,
				SafetyScore: td.SafetyScore,
			}
			if detail != DetailNames {
				tj.Description = common.TruncateWords(td.Tool.Description, 100)
			}
			if detail == DetailSignatures || detail == DetailFull {
				tj.Required = td.Tool.InputSchema.Required
			}
			result.Tools = append(result.Tools, tj)
		}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		return tools.TextResult(string(jsonBytes)), nil
	}

	// Handle compact format (minimal text, ~30% smaller than markdown)
	if format == "compact" {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Tools %d-%d/%d [%s]\n", offset+1, offset+len(paginated), total, detail))
		for _, td := range paginated {
			switch detail {
			case DetailNames:
				sb.WriteString(td.Tool.Name + "\n")
			case DetailSignatures:
				if len(td.Tool.InputSchema.Required) > 0 {
					sb.WriteString(fmt.Sprintf("%s(%s)\n", td.Tool.Name, strings.Join(td.Tool.InputSchema.Required, ",")))
				} else {
					sb.WriteString(td.Tool.Name + "()\n")
				}
			case DetailDescriptions, DetailFull:
				// v128.0: Include safety grade badge
				gradeEmoji := gradeToEmoji(td.SafetyGrade)
				sb.WriteString(fmt.Sprintf("%s [%s%s]: %s\n", td.Tool.Name, gradeEmoji, td.SafetyGrade, common.TruncateWords(td.Tool.Description, 60)))
			}
		}
		if end < total {
			sb.WriteString(fmt.Sprintf("[offset=%d for more]\n", end))
		}
		return tools.TextResult(sb.String()), nil
	}

	// Default: Markdown format
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Tool Discovery (showing %d-%d of %d tools)\n\n", offset+1, offset+len(paginated), total))

	if category != "" {
		sb.WriteString(fmt.Sprintf("**Category filter:** %s\n", category))
	}
	if search != "" {
		sb.WriteString(fmt.Sprintf("**Search filter:** %s\n", search))
	}
	sb.WriteString(fmt.Sprintf("**Detail level:** %s\n\n", detail))

	switch detail {
	case DetailNames:
		// Most compact: just names grouped by category
		currentCategory := ""
		for _, td := range paginated {
			if td.Category != currentCategory {
				if currentCategory != "" {
					sb.WriteString("\n")
				}
				sb.WriteString(fmt.Sprintf("## %s\n", td.Category))
				currentCategory = td.Category
			}
			sb.WriteString(fmt.Sprintf("- %s\n", td.Tool.Name))
		}

	case DetailSignatures:
		// Names + required params only (~800 tokens)
		currentCategory := ""
		for _, td := range paginated {
			if td.Category != currentCategory {
				if currentCategory != "" {
					sb.WriteString("\n")
				}
				sb.WriteString(fmt.Sprintf("## %s\n", td.Category))
				currentCategory = td.Category
			}
			// Format: tool_name(required_param1, required_param2)
			sig := td.Tool.Name
			if td.Tool.InputSchema.Required != nil && len(td.Tool.InputSchema.Required) > 0 {
				sig += "(" + strings.Join(td.Tool.InputSchema.Required, ", ") + ")"
			} else {
				sig += "()"
			}
			sb.WriteString(fmt.Sprintf("- %s\n", sig))
		}

	case DetailDescriptions:
		// Names + short descriptions
		currentCategory := ""
		for _, td := range paginated {
			if td.Category != currentCategory {
				if currentCategory != "" {
					sb.WriteString("\n")
				}
				sb.WriteString(fmt.Sprintf("## %s\n", td.Category))
				currentCategory = td.Category
			}
			// Truncate description to first sentence or 100 chars
			desc := common.TruncateWords(td.Tool.Description, 100)
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", td.Tool.Name, desc))
		}

	case DetailFull:
		// Full schemas - use sparingly
		for _, td := range paginated {
			sb.WriteString(formatFullSchema(td, false))
			sb.WriteString("\n---\n\n")
		}
	}

	// Pagination hint
	if end < total {
		sb.WriteString(fmt.Sprintf("\n*Use offset=%d to see next page*\n", end))
	}

	// Usage hint
	sb.WriteString("\n**Tip:** Use `webb_tool_schema(tool_names=\"tool1,tool2\")` to get full schemas for specific tools.\n")

	return tools.TextResult(sb.String()), nil
}

// handleToolSchema returns full schema for specific tools
func handleToolSchema(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	toolNamesStr, err := req.RequireString("tool_names")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("tool_names parameter is required")), nil
	}

	requiredOnly := req.GetBool("required_only", false)
	format := req.GetString("format", "markdown")

	// Validate format
	if format != "markdown" && format != "compact" && format != "json" {
		format = "markdown"
	}

	toolNames := strings.Split(toolNamesStr, ",")
	for i := range toolNames {
		toolNames[i] = strings.TrimSpace(toolNames[i])
	}

	// Use cached tool map for O(1) lookups
	toolMap := getCachedToolMap()

	// Collect found and not found tools
	var foundTools []tools.ToolDefinition
	notFound := []string{}
	for _, name := range toolNames {
		if td, ok := toolMap[name]; ok {
			foundTools = append(foundTools, td)
		} else {
			notFound = append(notFound, name)
		}
	}

	// Handle JSON format
	if format == "json" {
		type ParamJSON struct {
			Name        string `json:"name"`
			Type        string `json:"type"`
			Required    bool   `json:"required"`
			Description string `json:"description,omitempty"`
		}
		type SchemaJSON struct {
			Name        string      `json:"name"`
			Category    string      `json:"category"`
			Description string      `json:"description"`
			Parameters  []ParamJSON `json:"parameters,omitempty"`
		}
		type SchemaResult struct {
			Tools    []SchemaJSON `json:"tools"`
			NotFound []string     `json:"not_found,omitempty"`
		}
		result := SchemaResult{
			Tools:    make([]SchemaJSON, 0, len(foundTools)),
			NotFound: notFound,
		}
		for _, td := range foundTools {
			sj := SchemaJSON{
				Name:        td.Tool.Name,
				Category:    td.Category,
				Description: td.Tool.Description,
			}
			// Build required lookup
			required := make(map[string]bool)
			for _, r := range td.Tool.InputSchema.Required {
				required[r] = true
			}
			// Add parameters
			if td.Tool.InputSchema.Properties != nil {
				for name, prop := range td.Tool.InputSchema.Properties {
					if requiredOnly && !required[name] {
						continue
					}
					pj := ParamJSON{Name: name, Required: required[name]}
					if propMap, ok := prop.(map[string]interface{}); ok {
						if t, ok := propMap["type"].(string); ok {
							pj.Type = t
						}
						if d, ok := propMap["description"].(string); ok {
							pj.Description = d
						}
					}
					sj.Parameters = append(sj.Parameters, pj)
				}
			}
			result.Tools = append(result.Tools, sj)
		}
		jsonBytes, _ := json.MarshalIndent(result, "", "  ")
		return tools.TextResult(string(jsonBytes)), nil
	}

	// Handle compact format
	if format == "compact" {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("Schemas: %d found, %d missing\n", len(foundTools), len(notFound)))
		for _, td := range foundTools {
			sb.WriteString(fmt.Sprintf("\n%s [%s]\n", td.Tool.Name, td.Category))
			sb.WriteString(fmt.Sprintf("  %s\n", common.TruncateWords(td.Tool.Description, 80)))
			if td.Tool.InputSchema.Properties != nil {
				required := make(map[string]bool)
				for _, r := range td.Tool.InputSchema.Required {
					required[r] = true
				}
				for name := range td.Tool.InputSchema.Properties {
					if requiredOnly && !required[name] {
						continue
					}
					if required[name] {
						sb.WriteString(fmt.Sprintf("  * %s (required)\n", name))
					} else {
						sb.WriteString(fmt.Sprintf("  - %s\n", name))
					}
				}
			}
		}
		if len(notFound) > 0 {
			sb.WriteString(fmt.Sprintf("\nNot found: %s\n", strings.Join(notFound, ", ")))
		}
		return tools.TextResult(sb.String()), nil
	}

	// Default: Markdown format
	var sb strings.Builder
	header := fmt.Sprintf("# Tool Schemas (%d requested)", len(toolNames))
	if requiredOnly {
		header += " [required params only]"
	}
	sb.WriteString(header + "\n\n")

	for _, td := range foundTools {
		sb.WriteString(formatFullSchema(td, requiredOnly))
		sb.WriteString("\n---\n\n")
	}

	if len(notFound) > 0 {
		sb.WriteString(fmt.Sprintf("\n**Not found:** %s\n", strings.Join(notFound, ", ")))
		sb.WriteString("Use `webb_tool_discover(search=\"keyword\")` to find tools.\n")
	}

	sb.WriteString(fmt.Sprintf("\n*Found %d of %d requested tools*\n", len(foundTools), len(toolNames)))

	return tools.TextResult(sb.String()), nil
}

// handleToolStats returns registry statistics
func handleToolStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	registry := tools.GetRegistry()
	stats := registry.GetToolStats()

	var sb strings.Builder
	sb.WriteString("# Tool Registry Statistics\n\n")

	sb.WriteString(fmt.Sprintf("**Total Tools:** %d\n", stats.TotalTools))
	sb.WriteString(fmt.Sprintf("**Total Modules:** %d\n\n", stats.ModuleCount))

	sb.WriteString("## Tools by Category\n\n")

	// Sort categories
	categories := make([]string, 0, len(stats.ByCategory))
	for cat := range stats.ByCategory {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	for _, cat := range categories {
		count := stats.ByCategory[cat]
		sb.WriteString(fmt.Sprintf("- **%s:** %d tools\n", cat, count))
	}

	// Token savings estimation with complexity-weighted calculation
	allTools := registry.GetAllToolDefinitions()
	totalTokens := estimateTotalTokens(allTools)
	avgTokensPerTool := totalTokens / stats.TotalTools
	avgOnDemandTokens := estimateOnDemandTokens(5, avgTokensPerTool) // 5 tools average

	sb.WriteString("\n## Token Savings Estimation\n\n")
	sb.WriteString("| Approach | Estimated Tokens |\n")
	sb.WriteString("|----------|------------------|\n")
	sb.WriteString(fmt.Sprintf("| All schemas upfront | ~%d |\n", totalTokens))
	sb.WriteString(fmt.Sprintf("| Names only | ~%d |\n", stats.TotalTools*2))
	sb.WriteString(fmt.Sprintf("| Descriptions | ~%d |\n", stats.TotalTools*15))
	sb.WriteString(fmt.Sprintf("| On-demand (avg 5 tools) | ~%d |\n", avgOnDemandTokens))
	sb.WriteString(fmt.Sprintf("\n**Potential savings:** ~%.0f%% with progressive discovery\n",
		(1.0-float64(avgOnDemandTokens)/float64(totalTokens))*100))

	return tools.TextResult(sb.String()), nil
}

// handleToolHelp returns usage examples and detailed help for a tool
func handleToolHelp(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	toolName, err := req.RequireString("tool_name")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("tool_name parameter is required")), nil
	}

	// Get tool definition
	toolMap := getCachedToolMap()
	td, found := toolMap[toolName]
	if !found {
		// Try with webb_ prefix if not found
		if !strings.HasPrefix(toolName, "webb_") {
			toolName = "webb_" + toolName
			td, found = toolMap[toolName]
		}
	}

	if !found {
		return tools.ErrorResult(fmt.Errorf("tool not found: %s. Use webb_tool_discover to search for tools", toolName)), nil
	}

	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("# %s\n\n", td.Tool.Name))
	sb.WriteString(fmt.Sprintf("**Category:** %s", td.Category))
	if td.Subcategory != "" {
		sb.WriteString(fmt.Sprintf(" / %s", td.Subcategory))
	}
	sb.WriteString("\n\n")

	// Description
	sb.WriteString("## Description\n\n")
	sb.WriteString(td.Tool.Description + "\n\n")

	// Parameters
	if td.Tool.InputSchema.Properties != nil && len(td.Tool.InputSchema.Properties) > 0 {
		sb.WriteString("## Parameters\n\n")

		required := make(map[string]bool)
		for _, r := range td.Tool.InputSchema.Required {
			required[r] = true
		}

		// Sort parameters: required first, then optional
		paramNames := make([]string, 0, len(td.Tool.InputSchema.Properties))
		for name := range td.Tool.InputSchema.Properties {
			paramNames = append(paramNames, name)
		}
		sort.Slice(paramNames, func(i, j int) bool {
			ri, rj := required[paramNames[i]], required[paramNames[j]]
			if ri != rj {
				return ri // required comes first
			}
			return paramNames[i] < paramNames[j]
		})

		for _, name := range paramNames {
			prop := td.Tool.InputSchema.Properties[name]
			reqStr := ""
			if required[name] {
				reqStr = " (required)"
			}

			propMap, ok := prop.(map[string]interface{})
			if ok {
				propType := propMap["type"]
				propDesc := propMap["description"]
				sb.WriteString(fmt.Sprintf("- **%s** (%v)%s: %v\n", name, propType, reqStr, propDesc))
			} else {
				sb.WriteString(fmt.Sprintf("- **%s**%s\n", name, reqStr))
			}
		}
		sb.WriteString("\n")
	}

	// Examples
	examples, hasExamples := toolExamples[toolName]
	if hasExamples && len(examples) > 0 {
		sb.WriteString("## Examples\n\n")
		for i, ex := range examples {
			sb.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, ex.Description))
			sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", ex.Code))
			if ex.Notes != "" {
				sb.WriteString(fmt.Sprintf("*%s*\n\n", ex.Notes))
			}
		}
	} else {
		sb.WriteString("## Examples\n\n")
		sb.WriteString("*No curated examples available for this tool.*\n\n")
		sb.WriteString("Use `webb_tool_schema(tool_names=\"" + toolName + "\")` for full parameter details.\n\n")
	}

	// Use cases
	if len(td.UseCases) > 0 {
		sb.WriteString("## Use Cases\n\n")
		for _, uc := range td.UseCases {
			sb.WriteString(fmt.Sprintf("- %s\n", uc))
		}
		sb.WriteString("\n")
	}

	// Related tools hint
	sb.WriteString("## Related\n\n")
	sb.WriteString(fmt.Sprintf("Find similar tools: `webb_tool_discover(category=\"%s\")`\n", td.Category))

	return tools.TextResult(sb.String()), nil
}

// formatFullSchema formats a complete tool schema
// If requiredOnly is true, only required parameters are shown (20-30% token savings)
func formatFullSchema(td tools.ToolDefinition, requiredOnly bool) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## %s\n\n", td.Tool.Name))
	sb.WriteString(fmt.Sprintf("**Category:** %s\n", td.Category))
	if td.Subcategory != "" {
		sb.WriteString(fmt.Sprintf("**Subcategory:** %s\n", td.Subcategory))
	}
	sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", td.Tool.Description))

	// Deprecation warning (v38.0)
	if td.Deprecated {
		sb.WriteString("**[DEPRECATED")
		if td.DeprecatedSince != "" {
			sb.WriteString(fmt.Sprintf(" since %s", td.DeprecatedSince))
		}
		sb.WriteString("]**")
		if td.DeprecatedReason != "" {
			sb.WriteString(fmt.Sprintf(" %s", td.DeprecatedReason))
		}
		sb.WriteString("\n")
		if td.Successor != "" {
			sb.WriteString(fmt.Sprintf("**Use instead:** `%s`\n", td.Successor))
		}
		sb.WriteString("\n")
	}

	// Parameters
	if td.Tool.InputSchema.Properties != nil && len(td.Tool.InputSchema.Properties) > 0 {
		// Build required lookup
		required := make(map[string]bool)
		for _, r := range td.Tool.InputSchema.Required {
			required[r] = true
		}

		// Sort parameters for consistent output
		paramNames := make([]string, 0, len(td.Tool.InputSchema.Properties))
		for name := range td.Tool.InputSchema.Properties {
			// Skip optional params when requiredOnly is set
			if requiredOnly && !required[name] {
				continue
			}
			paramNames = append(paramNames, name)
		}
		sort.Strings(paramNames)

		if len(paramNames) > 0 {
			if requiredOnly {
				sb.WriteString("**Required Parameters:**\n\n")
			} else {
				sb.WriteString("**Parameters:**\n\n")
			}

			for _, name := range paramNames {
				prop := td.Tool.InputSchema.Properties[name]
				reqStr := ""
				if required[name] {
					reqStr = " **(required)**"
				}

				// Extract type and description from property
				propMap, ok := prop.(map[string]interface{})
				if ok {
					propType := propMap["type"]
					propDesc := propMap["description"]
					sb.WriteString(fmt.Sprintf("- `%s` (%v)%s: %v\n", name, propType, reqStr, propDesc))
				} else {
					sb.WriteString(fmt.Sprintf("- `%s`%s\n", name, reqStr))
				}
			}

			// Show count of omitted optional params
			if requiredOnly {
				totalParams := len(td.Tool.InputSchema.Properties)
				omitted := totalParams - len(paramNames)
				if omitted > 0 {
					sb.WriteString(fmt.Sprintf("\n*(%d optional parameters omitted - use required_only=false for full schema)*\n", omitted))
				}
			}
			sb.WriteString("\n")
		}
	}

	// Metadata
	if len(td.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("**Tags:** %s\n", strings.Join(td.Tags, ", ")))
	}
	if len(td.UseCases) > 0 {
		sb.WriteString(fmt.Sprintf("**Use cases:** %s\n", strings.Join(td.UseCases, "; ")))
	}
	if td.IsWrite {
		sb.WriteString("**Note:** This tool modifies state (write operation)\n")
	}

	return sb.String()
}

// estimateToolTokens estimates the token count for a single tool based on complexity
func estimateToolTokens(td tools.ToolDefinition) int {
	// Base tokens by complexity
	var baseTokens int
	switch td.Complexity {
	case tools.ComplexitySimple:
		baseTokens = 300
	case tools.ComplexityModerate:
		baseTokens = 600
	case tools.ComplexityComplex:
		baseTokens = 1200
	default:
		baseTokens = 600 // default to moderate
	}

	// Add tokens for parameters (approx 50 tokens per parameter)
	paramCount := 0
	if td.Tool.InputSchema.Properties != nil {
		paramCount = len(td.Tool.InputSchema.Properties)
	}
	baseTokens += paramCount * 50

	// Add tokens for thinking budget recommendation
	if td.ThinkingBudget > 0 {
		baseTokens += 100
	}

	return baseTokens
}

// estimateTotalTokens estimates total tokens for all tools
func estimateTotalTokens(allTools []tools.ToolDefinition) int {
	total := 0
	for _, td := range allTools {
		total += estimateToolTokens(td)
	}
	return total
}

// estimateOnDemandTokens estimates tokens for on-demand loading of N tools
func estimateOnDemandTokens(numTools, avgTokensPerTool int) int {
	// Discovery tools overhead: ~150 tokens for the 3 discovery tools
	discoveryOverhead := 150
	return discoveryOverhead + (numTools * avgTokensPerTool)
}

// ToolGroupingTools returns tools for alias management and namespace browsing (v7.00)
func ToolGroupingTools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("webb_tool_alias",
				mcp.WithDescription("Manage tool aliases for shorter names. List, add, or remove aliases that map to full tool names."),
				mcp.WithString("action",
					mcp.Description("Action: 'list' (default), 'add', 'remove', 'resolve'"),
				),
				mcp.WithString("alias",
					mcp.Description("The short alias name (required for add/remove/resolve)"),
				),
				mcp.WithString("tool_name",
					mcp.Description("The full tool name (required for add)"),
				),
			),
			Handler:     handleToolAlias,
			Category:    "discovery",
			Subcategory: "aliases",
			Tags:        []string{"discovery", "aliases", "shortcuts", "efficiency"},
			UseCases:    []string{"Create shortcuts for common tools", "List available aliases", "Resolve alias to tool name"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_tool_tree",
				mcp.WithDescription("View tools organized in a hierarchical tree by category and subcategory. Helps understand tool organization."),
				mcp.WithString("category",
					mcp.Description("Filter to specific category (optional)"),
				),
				mcp.WithBoolean("show_tools",
					mcp.Description("Show individual tool names in tree (default: false for compact view)"),
				),
			),
			Handler:     handleToolTree,
			Category:    "discovery",
			Subcategory: "browse",
			Tags:        []string{"discovery", "tree", "organization", "browse"},
			UseCases:    []string{"Understand tool organization", "Browse by category", "Find related tools"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_tool_related",
				mcp.WithDescription("Find tools related to a given tool by category, subcategory, and tags."),
				mcp.WithString("tool_name",
					mcp.Required(),
					mcp.Description("Tool name to find related tools for"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum related tools to return (default: 10)"),
				),
			),
			Handler:     handleToolRelated,
			Category:    "discovery",
			Subcategory: "search",
			Tags:        []string{"discovery", "related", "similar", "suggestions"},
			UseCases:    []string{"Find similar tools", "Discover alternatives", "Explore related functionality"},
			Complexity:  tools.ComplexitySimple,
		},
	}
}

// handleToolAlias manages tool aliases
func handleToolAlias(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := req.GetString("action", "list")
	alias := req.GetString("alias", "")
	toolName := req.GetString("tool_name", "")

	registry := tools.GetAliasRegistry()

	switch action {
	case "list":
		var sb strings.Builder
		sb.WriteString("# Tool Aliases\n\n")
		sb.WriteString("Aliases provide shorter names for common tools.\n\n")

		aliasesByCategory := registry.ListAliases()
		categories := make([]string, 0, len(aliasesByCategory))
		for cat := range aliasesByCategory {
			categories = append(categories, cat)
		}
		sort.Strings(categories)

		for _, cat := range categories {
			aliases := aliasesByCategory[cat]
			sb.WriteString(fmt.Sprintf("## %s\n\n", cat))
			sb.WriteString("| Alias | Tool |\n|-------|------|\n")
			for _, entry := range aliases {
				sb.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", entry.Alias, entry.ToolName))
			}
			sb.WriteString("\n")
		}

		sb.WriteString("\n**Usage:** Use aliases in place of full tool names, e.g., `pods` instead of `webb_k8s_pods`\n")

		return mcp.NewToolResultText(sb.String()), nil

	case "add":
		if alias == "" || toolName == "" {
			return mcp.NewToolResultError("Both 'alias' and 'tool_name' are required for add action"), nil
		}
		// Verify tool exists
		if _, exists := getCachedToolMap()[toolName]; !exists {
			return mcp.NewToolResultError(fmt.Sprintf("Tool '%s' not found", toolName)), nil
		}
		if err := registry.RegisterAlias(alias, toolName, true); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to add alias: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("✅ Added alias: `%s` → `%s`", alias, toolName)), nil

	case "remove":
		if alias == "" {
			return mcp.NewToolResultError("'alias' is required for remove action"), nil
		}
		if err := registry.RemoveAlias(alias); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to remove alias: %v", err)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("✅ Removed alias: `%s`", alias)), nil

	case "resolve":
		if alias == "" {
			return mcp.NewToolResultError("'alias' is required for resolve action"), nil
		}
		resolved := registry.Resolve(alias)
		if resolved == alias {
			return mcp.NewToolResultText(fmt.Sprintf("`%s` is not an alias, using as tool name", alias)), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("`%s` → `%s`", alias, resolved)), nil

	default:
		return mcp.NewToolResultError(fmt.Sprintf("Unknown action: %s. Use 'list', 'add', 'remove', or 'resolve'", action)), nil
	}
}

// handleToolTree shows hierarchical tool organization
func handleToolTree(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	category := req.GetString("category", "")
	showTools := req.GetBool("show_tools", false)

	tree := tools.BuildNamespaceTree()

	var sb strings.Builder
	sb.WriteString("# Tool Namespace Tree\n\n")

	if category != "" {
		// Show specific category
		if catNode, ok := tree.Children[category]; ok {
			sb.WriteString(fmt.Sprintf("## %s\n\n", category))
			formatCategoryNode(&sb, catNode, showTools, "")
		} else {
			return mcp.NewToolResultError(fmt.Sprintf("Category '%s' not found", category)), nil
		}
	} else {
		// Show all categories
		sb.WriteString("```\n")
		sb.WriteString("webb/\n")
		sb.WriteString(tools.FormatNamespaceTree(tree, ""))
		sb.WriteString("```\n\n")

		// Summary stats
		allTools := getCachedTools()
		categoryCount := make(map[string]int)
		for _, td := range allTools {
			categoryCount[td.Category]++
		}

		sb.WriteString("## Summary\n\n")
		sb.WriteString(fmt.Sprintf("**Total Tools:** %d\n", len(allTools)))
		sb.WriteString(fmt.Sprintf("**Categories:** %d\n\n", len(categoryCount)))

		// Top categories
		type catCount struct {
			name  string
			count int
		}
		counts := make([]catCount, 0, len(categoryCount))
		for cat, count := range categoryCount {
			counts = append(counts, catCount{cat, count})
		}
		sort.Slice(counts, func(i, j int) bool {
			return counts[i].count > counts[j].count
		})

		sb.WriteString("**Top Categories:**\n")
		for i, cc := range counts {
			if i >= 10 {
				break
			}
			sb.WriteString(fmt.Sprintf("- %s: %d tools\n", cc.name, cc.count))
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// formatCategoryNode formats a category node with optional tool listing
func formatCategoryNode(sb *strings.Builder, node *tools.ToolNamespace, showTools bool, indent string) {
	// Show subcategories
	subcats := make([]string, 0, len(node.Children))
	for subcat := range node.Children {
		subcats = append(subcats, subcat)
	}
	sort.Strings(subcats)

	for _, subcat := range subcats {
		subNode := node.Children[subcat]
		sb.WriteString(fmt.Sprintf("%s### %s (%d tools)\n\n", indent, subcat, len(subNode.Tools)))
		if showTools {
			for _, tool := range subNode.Tools {
				sb.WriteString(fmt.Sprintf("%s- `%s`\n", indent, tool))
			}
			sb.WriteString("\n")
		}
	}

	// Show direct tools
	if len(node.Tools) > 0 {
		if len(node.Children) > 0 {
			sb.WriteString(fmt.Sprintf("%s### (uncategorized) (%d tools)\n\n", indent, len(node.Tools)))
		}
		if showTools {
			for _, tool := range node.Tools {
				sb.WriteString(fmt.Sprintf("%s- `%s`\n", indent, tool))
			}
			sb.WriteString("\n")
		}
	}
}

// handleToolRelated finds tools related to a given tool
func handleToolRelated(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	toolName := req.GetString("tool_name", "")
	limit := req.GetInt("limit", 10)

	if toolName == "" {
		return mcp.NewToolResultError("tool_name is required"), nil
	}

	// Resolve alias if needed
	toolName = tools.GetAliasRegistry().Resolve(toolName)

	toolMap := getCachedToolMap()
	targetTool, exists := toolMap[toolName]
	if !exists {
		return mcp.NewToolResultError(fmt.Sprintf("Tool '%s' not found", toolName)), nil
	}

	// Score all other tools by relevance
	type scoredTool struct {
		name  string
		score int
		reason string
	}
	var scored []scoredTool

	allTools := getCachedTools()
	for _, td := range allTools {
		if td.Tool.Name == toolName {
			continue
		}

		score := 0
		reasons := []string{}

		// Same category: +3
		if td.Category == targetTool.Category {
			score += 3
			reasons = append(reasons, "same category")
		}

		// Same subcategory: +5
		if td.Subcategory == targetTool.Subcategory && td.Subcategory != "" {
			score += 5
			reasons = append(reasons, "same subcategory")
		}

		// Overlapping tags: +2 per tag
		targetTags := make(map[string]bool)
		for _, tag := range targetTool.Tags {
			targetTags[tag] = true
		}
		for _, tag := range td.Tags {
			if targetTags[tag] {
				score += 2
				reasons = append(reasons, fmt.Sprintf("shared tag: %s", tag))
			}
		}

		// Similar name prefix: +2
		if len(td.Tool.Name) > 5 && len(toolName) > 5 {
			prefix := toolName[:strings.LastIndex(toolName, "_")+1]
			if strings.HasPrefix(td.Tool.Name, prefix) {
				score += 2
				reasons = append(reasons, "similar name")
			}
		}

		if score > 0 {
			reasonStr := ""
			if len(reasons) > 0 {
				reasonStr = reasons[0]
				if len(reasons) > 1 {
					reasonStr += fmt.Sprintf(" (+%d more)", len(reasons)-1)
				}
			}
			scored = append(scored, scoredTool{td.Tool.Name, score, reasonStr})
		}
	}

	// Sort by score descending
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	// Limit results
	if len(scored) > limit {
		scored = scored[:limit]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Tools Related to `%s`\n\n", toolName))
	sb.WriteString(fmt.Sprintf("**Category:** %s\n", targetTool.Category))
	if targetTool.Subcategory != "" {
		sb.WriteString(fmt.Sprintf("**Subcategory:** %s\n", targetTool.Subcategory))
	}
	if len(targetTool.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("**Tags:** %s\n", strings.Join(targetTool.Tags, ", ")))
	}
	sb.WriteString("\n## Related Tools\n\n")

	if len(scored) == 0 {
		sb.WriteString("No related tools found.\n")
	} else {
		sb.WriteString("| Tool | Relevance | Reason |\n|------|-----------|--------|\n")
		for _, st := range scored {
			stars := strings.Repeat("★", st.score/2)
			if st.score%2 == 1 {
				stars += "½"
			}
			sb.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", st.name, stars, st.reason))
		}
	}

	// Show aliases
	aliases := tools.GetAliasRegistry().GetAliasesForTool(toolName)
	if len(aliases) > 0 {
		sb.WriteString(fmt.Sprintf("\n**Aliases:** %s\n", strings.Join(aliases, ", ")))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// handleToolDeprecated lists all deprecated tools and their successors (v38.0)
func handleToolDeprecated(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	registry := tools.GetRegistry()
	allTools := registry.GetAllToolDefinitions()

	var deprecated []tools.ToolDefinition
	for _, td := range allTools {
		if td.Deprecated {
			deprecated = append(deprecated, td)
		}
	}

	var sb strings.Builder
	sb.WriteString("# Deprecated Tools\n\n")

	if len(deprecated) == 0 {
		sb.WriteString("No deprecated tools found. All tools are current.\n")
		return mcp.NewToolResultText(sb.String()), nil
	}

	sb.WriteString(fmt.Sprintf("Found %d deprecated tools:\n\n", len(deprecated)))
	sb.WriteString("| Tool | Deprecated Since | Reason | Successor |\n")
	sb.WriteString("|------|-----------------|--------|----------|\n")

	for _, td := range deprecated {
		since := td.DeprecatedSince
		if since == "" {
			since = "unknown"
		}
		reason := td.DeprecatedReason
		if reason == "" {
			reason = "-"
		}
		successor := td.Successor
		if successor == "" {
			successor = "-"
		}
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | `%s` |\n",
			td.Tool.Name, since, reason, successor))
	}

	sb.WriteString("\n## Migration Guide\n\n")
	sb.WriteString("To migrate from deprecated tools:\n")
	sb.WriteString("1. Identify the successor tool in the table above\n")
	sb.WriteString("2. Use `webb_tool_schema` to get the new tool's parameters\n")
	sb.WriteString("3. Update your calls to use the successor tool\n")

	return mcp.NewToolResultText(sb.String()), nil
}

// gradeToEmoji converts a safety grade to a visual indicator
// Note: Using text indicators instead of emojis for terminal compatibility
func gradeToEmoji(grade string) string {
	switch grade {
	case "A":
		return "+" // Safe
	case "B":
		return "+" // Safe
	case "C":
		return "~" // Caution
	case "D":
		return "!" // Risky
	case "F":
		return "!!" // Dangerous
	default:
		return "?" // Unknown
	}
}

// init registers this module with the global registry
func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}
