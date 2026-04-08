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

// WorkflowDef summarizes a common operator workflow that can be discovered
// through MCP resources before any mutating tool calls happen.
type WorkflowDef struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Resources   []string `json:"resources"`
	Prompts     []string `json:"prompts"`
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
}

var workflowDefs = []WorkflowDef{
	{
		Name:        "discover-and-load",
		Description: "Inspect the server contract and load only the tool groups needed for the current task.",
		Resources:   []string{"ralph:///catalog/server", "ralph:///catalog/tool-groups"},
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
		Resources:   []string{"ralph:///{repo}/status", "ralph:///{repo}/progress", "ralph:///{repo}/logs"},
		Prompts:     []string{"code-review", "test-generation"},
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
		Resources:   []string{"ralph:///catalog/server", "ralph:///catalog/workflows"},
		Prompts:     []string{"bootstrap-firstboot"},
		ToolGroups:  []string{"core", "repo", "tenant"},
		KeyTools: []string{
			"ralphglasses_scan",
			"ralphglasses_repo_scaffold",
		},
	},
	{
		Name:        "provider-parity",
		Description: "Compare repo instructions, MCP registration, and generated skills across supported providers before rollout.",
		Resources:   []string{"ralph:///catalog/server", "ralph:///catalog/tool-groups"},
		Prompts:     []string{"provider-parity-audit"},
		ToolGroups:  []string{"repo", "roadmap", "docs"},
		KeyTools: []string{
			"ralphglasses_server_health",
			"ralphglasses_skill_export",
			"ralphglasses_roadmap_analyze",
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

func workflowCatalog() []WorkflowDef {
	out := make([]WorkflowDef, len(workflowDefs))
	copy(out, workflowDefs)
	return out
}

func managementToolNames() []string {
	return []string{
		"ralphglasses_tool_groups",
		"ralphglasses_load_tool_group",
		"ralphglasses_skill_export",
		"ralphglasses_server_health",
	}
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
- Read ralph:///catalog/workflows for common operator playbooks.
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
