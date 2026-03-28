package themekit

import (
	"encoding/json"
	"os"
	"testing"
)

func TestExportITerm2(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := ExportITerm2(Catppuccin(Mocha))
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var profile map[string]any
	if err := json.Unmarshal(data, &profile); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	profiles, ok := profile["Profiles"].([]any)
	if !ok || len(profiles) == 0 {
		t.Fatal("Profiles missing or empty")
	}
	p, ok := profiles[0].(map[string]any)
	if !ok {
		t.Fatal("first profile is not a map")
	}

	if got, ok := p["Name"].(string); !ok || got != "Claudekit Catppuccin Mocha" {
		t.Errorf("Name = %v", p["Name"])
	}

	// Verify background color components
	bg, ok := p["Background Color"].(map[string]any)
	if !ok {
		t.Fatal("Background Color missing or wrong type")
	}
	if r, ok := bg["Red Component"].(float64); !ok || r < 0.11 || r > 0.13 {
		t.Errorf("Background Red = %v, expected ~0.118 (30/255)", bg["Red Component"])
	}
}
