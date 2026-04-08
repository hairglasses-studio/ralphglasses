package mcpserver

import (
	"fmt"
	"strings"
)

// ResourceTemplateDef describes one templated read-only MCP resource.
type ResourceTemplateDef struct {
	URI         string
	Name        string
	Description string
	MIMEType    string
}

// ResourceDef describes one static read-only MCP resource.
type ResourceDef struct {
	URI         string
	Name        string
	Description string
	MIMEType    string
}

// PromptDef describes one MCP prompt exposed by the server.
type PromptDef struct {
	Name        string
	Description string
}

// SkillCatalogDef describes one canonical repo workflow skill that can be
// discovered alongside tools, resources, and prompts.
type SkillCatalogDef struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Tags          []string `json:"tags,omitempty"`
	Workflows     []string `json:"workflows,omitempty"`
	ToolGroups    []string `json:"tool_groups,omitempty"`
	Resources     []string `json:"resources,omitempty"`
	Prompts       []string `json:"prompts,omitempty"`
	KeyTools      []string `json:"key_tools,omitempty"`
	CanonicalPath string   `json:"canonical_path,omitempty"`
}

// WorkflowDef summarizes a common operator workflow that can be discovered
// through MCP resources before any mutating tool calls happen.
type WorkflowDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Resources   []string `json:"resources"`
	Prompts     []string `json:"prompts"`
	Skills      []string `json:"skills,omitempty"`
	ToolGroups  []string `json:"tool_groups"`
	KeyTools    []string `json:"key_tools"`
}

var resourceTemplateDefs = []ResourceTemplateDef{
	{
		URI:         "ralph:///{repo}/status",
		Name:        "Repo status",
		Description: "Read .ralph/status.json for a repository.",
		MIMEType:    "application/json",
	},
	{
		URI:         "ralph:///{repo}/progress",
		Name:        "Repo progress",
		Description: "Read .ralph/progress.json for a repository.",
		MIMEType:    "application/json",
	},
	{
		URI:         "ralph:///{repo}/logs",
		Name:        "Repo logs",
		Description: "Read the last 100 lines of .ralph/logs/ralph.log for a repository.",
		MIMEType:    "text/plain",
	},
}

var staticResourceDefs = []ResourceDef{
	{
		URI:         "ralph:///catalog/server",
		Name:        "Server catalog",
		Description: "Read the live MCP contract: tool counts, tool groups, resources, prompts, and default discovery guidance.",
		MIMEType:    "application/json",
	},
	{
		URI:         "ralph:///catalog/tool-groups",
		Name:        "Tool group catalog",
		Description: "Read the grouped tool catalog with descriptions and tool counts.",
		MIMEType:    "application/json",
	},
	{
		URI:         "ralph:///catalog/workflows",
		Name:        "Workflow catalog",
		Description: "Read the common ralphglasses operator workflows and their discovery entrypoints.",
		MIMEType:    "application/json",
	},
	{
		URI:         "ralph:///catalog/skills",
		Name:        "Skill catalog",
		Description: "Read the canonical ralphglasses workflow skills, their scopes, and their primary discovery entrypoints.",
		MIMEType:    "application/json",
	},
	{
		URI:         "ralph:///catalog/cli-parity",
		Name:        "CLI parity catalog",
		Description: "Read the current CLI-to-MCP/skill parity matrix, its static coverage summary, and the rolling usage telemetry snapshot from tool benchmarks.",
		MIMEType:    "application/json",
	},
	{
		URI:         "ralph:///catalog/discovery-adoption",
		Name:        "Discovery adoption catalog",
		Description: "Read rolling adoption telemetry for resources, prompts, and focused skill front doors, including inactive surfaces that should drive the next workflow tranche.",
		MIMEType:    "application/json",
	},
	{
		URI:         "ralph:///bootstrap/checklist",
		Name:        "Bootstrap checklist",
		Description: "Read the MCP-first bootstrap checklist for provider readiness, config validation, and firstboot flows.",
		MIMEType:    "application/json",
	},
	{
		URI:         "ralph:///runtime/health",
		Name:        "Runtime health",
		Description: "Read the current ralphglasses runtime health snapshot, including loaded groups and discovery coverage.",
		MIMEType:    "application/json",
	},
}

var promptDefs = []PromptDef{
	{
		Name:        "self-improvement-planner",
		Description: "Plan a self-improvement iteration for a repository with goals, steps, validation criteria, and rollback strategy.",
	},
	{
		Name:        "code-review",
		Description: "Review code changes in a repository file with structured severity-focused feedback.",
	},
	{
		Name:        "test-generation",
		Description: "Generate a structured test plan for a repository file, including coverage targets and edge cases.",
	},
	{
		Name:        "bootstrap-firstboot",
		Description: "Build a first-boot checklist for bringing a workspace and provider CLIs into a healthy Ralph-ready state.",
	},
	{
		Name:        "provider-parity-audit",
		Description: "Audit provider parity for a repository across AGENTS, provider config, MCP config, generated skills, and prompts.",
	},
	{
		Name:        "repo-triage-brief",
		Description: "Build a repo triage brief from status, progress, logs, runtime health, and the recommended next skill/tool path.",
	},
}

var skillCatalogDefs = []SkillCatalogDef{
	{
		Name:          "ralphglasses-discovery",
		Description:   "Discover the live MCP contract, inspect tool groups, and route to the right workflow or skill family before starting work.",
		Tags:          []string{"discovery", "contract", "routing", "deferred-loading"},
		Workflows:     []string{"discover-and-load"},
		ToolGroups:    []string{"management", "core"},
		Resources:     []string{"ralph:///catalog/server", "ralph:///catalog/tool-groups", "ralph:///catalog/skills", "ralph:///catalog/workflows", "ralph:///catalog/discovery-adoption"},
		KeyTools:      []string{"ralphglasses_server_health", "ralphglasses_tool_groups", "ralphglasses_load_tool_group", "ralphglasses_skill_export"},
		CanonicalPath: ".agents/skills/ralphglasses-discovery/SKILL.md",
	},
	{
		Name:          "ralphglasses-session-ops",
		Description:   "Launch, resume, inspect, compare, export, and hand off provider sessions with budget awareness.",
		Tags:          []string{"sessions", "teams", "loops", "budget"},
		Workflows:     []string{"session-execution"},
		ToolGroups:    []string{"session", "team", "loop", "fleet", "tenant"},
		Resources:     []string{"ralph:///runtime/health", "ralph:///catalog/skills"},
		KeyTools:      []string{"ralphglasses_session_launch", "ralphglasses_session_list", "ralphglasses_session_status", "ralphglasses_session_budget", "ralphglasses_session_output"},
		CanonicalPath: ".agents/skills/ralphglasses-session-ops/SKILL.md",
	},
	{
		Name:          "ralphglasses-repo-admin",
		Description:   "Run repo readiness, validation, scaffold, worktree, debug-bundle, and config-schema flows through MCP-native tools.",
		Tags:          []string{"repo", "bootstrap", "validation", "worktrees"},
		Workflows:     []string{"repo-triage", "provider-parity"},
		ToolGroups:    []string{"core", "repo", "observability"},
		Resources:     []string{"ralph:///catalog/server", "ralph:///catalog/workflows", "ralph:///catalog/cli-parity", "ralph:///catalog/discovery-adoption"},
		KeyTools:      []string{"ralphglasses_doctor", "ralphglasses_validate", "ralphglasses_repo_scaffold", "ralphglasses_worktree_list", "ralphglasses_debug_bundle"},
		CanonicalPath: ".agents/skills/ralphglasses-repo-admin/SKILL.md",
	},
	{
		Name:          "ralphglasses-bootstrap",
		Description:   "Bootstrap firstboot profiles, provider readiness, config validation, and repo bring-up through MCP-native control surfaces.",
		Tags:          []string{"runtime", "bootstrap", "serve", "marathon"},
		Workflows:     []string{"bootstrap-and-firstboot", "runtime-recovery"},
		ToolGroups:    []string{"core", "fleet", "repo"},
		Resources:     []string{"ralph:///bootstrap/checklist", "ralph:///runtime/health", "ralph:///catalog/skills"},
		Prompts:       []string{"bootstrap-firstboot", "repo-triage-brief"},
		KeyTools:      []string{"ralphglasses_doctor", "ralphglasses_validate", "ralphglasses_firstboot_profile", "ralphglasses_fleet_runtime", "ralphglasses_marathon"},
		CanonicalPath: ".agents/skills/ralphglasses-bootstrap/SKILL.md",
	},
	{
		Name:          "ralphglasses-recovery-observability",
		Description:   "Investigate runtime health, logs, recovery plans, and session salvage when execution is degraded or interrupted.",
		Tags:          []string{"recovery", "salvage", "incident", "verification"},
		Workflows:     []string{"repo-triage", "runtime-recovery"},
		ToolGroups:    []string{"recovery", "session", "observability"},
		Resources:     []string{"ralph:///runtime/health"},
		KeyTools:      []string{"ralphglasses_logs", "ralphglasses_debug_bundle", "ralphglasses_recovery_plan", "ralphglasses_session_triage", "ralphglasses_session_salvage"},
		CanonicalPath: ".agents/skills/ralphglasses-recovery-observability/SKILL.md",
	},
	{
		Name:          "ralphglasses-operator",
		Description:   "Bridge the interactive TUI, tmux, and firstboot wizard with the MCP control plane when a task is terminal-native by design.",
		Tags:          []string{"interactive", "operator", "tmux", "tui"},
		Workflows:     []string{"bootstrap-and-firstboot"},
		ToolGroups:    []string{"management"},
		Resources:     []string{"ralph:///bootstrap/checklist", "ralph:///runtime/health"},
		KeyTools:      []string{"ralphglasses_server_health", "ralphglasses_fleet_runtime", "ralphglasses_marathon"},
		CanonicalPath: ".agents/skills/ralphglasses-operator/SKILL.md",
	},
	{
		Name:          "ralphglasses-self-dev",
		Description:   "Improve ralphglasses itself through parity audits, roadmap analysis, loop execution, merge verification, and docs writeback.",
		Tags:          []string{"self-dev", "roadmap", "parity", "docs"},
		Workflows:     []string{"repo-triage", "provider-parity"},
		ToolGroups:    []string{"repo", "roadmap", "loop", "fleet", "docs"},
		Resources:     []string{"ralph:///catalog/server", "ralph:///catalog/skills", "ralph:///catalog/workflows", "ralph:///catalog/cli-parity", "ralph:///catalog/discovery-adoption"},
		Prompts:       []string{"provider-parity-audit", "repo-triage-brief"},
		KeyTools:      []string{"ralphglasses_repo_surface_audit", "ralphglasses_roadmap_analyze", "ralphglasses_roadmap_prioritize", "ralphglasses_marathon"},
		CanonicalPath: ".agents/skills/ralphglasses-self-dev/SKILL.md",
	},
}

var workflowDefs = []WorkflowDef{
	{
		Name:        "discover-and-load",
		Description: "Inspect the server contract and load only the tool groups needed for the current task.",
		Resources:   []string{"ralph:///catalog/server", "ralph:///catalog/tool-groups", "ralph:///catalog/skills"},
		Skills:      []string{"ralphglasses-discovery"},
		ToolGroups:  []string{"core"},
		KeyTools: []string{
			"ralphglasses_server_health",
			"ralphglasses_tool_groups",
			"ralphglasses_load_tool_group",
		},
	},
	{
		Name:        "repo-triage",
		Description: "Assess one repository before mutating it by reading current status, progress, logs, and health signals.",
		Resources:   []string{"ralph:///{repo}/status", "ralph:///{repo}/progress", "ralph:///{repo}/logs", "ralph:///runtime/health"},
		Prompts:     []string{"repo-triage-brief", "code-review", "test-generation"},
		Skills:      []string{"ralphglasses-repo-admin", "ralphglasses-recovery-observability"},
		ToolGroups:  []string{"core", "repo", "observability"},
		KeyTools: []string{
			"ralphglasses_status",
			"ralphglasses_repo_health",
			"ralphglasses_logs",
		},
	},
	{
		Name:        "bootstrap-and-firstboot",
		Description: "Bring a new workspace or operator environment into a healthy state before launching sessions or loops.",
		Resources:   []string{"ralph:///catalog/server", "ralph:///catalog/skills", "ralph:///bootstrap/checklist"},
		Prompts:     []string{"bootstrap-firstboot"},
		Skills:      []string{"ralphglasses-bootstrap", "ralphglasses-operator"},
		ToolGroups:  []string{"core", "repo", "tenant"},
		KeyTools: []string{
			"ralphglasses_scan",
			"ralphglasses_repo_scaffold",
		},
	},
	{
		Name:        "provider-parity",
		Description: "Compare repo instructions, MCP registration, and generated skills across supported providers before rollout.",
		Resources:   []string{"ralph:///catalog/server", "ralph:///catalog/tool-groups", "ralph:///catalog/skills"},
		Prompts:     []string{"provider-parity-audit"},
		Skills:      []string{"ralphglasses-self-dev", "ralphglasses-discovery"},
		ToolGroups:  []string{"repo", "roadmap", "docs"},
		KeyTools: []string{
			"ralphglasses_server_health",
			"ralphglasses_skill_export",
			"ralphglasses_roadmap_analyze",
		},
	},
	{
		Name:        "runtime-recovery",
		Description: "Investigate runtime health, logs, and recovery state before resuming sessions or marathon work.",
		Resources:   []string{"ralph:///runtime/health", "ralph:///catalog/skills", "ralph:///{repo}/logs"},
		Skills:      []string{"ralphglasses-bootstrap", "ralphglasses-recovery-observability"},
		ToolGroups:  []string{"core", "recovery", "observability", "fleet"},
		KeyTools: []string{
			"ralphglasses_server_health",
			"ralphglasses_logs",
			"ralphglasses_recovery_plan",
			"ralphglasses_session_triage",
		},
	},
	{
		Name:        "session-execution",
		Description: "Launch, inspect, compare, and hand off provider sessions with budget awareness.",
		Resources:   []string{"ralph:///runtime/health", "ralph:///catalog/skills"},
		Skills:      []string{"ralphglasses-session-ops"},
		ToolGroups:  []string{"session", "core"},
		KeyTools: []string{
			"ralphglasses_session_launch",
			"ralphglasses_session_list",
			"ralphglasses_session_status",
			"ralphglasses_session_budget",
			"ralphglasses_session_handoff",
		},
	},
}

func resourceTemplateCatalog() []ResourceTemplateDef {
	out := make([]ResourceTemplateDef, len(resourceTemplateDefs))
	copy(out, resourceTemplateDefs)
	return out
}

func staticResourceCatalog() []ResourceDef {
	out := make([]ResourceDef, len(staticResourceDefs))
	copy(out, staticResourceDefs)
	return out
}

func promptCatalog() []PromptDef {
	out := make([]PromptDef, len(promptDefs))
	copy(out, promptDefs)
	return out
}

func skillCatalog() []SkillCatalogDef {
	out := make([]SkillCatalogDef, len(skillCatalogDefs))
	copy(out, skillCatalogDefs)
	return out
}

func workflowCatalog() []WorkflowDef {
	out := make([]WorkflowDef, len(workflowDefs))
	copy(out, workflowDefs)
	return out
}

func StaticResources() []ResourceDef {
	return staticResourceCatalog()
}

func ResourceTemplates() []ResourceTemplateDef {
	return resourceTemplateCatalog()
}

func Prompts() []PromptDef {
	return promptCatalog()
}

func Skills() []SkillCatalogDef {
	return skillCatalog()
}

func Workflows() []WorkflowDef {
	return workflowCatalog()
}

func toolGroupListSummary() string {
	return strings.Join(ToolGroupNames, ", ")
}

func loadToolGroupDescription() string {
	return fmt.Sprintf(
		"Load all tools in a named group (%s). Use ralphglasses_tool_groups or ralph:///catalog/tool-groups first if you need discovery.",
		toolGroupListSummary(),
	)
}

func ServerInstructions() string {
	return fmt.Sprintf(`Use ralphglasses as a deferred-loading repo and fleet control plane.

Start with discovery instead of guessing:
- Read ralph:///catalog/server for the live contract summary.
- Read ralph:///catalog/tool-groups for grouped capabilities and counts.
- Read ralph:///catalog/skills for the focused workflow skill families.
- Read ralph:///catalog/workflows for common operator playbooks.
- Read ralph:///catalog/cli-parity when the task is about CLI-to-MCP workflow coverage or operator parity.
- Read ralph:///catalog/discovery-adoption when you need live adoption telemetry for resources, prompts, and focused skill entrypoints.
- Read ralph:///runtime/health or ralph:///bootstrap/checklist when the task is runtime- or bootstrap-heavy.
- Call ralphglasses_tool_groups, then ralphglasses_load_tool_group before using non-core tools.
- Prefer repo read-only resources (ralph:///{repo}/status, /progress, /logs) before mutating tools.

Current contract: %d grouped tools, %d management tools, %d tool groups, %d static resources, %d resource templates, and %d prompts.`,
		GeneratedTotalTools,
		len(managementToolNames()),
		len(ToolGroupNames),
		len(staticResourceDefs),
		len(resourceTemplateDefs),
		len(promptDefs),
	)
}

func TotalToolCount() int {
	return GeneratedTotalTools + len(managementToolNames())
}
