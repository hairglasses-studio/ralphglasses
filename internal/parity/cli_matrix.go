package parity

import "math"

type CLIParityStatus string

const (
	CLIParityMCPNative         CLIParityStatus = "mcp_native"
	CLIParitySkillBacked       CLIParityStatus = "skill_backed"
	CLIParityHybrid            CLIParityStatus = "hybrid"
	CLIParityCommandOnlyDesign CLIParityStatus = "command_only_by_design"
)

type CLIParityEntry struct {
	Surface      string          `json:"surface"`
	Status       CLIParityStatus `json:"status"`
	MCPSurfaces  []string        `json:"mcp_surfaces,omitempty"`
	UsageSignals []string        `json:"usage_signals,omitempty"`
	Notes        string          `json:"notes,omitempty"`
}

type CLIParitySummary struct {
	TotalSurfaces       int     `json:"total_surfaces"`
	MCPNative           int     `json:"mcp_native"`
	SkillBacked         int     `json:"skill_backed"`
	Hybrid              int     `json:"hybrid"`
	CommandOnlyByDesign int     `json:"command_only_by_design"`
	CoveredSurfaces     int     `json:"covered_surfaces"`
	BespokeCoveragePct  float64 `json:"bespoke_coverage_pct"`
	BusinessSurfaces    int     `json:"business_surfaces"`
	BusinessCoveragePct float64 `json:"business_coverage_pct"`
}

var cliParityEntries = []CLIParityEntry{
	{Surface: "ralphglasses root TUI", Status: CLIParitySkillBacked, MCPSurfaces: []string{"ralphglasses-operator"}, Notes: "Interactive terminal UI; not a stable MCP business primitive"},
	{Surface: "ralphglasses mcp", Status: CLIParityCommandOnlyDesign, Notes: "Transport entrypoint for stdio MCP serving"},
	{Surface: "ralphglasses mcp-call", Status: CLIParityCommandOnlyDesign, Notes: "Local debugging and direct invocation helper"},
	{Surface: "ralphglasses completion", Status: CLIParityCommandOnlyDesign, Notes: "Shell completion generation is transport/shell-specific"},
	{Surface: "ralphglasses tmux list/attach/detach", Status: CLIParitySkillBacked, MCPSurfaces: []string{"ralphglasses-operator"}, Notes: "Terminal multiplexing remains interactive/operator-focused"},
	{Surface: "ralphglasses firstboot", Status: CLIParityHybrid, MCPSurfaces: []string{"ralphglasses_firstboot_profile", "ralphglasses-operator"}, UsageSignals: []string{"ralphglasses_firstboot_profile"}, Notes: "Profile/config flows are MCP-native; wizard remains skill-backed"},
	{Surface: "ralphglasses doctor", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_doctor"}, UsageSignals: []string{"ralphglasses_doctor"}, Notes: "Workspace and provider readiness checks"},
	{Surface: "ralphglasses validate", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_validate"}, UsageSignals: []string{"ralphglasses_validate"}, Notes: ".ralphrc validation across scan path or selected repos"},
	{Surface: "ralphglasses debug-bundle", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_debug_bundle"}, UsageSignals: []string{"ralphglasses_debug_bundle"}, Notes: "View or save deterministic debug bundles"},
	{Surface: "ralphglasses theme-export", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_theme_export"}, UsageSignals: []string{"ralphglasses_theme_export"}, Notes: "Export snippets for downstream tools"},
	{Surface: "ralphglasses telemetry export", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_telemetry_export"}, UsageSignals: []string{"ralphglasses_telemetry_export"}, Notes: "JSON/CSV export with filters"},
	{Surface: "ralphglasses config list-keys", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_config_schema"}, UsageSignals: []string{"ralphglasses_config_schema"}, Notes: "Structured schema, defaults, and constraints"},
	{Surface: "ralphglasses config init", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_repo_scaffold"}, UsageSignals: []string{"ralphglasses_repo_scaffold"}, Notes: "Alias behavior covered through scaffold flows"},
	{Surface: "ralphglasses init", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_repo_scaffold"}, UsageSignals: []string{"ralphglasses_repo_scaffold"}, Notes: "Supports full scaffold and minimal mode"},
	{Surface: "ralphglasses worktree list", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_worktree_list"}, UsageSignals: []string{"ralphglasses_worktree_list"}, Notes: "Dirty/stale filtering parity"},
	{Surface: "ralphglasses worktree create", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_worktree_create"}, UsageSignals: []string{"ralphglasses_worktree_create"}, Notes: "Existing parity retained"},
	{Surface: "ralphglasses worktree clean", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_worktree_cleanup"}, UsageSignals: []string{"ralphglasses_worktree_cleanup"}, Notes: "Dry-run parity included"},
	{Surface: "ralphglasses gate-check", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_loop_gates"}, UsageSignals: []string{"ralphglasses_loop_gates"}, Notes: "Supports explicit baseline_path override"},
	{Surface: "ralphglasses budget status", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_budget_status"}, UsageSignals: []string{"ralphglasses_budget_status"}, Notes: "Aggregate session budget view"},
	{Surface: "ralphglasses budget set/reset", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_session_budget"}, UsageSignals: []string{"ralphglasses_session_budget"}, Notes: "Action=set|get|reset_spend parity"},
	{Surface: "ralphglasses session list/status/stop", Status: CLIParityMCPNative, MCPSurfaces: []string{"existing session tools"}, UsageSignals: []string{"ralphglasses_session_list", "ralphglasses_session_status", "ralphglasses_session_stop"}, Notes: "Existing parity retained"},
	{Surface: "ralphglasses tenant *", Status: CLIParityMCPNative, MCPSurfaces: []string{"existing tenant tools"}, UsageSignals: []string{"ralphglasses_tenant_list", "ralphglasses_tenant_create", "ralphglasses_tenant_status", "ralphglasses_tenant_rotate_trigger_token", "ralphglasses_tenant_role_leaderboards"}, Notes: "Existing parity retained"},
	{Surface: "ralphglasses serve", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_fleet_runtime"}, UsageSignals: []string{"ralphglasses_fleet_runtime"}, Notes: "Coordinator/worker runtime lifecycle and discovery"},
	{Surface: "ralphglasses marathon", Status: CLIParityMCPNative, MCPSurfaces: []string{"ralphglasses_marathon"}, UsageSignals: []string{"ralphglasses_marathon"}, Notes: "Start, resume, status, and stop"},
}

func CLIParityEntries() []CLIParityEntry {
	out := make([]CLIParityEntry, len(cliParityEntries))
	copy(out, cliParityEntries)
	return out
}

func CLIParityCoverage() CLIParitySummary {
	summary := CLIParitySummary{TotalSurfaces: len(cliParityEntries)}
	for _, entry := range cliParityEntries {
		switch entry.Status {
		case CLIParityMCPNative:
			summary.MCPNative++
			summary.CoveredSurfaces++
		case CLIParitySkillBacked:
			summary.SkillBacked++
			summary.CoveredSurfaces++
		case CLIParityHybrid:
			summary.Hybrid++
			summary.CoveredSurfaces++
		case CLIParityCommandOnlyDesign:
			summary.CommandOnlyByDesign++
		}
	}
	summary.BusinessSurfaces = summary.TotalSurfaces - summary.CommandOnlyByDesign
	summary.BespokeCoveragePct = roundPct(summary.CoveredSurfaces, summary.TotalSurfaces)
	summary.BusinessCoveragePct = roundPct(summary.CoveredSurfaces, summary.BusinessSurfaces)
	return summary
}

func CLIParityDocument() map[string]any {
	return map[string]any{
		"title":       "Ralphglasses CLI parity matrix",
		"description": "Canonical CLI-to-MCP/skill parity summary for the current ralphglasses command surface.",
		"summary":     CLIParityCoverage(),
		"statuses": map[string]string{
			string(CLIParityMCPNative):         "The CLI workflow is covered by MCP tools.",
			string(CLIParitySkillBacked):       "The workflow remains interactive and is intentionally covered by a focused skill instead of a raw MCP tool.",
			string(CLIParityHybrid):            "The workflow is split between MCP-native automation and an interactive skill-backed step.",
			string(CLIParityCommandOnlyDesign): "The command is transport- or shell-specific and is intentionally not modeled as a business MCP primitive.",
		},
		"entries": CLIParityEntries(),
	}
}

func observableCLIParityEntries() []CLIParityEntry {
	entries := make([]CLIParityEntry, 0, len(cliParityEntries))
	for _, entry := range cliParityEntries {
		if entry.Status == CLIParityCommandOnlyDesign || len(entry.UsageSignals) == 0 {
			continue
		}
		entries = append(entries, entry)
	}
	return entries
}

func roundPct(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return math.Round((float64(numerator)/float64(denominator))*1000) / 10
}
