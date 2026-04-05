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
