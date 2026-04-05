package discovery

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/clients"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
	"github.com/hairglasses-studio/webb/internal/mcp/tools/common"
)

// getServerTrackingCatalog returns a shared MCP server tracking catalog
func getServerTrackingCatalog() (*clients.ServerTrackingCatalog, error) {
	return clients.NewServerTrackingCatalog("")
}

// handleMCPRegistryList lists tracked MCP servers
func handleMCPRegistryList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	catalog, err := getServerTrackingCatalog()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load catalog: %w", err)), nil
	}

	status := req.GetString("status", "")
	priority := req.GetString("priority", "")
	category := req.GetString("category", "")
	minRelevance := req.GetInt("min_relevance", 0)
	limit := req.GetInt("limit", 50)
	format := req.GetString("format", "full")

	servers := catalog.List(clients.ServerTrackingListOptions{
		Status:       status,
		Priority:     priority,
		Category:     category,
		MinRelevance: minRelevance,
		Limit:        limit,
	})

	stats := catalog.Stats()

	var sb strings.Builder
	sb.WriteString("# MCP Server Tracking Catalog\n\n")
	sb.WriteString(fmt.Sprintf("**Total Servers:** %d | **Showing:** %d\n", stats.TotalServers, len(servers)))
	sb.WriteString(fmt.Sprintf("**High Priority Pending:** %d | **Avg Relevance:** %.1f\n\n", stats.HighPriorityPending, stats.AvgRelevance))

	// Active filters
	filters := []string{}
	if status != "" {
		filters = append(filters, fmt.Sprintf("status=%s", status))
	}
	if priority != "" {
		filters = append(filters, fmt.Sprintf("priority=%s", priority))
	}
	if category != "" {
		filters = append(filters, fmt.Sprintf("category=%s", category))
	}
	if minRelevance > 0 {
		filters = append(filters, fmt.Sprintf("min_relevance=%d", minRelevance))
	}
	if len(filters) > 0 {
		sb.WriteString(fmt.Sprintf("**Filters:** %s\n\n", strings.Join(filters, ", ")))
	}

	if len(servers) == 0 {
		sb.WriteString("*No servers match the specified filters.*\n\n")
		sb.WriteString("Use `webb_mcp_registry_add` to add servers to track.\n")
		return tools.TextResult(sb.String()), nil
	}

	switch format {
	case "compact":
		sb.WriteString("| Server | Cat | Rel | Pri | Status |\n")
		sb.WriteString("|--------|-----|-----|-----|--------|\n")
		for _, s := range servers {
			sb.WriteString(fmt.Sprintf("| %s | %s | %d | %s | %s |\n",
				common.Truncate(s.Name, 30), common.Truncate(s.Category, 10), s.RelevanceScore, s.Priority, s.Status))
		}

	case "stats":
		sb.WriteString("## Status Breakdown\n\n")
		for status, count := range stats.ByStatus {
			sb.WriteString(fmt.Sprintf("- **%s:** %d\n", status, count))
		}
		sb.WriteString("\n## Priority Breakdown\n\n")
		for priority, count := range stats.ByPriority {
			sb.WriteString(fmt.Sprintf("- **%s:** %d\n", priority, count))
		}
		sb.WriteString("\n## Category Breakdown\n\n")
		for category, count := range stats.ByCategory {
			sb.WriteString(fmt.Sprintf("- **%s:** %d\n", category, count))
		}

	default: // full
		// Group by priority for better visibility
		highPri := []*clients.TrackedMCPServer{}
		medPri := []*clients.TrackedMCPServer{}
		lowPri := []*clients.TrackedMCPServer{}

		for _, s := range servers {
			switch s.Priority {
			case clients.TrackingPriorityHigh:
				highPri = append(highPri, s)
			case clients.TrackingPriorityMedium:
				medPri = append(medPri, s)
			default:
				lowPri = append(lowPri, s)
			}
		}

		if len(highPri) > 0 {
			sb.WriteString("## High Priority\n\n")
			for _, s := range highPri {
				sb.WriteString(formatServerEntry(s))
			}
		}

		if len(medPri) > 0 {
			sb.WriteString("## Medium Priority\n\n")
			for _, s := range medPri {
				sb.WriteString(formatServerEntry(s))
			}
		}

		if len(lowPri) > 0 {
			sb.WriteString("## Low Priority\n\n")
			for _, s := range lowPri {
				sb.WriteString(formatServerEntry(s))
			}
		}
	}

	return tools.TextResult(sb.String()), nil
}

// handleMCPRegistryAdd adds a server to the catalog
func handleMCPRegistryAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	catalog, err := getServerTrackingCatalog()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load catalog: %w", err)), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("name parameter is required")), nil
	}

	description := req.GetString("description", "")
	repoURL := req.GetString("repo_url", "")
	category := req.GetString("category", "general")
	notes := req.GetString("notes", "")
	relevance := req.GetInt("relevance", 0) // 0 means auto-calculate

	entry := &clients.TrackedMCPServer{
		Name:           name,
		Description:    description,
		RepoURL:        repoURL,
		Category:       category,
		Notes:          notes,
		RelevanceScore: relevance,
	}

	// Auto-suggest integration approach and target module
	entry.IntegrationApproach = catalog.SuggestIntegrationApproach(entry)
	entry.TargetModule = catalog.SuggestTargetModule(entry)

	if err := catalog.Add(entry); err != nil {
		return tools.ErrorResult(err), nil
	}

	// Reload to get auto-calculated values
	entry, _ = catalog.Get(name)

	var sb strings.Builder
	sb.WriteString("# Server Added to Catalog\n\n")
	sb.WriteString(formatServerEntry(entry))
	sb.WriteString("\n## Auto-Suggestions\n\n")
	sb.WriteString(fmt.Sprintf("- **Relevance Score:** %d (auto-calculated)\n", entry.RelevanceScore))
	sb.WriteString(fmt.Sprintf("- **Priority:** %s\n", entry.Priority))
	sb.WriteString(fmt.Sprintf("- **Integration Approach:** %s\n", entry.IntegrationApproach))
	sb.WriteString(fmt.Sprintf("- **Target Module:** %s\n", entry.TargetModule))

	return tools.TextResult(sb.String()), nil
}

// handleMCPRegistryRemove removes a server from the catalog
func handleMCPRegistryRemove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	catalog, err := getServerTrackingCatalog()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load catalog: %w", err)), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("name parameter is required")), nil
	}

	// Get entry before removing for confirmation
	entry, exists := catalog.Get(name)
	if !exists {
		return tools.ErrorResult(fmt.Errorf("server %s not found in catalog", name)), nil
	}

	if err := catalog.Remove(name); err != nil {
		return tools.ErrorResult(err), nil
	}

	var sb strings.Builder
	sb.WriteString("# Server Removed from Catalog\n\n")
	sb.WriteString(fmt.Sprintf("**Name:** %s\n", entry.Name))
	sb.WriteString(fmt.Sprintf("**Category:** %s\n", entry.Category))
	sb.WriteString(fmt.Sprintf("**Status:** %s -> removed\n", entry.Status))
	sb.WriteString(fmt.Sprintf("**Added:** %s\n", entry.AddedAt.Format("2006-01-02")))

	return tools.TextResult(sb.String()), nil
}

// handleMCPRegistrySearch searches servers in the catalog
func handleMCPRegistrySearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	catalog, err := getServerTrackingCatalog()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load catalog: %w", err)), nil
	}

	query, err := req.RequireString("query")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("query parameter is required")), nil
	}

	servers := catalog.Search(query)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Search Results: \"%s\"\n\n", query))
	sb.WriteString(fmt.Sprintf("**Found:** %d servers\n\n", len(servers)))

	if len(servers) == 0 {
		sb.WriteString("*No servers match your search.*\n\n")
		sb.WriteString("Tips:\n")
		sb.WriteString("- Try broader search terms\n")
		sb.WriteString("- Search by category: kubernetes, database, monitoring\n")
		sb.WriteString("- Use `webb_mcp_registry_list` to see all servers\n")
		return tools.TextResult(sb.String()), nil
	}

	for _, s := range servers {
		sb.WriteString(formatServerEntry(s))
	}

	return tools.TextResult(sb.String()), nil
}

// handleMCPServerVersions checks and updates server version info
func handleMCPServerVersions(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	catalog, err := getServerTrackingCatalog()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load catalog: %w", err)), nil
	}

	name := req.GetString("name", "") // Optional: specific server
	checkAll := req.GetBool("check_all", false)

	var servers []*clients.TrackedMCPServer
	if name != "" {
		if s, exists := catalog.Get(name); exists {
			servers = []*clients.TrackedMCPServer{s}
		} else {
			return tools.ErrorResult(fmt.Errorf("server %s not found", name)), nil
		}
	} else if checkAll {
		servers = catalog.List(clients.ServerTrackingListOptions{})
	} else {
		// Default: high-priority servers only
		servers = catalog.GetHighPriorityPending()
	}

	var sb strings.Builder
	sb.WriteString("# MCP Server Version Check\n\n")
	sb.WriteString(fmt.Sprintf("**Checking:** %d servers\n\n", len(servers)))

	if len(servers) == 0 {
		sb.WriteString("*No servers to check.*\n")
		return tools.TextResult(sb.String()), nil
	}

	sb.WriteString("| Server | Category | Current Version | Last Checked | Stars |\n")
	sb.WriteString("|--------|----------|-----------------|--------------|-------|\n")

	for _, s := range servers {
		version := s.Version
		if version == "" {
			version = "unknown"
		}
		lastChecked := "never"
		if s.LastChecked != nil {
			lastChecked = s.LastChecked.Format("2006-01-02")
		}
		stars := "-"
		if s.Stars > 0 {
			stars = fmt.Sprintf("%d", s.Stars)
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n",
			common.Truncate(s.Name, 25), s.Category, version, lastChecked, stars))
	}

	sb.WriteString("\n*Note: Version data from last check. Versions may have been updated since.*\n")
	sb.WriteString("\nTo update version info, use `webb_mcp_ecosystem_updates` to scan ecosystem.\n")

	return tools.TextResult(sb.String()), nil
}

// formatServerEntry formats a single server entry for display
func formatServerEntry(s *clients.TrackedMCPServer) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("### %s\n\n", s.Name))
	sb.WriteString(fmt.Sprintf("- **Category:** %s\n", s.Category))
	sb.WriteString(fmt.Sprintf("- **Status:** %s | **Priority:** %s | **Relevance:** %d\n", s.Status, s.Priority, s.RelevanceScore))

	if s.Description != "" {
		sb.WriteString(fmt.Sprintf("- **Description:** %s\n", s.Description))
	}
	if s.RepoURL != "" {
		sb.WriteString(fmt.Sprintf("- **Repo:** %s\n", s.RepoURL))
	}
	if s.IntegrationApproach != "" {
		sb.WriteString(fmt.Sprintf("- **Integration:** %s -> %s\n", s.IntegrationApproach, s.TargetModule))
	}
	if s.Notes != "" {
		sb.WriteString(fmt.Sprintf("- **Notes:** %s\n", s.Notes))
	}
	sb.WriteString(fmt.Sprintf("- **Added:** %s\n", s.AddedAt.Format("2006-01-02")))
	sb.WriteString("\n")

	return sb.String()
}

// handleMCPRegistrySync syncs from the ecosystem integration catalog
func handleMCPRegistrySync(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	catalog, err := getServerTrackingCatalog()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load tracking catalog: %w", err)), nil
	}

	// Load ecosystem catalog via MCPEcosystemClient
	ecosystemClient := clients.NewMCPEcosystemClient()
	ecosystemCatalog, err := ecosystemClient.GetIntegrationCatalog()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load ecosystem catalog: %w", err)), nil
	}

	dryRun := req.GetBool("dry_run", true)
	minRelevance := req.GetInt("min_relevance", 0)
	category := req.GetString("category", "")

	// Filter entries
	entries := make([]clients.MCPIntegrationEntry, 0)
	for _, e := range ecosystemCatalog.Entries {
		if minRelevance > 0 && e.Relevance < minRelevance {
			continue
		}
		if category != "" && e.Category != category {
			continue
		}
		entries = append(entries, e)
	}

	var sb strings.Builder
	sb.WriteString("# MCP Registry Sync\n\n")
	sb.WriteString(fmt.Sprintf("**Source:** Ecosystem Catalog (%d entries)\n", len(entries)))
	sb.WriteString(fmt.Sprintf("**Mode:** %s\n\n", map[bool]string{true: "Dry Run", false: "Live"}[dryRun]))

	if len(entries) == 0 {
		sb.WriteString("*No entries in ecosystem catalog to sync.*\n\n")
		sb.WriteString("Use `webb_mcp_integration_ingest` or `webb_mcp_ecosystem_updates` to populate the ecosystem catalog first.\n")
		return tools.TextResult(sb.String()), nil
	}

	if dryRun {
		// Preview what would be imported
		wouldImport := 0
		wouldSkip := 0

		for _, e := range entries {
			if _, exists := catalog.Get(e.Name); exists {
				wouldSkip++
			} else {
				wouldImport++
			}
		}

		sb.WriteString("## Preview\n\n")
		sb.WriteString(fmt.Sprintf("- **Would import:** %d servers\n", wouldImport))
		sb.WriteString(fmt.Sprintf("- **Would skip (duplicates):** %d servers\n\n", wouldSkip))

		if wouldImport > 0 {
			sb.WriteString("### Servers to Import\n\n")
			sb.WriteString("| Server | Category | Relevance | Approach |\n")
			sb.WriteString("|--------|----------|-----------|----------|\n")
			for _, e := range entries {
				if _, exists := catalog.Get(e.Name); !exists {
					sb.WriteString(fmt.Sprintf("| %s | %s | %d | %s |\n",
						common.Truncate(e.Name, 30), e.Category, e.Relevance, e.Approach))
				}
			}
		}

		sb.WriteString("\n*Run with `dry_run=false` to execute sync.*\n")
	} else {
		// Actually import
		imported, skipped := catalog.ImportFromEcosystem(entries)

		sb.WriteString("## Results\n\n")
		sb.WriteString(fmt.Sprintf("- **Imported:** %d servers\n", imported))
		sb.WriteString(fmt.Sprintf("- **Skipped (duplicates):** %d servers\n\n", skipped))

		if imported > 0 {
			sb.WriteString("Use `webb_mcp_registry_list` to view the updated catalog.\n")
		}
	}

	return tools.TextResult(sb.String()), nil
}

// handleMCPRegistryExport exports the catalog to markdown
func handleMCPRegistryExport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	catalog, err := getServerTrackingCatalog()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load catalog: %w", err)), nil
	}

	format := req.GetString("format", "markdown")

	var sb strings.Builder

	switch format {
	case "json":
		// Export as JSON
		servers := catalog.GetAll()
		sb.WriteString("```json\n[\n")
		for i, s := range servers {
			sb.WriteString(fmt.Sprintf(`  {"name": "%s", "category": "%s", "status": "%s", "priority": "%s", "relevance": %d}`,
				s.Name, s.Category, s.Status, s.Priority, s.RelevanceScore))
			if i < len(servers)-1 {
				sb.WriteString(",")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("]\n```\n")

	case "csv":
		// Export as CSV
		sb.WriteString("```csv\n")
		sb.WriteString("name,category,status,priority,relevance,approach,module\n")
		for _, s := range catalog.GetAll() {
			sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%d,%s,%s\n",
				s.Name, s.Category, s.Status, s.Priority, s.RelevanceScore, s.IntegrationApproach, s.TargetModule))
		}
		sb.WriteString("```\n")

	default: // markdown
		sb.WriteString(catalog.ExportMarkdown())
	}

	return tools.TextResult(sb.String()), nil
}

// handleMCPRegistryScaffold generates scaffold code for a tracked server
func handleMCPRegistryScaffold(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	catalog, err := getServerTrackingCatalog()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load catalog: %w", err)), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("name parameter is required")), nil
	}

	entry, exists := catalog.Get(name)
	if !exists {
		return tools.ErrorResult(fmt.Errorf("server %s not found in catalog", name)), nil
	}

	format := req.GetString("format", "full")

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Integration Scaffold: %s\n\n", entry.Name))
	sb.WriteString(fmt.Sprintf("**Category:** %s | **Module:** %s | **Relevance:** %d\n", entry.Category, entry.TargetModule, entry.RelevanceScore))
	sb.WriteString(fmt.Sprintf("**Integration Approach:** %s\n\n", entry.IntegrationApproach))

	if entry.RepoURL != "" {
		sb.WriteString(fmt.Sprintf("**Repository:** %s\n\n", entry.RepoURL))
	}

	// Get suggested tools
	suggestions := catalog.SuggestTools(entry)

	sb.WriteString("## Suggested Tools\n\n")
	sb.WriteString("| Tool | Description | Parameters |\n")
	sb.WriteString("|------|-------------|------------|\n")
	for _, tool := range suggestions {
		params := strings.Join(tool.Parameters, ", ")
		if params == "" {
			params = "-"
		}
		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", tool.Name, tool.Description, params))
	}

	switch format {
	case "suggestions":
		// Just the suggestions table, no code
		sb.WriteString("\n*Use `format=full` to generate scaffold code.*\n")

	case "full":
		// Full scaffold code
		sb.WriteString("\n## Scaffold Code\n\n")
		sb.WriteString("```go\n")
		sb.WriteString(catalog.GenerateScaffold(entry))
		sb.WriteString("```\n")

		sb.WriteString("\n## Implementation Steps\n\n")
		sb.WriteString("1. Create client in `internal/clients/` (if not exists)\n")
		sb.WriteString(fmt.Sprintf("2. Add tools to `internal/mcp/tools/%s/module.go`\n", entry.TargetModule))
		sb.WriteString("3. Implement handlers with actual API calls\n")
		sb.WriteString("4. Add tests\n")
		sb.WriteString(fmt.Sprintf("5. Update server status: `webb_mcp_registry_update(name=\"%s\", status=\"in_progress\")`\n", entry.Name))

	case "markdown":
		// Implementation guide markdown
		sb.WriteString("\n## Implementation Guide\n\n")
		sb.WriteString(fmt.Sprintf("### Target Module: `%s`\n\n", entry.TargetModule))
		sb.WriteString(fmt.Sprintf("File: `internal/mcp/tools/%s/module.go`\n\n", entry.TargetModule))
		sb.WriteString("### Suggested Client\n\n")
		sb.WriteString(fmt.Sprintf("Create: `internal/clients/%s.go`\n\n", strings.ReplaceAll(entry.Name, "-", "_")))
		sb.WriteString("### Tools to Implement\n\n")
		for _, tool := range suggestions {
			sb.WriteString(fmt.Sprintf("- [ ] `%s` - %s\n", tool.Name, tool.Description))
		}
	}

	return tools.TextResult(sb.String()), nil
}

// handleMCPRegistryUpdate updates a server's status or metadata
func handleMCPRegistryUpdate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	catalog, err := getServerTrackingCatalog()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load catalog: %w", err)), nil
	}

	name, err := req.RequireString("name")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("name parameter is required")), nil
	}

	// Get existing entry
	entry, exists := catalog.Get(name)
	if !exists {
		return tools.ErrorResult(fmt.Errorf("server %s not found in catalog", name)), nil
	}

	// Build updates map
	updates := make(map[string]interface{})

	if status := req.GetString("status", ""); status != "" {
		updates["status"] = status
	}
	if priority := req.GetString("priority", ""); priority != "" {
		updates["priority"] = priority
	}
	if notes := req.GetString("notes", ""); notes != "" {
		updates["notes"] = notes
	}
	if approach := req.GetString("integration_approach", ""); approach != "" {
		updates["integration_approach"] = approach
	}
	if module := req.GetString("target_module", ""); module != "" {
		updates["target_module"] = module
	}
	if relevance := req.GetInt("relevance", 0); relevance > 0 {
		updates["relevance_score"] = relevance
	}

	if len(updates) == 0 {
		return tools.ErrorResult(fmt.Errorf("no updates specified")), nil
	}

	if err := catalog.Update(name, updates); err != nil {
		return tools.ErrorResult(err), nil
	}

	// Reload to get updated entry
	entry, _ = catalog.Get(name)

	var sb strings.Builder
	sb.WriteString("# Server Updated\n\n")
	sb.WriteString(formatServerEntry(entry))
	sb.WriteString("\n## Changes Applied\n\n")
	for key := range updates {
		sb.WriteString(fmt.Sprintf("- %s updated\n", key))
	}

	return tools.TextResult(sb.String()), nil
}

// MCPRegistryTools returns the MCP registry management tools
func MCPRegistryTools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("webb_mcp_registry_list",
				mcp.WithDescription("List MCP servers tracked for integration. Shows status, priority, relevance, and integration approach."),
				mcp.WithString("status",
					mcp.Description("Filter by status: planned, in_progress, completed, rejected, deferred"),
				),
				mcp.WithString("priority",
					mcp.Description("Filter by priority: high, medium, low"),
				),
				mcp.WithString("category",
					mcp.Description("Filter by category: kubernetes, database, monitoring, slack, aws, etc."),
				),
				mcp.WithNumber("min_relevance",
					mcp.Description("Minimum relevance score (0-100)"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum servers to return (default: 50)"),
				),
				mcp.WithString("format",
					mcp.Description("Output format: full (default), compact, stats"),
				),
			),
			Handler:     handleMCPRegistryList,
			Category:    "discovery",
			Subcategory: "mcp_registry",
			Tags:        []string{"mcp", "registry", "integration", "catalog"},
			UseCases:    []string{"Track MCP servers for integration", "View integration catalog", "Plan implementation sprints"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_mcp_registry_add",
				mcp.WithDescription("Create MCP server entry in the tracking catalog. Auto-calculates relevance, priority, and suggests integration approach."),
				mcp.WithString("name",
					mcp.Required(),
					mcp.Description("Server name (e.g., 'opentelemetry-mcp-server')"),
				),
				mcp.WithString("description",
					mcp.Description("Server description"),
				),
				mcp.WithString("repo_url",
					mcp.Description("GitHub repository URL"),
				),
				mcp.WithString("category",
					mcp.Description("Category: kubernetes, database, monitoring, slack, aws, ticketing, infrastructure, general"),
				),
				mcp.WithString("notes",
					mcp.Description("Implementation notes"),
				),
				mcp.WithNumber("relevance",
					mcp.Description("Override relevance score 0-100 (auto-calculated if not provided)"),
				),
			),
			Handler:     handleMCPRegistryAdd,
			Category:    "discovery",
			Subcategory: "mcp_registry",
			Tags:        []string{"mcp", "registry", "add"},
			UseCases:    []string{"Track new MCP server for integration", "Add discovered server to catalog"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		{
			Tool: mcp.NewTool("webb_mcp_registry_remove",
				mcp.WithDescription("Delete MCP server from the tracking catalog."),
				mcp.WithString("name",
					mcp.Required(),
					mcp.Description("Server name to remove"),
				),
			),
			Handler:     handleMCPRegistryRemove,
			Category:    "discovery",
			Subcategory: "mcp_registry",
			Tags:        []string{"mcp", "registry", "remove"},
			UseCases:    []string{"Remove server from tracking", "Clean up catalog"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		{
			Tool: mcp.NewTool("webb_mcp_registry_search",
				mcp.WithDescription("List MCP servers in the tracking catalog matching query by name, description, or category."),
				mcp.WithString("query",
					mcp.Required(),
					mcp.Description("Search query (searches name, description, category, notes)"),
				),
			),
			Handler:     handleMCPRegistrySearch,
			Category:    "discovery",
			Subcategory: "mcp_registry",
			Tags:        []string{"mcp", "registry", "search"},
			UseCases:    []string{"Find tracked servers", "Search integration catalog"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_mcp_server_versions",
				mcp.WithDescription("Check version info for tracked MCP servers. Shows current version, last check date, and GitHub stars."),
				mcp.WithString("name",
					mcp.Description("Specific server name to check (optional)"),
				),
				mcp.WithBoolean("check_all",
					mcp.Description("Check all servers, not just high-priority (default: false)"),
				),
			),
			Handler:     handleMCPServerVersions,
			Category:    "discovery",
			Subcategory: "mcp_registry",
			Tags:        []string{"mcp", "registry", "versions"},
			UseCases:    []string{"Check server versions", "Track ecosystem updates"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_mcp_registry_sync",
				mcp.WithDescription("Update tracking catalog by syncing MCP servers from the ecosystem integration catalog. Imports entries from webb_mcp_integration_* tools."),
				mcp.WithBoolean("dry_run",
					mcp.Description("Preview what would be imported without making changes (default: true)"),
				),
				mcp.WithNumber("min_relevance",
					mcp.Description("Only import servers with relevance >= this value (0-100)"),
				),
				mcp.WithString("category",
					mcp.Description("Filter by category to import (optional)"),
				),
			),
			Handler:     handleMCPRegistrySync,
			Category:    "discovery",
			Subcategory: "mcp_registry",
			Tags:        []string{"mcp", "registry", "sync", "import"},
			UseCases:    []string{"Sync from ecosystem scans", "Import discovered servers", "Batch import"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		{
			Tool: mcp.NewTool("webb_mcp_registry_export",
				mcp.WithDescription("Get MCP server tracking catalog export in various formats."),
				mcp.WithString("format",
					mcp.Description("Export format: markdown (default), json, csv"),
				),
			),
			Handler:     handleMCPRegistryExport,
			Category:    "discovery",
			Subcategory: "mcp_registry",
			Tags:        []string{"mcp", "registry", "export"},
			UseCases:    []string{"Export catalog for sharing", "Generate reports", "Backup catalog"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_mcp_registry_update",
				mcp.WithDescription("Update a tracked MCP server's status, priority, or metadata."),
				mcp.WithString("name",
					mcp.Required(),
					mcp.Description("Server name to update"),
				),
				mcp.WithString("status",
					mcp.Description("New status: planned, in_progress, completed, rejected, deferred"),
				),
				mcp.WithString("priority",
					mcp.Description("New priority: high, medium, low"),
				),
				mcp.WithString("notes",
					mcp.Description("Implementation notes"),
				),
				mcp.WithString("integration_approach",
					mcp.Description("Integration approach: native_port, mcp_proxy, pattern_ref"),
				),
				mcp.WithString("target_module",
					mcp.Description("Target webb module for implementation"),
				),
				mcp.WithNumber("relevance",
					mcp.Description("Override relevance score (0-100)"),
				),
			),
			Handler:     handleMCPRegistryUpdate,
			Category:    "discovery",
			Subcategory: "mcp_registry",
			Tags:        []string{"mcp", "registry", "update"},
			UseCases:    []string{"Update server status", "Change priority", "Add implementation notes"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		{
			Tool: mcp.NewTool("webb_mcp_registry_scaffold",
				mcp.WithDescription("Get scaffold code for a tracked MCP server. Suggests tools based on category and generates boilerplate handler code."),
				mcp.WithString("name",
					mcp.Required(),
					mcp.Description("Server name to generate scaffold for"),
				),
				mcp.WithString("format",
					mcp.Description("Output format: full (default with code), suggestions (tools only), markdown (implementation guide)"),
				),
			),
			Handler:     handleMCPRegistryScaffold,
			Category:    "discovery",
			Subcategory: "mcp_registry",
			Tags:        []string{"mcp", "registry", "scaffold", "codegen"},
			UseCases:    []string{"Generate integration code", "Plan implementation", "Scaffold tools from registry"},
			Complexity:  tools.ComplexitySimple,
		},
	}
}
