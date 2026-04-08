// Package firecracker provides Firecracker microVM management for sandboxed LLM sessions.
//
//go:build linux

package firecracker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// VMStatus represents the current state of a microVM.
type VMStatus int

const (
	// VMStatusStopped indicates the VM is not running.
	VMStatusStopped VMStatus = iota
	// VMStatusStarting indicates the VM is booting.
	VMStatusStarting
	// VMStatusRunning indicates the VM is operational and accepting commands.
	VMStatusRunning
	// VMStatusError indicates the VM encountered an unrecoverable error.
	VMStatusError
)

// String returns a human-readable label for a VMStatus.
func (s VMStatus) String() string {
	switch s {
	case VMStatusStopped:
		return "stopped"
	case VMStatusStarting:
		return "starting"
	case VMStatusRunning:
		return "running"
	case VMStatusError:
		return "error"
	default:
		return "unknown"
	}
}

// NetworkMode controls how a microVM is connected to the host network.
type NetworkMode string

const (
	NetworkNone   NetworkMode = "none"
	NetworkTap    NetworkMode = "tap"
	NetworkBridge NetworkMode = "bridge"
)

// VMConfig holds the static configuration for a Firecracker microVM.
type VMConfig struct {
	// Kernel is the path to the uncompressed Linux kernel image (vmlinux).
	Kernel string `json:"kernel"`
	// RootFS is the path to the ext4 root filesystem image.
	RootFS string `json:"rootfs"`
	// VCPUs is the number of virtual CPUs assigned to the VM (default: 1).
	VCPUs int `json:"vcpus"`
	// MemoryMB is the amount of RAM in megabytes (default: 512).
	MemoryMB int `json:"memory_mb"`
	// NetworkMode controls network attachment (default: NetworkNone).
	NetworkMode NetworkMode `json:"network_mode"`
	// SSHPort is the host port forwarded to the guest's port 22.
	// Only relevant when NetworkMode is not NetworkNone.
	SSHPort int `json:"ssh_port,omitempty"`
	// SSHKeyPath is the path to the private SSH key for guest access.
	SSHKeyPath string `json:"ssh_key_path,omitempty"`
	// FirecrackerBin overrides the firecracker binary path (default: "firecracker").
	FirecrackerBin string `json:"firecracker_bin,omitempty"`
}

// validate returns an error if the config is incomplete or invalid.
func (c VMConfig) validate() error {
	if c.Kernel == "" {
		return errors.New("firecracker: kernel path is required")
	}
	if c.RootFS == "" {
		return errors.New("firecracker: rootfs path is required")
	}
	if c.VCPUs < 1 {
		return errors.New("firecracker: vcpus must be >= 1")
	}
	if c.VCPUs > 32 {
		return errors.New("firecracker: vcpus must be <= 32")
	}
	if c.MemoryMB < 128 {
		return errors.New("firecracker: memory_mb must be >= 128")
	}
	if c.MemoryMB > 65536 {
		return errors.New("firecracker: memory_mb must be <= 65536")
	}
	switch c.NetworkMode {
	case NetworkNone, NetworkTap, NetworkBridge, "":
	default:
		return fmt.Errorf("firecracker: unknown network mode %q", c.NetworkMode)
	}
	return nil
}

// firecrackerBin returns the resolved binary path.
func (c VMConfig) firecrackerBin() string {
	if c.FirecrackerBin != "" {
		return c.FirecrackerBin
	}
	return "firecracker"
}

// VMOption configures a VM during construction.
type VMOption func(*VM)

// WithKernel sets the kernel image path.
func WithKernel(path string) VMOption {
	return func(v *VM) { v.config.Kernel = path }
}

// WithRootFS sets the root filesystem image path.
func WithRootFS(path string) VMOption {
	return func(v *VM) { v.config.RootFS = path }
}

// WithVCPUs sets the virtual CPU count.
func WithVCPUs(n int) VMOption {
	return func(v *VM) { v.config.VCPUs = n }
}

// WithMemMB sets the memory allocation in megabytes.
func WithMemMB(mb int) VMOption {
	return func(v *VM) { v.config.MemoryMB = mb }
}

// WithNetwork sets the network mode and optional SSH port.
func WithNetwork(mode NetworkMode, sshPort int) VMOption {
	return func(v *VM) {
		v.config.NetworkMode = mode
		v.config.SSHPort = sshPort
	}
}

// WithSSHKey sets the path to the SSH private key for guest access.
func WithSSHKey(path string) VMOption {
	return func(v *VM) { v.config.SSHKeyPath = path }
}

// WithFirecrackerBin overrides the firecracker binary path.
func WithFirecrackerBin(bin string) VMOption {
	return func(v *VM) { v.config.FirecrackerBin = bin }
}

// VM manages a single Firecracker microVM instance.
type VM struct {
	name       string
	config     VMConfig
	socketPath string
	pid        int
	status     VMStatus
	lastErr    error
	mu         sync.RWMutex
	cmd        *exec.Cmd
	logger     *slog.Logger

	// httpClient is the HTTP client configured to talk over the Unix socket.
	httpClient *http.Client

	// startFn is overridable for testing (defaults to startReal).
	startFn func(ctx context.Context) error
	// execFn is overridable for testing (defaults to execSSHReal).
	execFn func(ctx context.Context, cmd string) (string, error)
}

// NewVM creates a new VM manager with the given name and options.
// Default config: 1 vCPU, 512 MB RAM, no network.
func NewVM(name string, opts ...VMOption) *VM {
	vm := &VM{
		name: name,
		config: VMConfig{
			VCPUs:    1,
			MemoryMB: 512,
		},
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(vm)
	}
	if vm.startFn == nil {
		vm.startFn = vm.startReal
	}
	if vm.execFn == nil {
		vm.execFn = vm.execSSHReal
	}
	return vm
}

// Name returns the VM identifier.
func (vm *VM) Name() string { return vm.name }

// Config returns a copy of the VM configuration.
func (vm *VM) Config() VMConfig {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.config
}

// SocketPath returns the Unix socket path used for the Firecracker API.
func (vm *VM) SocketPath() string {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.socketPath
}

// PID returns the Firecracker process ID, or 0 if not running.
func (vm *VM) PID() int {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.pid
}

// Status returns the current VM status.
func (vm *VM) Status() VMStatus {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.status
}

// Err returns the last error, if any.
func (vm *VM) Err() error {
	vm.mu.RLock()
	defer vm.mu.RUnlock()
	return vm.lastErr
}

// Start boots the microVM. It creates a Unix socket, launches the firecracker
// process, then configures the VM via the Firecracker REST API.
func (vm *VM) Start(ctx context.Context) error {
	vm.mu.Lock()
	if vm.status == VMStatusRunning || vm.status == VMStatusStarting {
		vm.mu.Unlock()
		return errors.New("firecracker: VM is already running or starting")
	}
	vm.status = VMStatusStarting
	vm.lastErr = nil
	vm.mu.Unlock()

	if err := vm.config.validate(); err != nil {
		vm.setError(err)
		return err
	}

	if err := vm.startFn(ctx); err != nil {
		vm.setError(err)
		return err
	}

	vm.mu.Lock()
	vm.status = VMStatusRunning
	vm.mu.Unlock()
	return nil
}

// startReal performs the actual Firecracker process launch and API configuration.
func (vm *VM) startReal(ctx context.Context) error {
	// Create a temporary socket.
	dir, err := os.MkdirTemp("", "firecracker-"+vm.name+"-*")
	if err != nil {
		return fmt.Errorf("firecracker: create temp dir: %w", err)
	}
	socketPath := filepath.Join(dir, "firecracker.sock")

	vm.mu.Lock()
	vm.socketPath = socketPath
	vm.mu.Unlock()

	// Launch firecracker process.
	bin := vm.config.firecrackerBin()
	cmd := exec.CommandContext(ctx, bin, "--api-sock", socketPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("firecracker: start process: %w", err)
	}

	vm.mu.Lock()
	vm.cmd = cmd
	vm.pid = cmd.Process.Pid
	vm.mu.Unlock()

	vm.logger.Info("firecracker process started", "name", vm.name, "pid", cmd.Process.Pid, "socket", socketPath)

	// Create HTTP client over Unix socket.
	vm.httpClient = &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.DialTimeout("unix", socketPath, 5*time.Second)
			},
		},
		Timeout: 30 * time.Second,
	}

	// Wait for socket to become available.
	if err := vm.waitForSocket(ctx, socketPath); err != nil {
		_ = vm.killProcess()
		return fmt.Errorf("firecracker: wait for socket: %w", err)
	}

	// Configure the VM via REST API.
	if err := vm.configureMachineConfig(ctx); err != nil {
		_ = vm.killProcess()
		return fmt.Errorf("firecracker: set machine config: %w", err)
	}
	if err := vm.configureBootSource(ctx); err != nil {
		_ = vm.killProcess()
		return fmt.Errorf("firecracker: set boot source: %w", err)
	}
	if err := vm.configureRootDrive(ctx); err != nil {
		_ = vm.killProcess()
		return fmt.Errorf("firecracker: set root drive: %w", err)
	}

	// Issue InstanceStart action.
	if err := vm.apiPut(ctx, "/actions", map[string]string{"action_type": "InstanceStart"}); err != nil {
		_ = vm.killProcess()
		return fmt.Errorf("firecracker: instance start: %w", err)
	}

	return nil
}

// Stop gracefully shuts down the microVM via the Firecracker API, then cleans up.
func (vm *VM) Stop(ctx context.Context) error {
	vm.mu.RLock()
	status := vm.status
	vm.mu.RUnlock()

	if status == VMStatusStopped {
		return nil
	}

	// Try graceful shutdown via API.
	if vm.httpClient != nil {
		err := vm.apiPut(ctx, "/actions", map[string]string{"action_type": "SendCtrlAltDel"})
		if err != nil {
			vm.logger.Warn("firecracker: graceful shutdown failed, killing", "name", vm.name, "error", err)
		}
	}

	// Give the process a moment to exit, then kill.
	if err := vm.killProcess(); err != nil {
		vm.setError(err)
		return err
	}

	// Clean up socket.
	vm.mu.Lock()
	if vm.socketPath != "" {
		_ = os.RemoveAll(filepath.Dir(vm.socketPath))
		vm.socketPath = ""
	}
	vm.pid = 0
	vm.status = VMStatusStopped
	vm.httpClient = nil
	vm.cmd = nil
	vm.mu.Unlock()

	vm.logger.Info("firecracker VM stopped", "name", vm.name)
	return nil
}

// ExecSSH executes a command in the guest via SSH and returns combined output.
func (vm *VM) ExecSSH(ctx context.Context, cmd string) (string, error) {
	vm.mu.RLock()
	status := vm.status
	vm.mu.RUnlock()

	if status != VMStatusRunning {
		return "", fmt.Errorf("firecracker: cannot exec on VM in state %s", status)
	}

	return vm.execFn(ctx, cmd)
}

// execSSHReal performs the actual SSH command execution.
func (vm *VM) execSSHReal(ctx context.Context, cmd string) (string, error) {
	args, err := vm.sshArgs(cmd)
	if err != nil {
		return "", err
	}

	sshCmd := exec.CommandContext(ctx, "ssh", args...)
	var stdout, stderr bytes.Buffer
	sshCmd.Stdout = &stdout
	sshCmd.Stderr = &stderr

	if err := sshCmd.Run(); err != nil {
		return "", fmt.Errorf("firecracker: ssh exec: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func (vm *VM) sshArgs(cmd string) ([]string, error) {
	vm.mu.RLock()
	sshPort := vm.config.SSHPort
	keyPath := vm.config.SSHKeyPath
	vm.mu.RUnlock()

	if sshPort == 0 {
		return nil, errors.New("firecracker: SSH port not configured")
	}

	args := []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=10",
		"-p", strconv.Itoa(sshPort),
	}
	if keyPath != "" {
		args = append(args, "-i", keyPath)
	}
	args = append(args, "root@localhost", cmd)
	return args, nil
}

// waitForSocket polls until the Unix socket is connectable or the context expires.
func (vm *VM) waitForSocket(ctx context.Context, socketPath string) error {
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return errors.New("timeout waiting for firecracker socket")
		case <-ticker.C:
			conn, err := net.DialTimeout("unix", socketPath, time.Second)
			if err == nil {
				_ = conn.Close()
				return nil
			}
		}
	}
}

// configureMachineConfig sends PUT /machine-config.
func (vm *VM) configureMachineConfig(ctx context.Context) error {
	return vm.apiPut(ctx, "/machine-config", map[string]any{
		"vcpu_count":   vm.config.VCPUs,
		"mem_size_mib": vm.config.MemoryMB,
	})
}

// configureBootSource sends PUT /boot-source.
func (vm *VM) configureBootSource(ctx context.Context) error {
	return vm.apiPut(ctx, "/boot-source", map[string]any{
		"kernel_image_path": vm.config.Kernel,
		"boot_args":         "console=ttyS0 reboot=k panic=1 pci=off",
	})
}

// configureRootDrive sends PUT /drives/rootfs.
func (vm *VM) configureRootDrive(ctx context.Context) error {
	return vm.apiPut(ctx, "/drives/rootfs", map[string]any{
		"drive_id":       "rootfs",
		"path_on_host":   vm.config.RootFS,
		"is_root_device": true,
		"is_read_only":   false,
	})
}

// apiPut sends a PUT request to the Firecracker API.
func (vm *VM) apiPut(ctx context.Context, path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://localhost"+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := vm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("api request %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("api %s returned %d: %s", path, resp.StatusCode, string(respBody))
	}
	return nil
}

// killProcess terminates the firecracker process.
func (vm *VM) killProcess() error {
	vm.mu.RLock()
	cmd := vm.cmd
	vm.mu.RUnlock()

	if cmd == nil || cmd.Process == nil {
		return nil
	}

	// Try SIGTERM first.
	_ = cmd.Process.Signal(syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		// Force kill.
		_ = cmd.Process.Kill()
		<-done
		return nil
	}
}

// setError transitions the VM to error state.
func (vm *VM) setError(err error) {
	vm.mu.Lock()
	defer vm.mu.Unlock()
	vm.status = VMStatusError
	vm.lastErr = err
}
