// Package sandbox provides container isolation for LLM sessions.
// Phase 5 foundation: Docker containers with workspace bind-mounts and resource limits.
package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// ContainerConfig specifies resource limits and mounts for a sandbox container.
type ContainerConfig struct {
	Image        string            `json:"image"`                   // Docker image (default: "ubuntu:24.04")
	WorkDir      string            `json:"work_dir"`                // Host workspace path to bind-mount
	MountPath    string            `json:"mount_path,omitempty"`    // Container mount path (default: /workspace)
	CPUs         float64           `json:"cpus,omitempty"`          // CPU limit (e.g., 2.0)
	MemoryMB     int               `json:"memory_mb,omitempty"`     // Memory limit in MB (e.g., 4096)
	MemorySwapMB int               `json:"memory_swap_mb,omitempty"` // Memory+swap limit in MB (default: same as MemoryMB to disable swap)
	PidsLimit    int               `json:"pids_limit,omitempty"`    // Max PIDs to prevent fork bombs (default: 256)
	UlimitNofile string            `json:"ulimit_nofile,omitempty"` // File descriptor ulimit soft:hard (default: "1024:2048")
	NetworkMode  string            `json:"network_mode,omitempty"`  // "none", "host", "bridge" (default: "none")
	Env          map[string]string `json:"env,omitempty"`           // Environment variables
	Timeout      time.Duration     `json:"timeout,omitempty"`       // Container timeout (default: 1h)
	ReadOnly     bool              `json:"read_only,omitempty"`     // Read-only root filesystem
}

// DefaultContainerConfig returns a secure default configuration.
func DefaultContainerConfig(workDir string) ContainerConfig {
	return ContainerConfig{
		Image:        "ubuntu:24.04",
		WorkDir:      workDir,
		MountPath:    "/workspace",
		CPUs:         2.0,
		MemoryMB:     4096,
		MemorySwapMB: 4096,
		PidsLimit:    256,
		UlimitNofile: "1024:2048",
		NetworkMode:  "none",
		Timeout:      time.Hour,
		ReadOnly:     false,
	}
}

// envKeyRe validates that environment variable keys are safe POSIX identifiers.
var envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateEnvKey checks that an environment variable key is a safe POSIX identifier.
// Keys must start with a letter or underscore and contain only alphanumeric characters
// and underscores. This prevents injection of docker flags via crafted key names.
func ValidateEnvKey(key string) error {
	if !envKeyRe.MatchString(key) {
		return fmt.Errorf("invalid env key %q: must match [A-Za-z_][A-Za-z0-9_]*", key)
	}
	return nil
}

// validateEnvValue checks that an environment variable value contains no null bytes,
// which could be used to truncate strings or confuse argument parsing.
func validateEnvValue(value string) error {
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("env value contains null byte")
	}
	return nil
}

// Container represents a running sandbox container.
type Container struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Config    ContainerConfig `json:"config"`
	Status    string          `json:"status"` // created, running, exited, removed
	CreatedAt time.Time       `json:"created_at"`
	StartedAt *time.Time      `json:"started_at,omitempty"`
	ExitCode  int             `json:"exit_code,omitempty"`
}

// execCommandContext is a variable for creating exec.Cmd instances.
// Tests can override this to mock command execution.
var execCommandContext = exec.CommandContext

// DockerAvailable checks if docker is installed and the daemon is running.
func DockerAvailable() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := execCommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}").Output()
	if err != nil {
		return fmt.Errorf("docker not available: %w", err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return fmt.Errorf("docker daemon not running")
	}
	return nil
}

// Create creates a new container without starting it.
func Create(ctx context.Context, name string, config ContainerConfig) (*Container, error) {
	if config.Image == "" {
		config.Image = "ubuntu:24.04"
	}
	if config.MountPath == "" {
		config.MountPath = "/workspace"
	}
	if config.Timeout == 0 {
		config.Timeout = time.Hour
	}

	args := []string{"create", "--name", name}

	// Resource limits
	if config.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.1f", config.CPUs))
	}
	if config.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", config.MemoryMB))
		// Set memory-swap equal to memory to disable swap abuse.
		// If MemorySwapMB is explicitly set, use that; otherwise match MemoryMB.
		swapMB := config.MemorySwapMB
		if swapMB == 0 {
			swapMB = config.MemoryMB
		}
		args = append(args, "--memory-swap", fmt.Sprintf("%dm", swapMB))
	}

	// Fork-bomb protection: limit number of PIDs in the container.
	pidsLimit := config.PidsLimit
	if pidsLimit == 0 {
		pidsLimit = 256
	}
	args = append(args, "--pids-limit", fmt.Sprintf("%d", pidsLimit))

	// File descriptor limits to prevent resource exhaustion.
	ulimitNofile := config.UlimitNofile
	if ulimitNofile == "" {
		ulimitNofile = "1024:2048"
	}
	args = append(args, "--ulimit", fmt.Sprintf("nofile=%s", ulimitNofile))

	// Network isolation
	if config.NetworkMode != "" {
		args = append(args, "--network", config.NetworkMode)
	}

	// Security
	if config.ReadOnly {
		args = append(args, "--read-only")
	}
	args = append(args, "--security-opt", "no-new-privileges")

	// Workspace bind mount
	if config.WorkDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:%s", config.WorkDir, config.MountPath))
		args = append(args, "-w", config.MountPath)
	}

	// Environment — validate keys and values to prevent docker flag injection.
	for k, v := range config.Env {
		if err := ValidateEnvKey(k); err != nil {
			slog.Warn("skipping invalid env key", "key", k, "error", err)
			continue
		}
		if err := validateEnvValue(v); err != nil {
			slog.Warn("skipping env var with invalid value", "key", k, "error", err)
			continue
		}
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, config.Image)

	cmd := execCommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker create: %w: %s", err, strings.TrimSpace(string(out)))
	}

	containerID := strings.TrimSpace(string(out))
	return &Container{
		ID:        containerID,
		Name:      name,
		Config:    config,
		Status:    "created",
		CreatedAt: time.Now(),
	}, nil
}

// Start starts a created container.
func Start(ctx context.Context, c *Container) error {
	cmd := execCommandContext(ctx, "docker", "start", c.ID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker start: %w: %s", err, strings.TrimSpace(string(out)))
	}
	now := time.Now()
	c.StartedAt = &now
	c.Status = "running"
	return nil
}

// Exec runs a command inside a running container and returns its output.
func Exec(ctx context.Context, containerID string, command []string) (string, int, error) {
	args := append([]string{"exec", containerID}, command...)
	cmd := execCommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if ok := isExitError(err, &exitErr); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return string(out), -1, fmt.Errorf("docker exec: %w", err)
		}
	}

	return string(out), exitCode, nil
}

// Stop gracefully stops a running container.
func Stop(ctx context.Context, c *Container, timeout int) error {
	args := []string{"stop", "-t", fmt.Sprintf("%d", timeout), c.ID}
	cmd := execCommandContext(ctx, "docker", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop: %w: %s", err, strings.TrimSpace(string(out)))
	}
	c.Status = "exited"
	return nil
}

// Remove removes a container (must be stopped first).
func Remove(ctx context.Context, c *Container) error {
	cmd := execCommandContext(ctx, "docker", "rm", "-f", c.ID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker rm: %w: %s", err, strings.TrimSpace(string(out)))
	}
	c.Status = "removed"
	return nil
}

// Inspect returns container details.
func Inspect(ctx context.Context, containerID string) (map[string]any, error) {
	cmd := execCommandContext(ctx, "docker", "inspect", containerID)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}

	var results []map[string]any
	if err := json.Unmarshal(out, &results); err != nil {
		return nil, fmt.Errorf("parse inspect: %w", err)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("container not found: %s", containerID)
	}
	return results[0], nil
}

// isExitError checks if err wraps an *exec.ExitError and assigns it to target.
func isExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}
