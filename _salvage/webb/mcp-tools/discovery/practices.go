package discovery

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
	"github.com/hairglasses-studio/webb/internal/mcp/tools/common"
)

// BestPracticesTools returns the best practices discovery tools.
func BestPracticesTools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("webb_mcp_best_practices",
				mcp.WithDescription("Search Claude/MCP best practices for tool development. Returns guidelines for time handling, parameters, descriptions, errors, and schemas."),
				mcp.WithString("topic",
					mcp.Description("Topic to search: 'time', 'parameters', 'description', 'schema', 'all' (default: all)"),
				),
			),
			Handler:     handleBestPractices,
			Category:    "discovery",
			Subcategory: "practices",
			Tags:        []string{"discovery", "best-practices", "mcp", "claude", "development"},
			UseCases:    []string{"Learn MCP tool development patterns", "Understand time handling best practices", "Improve tool descriptions"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_tool_audit",
				mcp.WithDescription("Audit MCP tool definitions for best practices compliance. Checks time handling, descriptions, schemas, and parameter patterns."),
				mcp.WithString("tool_name",
					mcp.Required(),
					mcp.Description("Tool name to audit (e.g., 'webb_k8s_pods')"),
				),
				mcp.WithBoolean("fix_suggestions",
					mcp.Description("Include suggested fixes for each issue (default: true)"),
				),
			),
			Handler:     handleToolAudit,
			Category:    "discovery",
			Subcategory: "practices",
			Tags:        []string{"discovery", "audit", "best-practices", "compliance"},
			UseCases:    []string{"Check tool compliance", "Find improvement opportunities", "Validate tool definitions"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_tool_compliance_report",
				mcp.WithDescription("Generate best practices compliance report across all tools or a specific category. Shows overall score and top issues to fix."),
				mcp.WithString("category",
					mcp.Description("Filter to specific category (optional)"),
				),
				mcp.WithString("format",
					mcp.Description("Output format: 'summary', 'detailed', 'json' (default: summary)"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum tools to include in detailed output (default: 50)"),
				),
			),
			Handler:     handleComplianceReport,
			Category:    "discovery",
			Subcategory: "practices",
			Tags:        []string{"discovery", "compliance", "report", "audit"},
			UseCases:    []string{"Assess overall tool quality", "Find top issues to fix", "Track compliance over time"},
			Complexity:  tools.ComplexitySimple,
		},
	}
}

// handleBestPractices returns documentation on best practices for a topic.
func handleBestPractices(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	topic := req.GetString("topic", "all")

	var sb strings.Builder
	sb.WriteString(common.FormatBestPracticesAsMarkdown(topic))

	// Add topic-specific detailed docs
	docSection := common.GetDocSection(topic)
	if docSection != "" {
		sb.WriteString("\n---\n\n")
		sb.WriteString(docSection)
	}

	// Add available topics footer
	if topic == "all" {
		sb.WriteString("\n---\n\n")
		sb.WriteString("## Available Topics\n")
		for _, t := range common.GetBestPracticesTopics() {
			sb.WriteString(fmt.Sprintf("- `%s`\n", t))
		}
		sb.WriteString("\nUse `webb_mcp_best_practices(topic=\"<topic>\")` for detailed guidance.\n")
	}

	return tools.TextResult(sb.String()), nil
}

// handleToolAudit audits a single tool for best practices compliance.
func handleToolAudit(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	toolName := req.GetString("tool_name", "")
	if toolName == "" {
		return tools.ErrorResult(fmt.Errorf("tool_name is required")), nil
	}
	showFixes := req.GetBool("fix_suggestions", true)

	// Find the tool
	allTools := getAllToolsMap()
	td, ok := allTools[toolName]
	if !ok {
		// Try with webb_ prefix
		td, ok = allTools["webb_"+toolName]
		if !ok {
			return tools.ErrorResult(fmt.Errorf("tool not found: %s", toolName)), nil
		}
		toolName = "webb_" + toolName
	}

	// Convert to ToolInfo for auditing
	toolInfo := common.ToToolInfo(td.Tool, td.Category, td.Tags, td.UseCases)

	// Run audit
	result := common.AuditTool(toolInfo)

	// Format output
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Audit: %s\n\n", toolName))
	sb.WriteString(fmt.Sprintf("**Score:** %d/100", result.Score))
	if result.Score >= 80 {
		sb.WriteString(" (Good)\n\n")
	} else if result.Score >= 60 {
		sb.WriteString(" (Fair)\n\n")
	} else {
		sb.WriteString(" (Needs Improvement)\n\n")
	}

	// Show passed checks
	if len(result.Passed) > 0 {
		sb.WriteString(fmt.Sprintf("### Passed (%d)\n", len(result.Passed)))
		for _, id := range result.Passed {
			bp := findPractice(id)
			if bp != nil {
				sb.WriteString(fmt.Sprintf("✓ %s\n", bp.Name))
			}
		}
		sb.WriteString("\n")
	}

	// Show violations
	if len(result.Violations) > 0 {
		sb.WriteString(fmt.Sprintf("### Issues (%d)\n", len(result.Violations)))
		for _, v := range result.Violations {
			sb.WriteString(fmt.Sprintf("⚠ **%s**: %s\n", v.PracticeID, v.Message))
			if showFixes && v.Suggestion != "" {
				sb.WriteString(fmt.Sprintf("  → Fix: %s\n", v.Suggestion))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("### Issues (0)\n")
		sb.WriteString("No issues found! This tool follows all best practices.\n")
	}

	return tools.TextResult(sb.String()), nil
}

// handleComplianceReport generates a compliance report across tools.
func handleComplianceReport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	category := req.GetString("category", "")
	format := req.GetString("format", "summary")
	limit := req.GetInt("limit", 50)

	// Get all tools
	allTools := getAllToolsList()

	// Filter by category if specified
	var filtered []tools.ToolDefinition
	for _, td := range allTools {
		if category == "" || td.Category == category {
			filtered = append(filtered, td)
		}
	}

	if len(filtered) == 0 {
		return tools.ErrorResult(fmt.Errorf("no tools found for category: %s", category)), nil
	}

	// Convert to ToolInfo for auditing
	toolInfos := make([]common.ToolInfo, len(filtered))
	for i, td := range filtered {
		toolInfos[i] = common.ToToolInfo(td.Tool, td.Category, td.Tags, td.UseCases)
	}

	// Run audit
	result := common.AuditTools(toolInfos)
	if category != "" {
		result.ModuleName = category
	} else {
		result.ModuleName = "all"
	}

	// Format output
	var sb strings.Builder

	switch format {
	case "detailed":
		sb.WriteString(formatDetailedReport(result, limit))
	case "json":
		sb.WriteString(formatJSONReport(result))
	default:
		sb.WriteString(formatSummaryReport(result, filtered))
	}

	return tools.TextResult(sb.String()), nil
}

func formatSummaryReport(result common.ModuleAuditResult, filtered []tools.ToolDefinition) string {
	var sb strings.Builder
	sb.WriteString("# MCP Tools Compliance Report\n\n")
	sb.WriteString(fmt.Sprintf("**Overall Score:** %d/100\n\n", result.AverageScore))

	// Category breakdown
	categoryScores := make(map[string][]int)
	for _, tr := range result.ToolResults {
		// Find category for this tool
		for _, td := range filtered {
			if td.Tool.Name == tr.ToolName {
				categoryScores[td.Category] = append(categoryScores[td.Category], tr.Score)
				break
			}
		}
	}

	sb.WriteString("## By Category\n")
	sb.WriteString("| Category | Tools | Avg Score | Top Issue |\n")
	sb.WriteString("|----------|-------|-----------|------------|\n")

	for cat, scores := range categoryScores {
		avg := 0
		for _, s := range scores {
			avg += s
		}
		avg /= len(scores)
		topIssue := "None"
		if len(result.TopIssues) > 0 {
			topIssue = result.TopIssues[0].Name
		}
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %s |\n", cat, len(scores), avg, topIssue))
	}
	sb.WriteString("\n")

	// Top issues with examples and fix suggestions
	if len(result.TopIssues) > 0 {
		sb.WriteString("## Top Issues to Fix\n")
		// Sort by count descending
		sort.Slice(result.TopIssues, func(i, j int) bool {
			return result.TopIssues[i].Count > result.TopIssues[j].Count
		})
		for i, issue := range result.TopIssues {
			if i >= 8 {
				break
			}
			// Show issue with count and description
			sb.WriteString(fmt.Sprintf("%d. **%s** (%d tools) - %s\n", i+1, issue.PracticeID, issue.Count, issue.Name))
			// Show example tools
			if len(issue.ExampleTools) > 0 {
				examples := strings.Join(issue.ExampleTools, "`, `")
				sb.WriteString(fmt.Sprintf("   - Examples: `%s`\n", examples))
			}
		}
		sb.WriteString("\n")
	}

	// Quick wins - tools that would benefit most from small fixes
	sb.WriteString("## Quick Wins (Score 70-89, <=2 issues)\n")
	type quickWin struct {
		name       string
		score      int
		issues     []string
	}
	var quickWinList []quickWin
	for _, tr := range result.ToolResults {
		if tr.Score >= 70 && tr.Score < 90 && len(tr.Violations) <= 2 {
			issues := make([]string, len(tr.Violations))
			for j, v := range tr.Violations {
				issues[j] = v.PracticeID
			}
			quickWinList = append(quickWinList, quickWin{
				name:   tr.ToolName,
				score:  tr.Score,
				issues: issues,
			})
		}
	}
	// Sort by score ascending (most impactful first)
	sort.Slice(quickWinList, func(i, j int) bool {
		return quickWinList[i].score < quickWinList[j].score
	})
	for i, qw := range quickWinList {
		if i >= 10 {
			sb.WriteString(fmt.Sprintf("- ... and %d more\n", len(quickWinList)-10))
			break
		}
		issuesStr := strings.Join(qw.issues, ", ")
		sb.WriteString(fmt.Sprintf("- `%s` (score: %d, %d issues: %s)\n", qw.name, qw.score, len(qw.issues), issuesStr))
	}
	if len(quickWinList) == 0 {
		sb.WriteString("- No quick wins identified at this time\n")
	}

	return sb.String()
}

func formatDetailedReport(result common.ModuleAuditResult, limit int) string {
	var sb strings.Builder
	sb.WriteString("# Detailed Compliance Report\n\n")
	sb.WriteString(fmt.Sprintf("**Tools Audited:** %d | **Average Score:** %d/100\n\n", result.TotalTools, result.AverageScore))

	// Sort by score ascending (worst first)
	sort.Slice(result.ToolResults, func(i, j int) bool {
		return result.ToolResults[i].Score < result.ToolResults[j].Score
	})

	sb.WriteString("## Tools by Score (Lowest First)\n\n")
	for i, tr := range result.ToolResults {
		if i >= limit {
			sb.WriteString(fmt.Sprintf("\n*... and %d more tools*\n", len(result.ToolResults)-limit))
			break
		}

		sb.WriteString(fmt.Sprintf("### %s (Score: %d)\n", tr.ToolName, tr.Score))
		if len(tr.Violations) == 0 {
			sb.WriteString("✓ No issues\n\n")
		} else {
			for _, v := range tr.Violations {
				sb.WriteString(fmt.Sprintf("- ⚠ %s: %s\n", v.PracticeID, v.Message))
			}
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func formatJSONReport(result common.ModuleAuditResult) string {
	// Simple JSON-like output
	var sb strings.Builder
	sb.WriteString("{\n")
	sb.WriteString(fmt.Sprintf("  \"total_tools\": %d,\n", result.TotalTools))
	sb.WriteString(fmt.Sprintf("  \"average_score\": %d,\n", result.AverageScore))
	sb.WriteString("  \"top_issues\": [\n")
	for i, issue := range result.TopIssues {
		comma := ","
		if i == len(result.TopIssues)-1 {
			comma = ""
		}
		sb.WriteString(fmt.Sprintf("    {\"id\": \"%s\", \"name\": \"%s\", \"count\": %d}%s\n",
			issue.PracticeID, issue.Name, issue.Count, comma))
	}
	sb.WriteString("  ]\n")
	sb.WriteString("}\n")
	return sb.String()
}

// findPractice finds a best practice by ID.
func findPractice(id string) *common.BestPractice {
	for _, bp := range common.BestPractices {
		if bp.ID == id {
			return &bp
		}
	}
	return nil
}

// getAllToolsMap returns all tools indexed by name (cached).
func getAllToolsMap() map[string]tools.ToolDefinition {
	initCache()
	return cachedToolMap
}

// getAllToolsList returns all tools as a list (cached).
func getAllToolsList() []tools.ToolDefinition {
	initCache()
	return cachedToolList
}
