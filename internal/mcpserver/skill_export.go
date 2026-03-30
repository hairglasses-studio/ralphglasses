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

func (a toolEntryAdapter) ToolName() string              { return a.entry.Tool.Name }
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
// yields category "session"). Tools without an underscore-separated namespace default
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

// inferCategory extracts a namespace category from a tool name.
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
