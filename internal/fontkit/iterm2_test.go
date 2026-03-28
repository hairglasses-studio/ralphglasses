package fontkit

import (
	"encoding/json"
	"os"
	"testing"
)

func TestConfigureITerm2(t *testing.T) {
	// Use a temp dir to avoid polluting real iTerm2 config
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := ConfigureITerm2(ITerm2Opts{FontSize: 14})
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
	if !ok {
		t.Fatal("Profiles missing or wrong type")
	}
	if len(profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(profiles))
	}

	p, ok := profiles[0].(map[string]any)
	if !ok {
		t.Fatal("first profile is not a map")
	}
	if got, ok := p["Normal Font"].(string); !ok || got != "MonaspiceNeNFM-Regular 14" {
		t.Errorf("Normal Font = %v, want MonaspiceNeNFM-Regular 14", p["Normal Font"])
	}
	if got, ok := p["Non Ascii Font"].(string); !ok || got != "MenloRegular 14" {
		t.Errorf("Non Ascii Font = %v, want MenloRegular 14", p["Non Ascii Font"])
	}
	if got, ok := p["Use Non-ASCII Font"].(bool); !ok || !got {
		t.Error("Use Non-ASCII Font should be true")
	}
}
