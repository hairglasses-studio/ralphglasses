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
}
