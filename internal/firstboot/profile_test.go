package firstboot

import (
	"path/filepath"
	"testing"
)

func TestDefaultConfigDir_FallsBackToXDGWhenHomeMissing(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", xdg)

	if got, want := DefaultConfigDir(), filepath.Join(xdg, "ralphglasses"); got != want {
		t.Fatalf("DefaultConfigDir() = %q, want %q", got, want)
	}
}

func TestDefaultConfigDir_UsesLegacyHomeDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")

	if got, want := DefaultConfigDir(), filepath.Join(home, ".ralphglasses"); got != want {
		t.Fatalf("DefaultConfigDir() = %q, want %q", got, want)
	}
}

func TestSaveLoad_RoundTripAndMarker(t *testing.T) {
	configDir := t.TempDir()
	profile := Profile{
		Hostname: "ralph-02",
		APIKeys: map[string]string{
			"anthropic": "a",
			"google":    "g",
			"openai":    "o",
		},
		Autonomy: 2,
		FleetURL: "https://fleet.example",
	}

	if err := Save(configDir, profile, true); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	loaded, done, err := Load(configDir)
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if !done {
		t.Fatal("expected marker file to mark firstboot done")
	}
	if loaded.Hostname != profile.Hostname {
		t.Fatalf("hostname = %q, want %q", loaded.Hostname, profile.Hostname)
	}
	if loaded.Autonomy != profile.Autonomy {
		t.Fatalf("autonomy = %d, want %d", loaded.Autonomy, profile.Autonomy)
	}
	if loaded.FleetURL != profile.FleetURL {
		t.Fatalf("fleet url = %q, want %q", loaded.FleetURL, profile.FleetURL)
	}
}
