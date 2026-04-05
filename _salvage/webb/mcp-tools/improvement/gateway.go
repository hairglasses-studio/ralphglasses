// Package improvement provides self-improvement tools with a unified gateway interface
package improvement

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// GatewayToolDefinition returns the unified Improvement gateway tool
func GatewayToolDefinition() tools.ToolDefinition {
	return tools.ToolDefinition{
		Tool: mcp.NewTool("webb_improvement",
			mcp.WithDescription("Execute Improvement gateway: cycle, analysis, patterns, healing, digest, latency. Consolidates 28 self-improvement tools."),
			mcp.WithString("domain",
				mcp.Required(),
				mcp.Enum("cycle", "analysis", "patterns", "healing", "digest", "latency"),
				mcp.Description("Improvement domain: cycle (full), analysis (analyze/suggest/scaffold/noise_report/track/usage_aggregate), patterns (mine/list/suggest/to_chain), healing (dashboard/playbook_execute/approve/config/history), digest (weekly/history/opportunity_status/velocity_report/opportunities_list), latency (dashboard/tool_latency/profiles/session_budget/slow_alerts/predict/add_alternative)"),
			),
			mcp.WithString("action",
				mcp.Required(),
				mcp.Description("Action within domain. Cycle: full. Analysis: analyze,suggest,scaffold,noise_report,track,usage_aggregate. Patterns: mine,list,suggest,to_chain. Healing: dashboard,playbook_execute,approve,config,history. Digest: weekly,history,opportunity_status,velocity_report,opportunities_list. Latency: dashboard,tool_latency,profiles,session_budget,slow_alerts,predict,add_alternative"),
			),
			// Cycle/Analysis params
			mcp.WithString("mode",
				mcp.Description("Mode: full, quick, report (default: quick)"),
			),
			mcp.WithString("time_range",
				mcp.Description("Analysis period: 7d, 30d, 90d (default: 30d)"),
			),
			mcp.WithString("focus",
				mcp.Description("Focus area: all, workflows, noise, consolidation, compliance"),
			),
			mcp.WithString("output",
				mcp.Description("Output format: markdown, json, vault"),
			),
			mcp.WithString("format",
				mcp.Description("Output format: markdown, json"),
			),
			mcp.WithNumber("min_frequency",
				mcp.Description("Minimum frequency for suggestions/patterns"),
			),
			mcp.WithNumber("min_savings",
				mcp.Description("Minimum token savings % to include"),
			),
			mcp.WithNumber("limit",
				mcp.Description("Maximum results (default: 20)"),
			),
			mcp.WithBoolean("save",
				mcp.Description("Save results to vault"),
			),
			mcp.WithBoolean("auto_scaffold",
				mcp.Description("Auto-generate scaffolds for suggestions"),
			),
			mcp.WithBoolean("include_metrics",
				mcp.Description("Include usage metrics"),
			),
			// Scaffold params
			mcp.WithString("source_tools",
				mcp.Description("Comma-separated tool names to consolidate"),
			),
			mcp.WithString("name",
				mcp.Description("Name for new consolidated tool"),
			),
			mcp.WithString("description",
				mcp.Description("Tool description"),
			),
			mcp.WithBoolean("include_tests",
				mcp.Description("Generate test file"),
			),
			// Track/Analysis params
			mcp.WithString("baseline",
				mcp.Description("Baseline snapshot date (YYYY-MM-DD)"),
			),
			mcp.WithString("category",
				mcp.Description("Filter by category"),
			),
			mcp.WithNumber("threshold",
				mcp.Description("Output size threshold in KB"),
			),
			mcp.WithString("group_by",
				mcp.Description("Group by: user, tool, day, customer"),
			),
			// Pattern params
			mcp.WithString("type",
				mcp.Description("Pattern type: all, sequential, co_occur, temporal"),
			),
			mcp.WithString("sort",
				mcp.Description("Sort by: frequency, confidence, recent, score, created, impact"),
			),
			mcp.WithString("pattern_id",
				mcp.Description("Pattern ID for conversion"),
			),
			mcp.WithString("chain_name",
				mcp.Description("Custom name for chain"),
			),
			mcp.WithBoolean("register",
				mcp.Description("Register chain in registry"),
			),
			// Healing params
			mcp.WithString("service",
				mcp.Description("Filter by service: rabbitmq, redis, postgres, clickhouse, all"),
			),
			mcp.WithNumber("history_limit",
				mcp.Description("Number of recent executions to show"),
			),
			mcp.WithString("playbook_id",
				mcp.Description("Playbook ID to execute"),
			),
			mcp.WithBoolean("dry_run",
				mcp.Description("Preview without executing"),
			),
			mcp.WithString("execution_id",
				mcp.Description("Execution ID for approval"),
			),
			mcp.WithString("reason",
				mcp.Description("Reason for rejection"),
			),
			mcp.WithNumber("max_auto_risk",
				mcp.Description("Max risk score for auto-execution (0-100)"),
			),
			mcp.WithNumber("require_approval_above",
				mcp.Description("Risk score requiring approval (0-100)"),
			),
			mcp.WithBoolean("enabled",
				mcp.Description("Enable/disable self-healing"),
			),
			mcp.WithBoolean("learning_enabled",
				mcp.Description("Enable/disable learning from remediations"),
			),
			mcp.WithString("status",
				mcp.Description("Filter by status"),
			),
			// Digest params
			mcp.WithString("week",
				mcp.Description("Week to analyze: current, last, or YYYY-MM-DD"),
			),
			mcp.WithBoolean("include_opportunities",
				mcp.Description("Include opportunity details"),
			),
			mcp.WithString("opportunity_id",
				mcp.Description("Opportunity ID to update"),
			),
			mcp.WithString("notes",
				mcp.Description("Notes about status change"),
			),
			mcp.WithString("period",
				mcp.Description("Period: week, month, quarter"),
			),
			mcp.WithBoolean("include_breakdown",
				mcp.Description("Include per-type breakdown"),
			),
			// Latency params
			mcp.WithBoolean("include_slow_tools",
				mcp.Description("Include list of slow tools"),
			),
			mcp.WithNumber("alert_limit",
				mcp.Description("Number of recent alerts to show"),
			),
			mcp.WithString("tool",
				mcp.Description("Tool name for latency analysis"),
			),
			mcp.WithString("profile",
				mcp.Description("Latency profile: fast, medium, slow, all"),
			),
			mcp.WithString("session_id",
				mcp.Description("Session ID for budget"),
			),
			mcp.WithBoolean("unacknowledged_only",
				mcp.Description("Only show unacknowledged alerts"),
			),
			mcp.WithString("acknowledge",
				mcp.Description("Alert ID to acknowledge"),
			),
			mcp.WithString("slow_tool",
				mcp.Description("Slow tool name for alternative"),
			),
			mcp.WithString("fast_tool",
				mcp.Description("Faster alternative tool name"),
			),
			mcp.WithNumber("latency_saving_ms",
				mcp.Description("Estimated latency savings in ms"),
			),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:          "Improvement Gateway",
				ReadOnlyHint:   mcp.ToBoolPtr(false),
				IdempotentHint: mcp.ToBoolPtr(false),
				OpenWorldHint:  mcp.ToBoolPtr(true),
			}),
		),
		Handler:     handleImprovementGateway,
		Category:    "improvement",
		Subcategory: "gateway",
		Tags:        []string{"improvement", "self-healing", "patterns", "latency", "gateway", "consolidated"},
		UseCases:    []string{"self-improvement", "pattern mining", "latency management", "weekly digest"},
		Complexity:  tools.ComplexityModerate,
	}
}

// handleImprovementGateway is the unified gateway handler for all improvement operations
func handleImprovementGateway(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	domain, err := req.RequireString("domain")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("domain parameter is required")), nil
	}

	action, err := req.RequireString("action")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("action parameter is required")), nil
	}

	switch domain {
	case "cycle":
		return handleImprovementCycleDomain(ctx, req, action)
	case "analysis":
		return handleImprovementAnalysisDomain(ctx, req, action)
	case "patterns":
		return handleImprovementPatternsDomain(ctx, req, action)
	case "healing":
		return handleImprovementHealingDomain(ctx, req, action)
	case "digest":
		return handleImprovementDigestDomain(ctx, req, action)
	case "latency":
		return handleImprovementLatencyDomain(ctx, req, action)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid domain: %s (valid: cycle, analysis, patterns, healing, digest, latency)", domain)), nil
	}
}

func handleImprovementCycleDomain(ctx context.Context, req mcp.CallToolRequest, action string) (*mcp.CallToolResult, error) {
	switch action {
	case "full":
		return handleSelfImprove(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid cycle action: %s (valid: full)", action)), nil
	}
}

func handleImprovementAnalysisDomain(ctx context.Context, req mcp.CallToolRequest, action string) (*mcp.CallToolResult, error) {
	switch action {
	case "analyze":
		return handleAnalyze(ctx, req)
	case "suggest":
		return handleSuggest(ctx, req)
	case "scaffold":
		return handleScaffold(ctx, req)
	case "noise_report":
		return handleNoiseReport(ctx, req)
	case "track":
		return handleTrack(ctx, req)
	case "usage_aggregate":
		return handleUsageAggregate(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid analysis action: %s (valid: analyze, suggest, scaffold, noise_report, track, usage_aggregate)", action)), nil
	}
}

func handleImprovementPatternsDomain(ctx context.Context, req mcp.CallToolRequest, action string) (*mcp.CallToolResult, error) {
	switch action {
	case "mine":
		return handlePatternMine(ctx, req)
	case "list":
		return handlePatternList(ctx, req)
	case "suggest":
		return handlePatternSuggest(ctx, req)
	case "to_chain":
		return handlePatternToChain(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid patterns action: %s (valid: mine, list, suggest, to_chain)", action)), nil
	}
}

func handleImprovementHealingDomain(ctx context.Context, req mcp.CallToolRequest, action string) (*mcp.CallToolResult, error) {
	switch action {
	case "dashboard":
		return handleSelfHealingDashboard(ctx, req)
	case "playbook_execute":
		return handlePlaybookExecute(ctx, req)
	case "approve":
		return handleRemediationApprove(ctx, req)
	case "config":
		return handleSelfHealingConfig(ctx, req)
	case "history":
		return handleRemediationHistory(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid healing action: %s (valid: dashboard, playbook_execute, approve, config, history)", action)), nil
	}
}

func handleImprovementDigestDomain(ctx context.Context, req mcp.CallToolRequest, action string) (*mcp.CallToolResult, error) {
	switch action {
	case "weekly":
		return handleWeeklyDigest(ctx, req)
	case "history":
		return handleDigestHistory(ctx, req)
	case "opportunity_status":
		return handleOpportunityStatus(ctx, req)
	case "velocity_report":
		return handleVelocityReport(ctx, req)
	case "opportunities_list":
		return handleOpportunitiesList(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid digest action: %s (valid: weekly, history, opportunity_status, velocity_report, opportunities_list)", action)), nil
	}
}

func handleImprovementLatencyDomain(ctx context.Context, req mcp.CallToolRequest, action string) (*mcp.CallToolResult, error) {
	switch action {
	case "dashboard":
		return handleLatencyDashboard(ctx, req)
	case "tool_latency":
		return handleToolLatency(ctx, req)
	case "profiles":
		return handleLatencyProfiles(ctx, req)
	case "session_budget":
		return handleSessionBudget(ctx, req)
	case "slow_alerts":
		return handleSlowQueryAlerts(ctx, req)
	case "predict":
		return handlePredictLatency(ctx, req)
	case "add_alternative":
		return handleAddLatencyAlternative(ctx, req)
	default:
		return tools.ErrorResult(fmt.Errorf("invalid latency action: %s (valid: dashboard, tool_latency, profiles, session_budget, slow_alerts, predict, add_alternative)", action)), nil
	}
}
