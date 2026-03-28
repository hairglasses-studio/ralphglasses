package fontkit

import (
	"encoding/json"
	"os"
	"testing"
)

func TestITerm2Opts_FontSizeDefault(t *testing.T) {
	opts := ITerm2Opts{}
	if got := opts.fontSize(); got != 15 {
		t.Errorf("default fontSize() = %d, want 15", got)
	}
}

func TestITerm2Opts_FontSizeCustom(t *testing.T) {
	opts := ITerm2Opts{FontSize: 20}
	if got := opts.fontSize(); got != 20 {
		t.Errorf("fontSize() = %d, want 20", got)
	}
}

func TestITerm2Opts_ProfileNameDefault(t *testing.T) {
	opts := ITerm2Opts{}
	if got := opts.profileName(); got != iterm2ProfileName {
		t.Errorf("default profileName() = %q, want %q", got, iterm2ProfileName)
	}
}

func TestITerm2Opts_ProfileNameCustom(t *testing.T) {
	opts := ITerm2Opts{ProfileName: "Custom Profile"}
	if got := opts.profileName(); got != "Custom Profile" {
		t.Errorf("profileName() = %q, want %q", got, "Custom Profile")
	}
}

func TestConfigureITerm2_DefaultOpts(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)

	path, err := ConfigureITerm2(ITerm2Opts{})
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

	profiles := profile["Profiles"].([]any)
	p := profiles[0].(map[string]any)
	// Default font size is 15
	if got := p["Normal Font"].(string); got != "MonaspiceNeNFM-Regular 15" {
		t.Errorf("Normal Font = %v, want MonaspiceNeNFM-Regular 15", got)
	}
	// Default profile name
	if got := p["Name"].(string); got != iterm2ProfileName {
		t.Errorf("Name = %v, want %v", got, iterm2ProfileName)
	}
}

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
