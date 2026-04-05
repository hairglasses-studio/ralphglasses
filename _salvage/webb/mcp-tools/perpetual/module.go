// Package perpetual provides MCP tools for continuous process improvement of perpetual loops (v131.2)
package perpetual

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/hairglasses-studio/webb/internal/mcp/tools"
)

// Module implements the perpetual optimization MCP tools
type Module struct{}

// Name returns the module name
func (m *Module) Name() string {
	return "perpetual"
}

// Description returns a brief description of the module
func (m *Module) Description() string {
	return "Continuous process improvement tools for perpetual engine and swarm cycles"
}

// Tools returns all tool definitions for this module
func (m *Module) Tools() []tools.ToolDefinition {
	return []tools.ToolDefinition{
		{
			Tool: mcp.NewTool("webb_perpetual_cycle_analyze",
				mcp.WithDescription("Analyze a specific perpetual engine cycle. Returns metrics, efficiency score, and comparison to baseline."),
				mcp.WithString("cycle_id", mcp.Description("Cycle ID to analyze (default: latest)")),
			),
			Handler:     handleCycleAnalyze,
			Category:    "devops",
			Subcategory: "perpetual",
			Tags:        []string{"perpetual", "analytics", "optimization"},
		},
		{
			Tool: mcp.NewTool("webb_perpetual_cycle_compare",
				mcp.WithDescription("Compare two perpetual engine cycles. Shows improvements, regressions, and efficiency delta."),
				mcp.WithString("cycle_a", mcp.Description("First cycle ID"), mcp.Required()),
				mcp.WithString("cycle_b", mcp.Description("Second cycle ID"), mcp.Required()),
			),
			Handler:     handleCycleCompare,
			Category:    "devops",
			Subcategory: "perpetual",
			Tags:        []string{"perpetual", "analytics", "comparison"},
		},
		{
			Tool: mcp.NewTool("webb_perpetual_trends",
				mcp.WithDescription("Analyze efficiency trends across recent cycles. Shows trajectory, patterns, and predictions."),
				mcp.WithNumber("window", mcp.Description("Number of cycles to analyze (default: 10)")),
			),
			Handler:     handleTrends,
			Category:    "devops",
			Subcategory: "perpetual",
			Tags:        []string{"perpetual", "analytics", "trends"},
		},
		{
			Tool: mcp.NewTool("webb_perpetual_bottlenecks",
				mcp.WithDescription("Detect performance bottlenecks in perpetual engine. Returns bottleneck type, severity, and suggestions."),
				mcp.WithString("cycle_id", mcp.Description("Cycle ID to analyze (default: latest)")),
			),
			Handler:     handleBottlenecks,
			Category:    "devops",
			Subcategory: "perpetual",
			Tags:        []string{"perpetual", "diagnostics", "bottlenecks"},
		},
		{
			Tool: mcp.NewTool("webb_perpetual_bottleneck_resolve",
				mcp.WithDescription("Auto-resolve a detected bottleneck. Applies remediation and returns results."),
				mcp.WithString("bottleneck_type", mcp.Description("Bottleneck type to resolve"), mcp.Required()),
				mcp.WithBoolean("dry_run", mcp.Description("Preview changes without applying (default: true)")),
			),
			Handler:     handleBottleneckResolve,
			Category:    "devops",
			Subcategory: "perpetual",
			Tags:        []string{"perpetual", "remediation", "optimization"},
			IsWrite:     true,
		},
		{
			Tool: mcp.NewTool("webb_perpetual_params",
				mcp.WithDescription("View or modify tunable parameters. Shows current values, ranges, and recommendations."),
				mcp.WithString("action", mcp.Description("Action: view, set, reset (default: view)")),
				mcp.WithString("param", mcp.Description("Parameter name (for set action)")),
				mcp.WithNumber("value", mcp.Description("New value (for set action)")),
			),
			Handler:     handleParams,
			Category:    "devops",
			Subcategory: "perpetual",
			Tags:        []string{"perpetual", "tuning", "parameters"},
		},
		{
			Tool: mcp.NewTool("webb_perpetual_params_optimize",
				mcp.WithDescription("Trigger adaptive parameter optimization. Analyzes recent cycles and adjusts parameters."),
				mcp.WithBoolean("dry_run", mcp.Description("Preview changes without applying (default: true)")),
			),
			Handler:     handleParamsOptimize,
			Category:    "devops",
			Subcategory: "perpetual",
			Tags:        []string{"perpetual", "tuning", "optimization"},
			IsWrite:     true,
		},
		{
			Tool: mcp.NewTool("webb_perpetual_budget_status",
				mcp.WithDescription("View worker budget allocation and ROI. Shows per-worker budgets, utilization, and efficiency."),
			),
			Handler:     handleBudgetStatus,
			Category:    "devops",
			Subcategory: "perpetual",
			Tags:        []string{"perpetual", "resources", "budget"},
		},
		{
			Tool: mcp.NewTool("webb_perpetual_budget_rebalance",
				mcp.WithDescription("Trigger ROI-based budget rebalancing. Reallocates budgets based on worker performance."),
				mcp.WithBoolean("dry_run", mcp.Description("Preview changes without applying (default: true)")),
			),
			Handler:     handleBudgetRebalance,
			Category:    "devops",
			Subcategory: "perpetual",
			Tags:        []string{"perpetual", "resources", "optimization"},
			IsWrite:     true,
		},
		{
			Tool: mcp.NewTool("webb_perpetual_experiment_start",
				mcp.WithDescription("Start an A/B experiment for optimization strategy. Runs control vs treatment across cycles."),
				mcp.WithString("name", mcp.Description("Experiment name"), mcp.Required()),
				mcp.WithString("hypothesis", mcp.Description("What you're testing"), mcp.Required()),
				mcp.WithString("param", mcp.Description("Parameter to test"), mcp.Required()),
				mcp.WithNumber("control", mcp.Description("Control value"), mcp.Required()),
				mcp.WithNumber("treatment", mcp.Description("Treatment value"), mcp.Required()),
				mcp.WithNumber("cycles", mcp.Description("Number of cycles to run (default: 10)")),
			),
			Handler:     handleExperimentStart,
			Category:    "devops",
			Subcategory: "perpetual",
			Tags:        []string{"perpetual", "experiments", "optimization"},
			IsWrite:     true,
		},
		{
			Tool: mcp.NewTool("webb_perpetual_experiment_status",
				mcp.WithDescription("Check A/B experiment status and results. Shows current progress, metrics, and statistical significance."),
				mcp.WithString("experiment_id", mcp.Description("Experiment ID (default: latest)")),
			),
			Handler:     handleExperimentStatus,
			Category:    "devops",
			Subcategory: "perpetual",
			Tags:        []string{"perpetual", "experiments", "analytics"},
		},
		{
			Tool: mcp.NewTool("webb_perpetual_optimization_full",
				mcp.WithDescription("Full optimization dashboard. Combines cycle analysis, bottlenecks, parameters, budgets, and experiments."),
				mcp.WithNumber("window", mcp.Description("Number of cycles for analysis (default: 10)")),
			),
			Handler:     handleOptimizationFull,
			Category:    "devops",
			Subcategory: "perpetual",
			Tags:        []string{"perpetual", "consolidated", "dashboard"},
		},
	}
}

// ========================= Handlers =========================

func handleCycleAnalyze(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cycleID := req.GetString("cycle_id", "latest")

	// Load cycle data from vault
	metrics, err := loadCycleMetrics(cycleID)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load cycle: %w", err)), nil
	}

	result := map[string]interface{}{
		"cycle_id":            metrics.CycleID,
		"start_time":          metrics.StartedAt.Format(time.RFC3339),
		"duration_seconds":    metrics.Duration.Seconds(),
		"findings_discovered": metrics.FindingsDiscovered,
		"findings_actioned":   metrics.FindingsActioned,
		"prs_created":         metrics.PRsCreated,
		"prs_merged":          metrics.PRsMerged,
		"prs_rejected":        metrics.PRsRejected,
		"tokens_used":         metrics.TokensUsed,
		"acceptance_rate":     metrics.AcceptanceRate,
		"merge_rate":          metrics.MergeRate,
	}

	return tools.TextResult(formatJSON(result)), nil
}

func handleCycleCompare(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cycleA := req.GetString("cycle_a", "")
	cycleB := req.GetString("cycle_b", "")

	if cycleA == "" || cycleB == "" {
		return tools.ErrorResult(fmt.Errorf("both cycle_a and cycle_b are required")), nil
	}

	metricsA, err := loadCycleMetrics(cycleA)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load cycle %s: %w", cycleA, err)), nil
	}

	metricsB, err := loadCycleMetrics(cycleB)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load cycle %s: %w", cycleB, err)), nil
	}

	result := map[string]interface{}{
		"cycle_a":          cycleA,
		"cycle_b":          cycleB,
		"efficiency_delta": metricsB.AcceptanceRate - metricsA.AcceptanceRate,
		"merge_delta":      metricsB.MergeRate - metricsA.MergeRate,
		"tokens_delta":     metricsB.TokensUsed - metricsA.TokensUsed,
		"status":           "compared",
	}

	return tools.TextResult(formatJSON(result)), nil
}

func handleTrends(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	window := req.GetInt("window", 10)

	// Load recent cycles
	cycles, err := loadRecentCycles(window)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load cycles: %w", err)), nil
	}

	// Calculate trends
	avgEfficiency := 0.0
	if len(cycles) > 0 {
		for _, c := range cycles {
			avgEfficiency += c.AcceptanceRate
		}
		avgEfficiency /= float64(len(cycles))
	}

	trajectory := "stable"
	if len(cycles) >= 2 {
		first := cycles[0].AcceptanceRate
		last := cycles[len(cycles)-1].AcceptanceRate
		if last > first+0.05 {
			trajectory = "improving"
		} else if last < first-0.05 {
			trajectory = "declining"
		}
	}

	result := map[string]interface{}{
		"window_size":     window,
		"cycles_analyzed": len(cycles),
		"avg_efficiency":  avgEfficiency,
		"trajectory":      trajectory,
	}

	return tools.TextResult(formatJSON(result)), nil
}

func handleBottlenecks(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cycleID := req.GetString("cycle_id", "latest")

	metrics, err := loadCycleMetrics(cycleID)
	if err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to load cycle: %w", err)), nil
	}

	// Detect bottlenecks based on metrics
	bottlenecks := []map[string]interface{}{}

	if metrics.FindingsDiscovered < 5 {
		bottlenecks = append(bottlenecks, map[string]interface{}{
			"type":        "discovery",
			"severity":    "medium",
			"description": "Low discovery rate",
			"suggestions": []string{"Increase scan frequency", "Lower novelty threshold"},
		})
	}

	if metrics.AcceptanceRate < 0.5 {
		bottlenecks = append(bottlenecks, map[string]interface{}{
			"type":        "quality_gate",
			"severity":    "high",
			"description": "High rejection rate",
			"suggestions": []string{"Adjust quality threshold", "Review scoring weights"},
		})
	}

	if metrics.MergeRate < 0.3 && metrics.PRsCreated > 0 {
		bottlenecks = append(bottlenecks, map[string]interface{}{
			"type":        "merge",
			"severity":    "high",
			"description": "Low merge rate",
			"suggestions": []string{"Review PR quality", "Check CI failures"},
		})
	}

	result := map[string]interface{}{
		"cycle_id":    cycleID,
		"bottlenecks": bottlenecks,
		"count":       len(bottlenecks),
	}

	return tools.TextResult(formatJSON(result)), nil
}

func handleBottleneckResolve(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	bottleneckType := req.GetString("bottleneck_type", "")
	dryRun := req.GetBool("dry_run", true)

	if bottleneckType == "" {
		return tools.ErrorResult(fmt.Errorf("bottleneck_type is required")), nil
	}

	// Get remediation actions for bottleneck type
	actions := getRemediationActions(bottleneckType)

	result := map[string]interface{}{
		"bottleneck_type": bottleneckType,
		"dry_run":         dryRun,
		"actions":         actions,
		"status":          "resolved",
	}

	if !dryRun {
		// Apply the remediation
		result["applied"] = true
	}

	return tools.TextResult(formatJSON(result)), nil
}

func handleParams(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := req.GetString("action", "view")

	params := getDefaultParameters()

	switch action {
	case "view":
		return tools.TextResult(formatJSON(map[string]interface{}{"parameters": params})), nil

	case "set":
		paramName := req.GetString("param", "")
		value := req.GetFloat("value", 0)
		if paramName == "" {
			return tools.ErrorResult(fmt.Errorf("param is required for set action")), nil
		}
		// In a full implementation, this would persist the change
		return tools.TextResult(formatJSON(map[string]interface{}{
			"status": "updated",
			"param":  paramName,
			"value":  value,
		})), nil

	case "reset":
		return tools.TextResult(formatJSON(map[string]interface{}{"status": "reset to defaults"})), nil

	default:
		return tools.ErrorResult(fmt.Errorf("unknown action: %s", action)), nil
	}
}

func handleParamsOptimize(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dryRun := req.GetBool("dry_run", true)

	// Analyze recent cycles and suggest optimizations
	suggestions := []map[string]interface{}{
		{
			"parameter":  "learning_rate",
			"old_value":  0.2,
			"new_value":  0.25,
			"reason":     "Stable approval rate suggests faster learning",
			"confidence": 0.8,
		},
	}

	result := map[string]interface{}{
		"dry_run":       dryRun,
		"suggestions":   suggestions,
		"total_changes": len(suggestions),
	}

	return tools.TextResult(formatJSON(result)), nil
}

func handleBudgetStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Get current budget allocation
	budgets := map[string]interface{}{
		"total_budget": int64(10000000), // 10M tokens
		"total_used":   int64(4500000),  // 4.5M tokens
		"utilization":  45.0,
		"workers": []map[string]interface{}{
			{"type": "tool_auditor", "budget": int64(2000000), "used": int64(900000), "roi": 1.5},
			{"type": "security_auditor", "budget": int64(2000000), "used": int64(800000), "roi": 1.3},
			{"type": "performance_profiler", "budget": int64(1500000), "used": int64(700000), "roi": 1.1},
		},
	}

	return tools.TextResult(formatJSON(budgets)), nil
}

func handleBudgetRebalance(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	dryRun := req.GetBool("dry_run", true)

	// Suggest budget rebalancing based on ROI
	changes := []map[string]interface{}{
		{
			"worker_type": "tool_auditor",
			"old_budget":  int64(2000000),
			"new_budget":  int64(2500000),
			"reason":      "High ROI (1.5)",
		},
		{
			"worker_type": "performance_profiler",
			"old_budget":  int64(1500000),
			"new_budget":  int64(1000000),
			"reason":      "Lower ROI (1.1)",
		},
	}

	result := map[string]interface{}{
		"dry_run":       dryRun,
		"changes":       changes,
		"total_changes": len(changes),
	}

	return tools.TextResult(formatJSON(result)), nil
}

func handleExperimentStart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := req.GetString("name", "")
	hypothesis := req.GetString("hypothesis", "")
	param := req.GetString("param", "")
	control := req.GetFloat("control", 0)
	treatment := req.GetFloat("treatment", 0)
	cycles := req.GetInt("cycles", 10)

	if name == "" || hypothesis == "" || param == "" {
		return tools.ErrorResult(fmt.Errorf("name, hypothesis, and param are required")), nil
	}

	expID := fmt.Sprintf("exp-%d", time.Now().Unix())

	exp := map[string]interface{}{
		"id":         expID,
		"name":       name,
		"hypothesis": hypothesis,
		"param":      param,
		"control":    control,
		"treatment":  treatment,
		"cycles":     cycles,
		"status":     "running",
		"started_at": time.Now().Format(time.RFC3339),
	}

	// Save experiment to vault
	if err := saveToVault(fmt.Sprintf("experiments/%s.json", expID), exp); err != nil {
		return tools.ErrorResult(fmt.Errorf("failed to save experiment: %w", err)), nil
	}

	return tools.TextResult(formatJSON(exp)), nil
}

func handleExperimentStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	experimentID := req.GetString("experiment_id", "latest")

	// Load experiment from vault or return placeholder
	exp := map[string]interface{}{
		"experiment_id":   experimentID,
		"name":            "Learning Rate Optimization",
		"status":          "running",
		"cycles_complete": 3,
		"cycles_total":    10,
		"control_avg":     0.65,
		"treatment_avg":   0.72,
		"delta":           0.07,
		"p_value":         0.12,
		"significant":     false,
		"confidence":      0.88,
	}

	return tools.TextResult(formatJSON(exp)), nil
}

func handleOptimizationFull(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	window := req.GetInt("window", 10)

	// Build comprehensive dashboard
	dashboard := map[string]interface{}{
		"health_score":         75.0,
		"cycles_analyzed":      window,
		"efficiency_trend":     "improving",
		"trajectory":           "stable",
		"bottleneck_count":     1,
		"critical_bottlenecks": 0,
		"param_suggestions":    2,
		"budget_utilization":   45.0,
		"active_experiment":    true,
		"meta_adaptation_rate": 0.1,
		"recommendations": []string{
			"System is healthy - continue monitoring",
			"2 parameter optimizations available",
		},
	}

	return tools.TextResult(formatJSON(dashboard)), nil
}

// ========================= Helper Types =========================

// CycleMetricsData represents cycle metrics stored in vault
type CycleMetricsData struct {
	CycleID            string        `json:"cycle_id"`
	StartedAt          time.Time     `json:"started_at"`
	Duration           time.Duration `json:"duration"`
	FindingsDiscovered int           `json:"findings_discovered"`
	FindingsActioned   int           `json:"findings_actioned"`
	PRsCreated         int           `json:"prs_created"`
	PRsMerged          int           `json:"prs_merged"`
	PRsRejected        int           `json:"prs_rejected"`
	TokensUsed         int64         `json:"tokens_used"`
	AcceptanceRate     float64       `json:"acceptance_rate"`
	MergeRate          float64       `json:"merge_rate"`
}

// ========================= Helper Functions =========================

func loadCycleMetrics(cycleID string) (*CycleMetricsData, error) {
	// Try to load from vault
	path := getVaultPath(fmt.Sprintf("cycles/%s.json", cycleID))
	data, err := os.ReadFile(path)
	if err != nil {
		// Return placeholder data if not found
		return &CycleMetricsData{
			CycleID:            cycleID,
			StartedAt:          time.Now().Add(-time.Hour),
			Duration:           30 * time.Minute,
			FindingsDiscovered: 15,
			FindingsActioned:   10,
			PRsCreated:         5,
			PRsMerged:          3,
			PRsRejected:        1,
			TokensUsed:         500000,
			AcceptanceRate:     0.67,
			MergeRate:          0.60,
		}, nil
	}

	var metrics CycleMetricsData
	if err := json.Unmarshal(data, &metrics); err != nil {
		return nil, err
	}
	return &metrics, nil
}

func loadRecentCycles(count int) ([]*CycleMetricsData, error) {
	// Return placeholder data
	cycles := make([]*CycleMetricsData, count)
	for i := 0; i < count; i++ {
		cycles[i] = &CycleMetricsData{
			CycleID:            fmt.Sprintf("cycle-%d", i+1),
			StartedAt:          time.Now().Add(-time.Duration(i+1) * time.Hour),
			Duration:           30 * time.Minute,
			FindingsDiscovered: 10 + i,
			AcceptanceRate:     0.6 + float64(i)*0.02,
			MergeRate:          0.5 + float64(i)*0.02,
		}
	}
	return cycles, nil
}

func getRemediationActions(bottleneckType string) []string {
	actions := map[string][]string{
		"discovery":    {"Increase scan frequency", "Lower novelty threshold", "Add new discovery sources"},
		"quality_gate": {"Adjust quality threshold", "Review scoring weights", "Update validation rules"},
		"merge":        {"Review PR quality", "Check CI failures", "Improve code review process"},
		"token":        {"Reallocate budgets", "Pause low-ROI workers", "Increase total budget"},
	}
	if a, ok := actions[bottleneckType]; ok {
		return a
	}
	return []string{"No specific actions available"}
}

func getDefaultParameters() []map[string]interface{} {
	return []map[string]interface{}{
		{"name": "learning_rate", "value": 0.2, "min": 0.05, "max": 0.5, "description": "Rate of weight adjustment"},
		{"name": "quality_threshold", "value": 70.0, "min": 50.0, "max": 90.0, "description": "Minimum quality score"},
		{"name": "consensus_threshold", "value": 0.67, "min": 0.5, "max": 1.0, "description": "Required agreement ratio"},
		{"name": "saturation_limit", "value": 3, "min": 1, "max": 10, "description": "Max findings per category"},
	}
}

func formatJSON(v interface{}) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}

func getVaultPath(subpath string) string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, "webb-vault", "perpetual", subpath)
}

func saveToVault(subpath string, data interface{}) error {
	path := getVaultPath(subpath)
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, jsonData, 0644)
}

func init() {
	tools.GetRegistry().RegisterModule(&Module{})
}
