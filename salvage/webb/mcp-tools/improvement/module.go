// Package improvement provides self-improvement automation tools for Webb.
// The megatool analyzes usage patterns and generates optimization suggestions.
package improvement

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/chains"
	"github.com/hairglasses-studio/webb/internal/clients"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
	"github.com/hairglasses-studio/webb/internal/mcp/tools/common"
	"github.com/hairglasses-studio/webb/internal/patterns"
)

// Module implements the ToolModule interface for self-improvement tools
type Module struct{}

// Name returns the module name
func (m *Module) Name() string {
	return "improvement"
}

// Description returns a brief description of the module
func (m *Module) Description() string {
	return "Self-improvement automation tools for analyzing usage and generating optimizations"
}

// Tools returns all tool definitions in this module
func (m *Module) Tools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		// Gateway tool (consolidates 28 improvement tools into 6 domains)
		GatewayToolDefinition(),
		// Self Improve - The Megatool
		{
			Tool: mcp.NewTool("webb_self_improve",
				mcp.WithDescription("Execute full self-improvement cycle: analyze usage, generate suggestions, create scaffolds, track progress. Consolidated entry point for tool optimization."),
				mcp.WithString("mode",
					mcp.Description("Mode: full (all checks), quick (summary only), report (save to vault) (default: quick)"),
				),
				mcp.WithBoolean("auto_scaffold",
					mcp.Description("Auto-generate scaffolds for top suggestions (default: false)"),
				),
				mcp.WithBoolean("save",
					mcp.Description("Save results to Obsidian vault (default: true)"),
				),
				mcp.WithToolAnnotation(mcp.ToolAnnotation{
					Title:          "Self-Improvement Cycle",
					ReadOnlyHint:   mcp.ToBoolPtr(false),
					IdempotentHint: mcp.ToBoolPtr(true),
				}),
			),
			Handler:        handleSelfImprove,
			Category:       "improvement",
			Subcategory:    "cycle",
			Tags:           []string{"self-improvement", "automation", "optimization", "megatool"},
			UseCases:       []string{"Full optimization cycle", "Automated tool improvement", "Weekly self-analysis"},
			Complexity:     tools.ComplexityComplex,
			ThinkingBudget: 10000,
			IsWrite:        true,
		},
		// Improvement Analyze
		{
			Tool: mcp.NewTool("webb_improvement_analyze",
				mcp.WithDescription("Analyze tool usage patterns to identify consolidation opportunities and workflow optimizations."),
				mcp.WithString("time_range",
					mcp.Description("Analysis period (preset): 7d, 30d, 90d (default: 30d)"),
				),
				mcp.WithString("focus",
					mcp.Description("Focus area: all, workflows, noise, consolidation, compliance (default: all)"),
				),
				mcp.WithBoolean("include_metrics",
					mcp.Description("Include usage metrics from vault logs (default: true)"),
				),
				mcp.WithString("output",
					mcp.Description("Output format: markdown, json, vault (default: markdown)"),
				),
			),
			Handler:     handleAnalyze,
			Category:    "improvement",
			Subcategory: "analysis",
			Tags:        []string{"analysis", "patterns", "optimization"},
			UseCases:    []string{"Identify tool consolidation opportunities", "Find noisy tools", "Mine workflow patterns"},
			Complexity:  tools.ComplexityModerate,
			IsWrite:     false,
		},
		// Improvement Suggest
		{
			Tool: mcp.NewTool("webb_improvement_suggest",
				mcp.WithDescription("Create consolidation suggestions for frequently co-occurring tools. Returns prioritized list with estimated token savings."),
				mcp.WithNumber("min_frequency",
					mcp.Description("Minimum co-occurrence count to consider (default: 10)"),
				),
				mcp.WithNumber("min_savings",
					mcp.Description("Minimum token savings % to include (default: 20)"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum suggestions to return (default: 10)"),
				),
			),
			Handler:     handleSuggest,
			Category:    "improvement",
			Subcategory: "consolidation",
			Tags:        []string{"consolidation", "suggestions", "optimization"},
			UseCases:    []string{"Find consolidation candidates", "Prioritize optimizations"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Improvement Scaffold
		{
			Tool: mcp.NewTool("webb_improvement_scaffold",
				mcp.WithDescription("Create Go code scaffold for a new consolidated tool. Merges parameters and creates handler template."),
				mcp.WithString("source_tools",
					mcp.Required(),
					mcp.Description("Comma-separated tool names to consolidate"),
				),
				mcp.WithString("name",
					mcp.Required(),
					mcp.Description("Name for new tool (e.g., webb_cluster_full)"),
				),
				mcp.WithString("description",
					mcp.Description("Tool description (auto-generated if omitted)"),
				),
				mcp.WithBoolean("include_tests",
					mcp.Description("Generate test file (default: true)"),
				),
			),
			Handler:     handleScaffold,
			Category:    "improvement",
			Subcategory: "generation",
			Tags:        []string{"scaffold", "generation", "code"},
			UseCases:    []string{"Generate new consolidated tool", "Create tool template"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		// Improvement Noise Report
		{
			Tool: mcp.NewTool("webb_improvement_noise_report",
				mcp.WithDescription("Analyze tools with excessive output size or noisy patterns. Suggests format optimizations."),
				mcp.WithString("category",
					mcp.Description("Filter by category (optional)"),
				),
				mcp.WithNumber("threshold",
					mcp.Description("Output size threshold in KB to flag (default: 50)"),
				),
			),
			Handler:     handleNoiseReport,
			Category:    "improvement",
			Subcategory: "noise",
			Tags:        []string{"noise", "output", "optimization"},
			UseCases:    []string{"Find verbose tools", "Reduce token usage"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Improvement Track
		{
			Tool: mcp.NewTool("webb_improvement_track",
				mcp.WithDescription("Analyze improvement progress over time. Records analysis to vault and compares to previous runs."),
				mcp.WithString("action",
					mcp.Description("Action: snapshot, compare, history (default: snapshot)"),
				),
				mcp.WithString("baseline",
					mcp.Description("Baseline snapshot date for comparison (YYYY-MM-DD)"),
				),
			),
			Handler:     handleTrack,
			Category:    "improvement",
			Subcategory: "tracking",
			Tags:        []string{"tracking", "progress", "history"},
			UseCases:    []string{"Track optimization progress", "Compare to baseline"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		// Multi-user Usage Aggregate
		{
			Tool: mcp.NewTool("webb_usage_aggregate",
				mcp.WithDescription("Analyze usage logs from all users. Returns cross-user patterns and tool popularity."),
				mcp.WithString("time_range",
					mcp.Description("Time range (preset): 7d, 30d, 90d (default: 7d)"),
				),
				mcp.WithString("group_by",
					mcp.Description("Group by: user, tool, day, customer (default: tool)"),
				),
			),
			Handler:     handleUsageAggregate,
			Category:    "improvement",
			Subcategory: "analytics",
			Tags:        []string{"analytics", "multi-user", "aggregate"},
			UseCases:    []string{"Cross-user analytics", "Team tool usage", "Popular tools"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Pattern Mining (v7.05)
		{
			Tool: mcp.NewTool("webb_pattern_mine",
				mcp.WithDescription("Mine workflow patterns from session history. Discovers sequential, co-occurrence, and temporal patterns across sessions."),
				mcp.WithString("type",
					mcp.Description("Pattern type: all, sequential, co_occur, temporal (default: all)"),
				),
				mcp.WithNumber("min_frequency",
					mcp.Description("Minimum pattern occurrences to include (default: 3)"),
				),
				mcp.WithBoolean("save",
					mcp.Description("Save patterns to config file (default: true)"),
				),
			),
			Handler:     handlePatternMine,
			Category:    "improvement",
			Subcategory: "patterns",
			Tags:        []string{"patterns", "mining", "workflows", "analysis"},
			UseCases:    []string{"Discover workflow patterns", "Find common tool sequences", "Identify optimization opportunities"},
			Complexity:  tools.ComplexityModerate,
			IsWrite:     true,
		},
		{
			Tool: mcp.NewTool("webb_pattern_list",
				mcp.WithDescription("List discovered workflow patterns. Shows sequential workflows, co-occurring tools, and temporal patterns."),
				mcp.WithString("type",
					mcp.Description("Filter by type: all, sequential, co_occur, temporal (default: all)"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum patterns to show (default: 20)"),
				),
				mcp.WithString("sort",
					mcp.Description("Sort by: frequency, confidence, recent (default: frequency)"),
				),
			),
			Handler:     handlePatternList,
			Category:    "improvement",
			Subcategory: "patterns",
			Tags:        []string{"patterns", "list", "workflows"},
			UseCases:    []string{"View discovered patterns", "Review workflow insights"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		{
			Tool: mcp.NewTool("webb_pattern_suggest",
				mcp.WithDescription("Get optimization suggestions based on discovered patterns. Recommends consolidations and workflow improvements."),
				mcp.WithNumber("limit",
					mcp.Description("Maximum suggestions (default: 5)"),
				),
			),
			Handler:     handlePatternSuggest,
			Category:    "improvement",
			Subcategory: "patterns",
			Tags:        []string{"patterns", "suggestions", "optimization"},
			UseCases:    []string{"Get pattern-based recommendations", "Identify consolidation opportunities"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Pattern to Chain Conversion (v7.70)
		{
			Tool: mcp.NewTool("webb_pattern_to_chain",
				mcp.WithDescription("Convert a discovered pattern to a reusable workflow chain. Creates a chain definition that can be executed as an automation."),
				mcp.WithString("pattern_id",
					mcp.Description("Pattern ID to convert (required)"),
					mcp.Required(),
				),
				mcp.WithString("chain_name",
					mcp.Description("Custom name for the chain (optional, auto-generated if not provided)"),
				),
				mcp.WithBoolean("save",
					mcp.Description("Save chain to YAML file (default: false, just preview)"),
				),
				mcp.WithBoolean("register",
					mcp.Description("Register chain in registry for execution (default: false)"),
				),
			),
			Handler:     handlePatternToChain,
			Category:    "improvement",
			Subcategory: "patterns",
			Tags:        []string{"patterns", "chains", "automation", "workflows"},
			UseCases:    []string{"Convert patterns to automations", "Create reusable workflows", "Build chains from discoveries"},
			Complexity:  tools.ComplexityModerate,
			IsWrite:     true,
		},
		// =============================================================================
		// Self-Healing Tools (v7.80)
		// =============================================================================
		// Self-Healing Dashboard
		{
			Tool: mcp.NewTool("webb_self_healing_dashboard",
				mcp.WithDescription("View self-healing status: service health, pending approvals, recent remediations, and playbook stats."),
				mcp.WithString("service",
					mcp.Description("Filter by service: rabbitmq, redis, postgres, clickhouse, all (default: all)"),
				),
				mcp.WithNumber("history_limit",
					mcp.Description("Number of recent executions to show (default: 10)"),
				),
			),
			Handler:     handleSelfHealingDashboard,
			Category:    "improvement",
			Subcategory: "self-healing",
			Tags:        []string{"self-healing", "remediation", "health", "dashboard"},
			UseCases:    []string{"View service health", "Monitor remediations", "Check pending approvals"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// webb_playbook_list moved to investigation module
		// Playbook Execute
		{
			Tool: mcp.NewTool("webb_playbook_execute",
				mcp.WithDescription("Execute a remediation playbook manually. Bypasses auto-execution restrictions for approved operators."),
				mcp.WithString("playbook_id",
					mcp.Description("Playbook ID to execute (required)"),
					mcp.Required(),
				),
				mcp.WithString("service",
					mcp.Description("Target service (required for wildcard playbooks)"),
				),
				mcp.WithBoolean("dry_run",
					mcp.Description("Preview execution without running (default: true)"),
				),
			),
			Handler:     handlePlaybookExecute,
			Category:    "improvement",
			Subcategory: "self-healing",
			Tags:        []string{"playbooks", "remediation", "execute"},
			UseCases:    []string{"Trigger manual remediation", "Test playbook execution"},
			Complexity:  tools.ComplexityModerate,
			IsWrite:     true,
		},
		// Remediation Approve
		{
			Tool: mcp.NewTool("webb_remediation_approve",
				mcp.WithDescription("Approve or reject a pending remediation. Required for high-risk playbooks."),
				mcp.WithString("execution_id",
					mcp.Description("Execution ID to approve/reject (required)"),
					mcp.Required(),
				),
				mcp.WithString("action",
					mcp.Description("Action: approve or reject (required)"),
					mcp.Required(),
				),
				mcp.WithString("reason",
					mcp.Description("Reason for rejection (required if rejecting)"),
				),
			),
			Handler:     handleRemediationApprove,
			Category:    "improvement",
			Subcategory: "self-healing",
			Tags:        []string{"remediation", "approval", "security"},
			UseCases:    []string{"Approve high-risk remediations", "Reject unsafe actions"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		// Self-Healing Config
		{
			Tool: mcp.NewTool("webb_self_healing_config",
				mcp.WithDescription("View or update self-healing configuration: auto-execution thresholds, learning, cooldowns."),
				mcp.WithString("action",
					mcp.Description("Action: view (show config), update (change settings)"),
				),
				mcp.WithNumber("max_auto_risk",
					mcp.Description("Max risk score for auto-execution (0-100, default: 20)"),
				),
				mcp.WithNumber("require_approval_above",
					mcp.Description("Risk score requiring approval (0-100, default: 50)"),
				),
				mcp.WithBoolean("enabled",
					mcp.Description("Enable/disable self-healing"),
				),
				mcp.WithBoolean("learning_enabled",
					mcp.Description("Enable/disable learning from remediations"),
				),
			),
			Handler:     handleSelfHealingConfig,
			Category:    "improvement",
			Subcategory: "self-healing",
			Tags:        []string{"self-healing", "config", "settings"},
			UseCases:    []string{"Configure self-healing behavior", "Adjust risk thresholds"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		// Remediation History
		{
			Tool: mcp.NewTool("webb_remediation_history",
				mcp.WithDescription("View remediation execution history with outcomes and durations."),
				mcp.WithNumber("limit",
					mcp.Description("Maximum entries to show (default: 20)"),
				),
				mcp.WithString("status",
					mcp.Description("Filter by status: success, failed, cancelled, approval_required, all"),
				),
				mcp.WithString("service",
					mcp.Description("Filter by service"),
				),
			),
			Handler:     handleRemediationHistory,
			Category:    "improvement",
			Subcategory: "self-healing",
			Tags:        []string{"remediation", "history", "audit"},
			UseCases:    []string{"Audit remediation history", "Review outcomes"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// =============================================================================
		// Weekly Improvement Digest Tools (v7.90)
		// =============================================================================
		// Weekly Digest
		{
			Tool: mcp.NewTool("webb_weekly_digest",
				mcp.WithDescription("Generate weekly improvement digest: consolidation opportunities, velocity metrics, trends, and recommendations."),
				mcp.WithString("week",
					mcp.Description("Week to analyze: current, last, or YYYY-MM-DD (default: current)"),
				),
				mcp.WithBoolean("save",
					mcp.Description("Save digest to vault (default: true)"),
				),
				mcp.WithString("format",
					mcp.Description("Output format: markdown, json (default: markdown)"),
				),
			),
			Handler:     handleWeeklyDigest,
			Category:    "improvement",
			Subcategory: "digest",
			Tags:        []string{"digest", "weekly", "analysis", "opportunities"},
			UseCases:    []string{"Weekly improvement review", "Track optimization opportunities", "Generate improvement reports"},
			Complexity:  tools.ComplexityModerate,
			IsWrite:     true,
		},
		// Digest History
		{
			Tool: mcp.NewTool("webb_digest_history",
				mcp.WithDescription("View past weekly digests and improvement trends over time."),
				mcp.WithNumber("limit",
					mcp.Description("Number of weeks to show (default: 4)"),
				),
				mcp.WithBoolean("include_opportunities",
					mcp.Description("Include opportunity details (default: false for summary)"),
				),
			),
			Handler:     handleDigestHistory,
			Category:    "improvement",
			Subcategory: "digest",
			Tags:        []string{"digest", "history", "trends"},
			UseCases:    []string{"Review improvement history", "Track progress over time"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Opportunity Status
		{
			Tool: mcp.NewTool("webb_opportunity_status",
				mcp.WithDescription("Update improvement opportunity status. Track progress from new to shipped."),
				mcp.WithString("opportunity_id",
					mcp.Description("Opportunity ID to update (required)"),
					mcp.Required(),
				),
				mcp.WithString("status",
					mcp.Description("New status: new, in_progress, shipped, deferred (required)"),
					mcp.Required(),
				),
				mcp.WithString("notes",
					mcp.Description("Notes about the status change"),
				),
			),
			Handler:     handleOpportunityStatus,
			Category:    "improvement",
			Subcategory: "digest",
			Tags:        []string{"opportunities", "status", "tracking"},
			UseCases:    []string{"Track improvement progress", "Mark opportunities as shipped"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		// Velocity Report
		{
			Tool: mcp.NewTool("webb_velocity_report",
				mcp.WithDescription("View improvement velocity metrics: shipped items, backlog size, average time to ship."),
				mcp.WithString("period",
					mcp.Description("Period: week, month, quarter (default: month)"),
				),
				mcp.WithBoolean("include_breakdown",
					mcp.Description("Include per-type breakdown (default: true)"),
				),
			),
			Handler:     handleVelocityReport,
			Category:    "improvement",
			Subcategory: "digest",
			Tags:        []string{"velocity", "metrics", "tracking"},
			UseCases:    []string{"Track improvement velocity", "Monitor backlog health"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Opportunities List
		{
			Tool: mcp.NewTool("webb_opportunities_list",
				mcp.WithDescription("List improvement opportunities by type and status."),
				mcp.WithString("type",
					mcp.Description("Filter by type: consolidation, noise_reduction, workflow, documentation, all (default: all)"),
				),
				mcp.WithString("status",
					mcp.Description("Filter by status: new, in_progress, shipped, deferred, all (default: new)"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum opportunities to show (default: 20)"),
				),
				mcp.WithString("sort",
					mcp.Description("Sort by: score, created, impact (default: score)"),
				),
			),
			Handler:     handleOpportunitiesList,
			Category:    "improvement",
			Subcategory: "digest",
			Tags:        []string{"opportunities", "list", "backlog"},
			UseCases:    []string{"View improvement backlog", "Find high-impact opportunities"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// =============================================================================
		// Latency-Aware Routing Tools (v7.95)
		// =============================================================================
		// Latency Dashboard
		{
			Tool: mcp.NewTool("webb_latency_dashboard",
				mcp.WithDescription("View latency dashboard: tool profiles, slow query alerts, session budgets, and routing stats."),
				mcp.WithBoolean("include_slow_tools",
					mcp.Description("Include detailed list of slow tools (default: true)"),
				),
				mcp.WithNumber("alert_limit",
					mcp.Description("Number of recent alerts to show (default: 5)"),
				),
			),
			Handler:     handleLatencyDashboard,
			Category:    "improvement",
			Subcategory: "latency",
			Tags:        []string{"latency", "dashboard", "performance", "routing"},
			UseCases:    []string{"View tool performance", "Monitor latency budgets", "Check slow query alerts"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Tool Latency Stats
		{
			Tool: mcp.NewTool("webb_tool_latency",
				mcp.WithDescription("Get detailed latency statistics for a specific tool."),
				mcp.WithString("tool",
					mcp.Description("Tool name to analyze (required)"),
					mcp.Required(),
				),
			),
			Handler:     handleToolLatency,
			Category:    "improvement",
			Subcategory: "latency",
			Tags:        []string{"latency", "tool", "stats"},
			UseCases:    []string{"Analyze tool performance", "Find latency issues"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Latency Profiles
		{
			Tool: mcp.NewTool("webb_latency_profiles",
				mcp.WithDescription("List tools by latency profile: fast (<100ms), medium (100ms-1s), slow (>1s)."),
				mcp.WithString("profile",
					mcp.Description("Filter by profile: fast, medium, slow, all (default: all)"),
				),
				mcp.WithNumber("limit",
					mcp.Description("Maximum tools per profile (default: 20)"),
				),
			),
			Handler:     handleLatencyProfiles,
			Category:    "improvement",
			Subcategory: "latency",
			Tags:        []string{"latency", "profiles", "performance"},
			UseCases:    []string{"View tools by speed", "Find fast alternatives"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Session Budget
		{
			Tool: mcp.NewTool("webb_session_budget",
				mcp.WithDescription("View or manage session latency budget."),
				mcp.WithString("session_id",
					mcp.Description("Session ID (uses current session if not provided)"),
				),
				mcp.WithString("action",
					mcp.Description("Action: view, reset (default: view)"),
				),
			),
			Handler:     handleSessionBudget,
			Category:    "improvement",
			Subcategory: "latency",
			Tags:        []string{"latency", "budget", "session"},
			UseCases:    []string{"Check remaining latency budget", "Reset session budget"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		// Slow Query Alerts
		{
			Tool: mcp.NewTool("webb_slow_query_alerts",
				mcp.WithDescription("View and manage slow query alerts."),
				mcp.WithNumber("limit",
					mcp.Description("Maximum alerts to show (default: 20)"),
				),
				mcp.WithBoolean("unacknowledged_only",
					mcp.Description("Only show unacknowledged alerts (default: true)"),
				),
				mcp.WithString("acknowledge",
					mcp.Description("Alert ID to acknowledge (optional)"),
				),
			),
			Handler:     handleSlowQueryAlerts,
			Category:    "improvement",
			Subcategory: "latency",
			Tags:        []string{"latency", "alerts", "slow"},
			UseCases:    []string{"View slow query alerts", "Acknowledge alerts"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
		// Latency Prediction
		{
			Tool: mcp.NewTool("webb_predict_latency",
				mcp.WithDescription("Predict latency for a tool call based on historical data."),
				mcp.WithString("tool",
					mcp.Description("Tool name to predict (required)"),
					mcp.Required(),
				),
				mcp.WithNumber("limit",
					mcp.Description("Limit parameter value to factor in (default: 25)"),
				),
			),
			Handler:     handlePredictLatency,
			Category:    "improvement",
			Subcategory: "latency",
			Tags:        []string{"latency", "prediction", "planning"},
			UseCases:    []string{"Predict tool latency", "Plan workflow timing"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     false,
		},
		// Add Alternative
		{
			Tool: mcp.NewTool("webb_add_latency_alternative",
				mcp.WithDescription("Add a faster alternative for a slow tool."),
				mcp.WithString("slow_tool",
					mcp.Description("The slow tool name (required)"),
					mcp.Required(),
				),
				mcp.WithString("fast_tool",
					mcp.Description("The faster alternative tool name (required)"),
					mcp.Required(),
				),
				mcp.WithNumber("latency_saving_ms",
					mcp.Description("Estimated latency savings in milliseconds (required)"),
					mcp.Required(),
				),
			),
			Handler:     handleAddLatencyAlternative,
			Category:    "improvement",
			Subcategory: "latency",
			Tags:        []string{"latency", "alternatives", "routing"},
			UseCases:    []string{"Configure faster alternatives", "Optimize routing"},
			Complexity:  tools.ComplexitySimple,
			IsWrite:     true,
		},
	}
}

// Response schemas

// SelfImproveResponse is the output for the megatool
type SelfImproveResponse struct {
	Mode              string                    `json:"mode"`
	AnalyzedSessions  int                       `json:"analyzed_sessions"`
	TotalMessages     int                       `json:"total_messages"`
	NoisyPatterns     int                       `json:"noisy_patterns"`
	Suggestions       []ConsolidationSuggestion `json:"suggestions"`
	TokenSavings      int                       `json:"estimated_token_savings"`
	Recommendations   []string                  `json:"recommendations"`
	SavedTo           string                    `json:"saved_to,omitempty"`
	Timestamp         string                    `json:"timestamp"`
}

// ConsolidationSuggestion represents a tool consolidation opportunity
type ConsolidationSuggestion struct {
	SourceTools    []string `json:"source_tools"`
	ProposedName   string   `json:"proposed_name"`
	TokenSavings   int      `json:"token_savings_percent"`
	Priority       int      `json:"priority"`
	Implementation string   `json:"implementation,omitempty"`
}

// UsageAggregateEntry represents aggregated usage
type UsageAggregateEntry struct {
	Key       string `json:"key"`
	Count     int    `json:"count"`
	Users     int    `json:"users,omitempty"`
	AvgTokens int    `json:"avg_tokens,omitempty"`
}

// Handler implementations

func handleSelfImprove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	mode := req.GetString("mode", "quick")
	// autoScaffold := req.GetBool("auto_scaffold", false)
	save := req.GetBool("save", true)

	response := SelfImproveResponse{
		Mode:      mode,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	// Analyze sessions
	client, err := clients.NewClaudeSessionClient("", "")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("creating session client: %w", err)), nil
	}

	stats, err := client.AnalyzeAllSessions()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("analyzing sessions: %w", err)), nil
	}

	response.AnalyzedSessions = stats.TotalSessions
	response.TotalMessages = stats.TotalMessages
	response.NoisyPatterns = len(stats.NoisyPatterns)

	// Generate suggestions from noisy patterns
	for _, np := range stats.NoisyPatterns {
		response.Suggestions = append(response.Suggestions, ConsolidationSuggestion{
			SourceTools:  []string{"curl " + np.Pattern},
			ProposedName: np.Suggested,
			TokenSavings: np.TokenSavings,
			Priority:     np.Count,
		})
	}

	// Calculate estimated savings
	for _, np := range stats.NoisyPatterns {
		response.TokenSavings += np.Count * 500 * np.TokenSavings / 100
	}

	// Add recommendations
	response.Recommendations = []string{
		"Replace Slack API curl commands with webb_slack_* tools",
		"Use webb_quick_check for fast health checks",
		"Use webb_session_context to track investigation state",
		"Run webb_self_improve weekly for ongoing optimization",
	}

	// Save to vault if requested
	if save {
		if err := client.ExportToVault(stats); err == nil {
			response.SavedTo = "~/webb-vault/improvement/analysis/"
		}
	}

	// Format output based on mode
	var output string
	if mode == "quick" {
		output = fmt.Sprintf(`## Self-Improvement Summary

**Sessions Analyzed:** %d
**Noisy Patterns Found:** %d
**Estimated Token Savings:** %d tokens

### Top Suggestions
`, response.AnalyzedSessions, response.NoisyPatterns, response.TokenSavings)

		for i, s := range response.Suggestions {
			if i >= 5 {
				break
			}
			output += fmt.Sprintf("- %s -> `%s` (%d%% savings)\n", s.SourceTools[0], s.ProposedName, s.TokenSavings)
		}

		output += "\n### Recommendations\n"
		for _, r := range response.Recommendations {
			output += fmt.Sprintf("- %s\n", r)
		}
	} else {
		jsonOut, _ := json.MarshalIndent(response, "", "  ")
		output = string(jsonOut)
	}

	return tools.TextResult(output), nil
}

func handleAnalyze(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	focus := req.GetString("focus", "all")
	outputFormat := req.GetString("output", "markdown")

	client, err := clients.NewClaudeSessionClient("", "")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("creating session client: %w", err)), nil
	}

	stats, err := client.AnalyzeAllSessions()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("analyzing sessions: %w", err)), nil
	}

	if outputFormat == "json" {
		jsonOut, _ := json.MarshalIndent(stats, "", "  ")
		return tools.TextResult(string(jsonOut)), nil
	}

	output := client.FormatStats(stats, focus)
	return tools.TextResult(output), nil
}

func handleSuggest(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	minFreq := req.GetInt("min_frequency", 10)
	minSavings := req.GetInt("min_savings", 20)
	limit := req.GetInt("limit", 10)

	client, err := clients.NewClaudeSessionClient("", "")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("creating session client: %w", err)), nil
	}

	stats, err := client.AnalyzeAllSessions()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("analyzing sessions: %w", err)), nil
	}

	var suggestions []ConsolidationSuggestion
	for _, np := range stats.NoisyPatterns {
		if np.Count >= minFreq && np.TokenSavings >= minSavings {
			suggestions = append(suggestions, ConsolidationSuggestion{
				SourceTools:  []string{np.Pattern},
				ProposedName: np.Suggested,
				TokenSavings: np.TokenSavings,
				Priority:     np.Count,
			})
		}
	}

	// Sort by priority
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Priority > suggestions[j].Priority
	})

	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}

	var sb strings.Builder
	sb.WriteString("## Consolidation Suggestions\n\n")
	sb.WriteString("| Priority | Pattern | Suggested Tool | Savings |\n")
	sb.WriteString("|----------|---------|----------------|--------|\n")
	for _, s := range suggestions {
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %d%% |\n",
			s.Priority, s.SourceTools[0], s.ProposedName, s.TokenSavings))
	}

	return tools.TextResult(sb.String()), nil
}

func handleScaffold(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceTools, err := req.RequireString("source_tools")
	if err != nil {
		return tools.ErrorResult(err), nil
	}
	name, err := req.RequireString("name")
	if err != nil {
		return tools.ErrorResult(err), nil
	}
	description := req.GetString("description", "")
	includeTests := req.GetBool("include_tests", true)

	toolList := strings.Split(sourceTools, ",")
	for i := range toolList {
		toolList[i] = strings.TrimSpace(toolList[i])
	}

	if description == "" {
		description = fmt.Sprintf("Consolidated tool combining: %s", strings.Join(toolList, ", "))
	}

	// Generate scaffold
	scaffold := fmt.Sprintf(`// %s - Auto-generated consolidated tool
// Source tools: %s

package consolidated

import (
	"context"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// Add to module.go Tools() slice:
/*
{
	Tool: mcp.NewTool("%s",
		mcp.WithDescription("%s"),
		common.ParamContext(),
		common.ParamNamespaceAcme(),
	),
	Handler:     handle%s,
	Category:    "consolidated",
	Tags:        []string{%s},
	UseCases:    []string{"Combined operation for %s"},
	Complexity:  tools.ComplexityModerate,
	IsWrite:     false,
},
*/

func handle%s(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// TODO: Implement consolidated logic for:
	// %s

	return tools.TextResult("Not implemented"), nil
}
`, name, strings.Join(toolList, ", "), name, description,
		strings.Title(strings.ReplaceAll(name, "webb_", "")),
		fmt.Sprintf(`"%s"`, strings.Join(toolList, `", "`)),
		strings.Join(toolList, ", "),
		strings.Title(strings.ReplaceAll(name, "webb_", "")),
		strings.Join(toolList, "\n\t// "))

	if includeTests {
		scaffold += fmt.Sprintf(`

// Test file: module_test.go
/*
func TestHandle%s(t *testing.T) {
	ctx := context.Background()
	req := mcp.CallToolRequest{
		Params: map[string]interface{}{
			"context": "test-cluster",
		},
	}
	result, err := handle%s(ctx, req)
	require.NoError(t, err)
	require.NotNil(t, result)
}
*/
`, strings.Title(strings.ReplaceAll(name, "webb_", "")),
			strings.Title(strings.ReplaceAll(name, "webb_", "")))
	}

	// Save to vault scaffolds directory
	home, _ := os.UserHomeDir()
	scaffoldDir := filepath.Join(home, "webb-vault", "improvement", "scaffolds", "pending")
	os.MkdirAll(scaffoldDir, 0755)

	filename := fmt.Sprintf("%s.go", name)
	scaffoldPath := filepath.Join(scaffoldDir, filename)
	os.WriteFile(scaffoldPath, []byte(scaffold), 0644)

	return tools.TextResult(fmt.Sprintf("Scaffold generated:\n\n```go\n%s\n```\n\nSaved to: %s", scaffold, scaffoldPath)), nil
}

func handleNoiseReport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// threshold := req.GetInt("threshold", 50)

	client, err := clients.NewClaudeSessionClient("", "")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("creating session client: %w", err)), nil
	}

	stats, err := client.AnalyzeAllSessions()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("analyzing sessions: %w", err)), nil
	}

	var sb strings.Builder
	sb.WriteString("## Noise Report\n\n")

	if len(stats.NoisyPatterns) == 0 {
		sb.WriteString("No noisy patterns detected.\n")
	} else {
		sb.WriteString("### Noisy Patterns\n\n")
		sb.WriteString("| Pattern | Count | Example | Suggested |\n")
		sb.WriteString("|---------|-------|---------|----------|\n")
		for _, np := range stats.NoisyPatterns {
			example := ""
			if len(np.Examples) > 0 {
				example = np.Examples[0]
				if len(example) > 50 {
					example = example[:50] + "..."
				}
			}
			sb.WriteString(fmt.Sprintf("| %s | %d | %s | %s |\n",
				np.Pattern, np.Count, example, np.Suggested))
		}
	}

	return tools.TextResult(sb.String()), nil
}

func handleTrack(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := req.GetString("action", "snapshot")
	baseline := req.GetString("baseline", "")

	client, err := clients.NewClaudeSessionClient("", "")
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("creating session client: %w", err)), nil
	}

	stats, err := client.AnalyzeAllSessions()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("analyzing sessions: %w", err)), nil
	}

	switch action {
	case "snapshot":
		if err := client.ExportToVault(stats); err != nil {
			return tools.ErrorResult(fmt.Errorf("saving snapshot: %w", err)), nil
		}
		return tools.TextResult(fmt.Sprintf("Snapshot saved for %s", time.Now().Format("2006-01-02"))), nil

	case "compare":
		if baseline == "" {
			return tools.ErrorResult(fmt.Errorf("baseline date required for compare")), nil
		}
		// TODO: Load baseline and compare
		return tools.TextResult(fmt.Sprintf("Comparison with %s: Not yet implemented", baseline)), nil

	case "history":
		// List available snapshots
		home, _ := os.UserHomeDir()
		pattern := filepath.Join(home, "webb-vault", "improvement", "analysis", "*-analysis.md")
		files, _ := filepath.Glob(pattern)

		var sb strings.Builder
		sb.WriteString("## Available Snapshots\n\n")
		for _, f := range files {
			sb.WriteString(fmt.Sprintf("- %s\n", filepath.Base(f)))
		}
		return tools.TextResult(sb.String()), nil
	}

	return tools.ErrorResult(fmt.Errorf("unknown action: %s", action)), nil
}

func handleUsageAggregate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	timeRange := req.GetString("time_range", "7d")
	groupBy := req.GetString("group_by", "tool")

	// Parse time range
	days := 7
	switch timeRange {
	case "30d":
		days = 30
	case "90d":
		days = 90
	}

	// Read usage logs from vault
	home, _ := os.UserHomeDir()
	logDir := filepath.Join(home, "webb-vault", "improvement", "usage-logs")

	aggregates := make(map[string]*UsageAggregateEntry)
	userSets := make(map[string]map[string]bool)

	// Scan log files for the time range
	cutoff := time.Now().AddDate(0, 0, -days)
	pattern := filepath.Join(logDir, "usage-*.jsonl")
	files, _ := filepath.Glob(pattern)

	for _, file := range files {
		// Check if file is within time range
		base := filepath.Base(file)
		dateStr := strings.TrimPrefix(strings.TrimSuffix(base, ".jsonl"), "usage-")
		fileDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil || fileDate.Before(cutoff) {
			continue
		}

		// Read and parse entries
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}

		for _, line := range strings.Split(string(data), "\n") {
			if line == "" {
				continue
			}
			var entry struct {
				User     string `json:"user"`
				Tool     string `json:"tool"`
				Customer string `json:"customer"`
				Tokens   int    `json:"tokens"`
			}
			if json.Unmarshal([]byte(line), &entry) != nil {
				continue
			}

			// Determine aggregation key
			var key string
			switch groupBy {
			case "user":
				key = entry.User
			case "tool":
				key = entry.Tool
			case "customer":
				key = entry.Customer
				if key == "" {
					key = "(no customer)"
				}
			default:
				key = entry.Tool
			}

			if aggregates[key] == nil {
				aggregates[key] = &UsageAggregateEntry{Key: key}
				userSets[key] = make(map[string]bool)
			}
			aggregates[key].Count++
			aggregates[key].AvgTokens += entry.Tokens
			userSets[key][entry.User] = true
		}
	}

	// Calculate averages and user counts
	var results []UsageAggregateEntry
	for key, agg := range aggregates {
		if agg.Count > 0 && agg.AvgTokens > 0 {
			agg.AvgTokens = agg.AvgTokens / agg.Count
		}
		agg.Users = len(userSets[key])
		results = append(results, *agg)
	}

	// Sort by count
	sort.Slice(results, func(i, j int) bool {
		return results[i].Count > results[j].Count
	})

	// Format output
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Usage Aggregate (%s, grouped by %s)\n\n", timeRange, groupBy))
	sb.WriteString("| Key | Count | Users | Avg Tokens |\n")
	sb.WriteString("|-----|-------|-------|------------|\n")
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d |\n",
			r.Key, r.Count, r.Users, r.AvgTokens))
	}

	if len(results) == 0 {
		sb.WriteString("\nNo usage data found. Run `webb_log_usage` to start tracking.\n")
	}

	return tools.TextResult(sb.String()), nil
}

// Pattern Mining Handlers

func handlePatternMine(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	patternType := req.GetString("type", "all")
	minFreq := req.GetInt("min_frequency", 3)
	save := req.GetBool("save", true)

	miner := patterns.NewPatternMiner("")
	if minFreq > 0 {
		// Would need to expose this - for now use default
	}

	discoveredPatterns, err := miner.MinePatterns()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("mining patterns: %w", err)), nil
	}

	// Filter by type if specified
	if patternType != "all" {
		var filtered []patterns.DiscoveredPattern
		for _, p := range discoveredPatterns {
			if string(p.Type) == patternType {
				filtered = append(filtered, p)
			}
		}
		discoveredPatterns = filtered
	}

	if save && len(discoveredPatterns) > 0 {
		if err := miner.SavePatterns(discoveredPatterns); err != nil {
			// Log but don't fail
		}
	}

	summary := patterns.FormatPatternSummary(discoveredPatterns)
	return tools.TextResult(summary), nil
}

func handlePatternList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	patternType := req.GetString("type", "all")
	limit := req.GetInt("limit", 20)
	sortBy := req.GetString("sort", "frequency")

	miner := patterns.NewPatternMiner("")
	discoveredPatterns, err := miner.LoadPatterns()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("loading patterns: %w", err)), nil
	}

	if len(discoveredPatterns) == 0 {
		return tools.TextResult("No patterns discovered yet. Run `webb_pattern_mine` first to analyze session history."), nil
	}

	// Filter by type
	if patternType != "all" {
		var filtered []patterns.DiscoveredPattern
		for _, p := range discoveredPatterns {
			if string(p.Type) == patternType {
				filtered = append(filtered, p)
			}
		}
		discoveredPatterns = filtered
	}

	// Sort
	switch sortBy {
	case "confidence":
		sort.Slice(discoveredPatterns, func(i, j int) bool {
			return discoveredPatterns[i].Confidence > discoveredPatterns[j].Confidence
		})
	case "recent":
		sort.Slice(discoveredPatterns, func(i, j int) bool {
			return discoveredPatterns[i].LastSeen.After(discoveredPatterns[j].LastSeen)
		})
	default: // frequency
		sort.Slice(discoveredPatterns, func(i, j int) bool {
			return discoveredPatterns[i].Frequency > discoveredPatterns[j].Frequency
		})
	}

	// Limit
	if len(discoveredPatterns) > limit {
		discoveredPatterns = discoveredPatterns[:limit]
	}

	// Format output
	var sb strings.Builder
	sb.WriteString("# Workflow Patterns\n\n")
	sb.WriteString(fmt.Sprintf("Showing %d patterns (sorted by %s)\n\n", len(discoveredPatterns), sortBy))

	for i, p := range discoveredPatterns {
		sb.WriteString(fmt.Sprintf("### %d. %s\n", i+1, p.Name))
		sb.WriteString(fmt.Sprintf("- **Type:** %s\n", p.Type))
		sb.WriteString(fmt.Sprintf("- **Frequency:** %d occurrences\n", p.Frequency))
		sb.WriteString(fmt.Sprintf("- **Confidence:** %.0f%%\n", p.Confidence*100))
		sb.WriteString(fmt.Sprintf("- **Tools:** `%s`\n", strings.Join(p.ToolSequence, "` → `")))
		if p.OptimizationHint != "" {
			sb.WriteString(fmt.Sprintf("- **Hint:** %s\n", p.OptimizationHint))
		}
		sb.WriteString("\n")
	}

	return tools.TextResult(sb.String()), nil
}

func handlePatternSuggest(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := req.GetInt("limit", 5)

	miner := patterns.NewPatternMiner("")
	discoveredPatterns, err := miner.LoadPatterns()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("loading patterns: %w", err)), nil
	}

	if len(discoveredPatterns) == 0 {
		return tools.TextResult("No patterns found. Run `webb_pattern_mine` first."), nil
	}

	// Generate suggestions from high-frequency sequential patterns
	var suggestions []string
	seqCount := 0

	for _, p := range discoveredPatterns {
		if p.Type == patterns.PatternSequential && p.Frequency >= 5 {
			seqCount++
			if seqCount <= limit {
				suggestion := fmt.Sprintf("**%s** (freq: %d)\n", p.Name, p.Frequency)
				suggestion += fmt.Sprintf("  - Sequence: `%s`\n", strings.Join(p.ToolSequence, "` → `"))
				suggestion += fmt.Sprintf("  - Consider: Create a consolidated `%s` tool\n", generateConsolidatedName(p.ToolSequence))
				suggestions = append(suggestions, suggestion)
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("# Pattern-Based Optimization Suggestions\n\n")

	if len(suggestions) == 0 {
		sb.WriteString("No high-frequency patterns found for consolidation.\n")
		sb.WriteString("Keep using tools to build up pattern data.\n")
	} else {
		sb.WriteString("Based on discovered workflow patterns:\n\n")
		for _, s := range suggestions {
			sb.WriteString(s)
			sb.WriteString("\n")
		}

		sb.WriteString("\n## Next Steps\n\n")
		sb.WriteString("1. Use `webb_improvement_scaffold` to generate consolidated tool code\n")
		sb.WriteString("2. Review the generated scaffold and customize as needed\n")
		sb.WriteString("3. Add tests and integrate into the appropriate module\n")
	}

	return tools.TextResult(sb.String()), nil
}

func generateConsolidatedName(tools []string) string {
	if len(tools) == 0 {
		return "webb_workflow"
	}

	// Find common prefix
	categories := make(map[string]int)
	for _, t := range tools {
		parts := strings.Split(strings.TrimPrefix(t, "webb_"), "_")
		if len(parts) > 0 {
			categories[parts[0]]++
		}
	}

	var dominant string
	maxCount := 0
	for cat, count := range categories {
		if count > maxCount {
			dominant = cat
			maxCount = count
		}
	}

	return fmt.Sprintf("webb_%s_workflow", dominant)
}

// handlePatternToChain converts a discovered pattern to a workflow chain
func handlePatternToChain(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	patternID := req.GetString("pattern_id", "")
	chainName := req.GetString("chain_name", "")
	save := req.GetBool("save", false)
	register := req.GetBool("register", false)

	if patternID == "" {
		return tools.ErrorResult(fmt.Errorf("pattern_id is required")), nil
	}

	// Load patterns
	miner := patterns.NewPatternMiner("")
	discoveredPatterns, err := miner.LoadPatterns()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("loading patterns: %w", err)), nil
	}

	// Find the pattern
	var targetPattern *patterns.DiscoveredPattern
	for i := range discoveredPatterns {
		if discoveredPatterns[i].ID == patternID {
			targetPattern = &discoveredPatterns[i]
			break
		}
	}

	if targetPattern == nil {
		return tools.ErrorResult(fmt.Errorf("pattern %q not found. Use webb_pattern_list to see available patterns", patternID)), nil
	}

	// Get chain registry (may be nil for preview mode)
	chainRegistry := chains.NewRegistry()
	converter := chains.NewPatternConverter(chainRegistry, "")

	var sb strings.Builder
	sb.WriteString("# Pattern to Chain Conversion\n\n")

	if save || register {
		var result *chains.ConvertResult
		var err error

		if save && register {
			result, err = converter.SaveAndRegister(*targetPattern, chainName)
		} else if save {
			result, err = converter.SaveAsYAML(*targetPattern, chainName)
		} else {
			result, err = converter.RegisterChain(*targetPattern, chainName)
		}

		if err != nil {
			return tools.ErrorResult(fmt.Errorf("converting pattern: %w", err)), nil
		}

		sb.WriteString(fmt.Sprintf("**Pattern:** %s\n", targetPattern.Name))
		sb.WriteString(fmt.Sprintf("**Chain Name:** `%s`\n", result.ChainName))

		if result.SavedPath != "" {
			sb.WriteString(fmt.Sprintf("**Saved To:** %s\n", result.SavedPath))
		}
		if result.Registered {
			sb.WriteString("**Status:** Registered in chain registry\n")
		}

		sb.WriteString("\n## Chain Definition\n\n")
		sb.WriteString("```yaml\n")
		sb.WriteString(fmt.Sprintf("name: %s\n", result.Chain.Name))
		sb.WriteString(fmt.Sprintf("description: %s\n", result.Chain.Description))
		sb.WriteString(fmt.Sprintf("category: %s\n", result.Chain.Category))
		sb.WriteString("steps:\n")
		for _, step := range result.Chain.Steps {
			sb.WriteString(fmt.Sprintf("  - id: %s\n", step.ID))
			sb.WriteString(fmt.Sprintf("    tool: %s\n", step.Tool))
		}
		sb.WriteString("```\n")
	} else {
		// Preview mode
		chain := converter.ConvertPattern(*targetPattern, chainName)

		sb.WriteString("## Preview (not saved)\n\n")
		sb.WriteString(fmt.Sprintf("**Pattern:** %s\n", targetPattern.Name))
		sb.WriteString(fmt.Sprintf("**Frequency:** %d\n", targetPattern.Frequency))
		sb.WriteString(fmt.Sprintf("**Success Rate:** %.0f%%\n\n", targetPattern.SuccessRate*100))

		sb.WriteString("### Generated Chain\n\n")
		sb.WriteString("```yaml\n")
		sb.WriteString(fmt.Sprintf("name: %s\n", chain.Name))
		sb.WriteString(fmt.Sprintf("description: %s\n", chain.Description))
		sb.WriteString(fmt.Sprintf("category: %s\n", chain.Category))
		sb.WriteString(fmt.Sprintf("timeout: %s\n", chain.Timeout))
		sb.WriteString("tags:\n")
		for _, tag := range chain.Tags {
			sb.WriteString(fmt.Sprintf("  - %s\n", tag))
		}
		sb.WriteString("steps:\n")
		for _, step := range chain.Steps {
			sb.WriteString(fmt.Sprintf("  - id: %s\n", step.ID))
			sb.WriteString(fmt.Sprintf("    name: %s\n", step.Name))
			sb.WriteString(fmt.Sprintf("    tool: %s\n", step.Tool))
			sb.WriteString(fmt.Sprintf("    store_as: %s\n", step.StoreAs))
		}
		sb.WriteString("```\n")

		sb.WriteString("\n### To Save\n\n")
		sb.WriteString("```\n")
		sb.WriteString(fmt.Sprintf("webb_pattern_to_chain(pattern_id=\"%s\", save=true)\n", patternID))
		sb.WriteString("```\n")
	}

	return tools.TextResult(sb.String()), nil
}

// =============================================================================
// Self-Healing Handlers (v7.80)
// =============================================================================

func handleSelfHealingDashboard(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	service := req.GetString("service", "all")
	historyLimit := int(req.GetFloat("history_limit", 10))

	client := clients.GetSelfHealingClient()
	if client == nil {
		return tools.ErrorResult(fmt.Errorf("self-healing client not available")), nil
	}

	var sb strings.Builder
	sb.WriteString("# Self-Healing Dashboard\n\n")

	// Config status
	config := client.GetConfig()
	sb.WriteString("## Configuration\n\n")
	sb.WriteString(fmt.Sprintf("| Setting | Value |\n"))
	sb.WriteString(fmt.Sprintf("|---------|-------|\n"))
	sb.WriteString(fmt.Sprintf("| Enabled | %v |\n", config.Enabled))
	sb.WriteString(fmt.Sprintf("| Max Auto-Risk | %d |\n", config.MaxAutoRiskScore))
	sb.WriteString(fmt.Sprintf("| Require Approval Above | %d |\n", config.RequireApprovalAbove))
	sb.WriteString(fmt.Sprintf("| Learning Enabled | %v |\n", config.LearningEnabled))
	sb.WriteString("\n")

	// Service health
	sb.WriteString("## Service Health\n\n")
	healthMap := client.GetAllServiceHealth()
	if len(healthMap) == 0 {
		sb.WriteString("*No service health data available yet*\n\n")
	} else {
		sb.WriteString("| Service | Health | Score | Circuit | Last Check |\n")
		sb.WriteString("|---------|--------|-------|---------|------------|\n")
		for svc, health := range healthMap {
			if service != "all" && svc != service {
				continue
			}
			healthIcon := "✅"
			if !health.Healthy {
				healthIcon = "❌"
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %d | %s | %s |\n",
				svc, healthIcon, health.HealthScore, health.CircuitState,
				health.LastCheck.Format("15:04:05")))
		}
		sb.WriteString("\n")
	}

	// Pending approvals
	pending, err := client.GetPendingApprovals()
	if err == nil && len(pending) > 0 {
		sb.WriteString("## ⚠️ Pending Approvals\n\n")
		sb.WriteString("| ID | Playbook | Service | Risk | Requested |\n")
		sb.WriteString("|----|----------|---------|------|----------|\n")
		for _, exec := range pending {
			shortID := exec.ID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %s |\n",
				shortID, exec.PlaybookName, exec.Service, exec.RiskScore,
				exec.StartedAt.Format("15:04")))
		}
		sb.WriteString("\n**To approve:** `webb_remediation_approve(execution_id=\"ID\", action=\"approve\")`\n\n")
	}

	// Recent executions
	sb.WriteString("## Recent Executions\n\n")
	executions, err := client.GetRecentExecutions(historyLimit)
	if err != nil {
		sb.WriteString(fmt.Sprintf("*Error loading executions: %v*\n", err))
	} else if len(executions) == 0 {
		sb.WriteString("*No recent executions*\n")
	} else {
		sb.WriteString("| Status | Playbook | Service | Risk | Duration | Time |\n")
		sb.WriteString("|--------|----------|---------|------|----------|------|\n")
		for _, exec := range executions {
			statusIcon := "⏳"
			switch exec.Status {
			case "success":
				statusIcon = "✅"
			case "failed":
				statusIcon = "❌"
			case "cancelled":
				statusIcon = "⛔"
			case "approval_required":
				statusIcon = "⚠️"
			}
			duration := exec.Duration
			if duration == "" {
				duration = "-"
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %s | %d | %s | %s |\n",
				statusIcon, exec.PlaybookName, exec.Service, exec.RiskScore,
				duration, exec.StartedAt.Format("Jan 2 15:04")))
		}
	}

	// Playbook summary
	sb.WriteString("\n## Playbook Summary\n\n")
	playbooks := client.GetPlaybooks()
	sb.WriteString(fmt.Sprintf("**Total Playbooks:** %d\n\n", len(playbooks)))

	riskBuckets := map[string]int{"low (0-20)": 0, "medium (21-50)": 0, "high (51-100)": 0}
	for _, pb := range playbooks {
		if pb.RiskScore <= 20 {
			riskBuckets["low (0-20)"]++
		} else if pb.RiskScore <= 50 {
			riskBuckets["medium (21-50)"]++
		} else {
			riskBuckets["high (51-100)"]++
		}
	}
	sb.WriteString("| Risk Level | Count |\n")
	sb.WriteString("|------------|-------|\n")
	sb.WriteString(fmt.Sprintf("| Low (0-20) | %d |\n", riskBuckets["low (0-20)"]))
	sb.WriteString(fmt.Sprintf("| Medium (21-50) | %d |\n", riskBuckets["medium (21-50)"]))
	sb.WriteString(fmt.Sprintf("| High (51-100) | %d |\n", riskBuckets["high (51-100)"]))

	return tools.TextResult(sb.String()), nil
}

func handlePlaybookList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	service := req.GetString("service", "")
	triggerType := req.GetString("trigger_type", "")

	client := clients.GetSelfHealingClient()
	if client == nil {
		return tools.ErrorResult(fmt.Errorf("self-healing client not available")), nil
	}

	var playbooks []*clients.RemediationPlaybook
	if service != "" && service != "*" {
		playbooks = client.GetPlaybooksByService(service)
	} else {
		playbooks = client.GetPlaybooks()
	}

	// Filter by trigger type if specified
	if triggerType != "" {
		filtered := make([]*clients.RemediationPlaybook, 0)
		for _, pb := range playbooks {
			if pb.TriggerType == triggerType {
				filtered = append(filtered, pb)
			}
		}
		playbooks = filtered
	}

	var sb strings.Builder
	sb.WriteString("# Remediation Playbooks\n\n")

	if len(playbooks) == 0 {
		sb.WriteString("*No playbooks found matching criteria*\n")
		return tools.TextResult(sb.String()), nil
	}

	sb.WriteString("| ID | Name | Service | Trigger | Risk | Success Rate | Executions |\n")
	sb.WriteString("|----|------|---------|---------|------|--------------|------------|\n")

	for _, pb := range playbooks {
		riskLevel := "🟢"
		if pb.RiskScore > 50 {
			riskLevel = "🔴"
		} else if pb.RiskScore > 20 {
			riskLevel = "🟡"
		}
		successRate := fmt.Sprintf("%.0f%%", pb.SuccessRate*100)
		if pb.ExecCount == 0 {
			successRate = "-"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s %d | %s | %d |\n",
			pb.ID, pb.Name, pb.Service, pb.TriggerType,
			riskLevel, pb.RiskScore, successRate, pb.ExecCount))
	}

	sb.WriteString("\n## Risk Legend\n\n")
	sb.WriteString("- 🟢 **Low (0-20):** Auto-executed without approval\n")
	sb.WriteString("- 🟡 **Medium (21-50):** Manual trigger only\n")
	sb.WriteString("- 🔴 **High (51-100):** Requires explicit approval\n")

	sb.WriteString("\n## Usage\n\n")
	sb.WriteString("```\n")
	sb.WriteString("# Manual execution (with dry-run)\n")
	sb.WriteString("webb_playbook_execute(playbook_id=\"rabbitmq-restart-consumer\")\n\n")
	sb.WriteString("# Execute for real\n")
	sb.WriteString("webb_playbook_execute(playbook_id=\"rabbitmq-restart-consumer\", dry_run=false)\n")
	sb.WriteString("```\n")

	return tools.TextResult(sb.String()), nil
}

func handlePlaybookExecute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	playbookID := req.GetString("playbook_id", "")
	service := req.GetString("service", "")
	dryRun := req.GetBool("dry_run", true)

	if playbookID == "" {
		return tools.ErrorResult(fmt.Errorf("playbook_id is required")), nil
	}

	client := clients.GetSelfHealingClient()
	if client == nil {
		return tools.ErrorResult(fmt.Errorf("self-healing client not available")), nil
	}

	// Find the playbook
	playbooks := client.GetPlaybooks()
	var targetPlaybook *clients.RemediationPlaybook
	for _, pb := range playbooks {
		if pb.ID == playbookID {
			targetPlaybook = pb
			break
		}
	}

	if targetPlaybook == nil {
		return tools.ErrorResult(fmt.Errorf("playbook %q not found", playbookID)), nil
	}

	var sb strings.Builder

	if dryRun {
		sb.WriteString("# Playbook Execution Preview (Dry Run)\n\n")
		sb.WriteString(fmt.Sprintf("**Playbook:** %s\n", targetPlaybook.Name))
		sb.WriteString(fmt.Sprintf("**ID:** %s\n", targetPlaybook.ID))
		sb.WriteString(fmt.Sprintf("**Service:** %s\n", targetPlaybook.Service))
		sb.WriteString(fmt.Sprintf("**Risk Score:** %d\n", targetPlaybook.RiskScore))
		sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", targetPlaybook.Description))

		sb.WriteString("## Steps to Execute\n\n")
		for i, step := range targetPlaybook.Steps {
			sb.WriteString(fmt.Sprintf("%d. **%s** (%s)\n", i+1, step.Name, step.Type))
			if step.ToolName != "" {
				sb.WriteString(fmt.Sprintf("   - Tool: `%s`\n", step.ToolName))
			}
			if step.Command != "" {
				sb.WriteString(fmt.Sprintf("   - Command: `%s`\n", step.Command))
			}
			if step.OnError != "" {
				sb.WriteString(fmt.Sprintf("   - On Error: %s\n", step.OnError))
			}
		}

		sb.WriteString("\n## Risk Assessment\n\n")
		config := client.GetConfig()
		if targetPlaybook.RiskScore <= config.MaxAutoRiskScore {
			sb.WriteString("✅ This playbook can auto-execute (low risk)\n")
		} else if targetPlaybook.RiskScore <= config.RequireApprovalAbove {
			sb.WriteString("⚠️ This playbook requires manual trigger (medium risk)\n")
		} else {
			sb.WriteString("🔴 This playbook requires explicit approval (high risk)\n")
		}

		sb.WriteString("\n**To execute for real:**\n```\n")
		sb.WriteString(fmt.Sprintf("webb_playbook_execute(playbook_id=\"%s\", dry_run=false)\n", playbookID))
		sb.WriteString("```\n")

		return tools.TextResult(sb.String()), nil
	}

	// Actually execute
	if service == "" {
		service = targetPlaybook.Service
	}

	exec, err := client.TriggerManualRemediation(playbookID, service)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to trigger remediation: %w", err)), nil
	}

	sb.WriteString("# Playbook Execution Triggered\n\n")
	sb.WriteString(fmt.Sprintf("**Execution ID:** %s\n", exec.ID))
	sb.WriteString(fmt.Sprintf("**Playbook:** %s\n", exec.PlaybookName))
	sb.WriteString(fmt.Sprintf("**Service:** %s\n", exec.Service))
	sb.WriteString(fmt.Sprintf("**Status:** %s\n", exec.Status))
	sb.WriteString(fmt.Sprintf("**Risk Score:** %d\n\n", exec.RiskScore))

	if exec.Status == "approval_required" {
		sb.WriteString("⚠️ **Approval Required**\n\n")
		sb.WriteString("This playbook requires approval before execution.\n\n")
		sb.WriteString("```\n")
		sb.WriteString(fmt.Sprintf("webb_remediation_approve(execution_id=\"%s\", action=\"approve\")\n", exec.ID))
		sb.WriteString("```\n")
	} else {
		sb.WriteString("✅ Execution started. Monitor with:\n\n")
		sb.WriteString("```\n")
		sb.WriteString("webb_self_healing_dashboard()\n")
		sb.WriteString("```\n")
	}

	return tools.TextResult(sb.String()), nil
}

func handleRemediationApprove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	executionID := req.GetString("execution_id", "")
	action := req.GetString("action", "")
	reason := req.GetString("reason", "")

	if executionID == "" {
		return tools.ErrorResult(fmt.Errorf("execution_id is required")), nil
	}
	if action != "approve" && action != "reject" {
		return tools.ErrorResult(fmt.Errorf("action must be 'approve' or 'reject'")), nil
	}

	client := clients.GetSelfHealingClient()
	if client == nil {
		return tools.ErrorResult(fmt.Errorf("self-healing client not available")), nil
	}

	var sb strings.Builder

	if action == "approve" {
		err := client.ApproveExecution(executionID, "webb-operator")
		if err != nil {
			return tools.ErrorResult(fmt.Errorf("failed to approve: %w", err)), nil
		}
		sb.WriteString("# Remediation Approved ✅\n\n")
		sb.WriteString(fmt.Sprintf("**Execution ID:** %s\n", executionID))
		sb.WriteString("**Status:** Execution started\n\n")
		sb.WriteString("Monitor progress with `webb_self_healing_dashboard()`\n")
	} else {
		if reason == "" {
			return tools.ErrorResult(fmt.Errorf("reason is required when rejecting")), nil
		}
		err := client.RejectExecution(executionID, reason)
		if err != nil {
			return tools.ErrorResult(fmt.Errorf("failed to reject: %w", err)), nil
		}
		sb.WriteString("# Remediation Rejected ⛔\n\n")
		sb.WriteString(fmt.Sprintf("**Execution ID:** %s\n", executionID))
		sb.WriteString(fmt.Sprintf("**Reason:** %s\n", reason))
	}

	return tools.TextResult(sb.String()), nil
}

func handleSelfHealingConfig(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := req.GetString("action", "view")

	client := clients.GetSelfHealingClient()
	if client == nil {
		return tools.ErrorResult(fmt.Errorf("self-healing client not available")), nil
	}

	config := client.GetConfig()

	if action == "update" {
		// Apply updates from params
		if v := req.GetFloat("max_auto_risk", -1); v >= 0 {
			config.MaxAutoRiskScore = int(v)
		}
		if v := req.GetFloat("require_approval_above", -1); v >= 0 {
			config.RequireApprovalAbove = int(v)
		}
		if args, ok := req.Params.Arguments.(map[string]interface{}); ok {
			if enabled, exists := args["enabled"].(bool); exists {
				config.Enabled = enabled
			}
			if learning, exists := args["learning_enabled"].(bool); exists {
				config.LearningEnabled = learning
			}
		}

		if err := client.UpdateConfig(config); err != nil {
			return tools.ErrorResult(fmt.Errorf("failed to update config: %w", err)), nil
		}
	}

	var sb strings.Builder
	sb.WriteString("# Self-Healing Configuration\n\n")

	if action == "update" {
		sb.WriteString("✅ **Configuration updated**\n\n")
	}

	sb.WriteString("| Setting | Value | Description |\n")
	sb.WriteString("|---------|-------|-------------|\n")
	sb.WriteString(fmt.Sprintf("| enabled | %v | Self-healing active |\n", config.Enabled))
	sb.WriteString(fmt.Sprintf("| max_auto_risk | %d | Max risk for auto-execution |\n", config.MaxAutoRiskScore))
	sb.WriteString(fmt.Sprintf("| require_approval_above | %d | Risk threshold for approval |\n", config.RequireApprovalAbove))
	sb.WriteString(fmt.Sprintf("| max_retry_attempts | %d | Retries per remediation |\n", config.MaxRetryAttempts))
	sb.WriteString(fmt.Sprintf("| retry_backoff_seconds | %d | Backoff between retries |\n", config.RetryBackoffSeconds))
	sb.WriteString(fmt.Sprintf("| cooldown_minutes | %d | Cooldown after remediation |\n", config.CooldownMinutes))
	sb.WriteString(fmt.Sprintf("| learning_enabled | %v | Learn from outcomes |\n", config.LearningEnabled))
	sb.WriteString(fmt.Sprintf("| health_check_interval | %ds | Health check frequency |\n", config.HealthCheckInterval))
	sb.WriteString(fmt.Sprintf("| circuit_breaker_enabled | %v | Use circuit breaker |\n", config.CircuitBreakerEnabled))

	sb.WriteString("\n## Risk Thresholds\n\n")
	sb.WriteString(fmt.Sprintf("- **Auto-execute (0-%d):** Low-risk playbooks run automatically\n", config.MaxAutoRiskScore))
	sb.WriteString(fmt.Sprintf("- **Manual trigger (%d-%d):** Requires explicit invocation\n", config.MaxAutoRiskScore+1, config.RequireApprovalAbove))
	sb.WriteString(fmt.Sprintf("- **Approval required (%d-100):** Human approval before execution\n", config.RequireApprovalAbove+1))

	sb.WriteString("\n## Update Example\n\n")
	sb.WriteString("```\n")
	sb.WriteString("webb_self_healing_config(action=\"update\", max_auto_risk=30, learning_enabled=true)\n")
	sb.WriteString("```\n")

	return tools.TextResult(sb.String()), nil
}

func handleRemediationHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := int(req.GetFloat("limit", 20))
	statusFilter := req.GetString("status", "all")
	serviceFilter := req.GetString("service", "")

	client := clients.GetSelfHealingClient()
	if client == nil {
		return tools.ErrorResult(fmt.Errorf("self-healing client not available")), nil
	}

	executions, err := client.GetRecentExecutions(limit * 2) // Get extra for filtering
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load history: %w", err)), nil
	}

	// Filter
	filtered := make([]clients.RemediationExecution, 0)
	for _, exec := range executions {
		if statusFilter != "all" && exec.Status != statusFilter {
			continue
		}
		if serviceFilter != "" && exec.Service != serviceFilter {
			continue
		}
		filtered = append(filtered, exec)
		if len(filtered) >= limit {
			break
		}
	}

	var sb strings.Builder
	sb.WriteString("# Remediation History\n\n")

	if len(filtered) == 0 {
		sb.WriteString("*No executions found matching criteria*\n")
		return tools.TextResult(sb.String()), nil
	}

	// Summary stats
	successCount := 0
	failedCount := 0
	for _, exec := range filtered {
		if exec.Status == "success" {
			successCount++
		} else if exec.Status == "failed" {
			failedCount++
		}
	}
	if len(filtered) > 0 {
		sb.WriteString(fmt.Sprintf("**Showing:** %d executions | ", len(filtered)))
		sb.WriteString(fmt.Sprintf("**Success:** %d | ", successCount))
		sb.WriteString(fmt.Sprintf("**Failed:** %d\n\n", failedCount))
	}

	sb.WriteString("| Time | Status | Playbook | Service | Risk | Duration | Auto |\n")
	sb.WriteString("|------|--------|----------|---------|------|----------|------|\n")

	for _, exec := range filtered {
		statusIcon := "⏳"
		switch exec.Status {
		case "success":
			statusIcon = "✅"
		case "failed":
			statusIcon = "❌"
		case "cancelled":
			statusIcon = "⛔"
		case "approval_required":
			statusIcon = "⚠️"
		}

		duration := exec.Duration
		if duration == "" {
			duration = "-"
		}

		autoIcon := ""
		if exec.AutoExecuted {
			autoIcon = "🤖"
		}

		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %d | %s | %s |\n",
			exec.StartedAt.Format("Jan 2 15:04"),
			statusIcon, exec.PlaybookName, exec.Service,
			exec.RiskScore, duration, autoIcon))
	}

	sb.WriteString("\n**Legend:** 🤖 = Auto-executed\n")

	return tools.TextResult(sb.String()), nil
}

// =============================================================================
// Weekly Digest Handlers (v7.90)
// =============================================================================

func handleWeeklyDigest(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	week := req.GetString("week", "current")
	save := req.GetBool("save", true)
	format := req.GetString("format", "markdown")

	// Get or create digest client
	digestClient := clients.GetWeeklyDigestClient()
	if digestClient == nil {
		return tools.ErrorResult(fmt.Errorf("digest client not available")), nil
	}

	// Parse week parameter
	var weekStart time.Time
	now := time.Now()
	switch week {
	case "current":
		// Start of current week (Monday)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		weekStart = now.AddDate(0, 0, -weekday+1).Truncate(24 * time.Hour)
	case "last":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		weekStart = now.AddDate(0, 0, -weekday-6).Truncate(24 * time.Hour)
	default:
		// Try parsing as date
		parsed, err := time.Parse("2006-01-02", week)
		if err != nil {
			return tools.ErrorResult(fmt.Errorf("invalid week format: %s (use current, last, or YYYY-MM-DD)", week)), nil
		}
		weekStart = parsed
	}

	// Generate digest (client computes weekStart internally)
	_ = weekStart // Week start calculated by client
	digest, err := digestClient.GenerateDigest(ctx)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("generating digest: %w", err)), nil
	}

	// Digest is auto-saved to vault if configured, just note the path
	_ = save

	// Format output
	if format == "json" {
		data, _ := json.MarshalIndent(digest, "", "  ")
		return tools.TextResult(string(data)), nil
	}

	// Markdown format
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Weekly Improvement Digest\n\n"))
	sb.WriteString(fmt.Sprintf("**Week:** %s to %s\n\n", digest.WeekStart.Format("Jan 2"), digest.WeekEnd.Format("Jan 2, 2006")))

	// Summary
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- **New Opportunities:** %d\n", digest.Summary.NewOpportunities))
	sb.WriteString(fmt.Sprintf("- **Shipped:** %d\n", digest.Summary.ShippedImprovements))
	sb.WriteString(fmt.Sprintf("- **Total Token Savings:** %d\n", digest.Summary.EstimatedTokenSavings))
	topOpp := "None"
	if len(digest.Opportunities) > 0 {
		topOpp = digest.Opportunities[0].Title
	}
	sb.WriteString(fmt.Sprintf("- **Top Opportunity:** %s\n\n", topOpp))

	// Velocity
	sb.WriteString("## Velocity Metrics\n\n")
	sb.WriteString(fmt.Sprintf("- **Shipped This Week:** %d (vs %d last week)\n", digest.Velocity.ShippedThisWeek, digest.Velocity.ShippedLastWeek))
	sb.WriteString(fmt.Sprintf("- **Shipped This Month:** %d\n", digest.Velocity.ShippedThisMonth))
	sb.WriteString(fmt.Sprintf("- **Avg Time to Ship:** %.1f days\n", digest.Velocity.AvgTimeToShipDays))
	sb.WriteString(fmt.Sprintf("- **Backlog Size:** %d\n", digest.Velocity.BacklogSize))
	sb.WriteString(fmt.Sprintf("- **Trend:** %s\n\n", digest.Velocity.VelocityTrend))

	// Top Opportunities
	if len(digest.Opportunities) > 0 {
		sb.WriteString("## Top Opportunities\n\n")
		sb.WriteString("| Score | Type | Title | Impact | Effort |\n")
		sb.WriteString("|-------|------|-------|--------|--------|\n")
		limit := 10
		if len(digest.Opportunities) < limit {
			limit = len(digest.Opportunities)
		}
		for _, opp := range digest.Opportunities[:limit] {
			sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s |\n",
				opp.Score, opp.Type, opp.Title, opp.Impact, opp.Effort))
		}
		sb.WriteString("\n")
	}

	// Trends
	if len(digest.Trends) > 0 {
		sb.WriteString("## Trends\n\n")
		for _, trend := range digest.Trends {
			sb.WriteString(fmt.Sprintf("- **%s:** %s (confidence: %.0f%%)\n",
				trend.Type, trend.Description, trend.Confidence*100))
		}
	}

	return tools.TextResult(sb.String()), nil
}

func handleDigestHistory(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := int(req.GetFloat("limit", 4))
	includeOpportunities := req.GetBool("include_opportunities", false)

	digestClient := clients.GetWeeklyDigestClient()
	if digestClient == nil {
		return tools.ErrorResult(fmt.Errorf("digest client not available")), nil
	}

	digests, err := digestClient.GetRecentDigests(limit)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("fetching digest history: %w", err)), nil
	}

	if len(digests) == 0 {
		return tools.TextResult("No weekly digests found. Run webb_weekly_digest to generate one."), nil
	}

	var sb strings.Builder
	sb.WriteString("# Weekly Digest History\n\n")

	for _, digest := range digests {
		sb.WriteString(fmt.Sprintf("## Week of %s\n\n", digest.WeekStart.Format("Jan 2, 2006")))
		sb.WriteString(fmt.Sprintf("- **Opportunities Found:** %d\n", digest.Summary.NewOpportunities))
		sb.WriteString(fmt.Sprintf("- **Items Shipped:** %d\n", digest.Summary.ShippedImprovements))
		sb.WriteString(fmt.Sprintf("- **Token Savings:** %d\n", digest.Summary.EstimatedTokenSavings))
		sb.WriteString(fmt.Sprintf("- **Velocity Trend:** %s\n", digest.Velocity.VelocityTrend))

		if includeOpportunities && len(digest.Opportunities) > 0 {
			sb.WriteString("\n**Opportunities:**\n")
			for i, opp := range digest.Opportunities {
				if i >= 5 {
					sb.WriteString(fmt.Sprintf("... and %d more\n", len(digest.Opportunities)-5))
					break
				}
				sb.WriteString(fmt.Sprintf("- [%d] %s (%s)\n", opp.Score, opp.Title, opp.Status))
			}
		}
		sb.WriteString("\n---\n\n")
	}

	return tools.TextResult(sb.String()), nil
}

func handleOpportunityStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	opportunityID := req.GetString("opportunity_id", "")
	status := req.GetString("status", "")
	notes := req.GetString("notes", "")

	if opportunityID == "" {
		return tools.ErrorResult(fmt.Errorf("opportunity_id is required")), nil
	}
	if status == "" {
		return tools.ErrorResult(fmt.Errorf("status is required")), nil
	}

	// Validate status
	validStatuses := map[string]bool{"new": true, "in_progress": true, "shipped": true, "deferred": true}
	if !validStatuses[status] {
		return tools.ErrorResult(fmt.Errorf("invalid status: %s (valid: new, in_progress, shipped, deferred)", status)), nil
	}

	digestClient := clients.GetWeeklyDigestClient()
	if digestClient == nil {
		return tools.ErrorResult(fmt.Errorf("digest client not available")), nil
	}

	// notes is for documentation but not stored currently
	_ = notes
	if err := digestClient.UpdateOpportunityStatus(opportunityID, status, 0); err != nil {
		return tools.ErrorResult(fmt.Errorf("updating opportunity: %w", err)), nil
	}

	return tools.TextResult(fmt.Sprintf("Updated opportunity %s to status: %s", opportunityID, status)), nil
}

func handleVelocityReport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	period := req.GetString("period", "month")
	includeBreakdown := req.GetBool("include_breakdown", true)

	digestClient := clients.GetWeeklyDigestClient()
	if digestClient == nil {
		return tools.ErrorResult(fmt.Errorf("digest client not available")), nil
	}

	velocity, err := digestClient.GetVelocityReport(period)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("generating velocity report: %w", err)), nil
	}

	var sb strings.Builder
	sb.WriteString("# Improvement Velocity Report\n\n")
	sb.WriteString(fmt.Sprintf("**Period:** %s\n\n", period))

	sb.WriteString("## Metrics\n\n")
	sb.WriteString(fmt.Sprintf("- **Shipped This Week:** %d\n", velocity.ShippedThisWeek))
	sb.WriteString(fmt.Sprintf("- **Shipped Last Week:** %d\n", velocity.ShippedLastWeek))
	sb.WriteString(fmt.Sprintf("- **Shipped This Month:** %d\n", velocity.ShippedThisMonth))
	sb.WriteString(fmt.Sprintf("- **Avg Time to Ship:** %.1f days\n", velocity.AvgTimeToShipDays))
	sb.WriteString(fmt.Sprintf("- **Current Backlog:** %d items\n", velocity.BacklogSize))
	sb.WriteString(fmt.Sprintf("- **Velocity Trend:** %s\n\n", velocity.VelocityTrend))

	if includeBreakdown {
		breakdown, err := digestClient.GetOpportunityBreakdown()
		if err == nil && len(breakdown) > 0 {
			sb.WriteString("## Breakdown by Type\n\n")
			sb.WriteString("| Type | New | In Progress | Shipped | Deferred |\n")
			sb.WriteString("|------|-----|-------------|---------|----------|\n")
			for oppType, counts := range breakdown {
				sb.WriteString(fmt.Sprintf("| %s | %d | %d | %d | %d |\n",
					oppType,
					counts["new"],
					counts["in_progress"],
					counts["shipped"],
					counts["deferred"]))
			}
		}
	}

	return tools.TextResult(sb.String()), nil
}

func handleOpportunitiesList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	oppType := req.GetString("type", "all")
	status := req.GetString("status", "new")
	limit := int(req.GetFloat("limit", 20))
	sortBy := req.GetString("sort", "score")

	digestClient := clients.GetWeeklyDigestClient()
	if digestClient == nil {
		return tools.ErrorResult(fmt.Errorf("digest client not available")), nil
	}

	opportunities, err := digestClient.ListOpportunities(oppType, status, limit, sortBy)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("listing opportunities: %w", err)), nil
	}

	if len(opportunities) == 0 {
		return tools.TextResult(fmt.Sprintf("No opportunities found with type=%s, status=%s", oppType, status)), nil
	}

	var sb strings.Builder
	sb.WriteString("# Improvement Opportunities\n\n")
	sb.WriteString(fmt.Sprintf("**Filter:** type=%s, status=%s, sort=%s\n\n", oppType, status, sortBy))

	sb.WriteString("| Score | Type | Title | Impact | Effort | Status |\n")
	sb.WriteString("|-------|------|-------|--------|--------|--------|\n")

	for _, opp := range opportunities {
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s | %s |\n",
			opp.Score, opp.Type, opp.Title, opp.Impact, opp.Effort, opp.Status))
	}

	sb.WriteString(fmt.Sprintf("\n**Total:** %d opportunities\n", len(opportunities)))

	return tools.TextResult(sb.String()), nil
}

// =============================================================================
// Latency-Aware Routing Handlers (v7.95)
// =============================================================================

func handleLatencyDashboard(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	includeSlowTools := req.GetBool("include_slow_tools", true)
	alertLimit := int(req.GetFloat("alert_limit", 5))

	latencyTracker := clients.GetLatencyTrackerClient()
	if latencyTracker == nil {
		return tools.ErrorResult(fmt.Errorf("latency tracker not available")), nil
	}

	dashboard, err := latencyTracker.GetLatencyDashboard()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("getting dashboard: %w", err)), nil
	}

	var sb strings.Builder
	sb.WriteString("# Latency Dashboard\n\n")

	// Profile counts
	profileCounts := dashboard["profile_counts"].(map[string]int)
	sb.WriteString("## Tool Profiles\n\n")
	sb.WriteString(fmt.Sprintf("- **Fast (<100ms):** %d tools\n", profileCounts["fast"]))
	sb.WriteString(fmt.Sprintf("- **Medium (100ms-1s):** %d tools\n", profileCounts["medium"]))
	sb.WriteString(fmt.Sprintf("- **Slow (>1s):** %d tools\n", profileCounts["slow"]))
	sb.WriteString(fmt.Sprintf("- **Total tracked:** %d tools\n\n", dashboard["total_tools"]))

	// Sessions
	sb.WriteString("## Session Budgets\n\n")
	sb.WriteString(fmt.Sprintf("- **Active sessions:** %d\n", dashboard["active_sessions"]))
	sb.WriteString(fmt.Sprintf("- **Exhausted budgets:** %d\n\n", dashboard["exhausted_sessions"]))

	// Pending alerts
	pendingAlerts := dashboard["pending_alerts"].(int)
	if pendingAlerts > 0 {
		sb.WriteString(fmt.Sprintf("## Slow Query Alerts (%d pending)\n\n", pendingAlerts))
		alerts := dashboard["alerts"].([]clients.SlowQueryAlert)
		limit := alertLimit
		if len(alerts) < limit {
			limit = len(alerts)
		}
		sb.WriteString("| Time | Tool | Latency | Threshold |\n")
		sb.WriteString("|------|------|---------|----------|\n")
		for _, a := range alerts[:limit] {
			sb.WriteString(fmt.Sprintf("| %s | %s | %dms | %dms |\n",
				a.Timestamp.Format("Jan 2 15:04"), a.ToolName, a.LatencyMs, a.ThresholdMs))
		}
		sb.WriteString("\n")
	}

	// Slow tools
	if includeSlowTools {
		slowTools := dashboard["slow_tools"].([]clients.ToolLatencyStats)
		if len(slowTools) > 0 {
			sb.WriteString("## Slowest Tools\n\n")
			sb.WriteString("| Tool | P50 | P95 | Trend | Alternatives |\n")
			sb.WriteString("|------|-----|-----|-------|-------------|\n")
			for _, t := range slowTools {
				altStr := "-"
				if len(t.Alternatives) > 0 {
					altStr = strings.Join(t.Alternatives[:common.MinInt(2, len(t.Alternatives))], ", ")
				}
				trendIcon := ""
				if t.TrendPercent > 10 {
					trendIcon = " ↑"
				} else if t.TrendPercent < -10 {
					trendIcon = " ↓"
				}
				sb.WriteString(fmt.Sprintf("| %s | %dms | %dms | %.0f%%%s | %s |\n",
					t.ToolName, t.P50LatencyMs, t.P95LatencyMs, t.TrendPercent, trendIcon, altStr))
			}
		}
	}

	return tools.TextResult(sb.String()), nil
}

func handleToolLatency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	toolName := req.GetString("tool", "")
	if toolName == "" {
		return tools.ErrorResult(fmt.Errorf("tool name is required")), nil
	}

	// Ensure tool name has prefix
	if !strings.HasPrefix(toolName, "webb_") {
		toolName = "webb_" + toolName
	}

	latencyTracker := clients.GetLatencyTrackerClient()
	if latencyTracker == nil {
		return tools.ErrorResult(fmt.Errorf("latency tracker not available")), nil
	}

	stats, err := latencyTracker.GetToolLatencyStats(toolName)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("getting stats: %w", err)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Latency Stats: %s\n\n", toolName))

	sb.WriteString(fmt.Sprintf("**Profile:** %s\n\n", stats.Profile))

	sb.WriteString("## Latency Percentiles\n\n")
	sb.WriteString(fmt.Sprintf("- **P50:** %dms\n", stats.P50LatencyMs))
	sb.WriteString(fmt.Sprintf("- **P95:** %dms\n", stats.P95LatencyMs))
	sb.WriteString(fmt.Sprintf("- **P99:** %dms\n\n", stats.P99LatencyMs))

	sb.WriteString("## Trend\n\n")
	trendDesc := "stable"
	if stats.TrendPercent > 10 {
		trendDesc = "getting slower"
	} else if stats.TrendPercent < -10 {
		trendDesc = "getting faster"
	}
	sb.WriteString(fmt.Sprintf("- **Change:** %.1f%% (%s)\n", stats.TrendPercent, trendDesc))
	sb.WriteString(fmt.Sprintf("- **Sample count:** %d\n\n", stats.SampleCount))

	if len(stats.Alternatives) > 0 {
		sb.WriteString("## Faster Alternatives\n\n")
		for _, alt := range stats.Alternatives {
			sb.WriteString(fmt.Sprintf("- %s\n", alt))
		}
	}

	return tools.TextResult(sb.String()), nil
}

func handleLatencyProfiles(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	profile := req.GetString("profile", "all")
	limit := int(req.GetFloat("limit", 20))

	latencyTracker := clients.GetLatencyTrackerClient()
	if latencyTracker == nil {
		return tools.ErrorResult(fmt.Errorf("latency tracker not available")), nil
	}

	profiles, err := latencyTracker.GetAllToolProfiles()
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("getting profiles: %w", err)), nil
	}

	var sb strings.Builder
	sb.WriteString("# Tools by Latency Profile\n\n")

	writeProfile := func(name string, profileType clients.LatencyProfile, icon string) {
		tools := profiles[profileType]
		if profile != "all" && profile != string(profileType) {
			return
		}
		sb.WriteString(fmt.Sprintf("## %s %s (%d tools)\n\n", icon, name, len(tools)))
		if len(tools) == 0 {
			sb.WriteString("_No tools in this profile_\n\n")
			return
		}
		sb.WriteString("| Tool | P50 | P95 | Samples |\n")
		sb.WriteString("|------|-----|-----|--------|\n")
		count := common.MinInt(limit, len(tools))
		for _, t := range tools[:count] {
			sb.WriteString(fmt.Sprintf("| %s | %dms | %dms | %d |\n",
				t.ToolName, t.P50LatencyMs, t.P95LatencyMs, t.SampleCount))
		}
		if len(tools) > limit {
			sb.WriteString(fmt.Sprintf("\n_... and %d more_\n", len(tools)-limit))
		}
		sb.WriteString("\n")
	}

	writeProfile("Fast (<100ms)", clients.LatencyProfileFast, "⚡")
	writeProfile("Medium (100ms-1s)", clients.LatencyProfileMedium, "🔄")
	writeProfile("Slow (>1s)", clients.LatencyProfileSlow, "🐢")

	return tools.TextResult(sb.String()), nil
}

func handleSessionBudget(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := req.GetString("session_id", "")
	action := req.GetString("action", "view")

	latencyTracker := clients.GetLatencyTrackerClient()
	if latencyTracker == nil {
		return tools.ErrorResult(fmt.Errorf("latency tracker not available")), nil
	}

	if sessionID == "" {
		sessionID = fmt.Sprintf("default-%s", time.Now().Format("2006-01-02-15"))
	}

	if action == "reset" {
		budget := latencyTracker.StartSession(sessionID)
		return tools.TextResult(fmt.Sprintf("Session %s budget reset. New budget: %dms", sessionID, budget.TotalBudgetMs)), nil
	}

	budget := latencyTracker.GetSession(sessionID)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Session Budget: %s\n\n", sessionID))
	sb.WriteString(fmt.Sprintf("- **Total Budget:** %dms\n", budget.TotalBudgetMs))
	sb.WriteString(fmt.Sprintf("- **Used:** %dms\n", budget.UsedMs))
	sb.WriteString(fmt.Sprintf("- **Remaining:** %dms\n", budget.RemainingMs))
	sb.WriteString(fmt.Sprintf("- **Tool Calls:** %d\n", budget.ToolCalls))
	sb.WriteString(fmt.Sprintf("- **Status:** %s\n", map[bool]string{true: "EXHAUSTED", false: "Active"}[budget.IsExhausted]))

	// Progress bar
	pct := float64(budget.UsedMs) / float64(budget.TotalBudgetMs) * 100
	filled := int(pct / 5)
	if filled > 20 {
		filled = 20
	}
	bar := strings.Repeat("█", filled) + strings.Repeat("░", 20-filled)
	sb.WriteString(fmt.Sprintf("\n**Usage:** [%s] %.0f%%\n", bar, pct))

	return tools.TextResult(sb.String()), nil
}

func handleSlowQueryAlerts(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	limit := int(req.GetFloat("limit", 20))
	unackOnly := req.GetBool("unacknowledged_only", true)
	ackID := req.GetString("acknowledge", "")

	latencyTracker := clients.GetLatencyTrackerClient()
	if latencyTracker == nil {
		return tools.ErrorResult(fmt.Errorf("latency tracker not available")), nil
	}

	// Handle acknowledgment
	if ackID != "" {
		if err := latencyTracker.AcknowledgeAlert(ackID); err != nil {
			return tools.ErrorResult(fmt.Errorf("acknowledging alert: %w", err)), nil
		}
		return tools.TextResult(fmt.Sprintf("Alert %s acknowledged", ackID)), nil
	}

	alerts, err := latencyTracker.GetSlowQueryAlerts(limit, unackOnly)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("getting alerts: %w", err)), nil
	}

	if len(alerts) == 0 {
		return tools.TextResult("No slow query alerts found."), nil
	}

	var sb strings.Builder
	sb.WriteString("# Slow Query Alerts\n\n")

	sb.WriteString("| ID | Time | Tool | Latency | Threshold | Status |\n")
	sb.WriteString("|-----|------|------|---------|-----------|--------|\n")

	for _, a := range alerts {
		status := "Pending"
		if a.Acknowledged {
			status = "Acked"
		}
		sb.WriteString(fmt.Sprintf("| %s | %s | %s | %dms | %dms | %s |\n",
			a.ID[:8], a.Timestamp.Format("Jan 2 15:04"), a.ToolName,
			a.LatencyMs, a.ThresholdMs, status))
	}

	sb.WriteString(fmt.Sprintf("\n**Total:** %d alerts\n", len(alerts)))
	sb.WriteString("\n_Use `acknowledge` parameter to acknowledge an alert_\n")

	return tools.TextResult(sb.String()), nil
}

func handlePredictLatency(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	toolName := req.GetString("tool", "")
	if toolName == "" {
		return tools.ErrorResult(fmt.Errorf("tool name is required")), nil
	}

	// Ensure tool name has prefix
	if !strings.HasPrefix(toolName, "webb_") {
		toolName = "webb_" + toolName
	}

	latencyTracker := clients.GetLatencyTrackerClient()
	if latencyTracker == nil {
		return tools.ErrorResult(fmt.Errorf("latency tracker not available")), nil
	}

	params := make(map[string]interface{})
	if limit := req.GetFloat("limit", 0); limit > 0 {
		params["limit"] = limit
	}

	prediction, err := latencyTracker.PredictLatency(toolName, params)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("predicting latency: %w", err)), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Latency Prediction: %s\n\n", toolName))
	sb.WriteString(fmt.Sprintf("**Predicted Latency:** %dms\n", prediction.PredictedMs))
	sb.WriteString(fmt.Sprintf("**Confidence:** %.0f%%\n", prediction.ConfidenceScore*100))
	sb.WriteString(fmt.Sprintf("**Based on:** %d samples\n\n", prediction.BasedOnSamples))

	if len(prediction.Factors) > 0 {
		sb.WriteString("## Factors\n\n")
		for factor, value := range prediction.Factors {
			sb.WriteString(fmt.Sprintf("- **%s:** %.2f\n", factor, value))
		}
	}

	return tools.TextResult(sb.String()), nil
}

func handleAddLatencyAlternative(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	slowTool := req.GetString("slow_tool", "")
	fastTool := req.GetString("fast_tool", "")
	savingMs := int64(req.GetFloat("latency_saving_ms", 0))

	if slowTool == "" || fastTool == "" {
		return tools.ErrorResult(fmt.Errorf("both slow_tool and fast_tool are required")), nil
	}
	if savingMs <= 0 {
		return tools.ErrorResult(fmt.Errorf("latency_saving_ms must be positive")), nil
	}

	// Ensure tool names have prefix
	if !strings.HasPrefix(slowTool, "webb_") {
		slowTool = "webb_" + slowTool
	}
	if !strings.HasPrefix(fastTool, "webb_") {
		fastTool = "webb_" + fastTool
	}

	latencyTracker := clients.GetLatencyTrackerClient()
	if latencyTracker == nil {
		return tools.ErrorResult(fmt.Errorf("latency tracker not available")), nil
	}

	if err := latencyTracker.AddAlternative(slowTool, fastTool, savingMs); err != nil {
		return tools.ErrorResult(fmt.Errorf("adding alternative: %w", err)), nil
	}

	return tools.TextResult(fmt.Sprintf("Added %s as faster alternative for %s (saves ~%dms)", fastTool, slowTool, savingMs)), nil
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}
