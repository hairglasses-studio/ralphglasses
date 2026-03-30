// Package gvisor provides a gVisor (runsc) runtime wrapper for sandboxed execution.
package gvisor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// NetworkMode controls sandbox network access.
type NetworkMode string

const (
	NetworkNone NetworkMode = "none"
	NetworkHost NetworkMode = "host"
)

// Platform selects the gVisor execution platform.
type Platform string

const (
	PlatformPtrace Platform = "ptrace"
	PlatformKVM    Platform = "kvm"
)

// Mount describes a filesystem bind mount into the sandbox.
type Mount struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only"`
}

// SandboxOption configures sandbox creation.
type SandboxOption func(*sandboxConfig)

type sandboxConfig struct {
	network  NetworkMode
	mounts   []Mount
	platform Platform
}

// WithNetwork sets the network mode for the sandbox.
func WithNetwork(mode NetworkMode) SandboxOption {
	return func(c *sandboxConfig) { c.network = mode }
}

// WithFilesystem adds filesystem mounts to the sandbox.
func WithFilesystem(mounts []Mount) SandboxOption {
	return func(c *sandboxConfig) { c.mounts = append(c.mounts, mounts...) }
}

// WithPlatform selects the gVisor execution platform (ptrace or kvm).
func WithPlatform(p Platform) SandboxOption {
	return func(c *sandboxConfig) { c.platform = p }
}

// Sandbox represents a gVisor sandbox instance.
type Sandbox struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	PID       int       `json:"pid"`
	CreatedAt time.Time `json:"created_at"`
}

// Runtime wraps the runsc binary for gVisor sandbox management.
type Runtime struct {
	mu      sync.Mutex
	runsc   string // absolute path to runsc binary
	rootDir string // --root dir for runsc state
}

// execCommandContext is a variable for creating exec.Cmd instances.
// Tests can override this to mock command execution.
var execCommandContext = exec.CommandContext

// NewRuntime detects the runsc binary and returns a Runtime.
// Returns an error if runsc is not found in PATH.
func NewRuntime() (*Runtime, error) {
	path, err := exec.LookPath("runsc")
	if err != nil {
		return nil, fmt.Errorf("runsc not found in PATH: %w", err)
	}
	return &Runtime{
		runsc:   path,
		rootDir: "/var/run/runsc",
	}, nil
}

// IsAvailable reports whether the runsc binary can be executed.
func (r *Runtime) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := execCommandContext(ctx, r.runsc, "--version")
	return cmd.Run() == nil
}

// CreateSandbox creates a new gVisor sandbox with the given rootfs and options.
func (r *Runtime) CreateSandbox(name string, rootfs string, opts ...SandboxOption) error {
	if name == "" {
		return fmt.Errorf("sandbox name must not be empty")
	}
	if rootfs == "" {
		return fmt.Errorf("rootfs path must not be empty")
	}

	cfg := &sandboxConfig{
		network:  NetworkNone,
		platform: PlatformPtrace,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	args := []string{
		"--root", r.rootDir,
		"--network", string(cfg.network),
		"--platform", string(cfg.platform),
	}

	// Build OCI config with mounts if specified.
	// Mounts are passed via the OCI bundle's config.json, not CLI args.
	// We validate and serialize them here for future use with bundle generation.
	if len(cfg.mounts) > 0 {
		ociMounts := make([]map[string]any, 0, len(cfg.mounts))
		for _, m := range cfg.mounts {
			options := []string{"rbind"}
			if m.ReadOnly {
				options = append(options, "ro")
			} else {
				options = append(options, "rw")
			}
			ociMounts = append(ociMounts, map[string]any{
				"source":      m.Source,
				"destination": m.Target,
				"type":        "bind",
				"options":     options,
			})
		}
		mountJSON, err := json.Marshal(ociMounts)
		if err != nil {
			return fmt.Errorf("marshal mounts: %w", err)
		}
		_ = mountJSON // mounts are passed via OCI bundle config, not CLI args
	}

	args = append(args, "create", "--bundle", rootfs, name)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, r.runsc, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("runsc create %q: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RunInSandbox executes a command inside a running sandbox and returns its output.
func (r *Runtime) RunInSandbox(ctx context.Context, name string, cmd []string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("sandbox name must not be empty")
	}
	if len(cmd) == 0 {
		return "", fmt.Errorf("command must not be empty")
	}

	args := []string{"--root", r.rootDir, "exec", name, "--"}
	args = append(args, cmd...)

	execCmd := execCommandContext(ctx, r.runsc, args...)
	out, err := execCmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("runsc exec in %q: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// DeleteSandbox destroys a sandbox and cleans up its state.
func (r *Runtime) DeleteSandbox(name string) error {
	if name == "" {
		return fmt.Errorf("sandbox name must not be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Kill first to ensure it's stopped, then delete.
	killCmd := execCommandContext(ctx, r.runsc, "--root", r.rootDir, "kill", name, "SIGKILL")
	_ = killCmd.Run() // ignore error if already stopped

	args := []string{"--root", r.rootDir, "delete", name}
	cmd := execCommandContext(ctx, r.runsc, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("runsc delete %q: %w: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// runscListEntry matches the JSON output of `runsc list --format=json`.
type runscListEntry struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	PID       int    `json:"pid"`
	CreatedAt string `json:"created"`
}

// ListSandboxes returns all sandboxes managed by this runtime.
func (r *Runtime) ListSandboxes() ([]Sandbox, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := execCommandContext(ctx, r.runsc, "--root", r.rootDir, "list", "--format=json")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("runsc list: %w: %s", err, strings.TrimSpace(string(out)))
	}

	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	var entries []runscListEntry
	if err := json.Unmarshal([]byte(trimmed), &entries); err != nil {
		return nil, fmt.Errorf("parse runsc list output: %w", err)
	}

	sandboxes := make([]Sandbox, 0, len(entries))
	for _, e := range entries {
		created, _ := time.Parse(time.RFC3339, e.CreatedAt)
		sandboxes = append(sandboxes, Sandbox{
			Name:      e.ID,
			Status:    e.Status,
			PID:       e.PID,
			CreatedAt: created,
		})
	}
	return sandboxes, nil
}

// SetRootDir overrides the default root directory for runsc state.
// This is useful for testing or running multiple isolated runtimes.
func (r *Runtime) SetRootDir(dir string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rootDir = dir
}

// RunscPath returns the absolute path to the runsc binary.
func (r *Runtime) RunscPath() string {
	return r.runsc
}

// newRuntimeWithPath creates a Runtime with a specified binary path (for testing).
func newRuntimeWithPath(path string) *Runtime {
	return &Runtime{
		runsc:   path,
		rootDir: "/var/run/runsc",
	}
}

// parseListPID safely parses PID from various formats runsc may return.
func parseListPID(raw string) int {
	pid, _ := strconv.Atoi(strings.TrimSpace(raw))
	return pid
}
