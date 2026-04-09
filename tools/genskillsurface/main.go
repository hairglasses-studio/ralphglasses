// genskillsurface writes the provider-native checked-in Ralph skill surfaces
// from the live MCP contract. This keeps `.agents/`, `.claude/`, and the local
// Codex plugin bundle aligned with the actual server surface.
//
// Usage:
//
//	go run ./tools/genskillsurface
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func main() {
	check := flag.Bool("check", false, "Verify checked-in skill surfaces are up to date instead of rewriting them")
	flag.Parse()

	repoRoot, err := filepath.Abs(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "genskillsurface: resolve repo root: %v\n", err)
		os.Exit(1)
	}
	if *check {
		if err := checkSkillSurfaces(repoRoot); err != nil {
			fmt.Fprintf(os.Stderr, "genskillsurface: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := generateSurfaces(repoRoot); err != nil {
		fmt.Fprintf(os.Stderr, "genskillsurface: %v\n", err)
		os.Exit(1)
	}
}

func buildToolDocs(repoRoot string) []session.ToolDescription {
	srv := mcpserver.NewServer(repoRoot)
	toolDocs := make([]session.ToolDescription, 0)

	for _, group := range srv.ToolGroups() {
		for _, entry := range group.Tools {
			toolDocs = append(toolDocs, session.ToolDescription{
				Name:        entry.Tool.Name,
				Description: entry.Tool.Description,
				Namespace:   group.Name,
			})
		}
	}

	for _, entry := range srv.ManagementTools() {
		toolDocs = append(toolDocs, session.ToolDescription{
			Name:        entry.Tool.Name,
			Description: entry.Tool.Description,
			Namespace:   "management",
		})
	}

	return toolDocs
}

func checkSkillSurfaces(repoRoot string) error {
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return fmt.Errorf("resolve repo root: %w", err)
	}
	tempRoot, err := os.MkdirTemp("", "ralphglasses-skill-surface-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tempRoot)

	cfg, err := session.LoadSkillSurfaceConfig(repoRoot)
	if err != nil {
		return err
	}
	if err := copyCanonicalSkillInputs(repoRoot, tempRoot, cfg); err != nil {
		return err
	}
	if err := generateSurfacesWithToolDocs(tempRoot, buildToolDocs(repoRoot)); err != nil {
		return err
	}

	filesToCheck := []string{
		filepath.Join(".agents", "skills", "ralphglasses", "SKILL.md"),
		filepath.Join(".claude", "skills", "ralphglasses", "SKILL.md"),
		filepath.Join("plugins", cfg.PluginRoot, "skills", "ralphglasses", "SKILL.md"),
		filepath.Join(".agents", "plugins", "marketplace.json"),
		filepath.Join("plugins", cfg.PluginRoot, ".codex-plugin", "plugin.json"),
		filepath.Join("plugins", cfg.PluginRoot, ".mcp.json"),
	}
	for _, skill := range cfg.Skills {
		if skill.Name == "" || skill.Name == "ralphglasses" {
			continue
		}
		filesToCheck = append(filesToCheck,
			filepath.Join(".claude", "skills", skill.Name, "SKILL.md"),
		)
		if skill.ExportPlugin {
			filesToCheck = append(filesToCheck,
				filepath.Join("plugins", cfg.PluginRoot, "skills", skill.Name, "SKILL.md"),
			)
		}
	}
	antigravityFiles, err := generatedAntigravityFiles(repoRoot)
	if err != nil {
		return err
	}
	filesToCheck = append(filesToCheck, antigravityFiles...)

	for _, rel := range filesToCheck {
		expected, err := os.ReadFile(filepath.Join(tempRoot, rel))
		if err != nil {
			return fmt.Errorf("read generated %s: %w", rel, err)
		}
		actual, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			return fmt.Errorf("read checked-in %s: %w", rel, err)
		}
		if string(actual) != string(expected) {
			return fmt.Errorf("checked-in skill surface drift detected in %s; run `go run ./tools/genskillsurface`", rel)
		}
	}

	return nil
}

func generateSurfaces(repoRoot string) error {
	return generateSurfacesWithToolDocs(repoRoot, buildToolDocs(repoRoot))
}

func generateSurfacesWithToolDocs(repoRoot string, toolDocs []session.ToolDescription) error {
	if err := session.GenerateSkillSurfaces(repoRoot, toolDocs); err != nil {
		return err
	}
	return generateAntigravitySurfaces(repoRoot)
}

func copyCanonicalSkillInputs(srcRoot, dstRoot string, cfg session.SkillSurfaceConfig) error {
	if err := os.MkdirAll(filepath.Join(dstRoot, ".agents", "skills"), 0o755); err != nil {
		return err
	}
	srcSurface := filepath.Join(srcRoot, ".agents", "skills", "surface.yaml")
	data, err := os.ReadFile(srcSurface)
	if err != nil {
		return fmt.Errorf("read %s: %w", srcSurface, err)
	}
	if err := os.WriteFile(filepath.Join(dstRoot, ".agents", "skills", "surface.yaml"), data, 0o644); err != nil {
		return fmt.Errorf("write surface config: %w", err)
	}
	if err := copyFile(filepath.Join(srcRoot, "AGENTS.md"), filepath.Join(dstRoot, "AGENTS.md")); err != nil {
		return err
	}
	ruleEntries, err := os.ReadDir(filepath.Join(srcRoot, ".claude", "rules"))
	if err != nil {
		return fmt.Errorf("read .claude/rules: %w", err)
	}
	for _, entry := range ruleEntries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		if err := copyFile(filepath.Join(srcRoot, ".claude", "rules", entry.Name()), filepath.Join(dstRoot, ".claude", "rules", entry.Name())); err != nil {
			return err
		}
	}

	for _, skill := range cfg.Skills {
		if skill.Name == "" || skill.Name == "ralphglasses" {
			continue
		}
		src := filepath.Join(srcRoot, ".agents", "skills", skill.Name, "SKILL.md")
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("read canonical skill %s: %w", src, err)
		}
		dst := filepath.Join(dstRoot, ".agents", "skills", skill.Name, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", dst, err)
		}
	}

	return nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(dst), err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}
