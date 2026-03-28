package envkit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMiseStatus(t *testing.T) {
	ctx := context.Background()
	info, err := MiseStatus(ctx)
	if err != nil {
		t.Fatalf("MiseStatus should not error even if mise is not installed: %v", err)
	}
	if info == nil {
		t.Fatal("MiseStatus returned nil")
	}
	if info.Tools == nil {
		t.Error("Tools map should be initialized, not nil")
	}
}

func TestGenerateMiseConfig(t *testing.T) {
	dir := t.TempDir()
	tools := map[string]string{
		"go":   "1.23",
		"node": "lts",
	}

	path, err := GenerateMiseConfig(dir, tools)
	if err != nil {
		t.Fatalf("GenerateMiseConfig: %v", err)
	}

	if filepath.Base(path) != ".mise.toml" {
		t.Errorf("expected .mise.toml, got %s", filepath.Base(path))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "[tools]") {
		t.Error("config should contain [tools] section")
	}
	if !strings.Contains(content, "go") {
		t.Error("config should contain go tool")
	}
	if !strings.Contains(content, "node") {
		t.Error("config should contain node tool")
	}
}

func TestGenerateMiseConfigDefaults(t *testing.T) {
	dir := t.TempDir()

	path, err := GenerateMiseConfig(dir, nil)
	if err != nil {
		t.Fatalf("GenerateMiseConfig with nil tools: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	content := string(data)
	for _, tool := range []string{"go", "node", "python"} {
		if !strings.Contains(content, tool) {
			t.Errorf("default config should contain %s", tool)
		}
	}
}

func TestDefaultMiseTools(t *testing.T) {
	tools := DefaultMiseTools()
	if len(tools) == 0 {
		t.Fatal("DefaultMiseTools should return non-empty map")
	}
	if _, ok := tools["go"]; !ok {
		t.Error("should include go")
	}
	if _, ok := tools["node"]; !ok {
		t.Error("should include node")
	}
	if _, ok := tools["python"]; !ok {
		t.Error("should include python")
	}
}
