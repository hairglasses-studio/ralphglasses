package mcpserver

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ToolRegistration describes a registered MCP tool for skill export.
// Both ToolEntry (via its Tool field) and ToolGroup satisfy this through
// the adapter functions provided below.
type ToolRegistration interface {
	// ToolName returns the tool's unique name.
	ToolName() string
	// ToolDescription returns the tool's human-readable description.
	ToolDescription() string
	// ToolProperties returns the JSON Schema properties map from InputSchema.
	ToolProperties() map[string]any
	// ToolRequired returns the list of required parameter names.
	ToolRequired() []string
}

// toolEntryAdapter wraps a ToolEntry to implement ToolRegistration.
type toolEntryAdapter struct {
	entry ToolEntry
}

func (a toolEntryAdapter) ToolName() string               { return a.entry.Tool.Name }
func (a toolEntryAdapter) ToolDescription() string        { return a.entry.Tool.Description }
func (a toolEntryAdapter) ToolProperties() map[string]any { return a.entry.Tool.InputSchema.Properties }
func (a toolEntryAdapter) ToolRequired() []string         { return a.entry.Tool.InputSchema.Required }

// AdaptToolEntry wraps a ToolEntry as a ToolRegistration.
func AdaptToolEntry(e ToolEntry) ToolRegistration {
	return toolEntryAdapter{entry: e}
}

// AdaptToolGroup converts all entries in a ToolGroup to ToolRegistrations.
func AdaptToolGroup(g ToolGroup) []ToolRegistration {
	regs := make([]ToolRegistration, len(g.Tools))
	for i, e := range g.Tools {
		regs[i] = AdaptToolEntry(e)
	}
	return regs
}

// SkillDef describes an exported skill derived from an MCP tool registration.
type SkillDef struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  []ParamDef `json:"parameters,omitempty"`
	Category    string     `json:"category,omitempty"`
	Examples    []Example  `json:"examples,omitempty"`
}

// ParamDef describes a single parameter for a skill.
type ParamDef struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
	Default     any    `json:"default,omitempty"`
}

// Example documents a sample invocation of a skill.
type Example struct {
	Description    string         `json:"description"`
	Args           map[string]any `json:"args,omitempty"`
	ExpectedOutput string         `json:"expected_output,omitempty"`
}

// ExportSkills converts registered tools to skill definitions.
// The category is inferred from the tool name prefix (e.g. "ralphglasses_session_launch"
// yields category "session"). Tools without an underscore-separated group prefix default
// to category "general".
func ExportSkills(tools []ToolRegistration) []SkillDef {
	skills := make([]SkillDef, 0, len(tools))
	for _, t := range tools {
		sd := SkillDef{
			Name:        t.ToolName(),
			Description: t.ToolDescription(),
			Category:    inferCategory(t.ToolName()),
			Parameters:  extractParams(t),
		}
		skills = append(skills, sd)
	}
	return skills
}

// inferCategory extracts a tool-group category from a tool name.
// "ralphglasses_session_launch" -> "session"
// "ralphglasses_scan" -> "core"
// "my_tool" -> "general"
func inferCategory(name string) string {
	parts := strings.SplitN(name, "_", 3)
	if len(parts) < 2 {
		return "general"
	}
	// If the prefix is "ralphglasses", the category is the second segment.
	if parts[0] == "ralphglasses" {
		if len(parts) >= 3 {
			return parts[1]
		}
		// Single-word tools like "ralphglasses_scan" are core tools.
		return "core"
	}
	return parts[0]
}

// extractParams builds ParamDef entries from a tool's JSON Schema properties.
func extractParams(t ToolRegistration) []ParamDef {
	props := t.ToolProperties()
	if len(props) == 0 {
		return nil
	}

	reqSet := make(map[string]bool, len(t.ToolRequired()))
	for _, r := range t.ToolRequired() {
		reqSet[r] = true
	}

	params := make([]ParamDef, 0, len(props))
	for name, raw := range props {
		pd := ParamDef{
			Name:     name,
			Required: reqSet[name],
		}
		if m, ok := raw.(map[string]any); ok {
			if v, ok := m["type"].(string); ok {
				pd.Type = v
			}
			if v, ok := m["description"].(string); ok {
				pd.Description = v
			}
			if v, exists := m["default"]; exists {
				pd.Default = v
			}
		}
		if pd.Type == "" {
			pd.Type = "string"
		}
		params = append(params, pd)
	}

	// Sort for deterministic output.
	sort.Slice(params, func(i, j int) bool {
		// Required params first, then alphabetical.
		if params[i].Required != params[j].Required {
			return params[i].Required
		}
		return params[i].Name < params[j].Name
	})

	return params
}

// ExportJSON serializes skill definitions as indented JSON.
func ExportJSON(skills []SkillDef) ([]byte, error) {
	return json.MarshalIndent(skills, "", "  ")
}

// ExportMarkdown generates markdown documentation for skill definitions.
func ExportMarkdown(skills []SkillDef) string {
	if len(skills) == 0 {
		return "# Skills\n\nNo skills defined.\n"
	}

	// Group by category.
	categories := make(map[string][]SkillDef)
	var catOrder []string
	for _, s := range skills {
		cat := s.Category
		if cat == "" {
			cat = "general"
		}
		if _, seen := categories[cat]; !seen {
			catOrder = append(catOrder, cat)
		}
		categories[cat] = append(categories[cat], s)
	}
	sort.Strings(catOrder)

	var b strings.Builder
	b.WriteString("# Skills\n\n")

	for _, cat := range catOrder {
		b.WriteString(fmt.Sprintf("## %s\n\n", cat))
		for _, s := range categories[cat] {
			b.WriteString(fmt.Sprintf("### %s\n\n", s.Name))
			if s.Description != "" {
				b.WriteString(s.Description + "\n\n")
			}
			if len(s.Parameters) > 0 {
				b.WriteString("**Parameters:**\n\n")
				b.WriteString("| Name | Type | Required | Description |\n")
				b.WriteString("|------|------|----------|-------------|\n")
				for _, p := range s.Parameters {
					req := "no"
					if p.Required {
						req = "yes"
					}
					b.WriteString(fmt.Sprintf("| %s | %s | %s | %s |\n",
						p.Name, p.Type, req, p.Description))
				}
				b.WriteString("\n")
			}
			if len(s.Examples) > 0 {
				b.WriteString("**Examples:**\n\n")
				for _, ex := range s.Examples {
					b.WriteString(fmt.Sprintf("- %s\n", ex.Description))
					if len(ex.Args) > 0 {
						data, _ := json.Marshal(ex.Args)
						b.WriteString(fmt.Sprintf("  ```json\n  %s\n  ```\n", string(data)))
					}
				}
				b.WriteString("\n")
			}
		}
	}

	return b.String()
}

// ExportSkillsFromGroups converts all tools across multiple ToolGroups to skill definitions.
func ExportSkillsFromGroups(groups []ToolGroup) []SkillDef {
	var regs []ToolRegistration
	for _, g := range groups {
		for _, entry := range g.Tools {
			reg := AdaptToolEntry(entry)
			regs = append(regs, reg)
		}
	}
	return ExportSkills(regs)
}

func managementSkillGroup(management []ToolEntry) ToolGroup {
	return ToolGroup{
		Name:        "management",
		Description: "Always-available discovery and contract tools registered ahead of deferred tool-group loading.",
		Tools:       management,
	}
}

func toolRegistrationsFromGroupsAndManagement(groups []ToolGroup, management []ToolEntry) []ToolRegistration {
	regs := make([]ToolRegistration, 0)
	if len(management) > 0 {
		for _, entry := range management {
			regs = append(regs, AdaptToolEntry(entry))
		}
	}
	for _, g := range groups {
		for _, entry := range g.Tools {
			regs = append(regs, AdaptToolEntry(entry))
		}
	}
	return regs
}

// ExportSkillsFromContract converts grouped and management tools from the live
// contract into skill definitions.
func ExportSkillsFromContract(groups []ToolGroup, management []ToolEntry) []SkillDef {
	return ExportSkills(toolRegistrationsFromGroupsAndManagement(groups, management))
}

// ExportSkillMarkdown generates a SKILL.md document from tool groups.
// It produces a table of contents, a tool count summary, and one section per
// group with each tool's name, description, parameters table, and example usage.
func ExportSkillMarkdown(groups []ToolGroup) string {
	return ExportSkillMarkdownFromContract(groups, nil)
}

// ExportSkillMarkdownFromContract generates a SKILL.md document from grouped
// tools plus the always-available management tools.
func ExportSkillMarkdownFromContract(groups []ToolGroup, management []ToolEntry) string {
	if len(groups) == 0 {
		if len(management) == 0 {
			return "# Ralphglasses Skills\n\nNo tool groups defined.\n"
		}
		groups = []ToolGroup{managementSkillGroup(management)}
		management = nil
	}

	groupToolCount := 0
	for _, g := range groups {
		groupToolCount += len(g.Tools)
	}

	sections := make([]ToolGroup, 0, len(groups)+1)
	if len(management) > 0 {
		sections = append(sections, managementSkillGroup(management))
	}
	sections = append(sections, groups...)
	totalTools := groupToolCount + len(management)

	var b strings.Builder

	// Header + summary.
	b.WriteString("# Ralphglasses Skills\n\n")
	if len(management) > 0 {
		b.WriteString(fmt.Sprintf("> %d tools total: %d grouped tools across %d tool groups plus %d always-available management tools\n\n",
			totalTools, groupToolCount, len(groups), len(management)))
	} else {
		b.WriteString(fmt.Sprintf("> %d tools across %d tool groups\n\n", totalTools, len(groups)))
	}

	// Table of contents.
	b.WriteString("## Table of Contents\n\n")
	for _, g := range sections {
		anchor := strings.ReplaceAll(g.Name, "_", "-")
		b.WriteString(fmt.Sprintf("- [%s](#%s) (%d tools) — %s\n", g.Name, anchor, len(g.Tools), g.Description))
	}
	b.WriteString("\n---\n\n")

	// Per-group sections.
	for _, g := range sections {
		b.WriteString(fmt.Sprintf("## %s\n\n", g.Name))
		if g.Description != "" {
			b.WriteString(g.Description + "\n\n")
		}

		skills := ExportSkillsFromGroups([]ToolGroup{g})
		for _, s := range skills {
			b.WriteString(fmt.Sprintf("### `%s`\n\n", s.Name))
			if s.Description != "" {
				b.WriteString(s.Description + "\n\n")
			}
			if len(s.Parameters) > 0 {
				b.WriteString("| Parameter | Type | Required | Description |\n")
				b.WriteString("|-----------|------|----------|-------------|\n")
				for _, p := range s.Parameters {
					req := ""
					if p.Required {
						req = "yes"
					}
					desc := p.Description
					if p.Default != nil {
						desc += fmt.Sprintf(" (default: %v)", p.Default)
					}
					b.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s |\n", p.Name, p.Type, req, desc))
				}
				b.WriteString("\n")
			}

			// Example usage block.
			b.WriteString("**Example:**\n\n```json\n")
			example := map[string]any{"tool": s.Name}
			if len(s.Parameters) > 0 {
				args := make(map[string]any)
				for _, p := range s.Parameters {
					if p.Required {
						args[p.Name] = exampleValue(p.Type)
					}
				}
				if len(args) > 0 {
					example["arguments"] = args
				}
			}
			data, _ := json.MarshalIndent(example, "", "  ")
			b.WriteString(string(data))
			b.WriteString("\n```\n\n")
		}
	}

	return b.String()
}

// exampleValue returns a placeholder value for a given JSON Schema type.
func exampleValue(typ string) any {
	switch typ {
	case "number", "integer":
		return 1
	case "boolean":
		return true
	case "array":
		return []any{}
	case "object":
		return map[string]any{}
	default:
		return "..."
	}
}

// ExportClaudeAgent exports skill definitions in Claude Code agent definition format.
// This produces a structured YAML-like text block that describes each skill as an
// agent tool with name, description, and parameter schema.
func ExportClaudeAgent(skills []SkillDef) string {
	if len(skills) == 0 {
		return "# Agent Skills\n\nNo skills defined.\n"
	}

	var b strings.Builder
	b.WriteString("# Agent Skills\n\n")
	b.WriteString(fmt.Sprintf("Total skills: %d\n\n", len(skills)))

	for i, s := range skills {
		b.WriteString(fmt.Sprintf("## Skill %d: %s\n\n", i+1, s.Name))
		if s.Description != "" {
			b.WriteString(fmt.Sprintf("Description: %s\n", s.Description))
		}
		if s.Category != "" {
			b.WriteString(fmt.Sprintf("Category: %s\n", s.Category))
		}
		if len(s.Parameters) > 0 {
			b.WriteString("Parameters:\n")
			for _, p := range s.Parameters {
				req := ""
				if p.Required {
					req = " (required)"
				}
				b.WriteString(fmt.Sprintf("  - %s: %s%s", p.Name, p.Type, req))
				if p.Description != "" {
					b.WriteString(fmt.Sprintf(" -- %s", p.Description))
				}
				b.WriteString("\n")
			}
		}
		if len(s.Examples) > 0 {
			b.WriteString("Examples:\n")
			for _, ex := range s.Examples {
				b.WriteString(fmt.Sprintf("  - %s\n", ex.Description))
				if len(ex.Args) > 0 {
					data, _ := json.Marshal(ex.Args)
					b.WriteString(fmt.Sprintf("    Args: %s\n", string(data)))
				}
				if ex.ExpectedOutput != "" {
					b.WriteString(fmt.Sprintf("    Expected: %s\n", ex.ExpectedOutput))
				}
			}
		}
		b.WriteString("\n")
	}

	return b.String()
}
