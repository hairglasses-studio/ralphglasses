package gvisor

// Profile represents a pre-built sandbox configuration profile.
type Profile struct {
	Name     string      `json:"name"`
	Network  NetworkMode `json:"network"`
	Platform Platform    `json:"platform"`
	Mounts   []Mount     `json:"mounts,omitempty"`
	ReadOnly bool        `json:"read_only"`
}

// ProfileMinimal returns a locked-down profile with no network access
// and a read-only filesystem. Suitable for pure computation tasks.
func ProfileMinimal() Profile {
	return Profile{
		Name:     "minimal",
		Network:  NetworkNone,
		Platform: PlatformPtrace,
		ReadOnly: true,
	}
}

// ProfileStandard returns a profile with limited network access (host networking
// filtered by the gVisor network stack) and read-write filesystem.
// Suitable for tasks that need outbound network access.
func ProfileStandard() Profile {
	return Profile{
		Name:     "standard",
		Network:  NetworkHost,
		Platform: PlatformPtrace,
		ReadOnly: false,
	}
}

// ProfilePrivileged returns a profile with full host network access and KVM
// platform for near-native performance. Suitable for trusted workloads
// that require maximum throughput.
func ProfilePrivileged() Profile {
	return Profile{
		Name:     "privileged",
		Network:  NetworkHost,
		Platform: PlatformKVM,
		ReadOnly: false,
	}
}

// Options converts a Profile into SandboxOptions suitable for CreateSandbox.
func (p Profile) Options() []SandboxOption {
	opts := []SandboxOption{
		WithNetwork(p.Network),
		WithPlatform(p.Platform),
	}
	if len(p.Mounts) > 0 {
		opts = append(opts, WithFilesystem(p.Mounts))
	}
	return opts
}

// AllProfiles returns all built-in profiles indexed by name.
func AllProfiles() map[string]Profile {
	return map[string]Profile{
		"minimal":    ProfileMinimal(),
		"standard":   ProfileStandard(),
		"privileged": ProfilePrivileged(),
	}
}
