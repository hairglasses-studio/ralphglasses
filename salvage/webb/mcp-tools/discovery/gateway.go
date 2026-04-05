// Package discovery provides tool discovery with a unified gateway interface
package discovery

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// GatewayToolDefinition returns the unified Discovery gateway tool
func GatewayToolDefinition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Tool: mcp.NewTool("webb_discovery",
			mcp.WithDescription("Execute Discovery gateway: browse, schema, stats, registry, ecosystem, quality. Consolidates 31 discovery tools."),
			mcp.WithString("domain",
				mcp.Required(),
				mcp.Enum("browse", "schema", "stats", "registry", "ecosystem", "quality"),
				mcp.Description("Discovery domain: browse (discover/tree/related/alias), schema (schema/help/deprecated), stats (stats/score/trends), registry (list/add/remove/search/sync/export/update/scaffold/versions), ecosystem (discover/install/catalog/installed/compliance/status/capabilities), quality (best_practices/audit/compliance_report)"),
			),
			mcp.WithString("action",
				mcp.Required(),
				mcp.Description("Action within domain. Browse: discover,tree,related,alias. Schema: schema,help,deprecated. Stats: stats,score,score_all,score_compare,score_trends. Registry: list,add,remove,search,sync,export,update,scaffold,versions. Ecosystem: discover,install,catalog,installed,compliance,status,capabilities. Quality: best_practices,audit,compliance_report"),
			),
			// Browse params
			mcp.WithString("detail_level",
				mcp.Description("Detail level: names, signatures, descriptions, full (default: descriptions)"),
			),
			mcp.WithString("category",
				mcp.Description("Filter by category"),
			),
			mcp.WithString("search",
				mcp.Description("Search by keyword"),
			),
			mcp.WithString("format",
				mcp.Description("Output format: markdown, compact, json"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum results (default: 50)"),
			),
			mcp.WithNumber("offset",
				mcp.Description("Offset for pagination"),
			),
			mcp.WithBoolean("show_tools",
				mcp.Description("Show individual tools in tree view"),
			),
			// Schema params
			mcp.WithString("tool_names",
				mcp.Description("Comma-separated tool names for schema lookup"),
			),
			mcp.WithString("tool_name",
				mcp.Description("Single tool name for help/related"),
			),
			mcp.WithBoolean("required_only",
				mcp.Description("Show only required parameters"),
			),
			// Alias params
			mcp.WithString("alias_action",
				mcp.Description("Alias action: list, add, remove, resolve"),
			),
			mcp.WithString("alias",
				mcp.Description("Alias name for alias operations"),
			),
			// Registry params
			mcp.WithString("server_name",
				mcp.Description("MCP server name"),
			),
			mcp.WithString("server_url",
				mcp.Description("MCP server URL or GitHub repo"),
			),
			mcp.WithString("status",
				mcp.Description("Filter by status"),
			),
			mcp.WithString("priority",
				mcp.Description("Filter by priority"),
			),
			// Ecosystem params
			mcp.WithString("query",
				mcp.Description("Search query for MCP discovery"),
			),
			mcp.WithString("package",
				mcp.Description("Package name for MCP install"),
			),
			mcp.WithString("source",
				mcp.Description("Source: npm, github, pypi"),
			),
			mcp.WithBoolean("add_to_config",
				mcp.Description("Add to Claude Desktop config"),
			),
			// Quality params
			mcp.WithBoolean("fix",
				mcp.Description("Auto-fix issues if possible"),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:          "Discovery Gateway",
				ReadOnlyHint:   mcp.ToBoolPtr(false),
				IdempotentHint: mcp.ToBoolPtr(false),
				OpenWorldHint:  mcp.ToBoolPtr(true),
			}),
		),
		Handler:     handleDiscoveryGateway,
		Category:    "discovery",
		Subcategory: "gateway",
		Tags:        []string{"discovery", "tools", "mcp", "registry", "gateway", "consolidated"},
		UseCases:    []string{"tool discovery", "schema lookup", "MCP management", "quality assessment"},
		Complexity:  tools.ComplexityModerate,
	}
}

// handleDiscoveryGateway is the unified gateway handler for all discovery operations
func handleDiscoveryGateway(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	domain, err := req.RequireString("domain")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("domain parameter is required")), nil
	}

	action, err := req.RequireString("action")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("action parameter is required")), nil
	}

	switch domain {
	case "browse":
		return handleDiscoveryBrowseDomain(ctx, req, action)
	case "schema":
		return handleDiscoverySchemaDomain(ctx, req, action)
	case "stats":
		return handleDiscoveryStatsDomain(ctx, req, action)
	case "registry":
		return handleDiscoveryRegistryDomain(ctx, req, action)
	case "ecosystem":
		return handleDiscoveryEcosystemDomain(ctx, req, action)
	case "quality":
		return handleDiscoveryQualityDomain(ctx, req, action)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid domain: %s (valid: browse, schema, stats, registry, ecosystem, quality)", domain)), nil
	}
}

func handleDiscoveryBrowseDomain(ctx context.Context, req mcp.CallToolRequest, action string) (*mcp.CallToolResult, error) {
	switch action {
	case "discover":
		return handleToolDiscover(ctx, req)
	case "tree":
		return handleToolTree(ctx, req)
	case "related":
		return handleToolRelated(ctx, req)
	case "alias":
		return handleToolAlias(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid browse action: %s (valid: discover, tree, related, alias)", action)), nil
	}
}

func handleDiscoverySchemaDomain(ctx context.Context, req mcp.CallToolRequest, action string) (*mcp.CallToolResult, error) {
	switch action {
	case "schema":
		return handleToolSchema(ctx, req)
	case "help":
		return handleToolHelp(ctx, req)
	case "deprecated":
		return handleToolDeprecated(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid schema action: %s (valid: schema, help, deprecated)", action)), nil
	}
}

func handleDiscoveryStatsDomain(ctx context.Context, req mcp.CallToolRequest, action string) (*mcp.CallToolResult, error) {
	switch action {
	case "stats":
		return handleToolStats(ctx, req)
	case "score":
		return handleToolScore(ctx, req)
	case "score_all":
		return handleToolScoreAll(ctx, req)
	case "score_compare":
		return handleToolScoreCompare(ctx, req)
	case "score_trends":
		return handleToolScoreTrends(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid stats action: %s (valid: stats, score, score_all, score_compare, score_trends)", action)), nil
	}
}

func handleDiscoveryRegistryDomain(ctx context.Context, req mcp.CallToolRequest, action string) (*mcp.CallToolResult, error) {
	switch action {
	case "list":
		return handleMCPRegistryList(ctx, req)
	case "add":
		return handleMCPRegistryAdd(ctx, req)
	case "remove":
		return handleMCPRegistryRemove(ctx, req)
	case "search":
		return handleMCPRegistrySearch(ctx, req)
	case "sync":
		return handleMCPRegistrySync(ctx, req)
	case "export":
		return handleMCPRegistryExport(ctx, req)
	case "update":
		return handleMCPRegistryUpdate(ctx, req)
	case "scaffold":
		return handleMCPRegistryScaffold(ctx, req)
	case "versions":
		return handleMCPServerVersions(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid registry action: %s (valid: list, add, remove, search, sync, export, update, scaffold, versions)", action)), nil
	}
}

func handleDiscoveryEcosystemDomain(ctx context.Context, req mcp.CallToolRequest, action string) (*mcp.CallToolResult, error) {
	switch action {
	case "discover":
		return handleMCPDiscover(ctx, req)
	case "install":
		return handleMCPInstall(ctx, req)
	case "catalog":
		return handleMCPCatalog(ctx, req)
	case "installed":
		return handleMCPInstalled(ctx, req)
	case "compliance":
		return handleMCPSpecCompliance(ctx, req)
	case "status":
		return handleMCPRegistryStatus(ctx, req)
	case "capabilities":
		return handleMCPCapabilities(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid ecosystem action: %s (valid: discover, install, catalog, installed, compliance, status, capabilities)", action)), nil
	}
}

func handleDiscoveryQualityDomain(ctx context.Context, req mcp.CallToolRequest, action string) (*mcp.CallToolResult, error) {
	switch action {
	case "best_practices":
		return handleBestPractices(ctx, req)
	case "audit":
		return handleToolAudit(ctx, req)
	case "compliance_report":
		return handleComplianceReport(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid quality action: %s (valid: best_practices, audit, compliance_report)", action)), nil
	}
}
