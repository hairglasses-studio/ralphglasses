// Package incus provides Incus container management for agent sandboxing.
// It wraps the incus CLI to create, start, stop, delete, and exec into
// system containers used as isolated execution environments for LLM sessions.
package incus

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// execCommandContext is a variable for creating exec.Cmd instances.
// Tests override this to mock command execution.
var execCommandContext = exec.CommandContext

// Container represents an Incus container with its current state and resource config.
type Container struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`     // Running, Stopped, etc.
	Image     string    `json:"image"`      // Source image alias or fingerprint.
	CreatedAt time.Time `json:"created_at"` // When the container was created.
	CPU       int       `json:"cpu"`        // CPU core limit (0 = unlimited).
	Memory    string    `json:"memory"`     // Memory limit (e.g. "4GB", "" = unlimited).
}

// ContainerOption configures optional settings when creating a container.
type ContainerOption func(*containerConfig)

// containerConfig holds the aggregated options for container creation.
type containerConfig struct {
	cpu     int
	memory  string
	network string
	mounts  []Mount
}

// Mount describes a host-to-container disk mount.
type Mount struct {
	Name   string // Device name within Incus.
	Source string // Host path.
	Path   string // Container path.
}

// WithCPU sets the CPU core limit for the container.
func WithCPU(cores int) ContainerOption {
	return func(c *containerConfig) {
		c.cpu = cores
	}
}

// WithMemory sets the memory limit (e.g. "2GB", "512MB").
func WithMemory(limit string) ContainerOption {
	return func(c *containerConfig) {
		c.memory = limit
	}
}

// WithNetwork sets the network name to attach (empty string for no network).
func WithNetwork(network string) ContainerOption {
	return func(c *containerConfig) {
		c.network = network
	}
}

// WithMounts adds host-to-container bind mounts.
func WithMounts(mounts ...Mount) ContainerOption {
	return func(c *containerConfig) {
		c.mounts = append(c.mounts, mounts...)
	}
}

// Client wraps the incus CLI for container management.
type Client struct {
	// incusBin is the path to the incus binary. Resolved at NewClient time.
	incusBin string
}

// NewClient creates a new Client by locating the incus binary and verifying
// that the Incus daemon is reachable.
func NewClient() (*Client, error) {
	bin, err := exec.LookPath("incus")
	if err != nil {
		return nil, fmt.Errorf("incus not found in PATH: %w", err)
	}

	c := &Client{incusBin: bin}

	// Verify daemon connectivity with a short timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, bin, "info")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("incus daemon not reachable: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return c, nil
}

// IsAvailable reports whether the incus CLI is installed and the daemon is
// responsive. It is a lightweight check suitable for feature-gating.
func IsAvailable() bool {
	bin, err := exec.LookPath("incus")
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return execCommandContext(ctx, bin, "info").Run() == nil
}

// CreateContainer creates a new stopped container from the given image.
func (c *Client) CreateContainer(name string, image string, opts ...ContainerOption) error {
	cfg := &containerConfig{}
	for _, o := range opts {
		o(cfg)
	}

	args := []string{"launch", image, name, "--no-start"}

	// Resource limits via config overrides.
	if cfg.cpu > 0 {
		args = append(args, "--config", fmt.Sprintf("limits.cpu=%d", cfg.cpu))
	}
	if cfg.memory != "" {
		args = append(args, "--config", fmt.Sprintf("limits.memory=%s", cfg.memory))
	}
	if cfg.network != "" {
		args = append(args, "--network", cfg.network)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, c.incusBin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("incus launch: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// Add disk mounts as devices after creation.
	for _, m := range cfg.mounts {
		if err := c.addDiskDevice(name, m); err != nil {
			// Best-effort cleanup: delete the container we just created.
			_ = c.DeleteContainer(name)
			return fmt.Errorf("adding mount %s: %w", m.Name, err)
		}
	}

	return nil
}

// addDiskDevice attaches a host directory as a disk device on the container.
func (c *Client) addDiskDevice(containerName string, m Mount) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	args := []string{
		"config", "device", "add", containerName, m.Name,
		"disk", fmt.Sprintf("source=%s", m.Source), fmt.Sprintf("path=%s", m.Path),
	}
	cmd := execCommandContext(ctx, c.incusBin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("incus config device add: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// StartContainer starts a stopped container.
func (c *Client) StartContainer(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, c.incusBin, "start", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("incus start: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// StopContainer stops a running container.
func (c *Client) StopContainer(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, c.incusBin, "stop", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("incus stop: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// DeleteContainer removes a container. The container must be stopped first.
func (c *Client) DeleteContainer(name string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, c.incusBin, "delete", name, "--force")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("incus delete: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ExecInContainer runs a command inside a running container and returns its
// combined stdout+stderr output. The provided context controls cancellation.
func (c *Client) ExecInContainer(ctx context.Context, name string, cmd []string) (string, error) {
	if len(cmd) == 0 {
		return "", fmt.Errorf("empty command")
	}

	args := append([]string{"exec", name, "--"}, cmd...)
	execCmd := execCommandContext(ctx, c.incusBin, args...)
	out, err := execCmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("incus exec: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// incusListEntry mirrors the JSON structure returned by "incus list --format=json".
type incusListEntry struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	Config    map[string]string `json:"config"`
}

// ListContainers returns all Incus containers visible to the client.
func (c *Client) ListContainers() ([]Container, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, c.incusBin, "list", "--format=json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("incus list: %w", err)
	}

	var entries []incusListEntry
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("parsing incus list output: %w", err)
	}

	containers := make([]Container, 0, len(entries))
	for _, e := range entries {
		ct := Container{
			Name:      e.Name,
			Status:    e.Status,
			CreatedAt: e.CreatedAt,
		}
		if v, ok := e.Config["limits.cpu"]; ok {
			fmt.Sscanf(v, "%d", &ct.CPU)
		}
		if v, ok := e.Config["limits.memory"]; ok {
			ct.Memory = v
		}
		if v, ok := e.Config["image.description"]; ok {
			ct.Image = v
		}
		containers = append(containers, ct)
	}

	return containers, nil
}
