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

	"github.com/hairglasses-studio/ralphglasses/internal/config"
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

// SkillSurfaceConfig describes the projected skill surfaces to keep in sync
// across Codex, Claude, and the local plugin bundle.
type SkillSurfaceConfig struct {
	Version    int                `json:"version"`
	PluginRoot string             `json:"plugin_root"`
	Skills     []SkillSurfaceSpec `json:"skills"`
}

// SkillSurfaceSpec describes one skill surface projection.
type SkillSurfaceSpec struct {
	Name                   string `json:"name"`
	ClaudeIncludeCanonical bool   `json:"claude_include_canonical,omitempty"`
	ExportPlugin           bool   `json:"export_plugin,omitempty"`
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
	b.WriteString(fmt.Sprintf("Auto-generated from the live MCP contract. %d tools available.\n\n", len(tools)))

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

// LoadSkillSurfaceConfig reads the repo-local skill surface config. The file is
// JSON-shaped even though it is named surface.yaml so it can be read without a
// YAML dependency from the generation path.
func LoadSkillSurfaceConfig(repoPath string) (SkillSurfaceConfig, error) {
	path := filepath.Join(repoPath, ".agents", "skills", "surface.yaml")
	var cfg SkillSurfaceConfig
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	if cfg.PluginRoot == "" {
		cfg.PluginRoot = "ralphglasses"
	}
	return cfg, nil
}

// SyncProjectedSkills copies canonical `.agents/skills/*` files into the
// provider and plugin skill surfaces declared in surface.yaml.
func SyncProjectedSkills(repoPath string) error {
	cfg, err := LoadSkillSurfaceConfig(repoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, skill := range cfg.Skills {
		if skill.Name == "" || skill.Name == "ralphglasses" {
			continue
		}
		src := filepath.Join(repoPath, ".agents", "skills", skill.Name, "SKILL.md")
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read canonical skill %s: %w", src, err)
		}

		if err := writeProjectedSkill(filepath.Join(repoPath, ".claude", "skills", skill.Name, "SKILL.md"), data); err != nil {
			return err
		}
		if skill.ExportPlugin {
			if err := writeProjectedSkill(filepath.Join(repoPath, "plugins", cfg.PluginRoot, "skills", skill.Name, "SKILL.md"), data); err != nil {
				return err
			}
		}
	}

	keepClaude := map[string]bool{"ralphglasses": true}
	keepPlugin := map[string]bool{"ralphglasses": true}
	for _, skill := range cfg.Skills {
		if skill.Name == "" {
			continue
		}
		keepClaude[skill.Name] = true
		if skill.ExportPlugin {
			keepPlugin[skill.Name] = true
		}
	}
	if err := pruneProjectedSkills(filepath.Join(repoPath, ".claude", "skills"), keepClaude); err != nil {
		return err
	}
	if err := pruneProjectedSkills(filepath.Join(repoPath, "plugins", cfg.PluginRoot, "skills"), keepPlugin); err != nil {
		return err
	}
	return nil
}

// GenerateSkillSurfaces regenerates the canonical mega-skill plus all declared
// focused skill projections.
func GenerateSkillSurfaces(repoPath string, tools []ToolDescription) error {
	if err := GenerateSkillFile(repoPath, tools); err != nil {
		return err
	}
	return SyncProjectedSkills(repoPath)
}

func writeProjectedSkill(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create projected skill dir %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write projected skill %s: %w", path, err)
	}
	return nil
}

func pruneProjectedSkills(baseDir string, keep map[string]bool) error {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read projected skills %s: %w", baseDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if keep[name] || !strings.HasPrefix(name, "ralphglasses") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(baseDir, name)); err != nil {
			return fmt.Errorf("remove stale projected skill %s: %w", filepath.Join(baseDir, name), err)
		}
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
				"args": []string{
					"-lc",
					"exec ./scripts/dev/run-mcp.sh --scan-path " + config.DefaultScanPath,
				},
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
