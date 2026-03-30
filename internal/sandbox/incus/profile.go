package incus

// Profile defines a pre-built container configuration for agent sandboxing.
// Each profile specifies resource limits and optional features appropriate
// for a particular workload class.
type Profile struct {
	Name        string   // Human-readable profile name.
	Description string   // What this profile is designed for.
	Image       string   // Default container image.
	CPU         int      // CPU core limit (0 = host default).
	Memory      string   // Memory limit (e.g. "2GB").
	Network     string   // Network to attach ("" = none).
	Mounts      []Mount  // Default bind mounts.
	GPUDevices  []string // GPU device IDs for passthrough (empty = none).
}

// ProfileMinimal provides a lightweight container for quick tasks like linting,
// formatting, or single-file operations. No network, no GPU, tight limits.
var ProfileMinimal = Profile{
	Name:        "minimal",
	Description: "Lightweight sandbox for linting, formatting, and single-file tasks",
	Image:       "ubuntu:24.04",
	CPU:         1,
	Memory:      "512MB",
	Network:     "",
	Mounts:      nil,
	GPUDevices:  nil,
}

// ProfileStandard provides a general-purpose container suitable for most agent
// work: code generation, test execution, multi-file edits. Bridge networking
// is enabled for package installation and API access.
var ProfileStandard = Profile{
	Name:        "standard",
	Description: "General-purpose sandbox for code generation, testing, and multi-file work",
	Image:       "ubuntu:24.04",
	CPU:         2,
	Memory:      "4GB",
	Network:     "incusbr0",
	Mounts:      nil,
	GPUDevices:  nil,
}

// ProfileGPUPassthrough provides a container with NVIDIA GPU access for
// ML/AI workloads, CUDA compilation, or model inference. Requires the host
// to have NVIDIA drivers and the Incus GPU device configured.
var ProfileGPUPassthrough = Profile{
	Name:        "gpu-passthrough",
	Description: "GPU-enabled sandbox for ML inference, CUDA builds, and model evaluation",
	Image:       "ubuntu:24.04",
	CPU:         4,
	Memory:      "16GB",
	Network:     "incusbr0",
	Mounts:      nil,
	GPUDevices:  []string{"gpu0"},
}

// Profiles returns all built-in profiles keyed by name.
func Profiles() map[string]Profile {
	return map[string]Profile{
		ProfileMinimal.Name:        ProfileMinimal,
		ProfileStandard.Name:       ProfileStandard,
		ProfileGPUPassthrough.Name: ProfileGPUPassthrough,
	}
}

// ProfileByName returns the named profile and true, or the zero value and false.
func ProfileByName(name string) (Profile, bool) {
	p, ok := Profiles()[name]
	return p, ok
}

// Options converts a Profile into ContainerOption values suitable for
// passing to Client.CreateContainer.
func (p Profile) Options() []ContainerOption {
	var opts []ContainerOption
	if p.CPU > 0 {
		opts = append(opts, WithCPU(p.CPU))
	}
	if p.Memory != "" {
		opts = append(opts, WithMemory(p.Memory))
	}
	if p.Network != "" {
		opts = append(opts, WithNetwork(p.Network))
	}
	if len(p.Mounts) > 0 {
		opts = append(opts, WithMounts(p.Mounts...))
	}
	return opts
}
