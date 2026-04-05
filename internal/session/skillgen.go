// Package session contains skillgen.go which generates provider-native skill files
// from MCP tool descriptions for the ralphglasses MCP server.
package session

import (
	"encoding/json"
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

// GenerateSkillFile creates provider-native skill exports from tool descriptions.
func GenerateSkillFile(repoPath string, tools []ToolDescription) error {
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

	for _, dir := range []string{
		filepath.Join(repoPath, ".claude", "skills", "ralphglasses"),
		filepath.Join(repoPath, ".agents", "skills", "ralphglasses"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create skill dir %s: %w", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(b.String()), 0644); err != nil {
			return fmt.Errorf("write skill file %s: %w", dir, err)
		}
	}

	if err := generateCodexPluginBundle(repoPath, b.String()); err != nil {
		return err
	}

	return nil
}

func generateCodexPluginBundle(repoPath, skillContent string) error {
	pluginRoot := filepath.Join(repoPath, "plugins", "ralphglasses")
	skillDir := filepath.Join(pluginRoot, "skills", "ralphglasses")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return fmt.Errorf("create codex plugin skill dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(pluginRoot, ".codex-plugin"), 0755); err != nil {
		return fmt.Errorf("create codex plugin manifest dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(repoPath, ".agents", "plugins"), 0755); err != nil {
		return fmt.Errorf("create codex marketplace dir: %w", err)
	}

	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		return fmt.Errorf("write codex plugin skill file: %w", err)
	}

	manifest := map[string]any{
		"name":        "ralphglasses",
		"version":     "0.1.0",
		"description": "Ralphglasses MCP tools and Codex-native workflow assets.",
		"homepage":    "https://github.com/hairglasses-studio/ralphglasses",
		"repository":  "https://github.com/hairglasses-studio/ralphglasses",
		"license":     "MIT",
		"keywords":    []string{"codex", "mcp", "skills", "fleet", "automation"},
		"skills":      "./skills/",
		"mcpServers":  "./.mcp.json",
		"interface": map[string]any{
			"displayName":      "Ralphglasses",
			"shortDescription": "Fleet control and repo automation for Codex.",
			"longDescription":  "Codex-native skill and MCP bundle for ralphglasses command-and-control workflows.",
			"developerName":    "hairglasses-studio",
			"category":         "Developer Tools",
			"capabilities":     []string{"Interactive", "Write"},
			"defaultPrompt": []string{
				"Launch a codex-first repo session with ralphglasses.",
				"Show the active fleet status and budget pressure.",
				"Export roadmap work into actionable loop tasks.",
			},
			"brandColor": "#0f766e",
		},
	}
	if err := writeJSONFile(filepath.Join(pluginRoot, ".codex-plugin", "plugin.json"), manifest); err != nil {
		return fmt.Errorf("write codex plugin manifest: %w", err)
	}

	mcpConfig := map[string]any{
		"mcpServers": map[string]any{
			"ralphglasses": map[string]any{
				"command": "bash",
				"args":    []string{"./scripts/dev/run-mcp.sh", "--scan-path", "~/hairglasses-studio"},
			},
		},
	}
	if err := writeJSONFile(filepath.Join(pluginRoot, ".mcp.json"), mcpConfig); err != nil {
		return fmt.Errorf("write codex plugin mcp config: %w", err)
	}

	marketplace := map[string]any{
		"name": "ralphglasses-local",
		"interface": map[string]any{
			"displayName": "Ralphglasses Local Plugins",
		},
		"plugins": []map[string]any{
			{
				"name": "ralphglasses",
				"source": map[string]any{
					"source": "local",
					"path":   "./plugins/ralphglasses",
				},
				"policy": map[string]any{
					"installation":   "AVAILABLE",
					"authentication": "ON_INSTALL",
				},
				"category": "Developer Tools",
			},
		},
	}
	if err := writeJSONFile(filepath.Join(repoPath, ".agents", "plugins", "marketplace.json"), marketplace); err != nil {
		return fmt.Errorf("write codex marketplace: %w", err)
	}

	return nil
}

func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0644)
}
