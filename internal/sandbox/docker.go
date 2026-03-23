// Package sandbox provides container isolation for LLM sessions.
// Phase 5 foundation: Docker containers with workspace bind-mounts and resource limits.
package sandbox

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ContainerConfig specifies resource limits and mounts for a sandbox container.
type ContainerConfig struct {
	Image       string            `json:"image"`                  // Docker image (default: "ubuntu:24.04")
	WorkDir     string            `json:"work_dir"`               // Host workspace path to bind-mount
	MountPath   string            `json:"mount_path,omitempty"`   // Container mount path (default: /workspace)
	CPUs        float64           `json:"cpus,omitempty"`         // CPU limit (e.g., 2.0)
	MemoryMB    int               `json:"memory_mb,omitempty"`    // Memory limit in MB (e.g., 4096)
	NetworkMode string            `json:"network_mode,omitempty"` // "none", "host", "bridge" (default: "none")
	Env         map[string]string `json:"env,omitempty"`          // Environment variables
	Timeout     time.Duration     `json:"timeout,omitempty"`      // Container timeout (default: 1h)
	ReadOnly    bool              `json:"read_only,omitempty"`    // Read-only root filesystem
}

// DefaultContainerConfig returns a secure default configuration.
func DefaultContainerConfig(workDir string) ContainerConfig {
	return ContainerConfig{
		Image:       "ubuntu:24.04",
		WorkDir:     workDir,
		MountPath:   "/workspace",
		CPUs:        2.0,
		MemoryMB:    4096,
		NetworkMode: "none",
		Timeout:     time.Hour,
		ReadOnly:    false,
	}
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

// DockerAvailable checks if docker is installed and the daemon is running.
func DockerAvailable() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "info", "--format", "{{.ServerVersion}}").Output()
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
	}

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

	// Environment
	for k, v := range config.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, config.Image)

	cmd := exec.CommandContext(ctx, "docker", args...)
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
	cmd := exec.CommandContext(ctx, "docker", "start", c.ID)
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
	cmd := exec.CommandContext(ctx, "docker", args...)
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
	cmd := exec.CommandContext(ctx, "docker", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker stop: %w: %s", err, strings.TrimSpace(string(out)))
	}
	c.Status = "exited"
	return nil
}

// Remove removes a container (must be stopped first).
func Remove(ctx context.Context, c *Container) error {
	cmd := exec.CommandContext(ctx, "docker", "rm", "-f", c.ID)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker rm: %w: %s", err, strings.TrimSpace(string(out)))
	}
	c.Status = "removed"
	return nil
}

// Inspect returns container details.
func Inspect(ctx context.Context, containerID string) (map[string]any, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", containerID)
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
