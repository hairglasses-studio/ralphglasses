// Package discovery provides tool discovery and scoring MCP tools.
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/clients"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// toolProviderAdapter adapts the tools.ToolRegistry to the clients.ToolProvider interface.
type toolProviderAdapter struct {
	registry *tools.ToolRegistry
}

// GetTool implements clients.ToolProvider.
func (a *toolProviderAdapter) GetTool(name string) *clients.ToolInfo {
	tool, ok := a.registry.GetTool(name)
	if !ok {
		return nil
	}
	return toolDefToInfo(&tool)
}

// ListAllTools implements clients.ToolProvider.
func (a *toolProviderAdapter) ListAllTools() []clients.ToolInfo {
	allDefs := a.registry.GetAllToolDefinitions()
	result := make([]clients.ToolInfo, 0, len(allDefs))
	for _, def := range allDefs {
		result = append(result, *toolDefToInfo(&def))
	}
	return result
}

// toolDefToInfo converts a ToolDefinition to a ToolInfo.
func toolDefToInfo(def *tools.ToolDefinition) *clients.ToolInfo {
	info := &clients.ToolInfo{
		Name:        def.Tool.Name,
		Description: def.Tool.Description,
		Category:    def.Category,
		Subcategory: def.Subcategory,
		Properties:  make(map[string]clients.PropertyInfo),
		Required:    def.Tool.InputSchema.Required,
	}

	// Extract properties from the tool schema
	if def.Tool.InputSchema.Properties != nil {
		for name, prop := range def.Tool.InputSchema.Properties {
			propMap, ok := prop.(map[string]interface{})
			if !ok {
				continue
			}
			propInfo := clients.PropertyInfo{}
			if t, ok := propMap["type"].(string); ok {
				propInfo.Type = t
			}
			if d, ok := propMap["description"].(string); ok {
				propInfo.Description = d
			}
			if def, ok := propMap["default"]; ok {
				propInfo.Default = def
			}
			info.Properties[name] = propInfo
		}
	}

	return info
}

// getToolProvider returns a tool provider adapter.
func getToolProvider() clients.ToolProvider {
	return &toolProviderAdapter{registry: tools.GetRegistry()}
}

// ScoringTools returns the MCP tool scoring tools.
func ScoringTools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("webb_tool_score",
				mcp.WithDescription("Evaluate an individual MCP tool on quality metrics. Returns overall score (0-100), grade (A-F), component breakdowns, and improvement suggestions."),
				mcp.WithString("tool_name",
					mcp.Required(),
					mcp.Description("Name of the tool to score (e.g., webb_k8s_pods)"),
				),
				mcp.WithBoolean("detailed",
					mcp.Description("Include detailed breakdown of all scoring components"),
				),
			),
			Handler:     handleToolScore,
			Category:    "discovery",
			Subcategory: "scoring",
			Tags:        []string{"discovery", "scoring", "quality", "audit"},
			UseCases:    []string{"Audit tool quality", "Find improvement opportunities", "Check compliance"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_tool_score_all",
				mcp.WithDescription("Evaluate all MCP tools and generate a quality report. Returns aggregate statistics, grade distribution, top issues, and lowest/highest scoring tools."),
				mcp.WithString("category",
					mcp.Description("Filter by category (e.g., kubernetes, slack, aws)"),
				),
				mcp.WithNumber("min_score",
					mcp.Description("Only show tools with score below this threshold"),
				),
				mcp.WithString("format",
					mcp.Description("Output format: summary, detailed, json (default: summary)"),
				),
				mcp.WithBoolean("include_benchmark",
					mcp.Description("Include industry benchmark comparison"),
				),
				mcp.WithString("benchmark",
					mcp.Description("Benchmark to compare against: mcp-tef, github, mcp-bench"),
				),
			),
			Handler:     handleToolScoreAll,
			Category:    "discovery",
			Subcategory: "scoring",
			Tags:        []string{"discovery", "scoring", "report", "quality"},
			UseCases:    []string{"Generate quality report", "Find lowest scoring tools", "Track quality metrics"},
			Complexity:  tools.ComplexityModerate,
		},
		{
			Tool: mcp.NewTool("webb_tool_score_compare",
				mcp.WithDescription("Compare webb tool quality against industry benchmarks. Shows where webb is above/below average on key metrics like description quality and parameter standards."),
				mcp.WithString("benchmark",
					mcp.Description("Benchmark: mcp-tef (Stacklok), github (GitHub MCP), mcp-bench (research)"),
				),
				mcp.WithString("category",
					mcp.Description("Filter to specific category for comparison"),
				),
			),
			Handler:     handleToolScoreCompare,
			Category:    "discovery",
			Subcategory: "scoring",
			Tags:        []string{"discovery", "scoring", "benchmark", "comparison"},
			UseCases:    []string{"Compare against industry standards", "Identify gaps", "Benchmark quality"},
			Complexity:  tools.ComplexityModerate,
		},
		{
			Tool: mcp.NewTool("webb_tool_score_trends",
				mcp.WithDescription("Show tool quality score trends over time. Identifies improving and declining tools correlated with commits."),
				mcp.WithNumber("days",
					mcp.Description("Number of days to analyze (default: 30)"),
				),
				mcp.WithString("category",
					mcp.Description("Filter to specific category"),
				),
			),
			Handler:     handleToolScoreTrends,
			Category:    "discovery",
			Subcategory: "scoring",
			Tags:        []string{"discovery", "scoring", "trends", "history"},
			UseCases:    []string{"Track quality over time", "Identify regressions", "Monitor improvements"},
			Complexity:  tools.ComplexitySimple,
		},
		{
			Tool: mcp.NewTool("webb_tool_lint",
				mcp.WithDescription("Validate tool definitions against quality standards. Returns issues by severity (error/warning/info) with fix suggestions. Useful for CI and pre-commit validation."),
				mcp.WithString("tool_name",
					mcp.Description("Specific tool to lint (lints all if omitted)"),
				),
				mcp.WithString("category",
					mcp.Description("Lint all tools in category (e.g., kubernetes, slack)"),
				),
				mcp.WithString("severity",
					mcp.Description("Minimum severity to report. Default: warning"),
					mcp.Enum("error", "warning", "info"),
				),
				mcp.WithString("format",
					mcp.Description("Output format. Default: summary"),
					mcp.Enum("summary", "github", "json"),
				),
				mcp.WithBoolean("fail_on_error",
					mcp.Description("Return error exit if any error-level issues found (for CI). Default: false"),
				),
			),
			Handler:     handleToolLint,
			Category:    "discovery",
			Subcategory: "quality",
			Tags:        []string{"discovery", "lint", "quality", "validation", "ci"},
			UseCases:    []string{"Validate tool descriptions", "Check parameter quality", "CI pre-commit validation", "Identify missing documentation"},
			Complexity:  tools.ComplexitySimple,
		},
	}
}

// Handler implementations

func handleToolScore(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	toolName, err := req.RequireString("tool_name")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("tool_name is required")), nil
	}

	detailed := req.GetBool("detailed", false)

	engine := clients.NewToolScoringEngineWithProvider(getToolProvider())
	score, err := engine.ScoreTool(ctx, toolName)
	if err != nil {
		return tools.ErrorResult(err), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Tool Score: %s\n\n", toolName))
	sb.WriteString(fmt.Sprintf("**Overall Score:** %.1f/100 (**%s**)\n", score.OverallScore, score.Grade))
	sb.WriteString(fmt.Sprintf("**Category:** %s\n\n", score.Category))

	sb.WriteString("## Component Scores\n\n")
	sb.WriteString("| Component | Score | Weight |\n")
	sb.WriteString("|-----------|-------|--------|\n")
	sb.WriteString(fmt.Sprintf("| MCP Compliance | %.1f | 25%% |\n", score.ComplianceScore))
	sb.WriteString(fmt.Sprintf("| Best Practices | %.1f | 25%% |\n", score.BestPracticesScore))
	sb.WriteString(fmt.Sprintf("| Description Quality | %.1f | 20%% |\n", score.DescriptionScore))
	sb.WriteString(fmt.Sprintf("| Parameter Quality | %.1f | 15%% |\n", score.ParameterScore))
	sb.WriteString(fmt.Sprintf("| Complexity/Cost | %.1f | 10%% |\n", score.ComplexityScore))
	sb.WriteString(fmt.Sprintf("| Success Rate | %.1f | 5%% |\n", score.SuccessRateScore))

	if detailed {
		sb.WriteString("\n## Description Analysis\n\n")
		sb.WriteString(fmt.Sprintf("- **Starts with verb:** %v", score.DescriptionDetails.StartsWithVerb))
		if score.DescriptionDetails.VerbUsed != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", score.DescriptionDetails.VerbUsed))
		}
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("- **Length:** %d chars (score: %.1f)\n", score.DescriptionDetails.Length, score.DescriptionDetails.LengthScore))
		sb.WriteString(fmt.Sprintf("- **Has return hint:** %v\n", score.DescriptionDetails.HasReturnHint))
		sb.WriteString(fmt.Sprintf("- **Clarity score:** %.1f\n", score.DescriptionDetails.ClarityScore))

		sb.WriteString("\n## Parameter Analysis\n\n")
		sb.WriteString(fmt.Sprintf("- **Total parameters:** %d\n", score.ParameterDetails.TotalParams))
		sb.WriteString(fmt.Sprintf("- **Required:** %d\n", score.ParameterDetails.RequiredParams))
		sb.WriteString(fmt.Sprintf("- **With defaults:** %d\n", score.ParameterDetails.ParamsWithDefaults))
		sb.WriteString(fmt.Sprintf("- **With descriptions:** %d\n", score.ParameterDetails.ParamsWithDescs))
		if len(score.ParameterDetails.StandardParams) > 0 {
			sb.WriteString(fmt.Sprintf("- **Standard params:** %s\n", strings.Join(score.ParameterDetails.StandardParams, ", ")))
		}
	}

	if len(score.Issues) > 0 {
		sb.WriteString("\n## Issues\n\n")
		for _, issue := range score.Issues {
			icon := "warning"
			if issue.Severity == "error" {
				icon = "error"
			} else if issue.Severity == "info" {
				icon = "info"
			}
			sb.WriteString(fmt.Sprintf("- [%s] **%s**: %s\n", icon, issue.Code, issue.Message))
			if issue.Suggestion != "" {
				sb.WriteString(fmt.Sprintf("  - Suggestion: %s\n", issue.Suggestion))
			}
		}
	}

	if len(score.Suggestions) > 0 {
		sb.WriteString("\n## Suggestions\n\n")
		for _, suggestion := range score.Suggestions {
			sb.WriteString(fmt.Sprintf("- %s\n", suggestion))
		}
	}

	return tools.TextResult(sb.String()), nil
}

func handleToolScoreAll(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	opts := clients.ScoreAllOptions{
		Category:         req.GetString("category", ""),
		MinScore:         req.GetFloat("min_score", 0),
		Format:           req.GetString("format", "summary"),
		IncludeBenchmark: req.GetBool("include_benchmark", false),
		BenchmarkName:    req.GetString("benchmark", "mcp-tef"),
	}

	engine := clients.NewToolScoringEngineWithProvider(getToolProvider())
	report, err := engine.ScoreAllTools(ctx, opts)
	if err != nil {
		return tools.ErrorResult(err), nil
	}

	if opts.Format == "json" {
		data, _ := json.MarshalIndent(report, "", "  ")
		return tools.TextResult(string(data)), nil
	}

	var sb strings.Builder
	sb.WriteString("# MCP Tool Quality Report\n\n")
	sb.WriteString(fmt.Sprintf("*Generated: %s*\n\n", report.GeneratedAt.Format("2006-01-02 15:04:05")))

	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Tools:** %d\n", report.TotalTools))
	sb.WriteString(fmt.Sprintf("- **Average Score:** %.1f/100\n", report.AverageScore))

	// Grade distribution
	sb.WriteString(fmt.Sprintf("- **Grade Distribution:** A(%d) B(%d) C(%d) D(%d) F(%d)\n\n",
		report.GradeDistribution["A"],
		report.GradeDistribution["B"],
		report.GradeDistribution["C"],
		report.GradeDistribution["D"],
		report.GradeDistribution["F"]))

	// By category
	sb.WriteString("## By Category\n\n")
	sb.WriteString("| Category | Tools | Avg Score | Grade |\n")
	sb.WriteString("|----------|-------|-----------|-------|\n")
	for _, stats := range report.ByCategory {
		sb.WriteString(fmt.Sprintf("| %s | %d | %.1f | %s |\n",
			stats.Category, stats.ToolCount, stats.AverageScore, stats.Grade))
	}

	// Top issues
	if len(report.TopIssues) > 0 {
		sb.WriteString("\n## Top Issues\n\n")
		for i, issue := range report.TopIssues {
			sb.WriteString(fmt.Sprintf("%d. **%s**: %s (%d tools)\n", i+1, issue.Code, issue.Message, issue.Count))
		}
	}

	// Lowest scoring
	if len(report.LowestScoring) > 0 && opts.Format == "detailed" {
		sb.WriteString("\n## Lowest Scoring Tools\n\n")
		for _, score := range report.LowestScoring {
			sb.WriteString(fmt.Sprintf("- **%s** (%.1f/%s): %s\n",
				score.ToolName, score.OverallScore, score.Grade, score.Category))
		}
	}

	// Benchmark comparison
	if report.Comparison != nil {
		sb.WriteString("\n## Industry Comparison\n\n")
		sb.WriteString(fmt.Sprintf("*Benchmark: %s*\n\n", report.Comparison.BenchmarkName))
		sb.WriteString("| Metric | Webb | Benchmark | Status |\n")
		sb.WriteString("|--------|------|-----------|--------|\n")
		for _, comp := range report.Comparison.Comparisons {
			status := "At"
			if comp.Status == "above" {
				status = "Above"
			} else if comp.Status == "below" {
				status = "Below"
			}
			sb.WriteString(fmt.Sprintf("| %s | %.2f | %.2f | %s |\n",
				comp.Metric, comp.WebbValue, comp.BenchmarkValue, status))
		}
		sb.WriteString(fmt.Sprintf("\n**Overall:** %s industry average\n", strings.ToUpper(report.Comparison.OverallStatus)))
	}

	return tools.TextResult(sb.String()), nil
}

func handleToolScoreCompare(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	benchmarkName := req.GetString("benchmark", "mcp-tef")
	category := req.GetString("category", "")

	engine := clients.NewToolScoringEngineWithProvider(getToolProvider())
	report, err := engine.ScoreAllTools(ctx, clients.ScoreAllOptions{
		Category:         category,
		IncludeBenchmark: true,
		BenchmarkName:    benchmarkName,
	})
	if err != nil {
		return tools.ErrorResult(err), nil
	}

	if report.Comparison == nil {
		return tools.ErrorResult(fmt.Errorf("no comparison data available")), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Webb vs %s Benchmark\n\n", benchmarkName))

	if category != "" {
		sb.WriteString(fmt.Sprintf("*Filtered to category: %s*\n\n", category))
	}

	sb.WriteString("## Metrics Comparison\n\n")
	sb.WriteString("| Metric | Webb | Benchmark | Diff | Status |\n")
	sb.WriteString("|--------|------|-----------|------|--------|\n")

	for _, comp := range report.Comparison.Comparisons {
		status := "At average"
		if comp.Status == "above" {
			status = "Above"
		} else if comp.Status == "below" {
			status = "Below"
		}
		diffStr := fmt.Sprintf("%+.2f", comp.Difference)
		sb.WriteString(fmt.Sprintf("| %s | %.2f | %.2f | %s | %s |\n",
			formatMetricName(comp.Metric), comp.WebbValue, comp.BenchmarkValue, diffStr, status))
	}

	sb.WriteString(fmt.Sprintf("\n## Overall Assessment\n\n"))
	switch report.Comparison.OverallStatus {
	case "above":
		sb.WriteString("**Webb tools are ABOVE industry average overall.**\n\n")
		sb.WriteString("Keep up the good work! Focus on maintaining quality as new tools are added.\n")
	case "below":
		sb.WriteString("**Webb tools are BELOW industry average on some metrics.**\n\n")
		sb.WriteString("### Improvement Recommendations:\n\n")
		for _, comp := range report.Comparison.Comparisons {
			if comp.Status == "below" {
				sb.WriteString(fmt.Sprintf("- **%s**: Currently %.2f, target %.2f\n",
					formatMetricName(comp.Metric), comp.WebbValue, comp.BenchmarkValue))
			}
		}
	default:
		sb.WriteString("**Webb tools are AT industry average.**\n\n")
		sb.WriteString("There's room for improvement to become a leader in tool quality.\n")
	}

	return tools.TextResult(sb.String()), nil
}

// ScoreSnapshot represents a point-in-time score record
type ScoreSnapshot struct {
	Date             string         `json:"date"`
	TotalTools       int            `json:"total_tools"`
	AverageScore     float64        `json:"average_score"`
	GradeDistribution map[string]int `json:"grade_distribution"`
	Category         string         `json:"category,omitempty"`
}

// ScoreHistory tracks historical score snapshots
type ScoreHistory struct {
	Snapshots []ScoreSnapshot `json:"snapshots"`
}

func getScoreHistoryPath() string {
	home := os.Getenv("HOME")
	return filepath.Join(home, ".config", "webb", "tool-score-history.json")
}

func loadScoreHistory() (*ScoreHistory, error) {
	path := getScoreHistoryPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &ScoreHistory{}, nil
		}
		return nil, err
	}
	var history ScoreHistory
	if err := json.Unmarshal(data, &history); err != nil {
		return nil, err
	}
	return &history, nil
}

func saveScoreHistory(history *ScoreHistory) error {
	path := getScoreHistoryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(history, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func handleToolScoreTrends(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	days := req.GetInt("days", 30)
	category := req.GetString("category", "")

	engine := clients.NewToolScoringEngineWithProvider(getToolProvider())
	report, err := engine.ScoreAllTools(ctx, clients.ScoreAllOptions{
		Category: category,
	})
	if err != nil {
		return tools.ErrorResult(err), nil
	}

	// Record current snapshot
	history, _ := loadScoreHistory()
	today := time.Now().Format("2006-01-02")

	// Update or add today's snapshot
	found := false
	for i, snap := range history.Snapshots {
		if snap.Date == today && snap.Category == category {
			history.Snapshots[i] = ScoreSnapshot{
				Date:             today,
				TotalTools:       report.TotalTools,
				AverageScore:     report.AverageScore,
				GradeDistribution: report.GradeDistribution,
				Category:         category,
			}
			found = true
			break
		}
	}
	if !found {
		history.Snapshots = append(history.Snapshots, ScoreSnapshot{
			Date:             today,
			TotalTools:       report.TotalTools,
			AverageScore:     report.AverageScore,
			GradeDistribution: report.GradeDistribution,
			Category:         category,
		})
	}
	_ = saveScoreHistory(history)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Tool Quality Trends (%d days)\n\n", days))

	if category != "" {
		sb.WriteString(fmt.Sprintf("*Category: %s*\n\n", category))
	}

	sb.WriteString("## Current Snapshot\n\n")
	sb.WriteString(fmt.Sprintf("- **Total Tools:** %d\n", report.TotalTools))
	sb.WriteString(fmt.Sprintf("- **Average Score:** %.1f\n", report.AverageScore))
	sb.WriteString(fmt.Sprintf("- **Grade Distribution:** A(%d) B(%d) C(%d) D(%d) F(%d)\n\n",
		report.GradeDistribution["A"],
		report.GradeDistribution["B"],
		report.GradeDistribution["C"],
		report.GradeDistribution["D"],
		report.GradeDistribution["F"]))

	// Show historical trends if available
	cutoff := time.Now().AddDate(0, 0, -days)
	var relevantSnapshots []ScoreSnapshot
	for _, snap := range history.Snapshots {
		if snap.Category != category {
			continue
		}
		snapDate, _ := time.Parse("2006-01-02", snap.Date)
		if snapDate.After(cutoff) {
			relevantSnapshots = append(relevantSnapshots, snap)
		}
	}

	sb.WriteString("## Trend Analysis\n\n")
	if len(relevantSnapshots) > 1 {
		first := relevantSnapshots[0]
		last := relevantSnapshots[len(relevantSnapshots)-1]
		scoreDelta := last.AverageScore - first.AverageScore
		trend := "→"
		if scoreDelta > 0.5 {
			trend = "↑"
		} else if scoreDelta < -0.5 {
			trend = "↓"
		}
		sb.WriteString(fmt.Sprintf("- **Score Trend:** %.1f → %.1f (%s%.1f) %s\n",
			first.AverageScore, last.AverageScore,
			map[bool]string{true: "+", false: ""}[scoreDelta >= 0], scoreDelta, trend))
		sb.WriteString(fmt.Sprintf("- **Data Points:** %d snapshots over %d days\n", len(relevantSnapshots), days))
	} else {
		sb.WriteString("*Insufficient history. Run `webb_tool_score_all` over time to build trends.*\n")
	}

	return tools.TextResult(sb.String()), nil
}

// formatMetricName converts metric keys to display names.
func formatMetricName(metric string) string {
	replacer := strings.NewReplacer(
		"_", " ",
		"avg", "Avg",
		"desc", "Description",
		"rate", "Rate",
	)
	name := replacer.Replace(metric)
	if len(name) > 0 {
		return strings.ToUpper(name[:1]) + name[1:]
	}
	return name
}

// handleToolLint validates tools and returns issues in lint format.
func handleToolLint(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	toolName := req.GetString("tool_name", "")
	category := req.GetString("category", "")
	severity := req.GetString("severity", "warning")
	format := req.GetString("format", "summary")
	failOnError := req.GetBool("fail_on_error", false)

	// Get tools to lint
	engine := clients.NewToolScoringEngineWithProvider(getToolProvider())
	var toolsToLint []string

	if toolName != "" {
		toolsToLint = []string{toolName}
	} else {
		allTools := getToolProvider().ListAllTools()
		for _, t := range allTools {
			if category == "" || t.Category == category {
				toolsToLint = append(toolsToLint, t.Name)
			}
		}
	}

	// Collect all issues
	type lintResult struct {
		ToolName string
		Score    float64
		Issues   []clients.ScoringIssue
	}
	var results []lintResult
	var errorCount, warningCount, infoCount int

	severityOrder := map[string]int{"error": 0, "warning": 1, "info": 2}
	minSeverity := severityOrder[severity]

	for _, name := range toolsToLint {
		score, err := engine.ScoreTool(ctx, name)
		if err != nil {
			continue
		}

		var filteredIssues []clients.ScoringIssue
		for _, issue := range score.Issues {
			if severityOrder[issue.Severity] <= minSeverity {
				filteredIssues = append(filteredIssues, issue)
				switch issue.Severity {
				case "error":
					errorCount++
				case "warning":
					warningCount++
				case "info":
					infoCount++
				}
			}
		}

		if len(filteredIssues) > 0 {
			results = append(results, lintResult{
				ToolName: name,
				Score:    score.OverallScore,
				Issues:   filteredIssues,
			})
		}
	}

	// Format output
	var sb strings.Builder

	switch format {
	case "json":
		type jsonOutput struct {
			ToolsLinted  int          `json:"tools_linted"`
			ErrorCount   int          `json:"error_count"`
			WarningCount int          `json:"warning_count"`
			InfoCount    int          `json:"info_count"`
			Results      []lintResult `json:"results"`
		}
		output := jsonOutput{
			ToolsLinted:  len(toolsToLint),
			ErrorCount:   errorCount,
			WarningCount: warningCount,
			InfoCount:    infoCount,
			Results:      results,
		}
		jsonBytes, _ := json.MarshalIndent(output, "", "  ")
		sb.Write(jsonBytes)

	case "github":
		// GitHub Actions annotation format
		for _, r := range results {
			for _, issue := range r.Issues {
				level := "warning"
				if issue.Severity == "error" {
					level = "error"
				} else if issue.Severity == "info" {
					level = "notice"
				}
				sb.WriteString(fmt.Sprintf("::%s title=%s::%s - %s\n", level, issue.Code, r.ToolName, issue.Message))
			}
		}

	default: // summary
		sb.WriteString("# Tool Lint Report\n\n")
		sb.WriteString(fmt.Sprintf("**Tools Linted:** %d\n", len(toolsToLint)))
		sb.WriteString(fmt.Sprintf("**Issues Found:** %d errors, %d warnings, %d info\n", errorCount, warningCount, infoCount))
		if category != "" {
			sb.WriteString(fmt.Sprintf("**Category:** %s\n", category))
		}
		sb.WriteString(fmt.Sprintf("**Min Severity:** %s\n\n", severity))

		if len(results) == 0 {
			sb.WriteString("✓ All tools pass lint checks!\n")
		} else {
			sb.WriteString("## Issues by Tool\n\n")
			for _, r := range results {
				sb.WriteString(fmt.Sprintf("### %s (score: %.1f)\n\n", r.ToolName, r.Score))
				for _, issue := range r.Issues {
					icon := "⚠️"
					if issue.Severity == "error" {
						icon = "❌"
					} else if issue.Severity == "info" {
						icon = "ℹ️"
					}
					sb.WriteString(fmt.Sprintf("- %s **[%s]** %s\n", icon, issue.Code, issue.Message))
					if issue.Suggestion != "" {
						sb.WriteString(fmt.Sprintf("  - Fix: %s\n", issue.Suggestion))
					}
				}
				sb.WriteString("\n")
			}
		}
	}

	// Check fail condition for CI
	if failOnError && errorCount > 0 {
		return tools.ErrorResult(fmt.Errorf("lint failed: %d error(s) found\n\n%s", errorCount, sb.String())), nil
	}

	return tools.TextResult(sb.String()), nil
}
