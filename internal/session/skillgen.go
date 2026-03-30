// Package session contains skillgen.go which generates Claude Code skill files
// from MCP tool descriptions for the ralphglasses MCP server.
package session

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// SkillMetadata holds the metadata for a generated SKILL.md file.
type SkillMetadata struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	AllowedTools []string `yaml:"allowed-tools"`
}

// ToolDescription describes an MCP tool for skill generation.
type ToolDescription struct {
	Name        string
	Description string
	Namespace   string
}

// GenerateSkillFile creates a .claude/skills/ralphglasses/SKILL.md from tool descriptions.
func GenerateSkillFile(repoPath string, tools []ToolDescription) error {
	dir := filepath.Join(repoPath, ".claude", "skills", "ralphglasses")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create skill dir: %w", err)
	}

	var b strings.Builder

	// YAML frontmatter
	b.WriteString("---\n")
	b.WriteString("name: ralphglasses\n")
	b.WriteString("description: Fleet management and self-improvement tools for ralphglasses\n")
	b.WriteString("allowed-tools:\n")

	// Collect and sort tool names
	names := make([]string, 0, len(tools))
	for _, t := range tools {
		names = append(names, t.Name)
	}
	sort.Strings(names)
	for _, name := range names {
		b.WriteString(fmt.Sprintf("  - %s\n", name))
	}
	b.WriteString("---\n\n")

	// Markdown body
	b.WriteString("# Ralphglasses MCP Tools\n\n")
	b.WriteString(fmt.Sprintf("Auto-generated on %s. %d tools available.\n\n", time.Now().Format("2006-01-02"), len(tools)))

	// Group by namespace
	byNS := make(map[string][]ToolDescription)
	for _, t := range tools {
		ns := t.Namespace
		if ns == "" {
			ns = "core"
		}
		byNS[ns] = append(byNS[ns], t)
	}

	nsNames := make([]string, 0, len(byNS))
	for ns := range byNS {
		nsNames = append(nsNames, ns)
	}
	sort.Strings(nsNames)

	for _, ns := range nsNames {
		b.WriteString(fmt.Sprintf("## %s\n\n", ns))
		nsTools := byNS[ns]
		sort.Slice(nsTools, func(i, j int) bool { return nsTools[i].Name < nsTools[j].Name })
		for _, t := range nsTools {
			b.WriteString(fmt.Sprintf("- **%s**: %s\n", t.Name, t.Description))
		}
		b.WriteString("\n")
	}

	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(b.String()), 0644)
}
