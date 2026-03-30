package gvisor

import (
	"testing"
)

func TestProfileMinimal(t *testing.T) {
	p := ProfileMinimal()
	if p.Name != "minimal" {
		t.Errorf("ProfileMinimal().Name = %q, want minimal", p.Name)
	}
	if p.Network != NetworkNone {
		t.Errorf("ProfileMinimal().Network = %v, want NetworkNone", p.Network)
	}
	if !p.ReadOnly {
		t.Error("ProfileMinimal().ReadOnly should be true")
	}
}

func TestProfileStandard(t *testing.T) {
	p := ProfileStandard()
	if p.Name != "standard" {
		t.Errorf("ProfileStandard().Name = %q, want standard", p.Name)
	}
	if p.Network != NetworkHost {
		t.Errorf("ProfileStandard().Network = %v, want NetworkHost", p.Network)
	}
	if p.ReadOnly {
		t.Error("ProfileStandard().ReadOnly should be false")
	}
	if p.Platform != PlatformPtrace {
		t.Errorf("ProfileStandard().Platform = %v, want PlatformPtrace", p.Platform)
	}
}

func TestProfilePrivileged(t *testing.T) {
	p := ProfilePrivileged()
	if p.Name != "privileged" {
		t.Errorf("ProfilePrivileged().Name = %q, want privileged", p.Name)
	}
	if p.Platform != PlatformKVM {
		t.Errorf("ProfilePrivileged().Platform = %v, want PlatformKVM", p.Platform)
	}
	if p.ReadOnly {
		t.Error("ProfilePrivileged().ReadOnly should be false")
	}
}

func TestProfile_Options_NoMounts(t *testing.T) {
	p := ProfileMinimal()
	opts := p.Options()
	// Should have at least 2 options: network + platform.
	if len(opts) < 2 {
		t.Errorf("Options() returned %d opts, want >= 2", len(opts))
	}
}

func TestProfile_Options_WithMounts(t *testing.T) {
	p := ProfileStandard()
	p.Mounts = []Mount{
		{Source: "/tmp", Target: "/workspace", ReadOnly: false},
	}
	opts := p.Options()
	// Should have 3 options: network + platform + mounts.
	if len(opts) < 3 {
		t.Errorf("Options() with mounts returned %d opts, want >= 3", len(opts))
	}
}

func TestAllProfiles(t *testing.T) {
	profiles := AllProfiles()
	if len(profiles) == 0 {
		t.Fatal("AllProfiles should return non-empty map")
	}

	expectedNames := []string{"minimal", "standard", "privileged"}
	for _, name := range expectedNames {
		p, ok := profiles[name]
		if !ok {
			t.Errorf("AllProfiles missing %q", name)
			continue
		}
		if p.Name != name {
			t.Errorf("profiles[%q].Name = %q, want %q", name, p.Name, name)
		}
	}
}

func TestAllProfiles_ReturnsNewMap(t *testing.T) {
	// Mutating the returned map should not affect subsequent calls.
	p1 := AllProfiles()
	p1["custom"] = Profile{Name: "custom"}

	p2 := AllProfiles()
	if _, ok := p2["custom"]; ok {
		t.Error("AllProfiles should return a new map on each call")
	}
}
