// Package discovery provides progressive tool discovery for reduced token usage.
// Implements 8 meta-tools: discover, schema, stats, help, usage, flow, next, workflow.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/runmylife/internal/mcp/tools"
	"github.com/hairglasses-studio/runmylife/internal/mcp/tools/common"
)

var (
	cachedToolList    []tools.ToolDefinition
	cachedToolMap     map[string]tools.ToolDefinition
	cachedSortedTools []tools.ToolDefinition
	cacheOnce         sync.Once
)

func initCache() {
	cacheOnce.Do(func() {
		registry := tools.GetRegistry()
		cachedToolList = registry.GetAllToolDefinitions()
		cachedToolMap = make(map[string]tools.ToolDefinition, len(cachedToolList))
		for _, td := range cachedToolList {
			cachedToolMap[td.Tool.Name] = td
		}
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

// Predefined workflows for common life management patterns.
var predefinedWorkflows = []workflowDef{
	{
		Name:        "morning_check",
		Description: "Morning briefing: sync data, assess readiness, generate briefing",
		Steps: []workflowStep{
			{Tool: "runmylife_sync", Action: "run/todoist", Desc: "Sync tasks from Todoist", Produces: []string{"task"}},
			{Tool: "runmylife_calendar", Action: "events/list", Desc: "Load today's calendar", Produces: []string{"calendar_event"}},
			{Tool: "runmylife_wellness", Action: "energy/estimate", Desc: "Estimate current energy level", Produces: []string{"energy_level"}},
			{Tool: "runmylife_briefing", Action: "generate/today", Desc: "Generate daily briefing", Consumes: []string{"task", "calendar_event", "energy_level"}},
		},
	},
	{
		Name:        "triage_inbox",
		Description: "Email triage: scan unread, check reply debt, prioritize responses",
		Steps: []workflowStep{
			{Tool: "runmylife_gmail", Action: "triage/unread", Desc: "Scan unread emails", Produces: []string{"gmail_message"}},
			{Tool: "runmylife_personal", Action: "reply/radar", Desc: "Check reply debt across channels", Produces: []string{"reply"}},
			{Tool: "runmylife_tasks", Action: "prioritize/matrix", Desc: "Prioritize by urgency", Consumes: []string{"gmail_message", "reply"}},
		},
	},
	{
		Name:        "weekly_review",
		Description: "Sunday review: stats, habits, social health, finances",
		Steps: []workflowStep{
			{Tool: "runmylife_analytics", Action: "report/weekly", Desc: "Compile weekly statistics", Produces: []string{"analytics"}},
			{Tool: "runmylife_social", Action: "health/scores", Desc: "Review relationship health", Produces: []string{"social_health"}},
			{Tool: "runmylife_habits", Action: "track/streaks", Desc: "Review habit streaks", Produces: []string{"habit"}},
			{Tool: "runmylife_finances", Action: "summary/weekly", Desc: "Weekly spending summary", Produces: []string{"transaction"}},
		},
	},
	{
		Name:        "reply_batch",
		Description: "Focused reply session: scan, prioritize, draft responses",
		Steps: []workflowStep{
			{Tool: "runmylife_personal", Action: "reply/radar", Desc: "Scan pending replies", Produces: []string{"reply"}},
			{Tool: "runmylife_personal", Action: "reply/prioritize", Desc: "Rank by urgency and relationship tier", Consumes: []string{"reply"}},
			{Tool: "runmylife_gmail", Action: "drafts/create", Desc: "Draft responses", Consumes: []string{"reply"}, Produces: []string{"gmail_message"}},
		},
	},
	{
		Name:        "energy_match",
		Description: "Match current energy to best task: estimate energy, find matching tasks",
		Steps: []workflowStep{
			{Tool: "runmylife_wellness", Action: "energy/estimate", Desc: "Estimate current energy", Produces: []string{"energy_level"}},
			{Tool: "runmylife_tasks", Action: "manage/list", Desc: "List open tasks", Produces: []string{"task"}},
			{Tool: "runmylife_tasks", Action: "prioritize/energy_match", Desc: "Match tasks to energy level", Consumes: []string{"task", "energy_level"}},
		},
	},
	{
		Name:        "evening_winddown",
		Description: "Evening review: check habits, log mood, preview tomorrow",
		Steps: []workflowStep{
			{Tool: "runmylife_habits", Action: "track/today", Desc: "Check today's habit completions", Produces: []string{"habit"}},
			{Tool: "runmylife_wellness", Action: "mood/log", Desc: "Log evening mood", Produces: []string{"mood"}},
			{Tool: "runmylife_calendar", Action: "events/list", Desc: "Preview tomorrow's calendar", Produces: []string{"calendar_event"}},
		},
	},
}

type workflowDef struct {
	Name        string
	Description string
	Steps       []workflowStep
}

type workflowStep struct {
	Tool     string
	Action   string
	Desc     string
	Produces []string
	Consumes []string
}

// Module implements the ToolModule interface for discovery tools.
type Module struct{}

func (m *Module) Name() string        { return "discovery" }
func (m *Module) Description() string { return "Progressive tool discovery and workflow composition" }

func (m *Module) Tools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("runmylife_tool_discover",
				mcp.WithDescription("Browse available tools with progressive detail levels. Start with 'names' for token efficiency, drill down with 'full' for complete schemas."),
				mcp.WithString("category", mcp.Description("Filter by category (tasks, calendar, gmail, contacts, habits, finances, discord, spotify, etc.)")),
				mcp.WithString("query", mcp.Description("Search tools by name, tag, use case, or description")),
				mcp.WithString("detail", mcp.Description("Detail level: names (~500 tokens), signatures (name+params), descriptions (default, ~2000 tokens), full (complete schemas)")),
				mcp.WithString("entity", mcp.Description("Filter by entity type (task, calendar_event, gmail_message, contact, habit, transaction, mood, reply)")),
				mcp.WithString("direction", mcp.Description("Entity filter direction: produces, consumes, or both (default)")),
				mcp.WithNumber("limit", mcp.Description("Max results to return (default: all)")),
			),
			Handler:    tools.ToolHandlerFunc(handleDiscover),
			Category:   "discovery",
			Tags:       []string{"discovery", "browse", "efficiency"},
			UseCases:   []string{"Find tools for a task", "Browse by category", "Token-efficient exploration"},
			Complexity: tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("runmylife_tool_schema",
				mcp.WithDescription("Get full JSON schema for specific tools. Supports comma-separated names for batch loading."),
				mcp.WithString("tool_names", mcp.Required(), mcp.Description("Tool name(s), comma-separated (e.g. 'runmylife_tasks,runmylife_calendar')")),
			),
			Handler:    tools.ToolHandlerFunc(handleSchema),
			Category:   "discovery",
			Tags:       []string{"discovery", "schema"},
			UseCases:   []string{"Load tool schema on demand", "Batch load multiple schemas"},
			Complexity: tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("runmylife_tool_stats",
				mcp.WithDescription("Registry statistics: tool counts, categories, most used, complexity breakdown."),
			),
			Handler:    tools.ToolHandlerFunc(handleStats),
			Category:   "discovery",
			Tags:       []string{"discovery", "stats"},
			Complexity: tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("runmylife_tool_help",
				mcp.WithDescription("Detailed tool help: parameters, entity flow, upstream/downstream tools, use cases."),
				mcp.WithString("tool_name", mcp.Description("Tool name for detailed help (omit for topic-based help)")),
				mcp.WithString("topic", mcp.Description("Help topic: tasks, calendar, gmail, sync, adhd, briefing, all")),
			),
			Handler:    tools.ToolHandlerFunc(handleHelp),
			Category:   "discovery",
			Tags:       []string{"discovery", "help"},
			UseCases:   []string{"Understand tool parameters", "Find related tools", "Quick workflow reference"},
			Complexity: tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("runmylife_tool_usage",
				mcp.WithDescription("Tool usage analytics: invocation counts and recency."),
				mcp.WithString("sort", mcp.Description("Sort by: count (default) or recent")),
				mcp.WithNumber("limit", mcp.Description("Max results (default: 20)")),
			),
			Handler:    tools.ToolHandlerFunc(handleUsage),
			Category:   "discovery",
			Tags:       []string{"discovery", "analytics"},
			UseCases:   []string{"See most used tools", "Find recently used tools"},
			Complexity: tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("runmylife_tool_flow",
				mcp.WithDescription("Visualize entity data flow between tools. Shows which tools produce and consume each entity type."),
				mcp.WithString("entity", mcp.Description("Filter to specific entity type (e.g. task, gmail_message, calendar_event)")),
			),
			Handler:    tools.ToolHandlerFunc(handleFlow),
			Category:   "discovery",
			Tags:       []string{"discovery", "flow", "entities"},
			UseCases:   []string{"Understand data flow", "Find tool chains", "Plan multi-step workflows"},
			Complexity: tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("runmylife_tool_next",
				mcp.WithDescription("Recommend next tools based on context. Auto-detects last used tool or accepts explicit input."),
				mcp.WithString("last_tool", mcp.Description("Tool just used (auto-detected from usage history if omitted)")),
				mcp.WithString("goal", mcp.Description("Current goal: organize, communicate, track, review, wellness")),
			),
			Handler:    tools.ToolHandlerFunc(handleNext),
			Category:   "discovery",
			Tags:       []string{"discovery", "workflow", "recommendation"},
			UseCases:   []string{"Get next-step suggestions", "Build ad-hoc workflows", "Goal-directed tool discovery"},
			Complexity: tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("runmylife_tool_workflow",
				mcp.WithDescription("Browse predefined multi-step workflows for common life management patterns."),
				mcp.WithString("name", mcp.Description("Workflow name for details (omit to list all). Options: morning_check, triage_inbox, weekly_review, reply_batch, energy_match, evening_winddown")),
			),
			Handler:    tools.ToolHandlerFunc(handleWorkflow),
			Category:   "discovery",
			Tags:       []string{"discovery", "workflow"},
			UseCases:   []string{"Follow predefined workflows", "Morning routine", "Weekly review"},
			Complexity: tools.ComplexitySimple,
		},
	}
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}

// --- Discover (enhanced with detail levels, entity filtering, pagination) ---

func handleDiscover(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	initCache()
	category := common.GetStringParam(req, "category", "")
	query := strings.ToLower(common.GetStringParam(req, "query", ""))
	detail := common.GetStringParam(req, "detail", "descriptions")
	entity := common.GetStringParam(req, "entity", "")
	direction := common.GetStringParam(req, "direction", "both")
	limit := common.GetIntParam(req, "limit", 0)

	// Filter
	var filtered []tools.ToolDefinition
	for _, td := range cachedSortedTools {
		if category != "" && td.Category != category {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(td.Tool.Name + " " + td.Tool.Description + " " +
				strings.Join(td.Tags, " ") + " " + strings.Join(td.UseCases, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		if entity != "" {
			match := false
			switch direction {
			case "produces":
				match = containsStr(td.ProducesRefs, entity)
			case "consumes":
				match = containsStr(td.ConsumesRefs, entity)
			default:
				match = containsStr(td.ProducesRefs, entity) || containsStr(td.ConsumesRefs, entity)
			}
			if !match {
				continue
			}
		}
		filtered = append(filtered, td)
	}

	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	if len(filtered) == 0 {
		return tools.TextResult("No matching tools found."), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Tools (%d found)\n\n", len(filtered)))

	switch detail {
	case "names":
		// Minimal: just names grouped by category
		currentCat := ""
		for _, td := range filtered {
			if td.Category != currentCat {
				if currentCat != "" {
					sb.WriteString("\n")
				}
				currentCat = td.Category
				sb.WriteString(fmt.Sprintf("**%s:** ", currentCat))
			} else {
				sb.WriteString(", ")
			}
			sb.WriteString(td.Tool.Name)
		}
		sb.WriteString("\n")

	case "signatures":
		// Names + required parameters
		for _, td := range filtered {
			params := extractRequiredParams(td)
			if len(params) > 0 {
				sb.WriteString(fmt.Sprintf("- `%s(%s)`\n", td.Tool.Name, strings.Join(params, ", ")))
			} else {
				sb.WriteString(fmt.Sprintf("- `%s()`\n", td.Tool.Name))
			}
		}

	case "full":
		// Complete schema for each tool
		for _, td := range filtered {
			sb.WriteString(fmt.Sprintf("## %s\n", td.Tool.Name))
			sb.WriteString(fmt.Sprintf("**Category:** %s | **Complexity:** %s", td.Category, td.Complexity))
			if td.IsWrite {
				sb.WriteString(" | **writes data**")
			}
			sb.WriteString("\n\n")
			sb.WriteString(td.Tool.Description + "\n\n")
			if props := td.Tool.InputSchema.Properties; len(props) > 0 {
				sb.WriteString("**Parameters:**\n")
				for name, schema := range props {
					desc := extractParamDesc(schema)
					req := isRequired(td, name)
					marker := ""
					if req {
						marker = " *(required)*"
					}
					sb.WriteString(fmt.Sprintf("- `%s`%s: %s\n", name, marker, desc))
				}
			}
			if len(td.ProducesRefs) > 0 {
				sb.WriteString(fmt.Sprintf("**Produces:** %s\n", strings.Join(td.ProducesRefs, ", ")))
			}
			if len(td.ConsumesRefs) > 0 {
				sb.WriteString(fmt.Sprintf("**Consumes:** %s\n", strings.Join(td.ConsumesRefs, ", ")))
			}
			sb.WriteString("\n")
		}

	default: // "descriptions"
		// Table with name, category, description
		sb.WriteString("| Tool | Category | Description |\n|------|----------|-------------|\n")
		for _, td := range filtered {
			desc := td.Tool.Description
			if len(desc) > 70 {
				desc = desc[:67] + "..."
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s |\n", td.Tool.Name, td.Category, desc))
		}
	}

	return tools.TextResult(sb.String()), nil
}

// --- Schema (enhanced with comma-separated batch loading) ---

func handleSchema(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	initCache()
	namesRaw, ok := common.RequireStringParam(req, "tool_names")
	if !ok {
		return common.CodedErrorResultf(common.ErrInvalidParam, "tool_names is required"), nil
	}

	names := strings.Split(namesRaw, ",")
	var results []map[string]interface{}
	var notFound []string

	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		td, exists := cachedToolMap[name]
		if !exists {
			notFound = append(notFound, name)
			continue
		}
		schema := map[string]interface{}{
			"name":        td.Tool.Name,
			"description": td.Tool.Description,
			"category":    td.Category,
			"tags":        td.Tags,
			"complexity":  td.Complexity,
			"is_write":    td.IsWrite,
			"schema":      td.Tool.InputSchema,
		}
		if len(td.ProducesRefs) > 0 {
			schema["produces"] = td.ProducesRefs
		}
		if len(td.ConsumesRefs) > 0 {
			schema["consumes"] = td.ConsumesRefs
		}
		if td.CircuitBreakerGroup != "" {
			schema["circuit_breaker_group"] = td.CircuitBreakerGroup
		}
		if td.Timeout > 0 {
			schema["timeout"] = td.Timeout.String()
		}
		results = append(results, schema)
	}

	output := map[string]interface{}{"tools": results}
	if len(notFound) > 0 {
		output["not_found"] = notFound
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	return tools.TextResult(string(data)), nil
}

// --- Stats (enhanced with complexity breakdown and most-used) ---

func handleStats(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	initCache()
	registry := tools.GetRegistry()
	stats := registry.GetToolStats()

	// Complexity breakdown
	complexity := map[string]int{}
	entityTypes := map[string]bool{}
	for _, td := range cachedSortedTools {
		complexity[string(td.Complexity)]++
		for _, e := range td.ProducesRefs {
			entityTypes[e] = true
		}
		for _, e := range td.ConsumesRefs {
			entityTypes[e] = true
		}
	}

	entities := make([]string, 0, len(entityTypes))
	for e := range entityTypes {
		entities = append(entities, e)
	}
	sort.Strings(entities)

	// Try to get top tools from DB
	var topTools []map[string]interface{}
	if db, err := common.SqlDB(); err == nil {
		if entries, err := tools.TopTools(db, 5); err == nil {
			for _, e := range entries {
				topTools = append(topTools, map[string]interface{}{
					"tool":  e.ToolName,
					"count": e.InvocationCount,
					"last":  e.LastUsedAt.Format("2006-01-02 15:04"),
				})
			}
		}
	}

	result := map[string]interface{}{
		"total_tools":  stats.TotalTools,
		"module_count": stats.ModuleCount,
		"by_category":  stats.ByCategory,
		"by_complexity": complexity,
		"entity_types": entities,
	}
	if len(topTools) > 0 {
		result["most_used"] = topTools
	}

	data, _ := json.MarshalIndent(result, "", "  ")
	return tools.TextResult(string(data)), nil
}

// --- Help (enhanced with entity flow, parameters, upstream/downstream) ---

func handleHelp(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	initCache()

	// If a specific tool is requested, show detailed help
	toolName := common.GetStringParam(req, "tool_name", "")
	if toolName != "" {
		return handleToolHelp(toolName)
	}

	// Otherwise, topic-based help
	topic := common.GetStringParam(req, "topic", "all")
	return handleTopicHelp(topic)
}

func handleToolHelp(toolName string) (*mcp.CallToolResult, error) {
	td, exists := cachedToolMap[toolName]
	if !exists {
		return common.CodedErrorResultf(common.ErrNotFound, "tool %s not found", toolName), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", td.Tool.Name))
	sb.WriteString(fmt.Sprintf("**Category:** %s", td.Category))
	if td.Subcategory != "" {
		sb.WriteString(fmt.Sprintf(" / %s", td.Subcategory))
	}
	sb.WriteString(fmt.Sprintf(" | **Complexity:** %s", td.Complexity))
	if td.IsWrite {
		sb.WriteString(" | **Modifies data**")
	}
	sb.WriteString("\n\n")
	sb.WriteString(td.Tool.Description + "\n\n")

	// Parameters
	if props := td.Tool.InputSchema.Properties; len(props) > 0 {
		sb.WriteString("## Parameters\n\n")
		for name, schema := range props {
			desc := extractParamDesc(schema)
			req := isRequired(td, name)
			if req {
				sb.WriteString(fmt.Sprintf("- **`%s`** *(required)*: %s\n", name, desc))
			} else {
				sb.WriteString(fmt.Sprintf("- `%s`: %s\n", name, desc))
			}
		}
		sb.WriteString("\n")
	}

	// Entity flow
	if len(td.ProducesRefs) > 0 || len(td.ConsumesRefs) > 0 {
		sb.WriteString("## Entity Flow\n\n")
		if len(td.ProducesRefs) > 0 {
			sb.WriteString(fmt.Sprintf("**Produces:** %s\n", strings.Join(td.ProducesRefs, ", ")))
		}
		if len(td.ConsumesRefs) > 0 {
			sb.WriteString(fmt.Sprintf("**Consumes:** %s\n", strings.Join(td.ConsumesRefs, ", ")))
		}

		// Upstream tools (produce what this consumes)
		if len(td.ConsumesRefs) > 0 {
			upstream := findToolsByEntity(td.ConsumesRefs, true, td.Tool.Name)
			if len(upstream) > 0 {
				sb.WriteString(fmt.Sprintf("**Upstream (provide input):** %s\n", strings.Join(upstream, ", ")))
			}
		}
		// Downstream tools (consume what this produces)
		if len(td.ProducesRefs) > 0 {
			downstream := findToolsByEntity(td.ProducesRefs, false, td.Tool.Name)
			if len(downstream) > 0 {
				sb.WriteString(fmt.Sprintf("**Downstream (use output):** %s\n", strings.Join(downstream, ", ")))
			}
		}
		sb.WriteString("\n")
	}

	// Use cases
	if len(td.UseCases) > 0 {
		sb.WriteString("## Use Cases\n\n")
		for _, uc := range td.UseCases {
			sb.WriteString(fmt.Sprintf("- %s\n", uc))
		}
		sb.WriteString("\n")
	}

	// Tags
	if len(td.Tags) > 0 {
		sb.WriteString(fmt.Sprintf("**Tags:** %s\n", strings.Join(td.Tags, ", ")))
	}

	return tools.TextResult(sb.String()), nil
}

func handleTopicHelp(topic string) (*mcp.CallToolResult, error) {
	workflows := map[string]string{
		"tasks": `## Task Management
1. List tasks: runmylife_tasks(domain=manage, action=list)
2. Add task: runmylife_tasks(domain=manage, action=add, title="...", priority=3)
3. Complete: runmylife_tasks(domain=manage, action=complete, task_id="...")
4. Eisenhower matrix: runmylife_tasks(domain=prioritize, action=matrix)`,

		"calendar": `## Calendar
1. View schedule: runmylife_calendar(domain=events, action=list, days=7)
2. Create event: runmylife_calendar(domain=events, action=create, summary="...", start_time="...", end_time="...")
3. Find free time: runmylife_calendar(domain=schedule, action=free)`,

		"gmail": `## Gmail
1. Search email: runmylife_gmail(domain=messages, action=search, query="...")
2. Read message: runmylife_gmail(domain=messages, action=read, message_id="...")
3. Triage unread: runmylife_gmail(domain=triage, action=unread)
4. Create draft: runmylife_gmail(domain=drafts, action=create, to="...", subject="...", body="...")`,

		"sync": `## Data Sync
1. Sync all: runmylife_sync(domain=run, action=all)
2. Sync one: runmylife_sync(domain=run, action=todoist)
3. Check status: runmylife_sync(domain=status, action=check)`,

		"adhd": `## ADHD Support
- Overwhelm detection is automatic (worker checks every 30min)
- Time blindness alerts fire 15/30min before calendar events
- Energy estimation: runmylife_wellness(domain=energy, action=estimate)
- Triage mode activates when overwhelm score > 0.7 (shows top 3 tasks only)
- Focus sessions track hyperfocus (alerts after 3+ hours in one category)
- Achievement milestones provide dopamine scaffolding`,

		"briefing": `## Daily Briefing
1. Generate briefing: runmylife_briefing(domain=generate, action=today)
2. Morning briefing runs automatically at 7 AM via worker
3. Includes: calendar, tasks, energy estimate, weather, pending replies, suggestions`,
	}

	if topic == "all" {
		md := common.NewMarkdownBuilder().Title("runmylife Quick Reference")
		for _, key := range []string{"tasks", "calendar", "gmail", "sync", "adhd", "briefing"} {
			md.Text(workflows[key])
		}
		md.Text("\n**Tip:** Use `runmylife_tool_help(tool_name=\"...\")` for detailed help on any specific tool.")
		return tools.TextResult(md.String()), nil
	}

	if w, ok := workflows[topic]; ok {
		return tools.TextResult(fmt.Sprintf("# %s Help\n\n%s", topic, w)), nil
	}

	return common.CodedErrorResultf(common.ErrInvalidParam,
		"unknown topic: %s. Valid: tasks, calendar, gmail, sync, adhd, briefing, all", topic), nil
}

// --- Usage (NEW: tool analytics from tool_usage table) ---

func handleUsage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sortBy := common.GetStringParam(req, "sort", "count")
	limit := common.GetIntParam(req, "limit", 20)
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	db, err := common.SqlDB()
	if err != nil {
		return common.CodedErrorResultf(common.ErrDBError, "database: %v", err), nil
	}

	orderClause := "invocation_count DESC"
	if sortBy == "recent" {
		orderClause = "last_used_at DESC"
	}

	rows, err := db.QueryContext(ctx,
		fmt.Sprintf("SELECT tool_name, invocation_count, last_used_at FROM tool_usage ORDER BY %s LIMIT ?", orderClause),
		limit,
	)
	if err != nil {
		return common.CodedErrorResultf(common.ErrDBError, "query: %v", err), nil
	}
	defer rows.Close()

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Tool Usage (sorted by %s)\n\n", sortBy))
	sb.WriteString("| Tool | Invocations | Last Used |\n|------|-------------|----------|\n")

	count := 0
	for rows.Next() {
		var name, lastUsed string
		var invocations int
		if err := rows.Scan(&name, &invocations, &lastUsed); err != nil {
			continue
		}
		// Trim timestamp to date+time
		if len(lastUsed) > 16 {
			lastUsed = lastUsed[:16]
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %s |\n", name, invocations, lastUsed))
		count++
	}

	if count == 0 {
		return tools.TextResult("No tool usage recorded yet."), nil
	}

	return tools.TextResult(sb.String()), nil
}

// --- Flow (NEW: entity flow graph visualization) ---

func handleFlow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	initCache()
	entityFilter := common.GetStringParam(req, "entity", "")

	// Build entity maps
	producers := map[string][]string{} // entity → tool names
	consumers := map[string][]string{} // entity → tool names

	for _, td := range cachedSortedTools {
		for _, e := range td.ProducesRefs {
			producers[e] = append(producers[e], td.Tool.Name)
		}
		for _, e := range td.ConsumesRefs {
			consumers[e] = append(consumers[e], td.Tool.Name)
		}
	}

	// Also include workflow entity flows
	for _, wf := range predefinedWorkflows {
		for _, step := range wf.Steps {
			for _, e := range step.Produces {
				if !containsStr(producers[e], step.Tool) {
					producers[e] = append(producers[e], step.Tool+" (workflow)")
				}
			}
			for _, e := range step.Consumes {
				if !containsStr(consumers[e], step.Tool) {
					consumers[e] = append(consumers[e], step.Tool+" (workflow)")
				}
			}
		}
	}

	// Collect all entity types
	allEntities := map[string]bool{}
	for e := range producers {
		allEntities[e] = true
	}
	for e := range consumers {
		allEntities[e] = true
	}

	sorted := make([]string, 0, len(allEntities))
	for e := range allEntities {
		sorted = append(sorted, e)
	}
	sort.Strings(sorted)

	var sb strings.Builder
	sb.WriteString("# Entity Flow Graph\n\n")

	if len(sorted) == 0 {
		sb.WriteString("No entity flow data available. Tools need ProducesRefs/ConsumesRefs metadata.\n")
		return tools.TextResult(sb.String()), nil
	}

	for _, entity := range sorted {
		if entityFilter != "" && entity != entityFilter {
			continue
		}

		sb.WriteString(fmt.Sprintf("## [%s]\n\n", entity))

		prods := producers[entity]
		cons := consumers[entity]

		if len(prods) > 0 {
			sb.WriteString(fmt.Sprintf("**Producers:** %s\n", strings.Join(prods, ", ")))
		}
		if len(cons) > 0 {
			sb.WriteString(fmt.Sprintf("**Consumers:** %s\n", strings.Join(cons, ", ")))
		}

		// Show flow arrows
		if len(prods) > 0 && len(cons) > 0 {
			sb.WriteString("\n```\n")
			for _, p := range prods {
				for _, c := range cons {
					sb.WriteString(fmt.Sprintf("%s → [%s] → %s\n", p, entity, c))
				}
			}
			sb.WriteString("```\n")
		}
		sb.WriteString("\n")
	}

	if entityFilter != "" && !allEntities[entityFilter] {
		sb.WriteString(fmt.Sprintf("Entity type %q not found. Available: %s\n", entityFilter, strings.Join(sorted, ", ")))
	}

	return tools.TextResult(sb.String()), nil
}

// --- Next (NEW: workflow recommendation based on context) ---

func handleNext(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	initCache()
	lastTool := common.GetStringParam(req, "last_tool", "")
	goal := strings.ToLower(common.GetStringParam(req, "goal", ""))

	// Auto-detect last tool from usage history
	if lastTool == "" {
		if db, err := common.SqlDB(); err == nil {
			_ = db.QueryRowContext(ctx,
				"SELECT tool_name FROM tool_usage ORDER BY last_used_at DESC LIMIT 1",
			).Scan(&lastTool)
		}
	}

	if lastTool == "" {
		return tools.TextResult("No recent tool usage found. Specify last_tool or use a tool first.\n\n**Tip:** Start with `runmylife_tool_workflow` to see predefined workflows."), nil
	}

	lastTD, lastExists := cachedToolMap[lastTool]

	// Score candidates
	type candidate struct {
		name      string
		rationale string
		score     int
	}

	var candidates []candidate

	// Goal keywords → categories
	goalCategories := map[string][]string{
		"organize":    {"tasks", "calendar", "admin"},
		"communicate": {"gmail", "discord", "messages", "contacts"},
		"track":       {"habits", "fitness", "finances", "clockify"},
		"review":      {"analytics", "briefing", "personal"},
		"wellness":    {"wellness", "fitness", "habits"},
	}

	for _, td := range cachedSortedTools {
		if td.Tool.Name == lastTool {
			continue
		}

		score := 0
		var reasons []string

		// Entity match: last tool produces what this consumes
		if lastExists {
			for _, produced := range lastTD.ProducesRefs {
				if containsStr(td.ConsumesRefs, produced) {
					score += 10
					reasons = append(reasons, fmt.Sprintf("consumes %s from %s", produced, lastTool))
				}
			}
			// Same category bonus
			if td.Category == lastTD.Category && td.Tool.Name != lastTool {
				score += 2
				reasons = append(reasons, "same category")
			}
		}

		// Goal match
		if goal != "" {
			if cats, ok := goalCategories[goal]; ok {
				for _, cat := range cats {
					if td.Category == cat {
						score += 5
						reasons = append(reasons, fmt.Sprintf("matches goal '%s'", goal))
						break
					}
				}
			}
		}

		if score > 0 {
			rationale := strings.Join(reasons, "; ")
			candidates = append(candidates, candidate{td.Tool.Name, rationale, score})
		}
	}

	// Sort by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	// Limit to top 5
	if len(candidates) > 5 {
		candidates = candidates[:5]
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Next Steps (after %s)\n\n", lastTool))

	if len(candidates) == 0 {
		sb.WriteString("No strong recommendations based on entity flow.\n\n")
		sb.WriteString("**Suggestions:**\n")
		sb.WriteString("- Use `runmylife_tool_workflow` to see predefined workflows\n")
		sb.WriteString("- Use `runmylife_tool_discover(query=\"...\")` to search by keyword\n")
		return tools.TextResult(sb.String()), nil
	}

	for i, c := range candidates {
		desc := ""
		if td, ok := cachedToolMap[c.name]; ok {
			desc = td.Tool.Description
			if len(desc) > 80 {
				desc = desc[:77] + "..."
			}
		}
		sb.WriteString(fmt.Sprintf("%d. **%s** (score: %d)\n   %s\n   *%s*\n\n", i+1, c.name, c.score, desc, c.rationale))
	}

	return tools.TextResult(sb.String()), nil
}

// --- Workflow (NEW: predefined multi-step workflows) ---

func handleWorkflow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := common.GetStringParam(req, "name", "")

	if name != "" {
		// Show specific workflow
		for _, wf := range predefinedWorkflows {
			if wf.Name == name {
				return formatWorkflowDetail(wf), nil
			}
		}
		names := make([]string, len(predefinedWorkflows))
		for i, wf := range predefinedWorkflows {
			names[i] = wf.Name
		}
		return common.CodedErrorResultf(common.ErrNotFound,
			"workflow %q not found. Available: %s", name, strings.Join(names, ", ")), nil
	}

	// List all workflows
	var sb strings.Builder
	sb.WriteString("# Predefined Workflows\n\n")
	sb.WriteString("| Workflow | Description | Steps |\n|----------|-------------|-------|\n")
	for _, wf := range predefinedWorkflows {
		sb.WriteString(fmt.Sprintf("| %s | %s | %d |\n", wf.Name, wf.Description, len(wf.Steps)))
	}
	sb.WriteString("\nUse `runmylife_tool_workflow(name=\"...\")` for step-by-step details.\n")

	return tools.TextResult(sb.String()), nil
}

func formatWorkflowDetail(wf workflowDef) *mcp.CallToolResult {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Workflow: %s\n\n", wf.Name))
	sb.WriteString(wf.Description + "\n\n")
	sb.WriteString("## Steps\n\n")

	for i, step := range wf.Steps {
		sb.WriteString(fmt.Sprintf("### Step %d: %s\n", i+1, step.Desc))
		sb.WriteString(fmt.Sprintf("**Tool:** `%s(%s)`\n", step.Tool, step.Action))
		if len(step.Consumes) > 0 {
			sb.WriteString(fmt.Sprintf("**Consumes:** %s\n", strings.Join(step.Consumes, ", ")))
		}
		if len(step.Produces) > 0 {
			sb.WriteString(fmt.Sprintf("**Produces:** %s\n", strings.Join(step.Produces, ", ")))
		}
		sb.WriteString("\n")
	}

	return tools.TextResult(sb.String())
}

// --- Helpers ---

func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func extractRequiredParams(td tools.ToolDefinition) []string {
	var params []string
	required := map[string]bool{}
	for _, r := range td.Tool.InputSchema.Required {
		required[r] = true
	}
	for name := range td.Tool.InputSchema.Properties {
		if required[name] {
			params = append(params, name)
		}
	}
	sort.Strings(params)
	return params
}

func extractParamDesc(schema interface{}) string {
	if m, ok := schema.(map[string]interface{}); ok {
		if desc, ok := m["description"].(string); ok {
			return desc
		}
	}
	return ""
}

func isRequired(td tools.ToolDefinition, paramName string) bool {
	for _, r := range td.Tool.InputSchema.Required {
		if r == paramName {
			return true
		}
	}
	return false
}

func findToolsByEntity(entities []string, findProducers bool, excludeTool string) []string {
	seen := map[string]bool{}
	var result []string
	for _, td := range cachedSortedTools {
		if td.Tool.Name == excludeTool {
			continue
		}
		refs := td.ConsumesRefs
		if findProducers {
			refs = td.ProducesRefs
		}
		for _, e := range entities {
			if containsStr(refs, e) && !seen[td.Tool.Name] {
				seen[td.Tool.Name] = true
				result = append(result, td.Tool.Name)
			}
		}
	}
	return result
}
