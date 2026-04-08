package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateSkillFile(t *testing.T) {
	dir := t.TempDir()
	tools := []ToolDescription{
		{Name: "ralphglasses_session_launch", Description: "Launch a new session", Namespace: "session"},
		{Name: "ralphglasses_session_stop", Description: "Stop a running session", Namespace: "session"},
		{Name: "ralphglasses_status", Description: "Get server status", Namespace: "core"},
		{Name: "ralphglasses_loop_start", Description: "Start improvement loop", Namespace: "loop"},
	}

	if err := GenerateSkillFile(dir, tools); err != nil {
		t.Fatalf("GenerateSkillFile: %v", err)
	}

	path := filepath.Join(dir, ".claude", "skills", "ralphglasses", "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read skill file: %v", err)
	}

	content := string(data)

	// Check frontmatter
	if !strings.Contains(content, "---") {
		t.Error("expected YAML frontmatter")
	}
	if !strings.Contains(content, "Auto-generated from the live MCP contract.") {
		t.Error("expected deterministic generation banner")
	}
	if !strings.Contains(content, "allowed-tools:") {
		t.Error("expected allowed-tools in frontmatter")
	}
	if !strings.Contains(content, "  - ralphglasses_session_launch") {
		t.Error("expected session_launch in allowed-tools")
	}

	// Check namespace grouping
	if !strings.Contains(content, "## session") {
		t.Error("expected session namespace heading")
	}
	if !strings.Contains(content, "## core") {
		t.Error("expected core namespace heading")
	}

	// Check tool descriptions
	if !strings.Contains(content, "**ralphglasses_status**: Get server status") {
		t.Error("expected tool description")
	}

	agentsPath := filepath.Join(dir, ".agents", "skills", "ralphglasses", "SKILL.md")
	agentsData, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read codex skill file: %v", err)
	}
	if string(agentsData) != content {
		t.Error("expected Claude and Codex skill exports to match")
	}

	pluginSkillPath := filepath.Join(dir, "plugins", "ralphglasses", "skills", "ralphglasses", "SKILL.md")
	pluginSkillData, err := os.ReadFile(pluginSkillPath)
	if err != nil {
		t.Fatalf("read codex plugin skill file: %v", err)
	}
	if string(pluginSkillData) != content {
		t.Error("expected plugin bundle skill export to match primary skill content")
	}

	pluginManifestPath := filepath.Join(dir, "plugins", "ralphglasses", ".codex-plugin", "plugin.json")
	if _, err := os.Stat(pluginManifestPath); err != nil {
		t.Fatalf("expected plugin manifest to exist: %v", err)
	}

	marketplacePath := filepath.Join(dir, ".agents", "plugins", "marketplace.json")
	if _, err := os.Stat(marketplacePath); err != nil {
		t.Fatalf("expected marketplace manifest to exist: %v", err)
	}
}

func TestGenerateSkillSurfaces_ProjectsCanonicalSkills(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".agents", "skills", "surface-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agents", "skills", "surface-audit", "SKILL.md"), []byte("---\nname: surface-audit\n---\n\n# Surface Audit\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".agents", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	surfaceConfig := `{
  "version": 1,
  "plugin_root": "ralphglasses",
  "skills": [
    {"name": "ralphglasses", "claude_include_canonical": true, "export_plugin": true},
    {"name": "surface-audit", "export_plugin": true}
  ]
}
`
	if err := os.WriteFile(filepath.Join(dir, ".agents", "skills", "surface.yaml"), []byte(surfaceConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".claude", "skills", "ralphglasses-bootstrap"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude", "skills", "ralphglasses-bootstrap", "SKILL.md"), []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "plugins", "ralphglasses", "skills", "ralphglasses-bootstrap"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugins", "ralphglasses", "skills", "ralphglasses-bootstrap", "SKILL.md"), []byte("legacy"), 0o644); err != nil {
		t.Fatal(err)
	}

	tools := []ToolDescription{
		{Name: "ralphglasses_status", Description: "Get server status", Namespace: "core"},
	}
	if err := GenerateSkillSurfaces(dir, tools); err != nil {
		t.Fatalf("GenerateSkillSurfaces: %v", err)
	}

	for _, path := range []string{
		filepath.Join(dir, ".claude", "skills", "surface-audit", "SKILL.md"),
		filepath.Join(dir, "plugins", "ralphglasses", "skills", "surface-audit", "SKILL.md"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read projected skill %s: %v", path, err)
		}
		if !strings.Contains(string(data), "# Surface Audit") {
			t.Fatalf("projected skill %s missing content", path)
		}
	}

	for _, path := range []string{
		filepath.Join(dir, ".claude", "skills", "ralphglasses-bootstrap"),
		filepath.Join(dir, "plugins", "ralphglasses", "skills", "ralphglasses-bootstrap"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected stale projected skill to be removed: %s", path)
		}
	}
}
