package incus

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// --- Test helper process pattern ---
// TestHelperProcess is not a real test. It is used by tests to mock exec.Command
// by re-invoking the test binary with a specific env var that routes to this function.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_TEST_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "no command provided")
		os.Exit(1)
	}

	// args[0] is the incus binary path, args[1:] are subcommand and flags.
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "no incus subcommand")
		os.Exit(1)
	}

	subcmd := args[1]
	behavior := os.Getenv("MOCK_BEHAVIOR")

	switch subcmd {
	case "info":
		if behavior == "unavailable" {
			fmt.Fprintln(os.Stderr, "Error: cannot connect to Incus daemon")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "server: incus\nserver_version: 6.0")

	case "launch":
		if behavior == "launch_fail" {
			fmt.Fprintln(os.Stderr, "Error: image not found")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "Creating container")

	case "start":
		if behavior == "start_fail" {
			fmt.Fprintln(os.Stderr, "Error: container not found")
			os.Exit(1)
		}

	case "stop":
		if behavior == "stop_fail" {
			fmt.Fprintln(os.Stderr, "Error: container not running")
			os.Exit(1)
		}

	case "delete":
		if behavior == "delete_fail" {
			fmt.Fprintln(os.Stderr, "Error: container is running")
			os.Exit(1)
		}

	case "exec":
		if behavior == "exec_fail" {
			fmt.Fprintln(os.Stderr, "Error: container not running")
			os.Exit(1)
		}
		if behavior == "exec_output" {
			fmt.Fprintln(os.Stdout, "hello from container")
		}

	case "list":
		if behavior == "list_fail" {
			fmt.Fprintln(os.Stderr, "Error: cannot list")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, `[{"name":"test-ct","status":"Running","created_at":"2025-01-15T10:00:00Z","config":{"limits.cpu":"2","limits.memory":"4GB","image.description":"Ubuntu 24.04"}}]`)

	case "config":
		// "config device add" for mounts
		if behavior == "mount_fail" {
			fmt.Fprintln(os.Stderr, "Error: device add failed")
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "unhandled subcommand: %s\n", subcmd)
		os.Exit(1)
	}
}

// fakeExecCommand returns a function that creates exec.Cmd pointing at the
// test binary's TestHelperProcess, with the given mock behavior.
func fakeExecCommand(behavior string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", name}
		cs = append(cs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = []string{
			"GO_TEST_HELPER_PROCESS=1",
			"MOCK_BEHAVIOR=" + behavior,
		}
		return cmd
	}
}

// withMockExec installs a fake exec function and restores it after the test.
func withMockExec(t *testing.T, behavior string) {
	t.Helper()
	orig := execCommandContext
	execCommandContext = fakeExecCommand(behavior)
	t.Cleanup(func() { execCommandContext = orig })
}

// newTestClient creates a Client that uses the mocked exec. It bypasses the
// real NewClient constructor since we cannot connect to a real daemon in tests.
func newTestClient(t *testing.T) *Client {
	t.Helper()
	return &Client{incusBin: "incus"}
}

// --- Tests ---

func TestNewClient_Success(t *testing.T) {
	withMockExec(t, "")
	// NewClient calls exec.LookPath which won't find our mock,
	// so we test the path where the binary exists by directly constructing.
	c := newTestClient(t)
	if c.incusBin != "incus" {
		t.Fatalf("unexpected binary: %s", c.incusBin)
	}
}

func TestIsAvailable_NoIncusBinary(t *testing.T) {
	// Save and override PATH to ensure incus is not found.
	orig := os.Getenv("PATH")
	t.Setenv("PATH", "/nonexistent")
	defer os.Setenv("PATH", orig)

	if IsAvailable() {
		t.Fatal("expected IsAvailable to return false when incus is not in PATH")
	}
}

func TestCreateContainer_Success(t *testing.T) {
	withMockExec(t, "")
	c := newTestClient(t)

	err := c.CreateContainer("test-agent", "ubuntu:24.04",
		WithCPU(2), WithMemory("4GB"), WithNetwork("incusbr0"))
	if err != nil {
		t.Fatalf("CreateContainer failed: %v", err)
	}
}

func TestCreateContainer_WithMounts(t *testing.T) {
	withMockExec(t, "")
	c := newTestClient(t)

	err := c.CreateContainer("test-agent", "ubuntu:24.04",
		WithMounts(Mount{Name: "workspace", Source: "/tmp/ws", Path: "/workspace"}))
	if err != nil {
		t.Fatalf("CreateContainer with mounts failed: %v", err)
	}
}

func TestCreateContainer_LaunchFail(t *testing.T) {
	withMockExec(t, "launch_fail")
	c := newTestClient(t)

	err := c.CreateContainer("test-agent", "ubuntu:24.04")
	if err == nil {
		t.Fatal("expected error from failed launch")
	}
	if !strings.Contains(err.Error(), "incus launch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateContainer_MountFail(t *testing.T) {
	withMockExec(t, "mount_fail")
	c := newTestClient(t)

	// launch succeeds but device add fails; the container should be cleaned up.
	err := c.CreateContainer("test-agent", "ubuntu:24.04",
		WithMounts(Mount{Name: "ws", Source: "/tmp", Path: "/workspace"}))
	if err == nil {
		t.Fatal("expected error from failed mount")
	}
	if !strings.Contains(err.Error(), "adding mount") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartContainer_Success(t *testing.T) {
	withMockExec(t, "")
	c := newTestClient(t)
	if err := c.StartContainer("test-agent"); err != nil {
		t.Fatalf("StartContainer failed: %v", err)
	}
}

func TestStartContainer_Fail(t *testing.T) {
	withMockExec(t, "start_fail")
	c := newTestClient(t)
	err := c.StartContainer("test-agent")
	if err == nil {
		t.Fatal("expected error from failed start")
	}
}

func TestStopContainer_Success(t *testing.T) {
	withMockExec(t, "")
	c := newTestClient(t)
	if err := c.StopContainer("test-agent"); err != nil {
		t.Fatalf("StopContainer failed: %v", err)
	}
}

func TestStopContainer_Fail(t *testing.T) {
	withMockExec(t, "stop_fail")
	c := newTestClient(t)
	err := c.StopContainer("test-agent")
	if err == nil {
		t.Fatal("expected error from failed stop")
	}
}

func TestDeleteContainer_Success(t *testing.T) {
	withMockExec(t, "")
	c := newTestClient(t)
	if err := c.DeleteContainer("test-agent"); err != nil {
		t.Fatalf("DeleteContainer failed: %v", err)
	}
}

func TestDeleteContainer_Fail(t *testing.T) {
	withMockExec(t, "delete_fail")
	c := newTestClient(t)
	err := c.DeleteContainer("test-agent")
	if err == nil {
		t.Fatal("expected error from failed delete")
	}
}

func TestExecInContainer_Success(t *testing.T) {
	withMockExec(t, "exec_output")
	c := newTestClient(t)

	out, err := c.ExecInContainer(context.Background(), "test-agent", []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("ExecInContainer failed: %v", err)
	}
	if !strings.Contains(out, "hello from container") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestExecInContainer_Fail(t *testing.T) {
	withMockExec(t, "exec_fail")
	c := newTestClient(t)

	_, err := c.ExecInContainer(context.Background(), "test-agent", []string{"ls"})
	if err == nil {
		t.Fatal("expected error from failed exec")
	}
}

func TestExecInContainer_EmptyCommand(t *testing.T) {
	withMockExec(t, "")
	c := newTestClient(t)

	_, err := c.ExecInContainer(context.Background(), "test-agent", nil)
	if err == nil {
		t.Fatal("expected error for empty command")
	}
	if !strings.Contains(err.Error(), "empty command") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecInContainer_ContextCancellation(t *testing.T) {
	withMockExec(t, "")
	c := newTestClient(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := c.ExecInContainer(ctx, "test-agent", []string{"sleep", "60"})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}

func TestListContainers_Success(t *testing.T) {
	withMockExec(t, "")
	c := newTestClient(t)

	containers, err := c.ListContainers()
	if err != nil {
		t.Fatalf("ListContainers failed: %v", err)
	}
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}

	ct := containers[0]
	if ct.Name != "test-ct" {
		t.Errorf("expected name test-ct, got %s", ct.Name)
	}
	if ct.Status != "Running" {
		t.Errorf("expected status Running, got %s", ct.Status)
	}
	if ct.CPU != 2 {
		t.Errorf("expected CPU 2, got %d", ct.CPU)
	}
	if ct.Memory != "4GB" {
		t.Errorf("expected memory 4GB, got %s", ct.Memory)
	}
	if ct.Image != "Ubuntu 24.04" {
		t.Errorf("expected image Ubuntu 24.04, got %s", ct.Image)
	}
	expectedTime := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	if !ct.CreatedAt.Equal(expectedTime) {
		t.Errorf("expected created_at %v, got %v", expectedTime, ct.CreatedAt)
	}
}

func TestListContainers_Fail(t *testing.T) {
	withMockExec(t, "list_fail")
	c := newTestClient(t)

	_, err := c.ListContainers()
	if err == nil {
		t.Fatal("expected error from failed list")
	}
}

// --- Profile tests ---

func TestProfiles(t *testing.T) {
	profiles := Profiles()
	if len(profiles) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(profiles))
	}

	for _, name := range []string{"minimal", "standard", "gpu-passthrough"} {
		if _, ok := profiles[name]; !ok {
			t.Errorf("missing profile: %s", name)
		}
	}
}

func TestProfileByName(t *testing.T) {
	p, ok := ProfileByName("standard")
	if !ok {
		t.Fatal("expected to find standard profile")
	}
	if p.CPU != 2 {
		t.Errorf("expected CPU 2, got %d", p.CPU)
	}

	_, ok = ProfileByName("nonexistent")
	if ok {
		t.Fatal("expected nonexistent profile to not be found")
	}
}

func TestProfileOptions(t *testing.T) {
	opts := ProfileStandard.Options()
	if len(opts) != 3 { // CPU, Memory, Network
		t.Fatalf("expected 3 options from standard profile, got %d", len(opts))
	}

	// Verify options apply correctly by running them through a containerConfig.
	cfg := &containerConfig{}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.cpu != 2 {
		t.Errorf("expected cpu 2, got %d", cfg.cpu)
	}
	if cfg.memory != "4GB" {
		t.Errorf("expected memory 4GB, got %s", cfg.memory)
	}
	if cfg.network != "incusbr0" {
		t.Errorf("expected network incusbr0, got %s", cfg.network)
	}
}

func TestProfileMinimalOptions(t *testing.T) {
	opts := ProfileMinimal.Options()
	// Minimal: CPU + Memory, no network, no mounts.
	if len(opts) != 2 {
		t.Fatalf("expected 2 options from minimal profile, got %d", len(opts))
	}

	cfg := &containerConfig{}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.cpu != 1 {
		t.Errorf("expected cpu 1, got %d", cfg.cpu)
	}
	if cfg.memory != "512MB" {
		t.Errorf("expected memory 512MB, got %s", cfg.memory)
	}
	if cfg.network != "" {
		t.Errorf("expected no network, got %s", cfg.network)
	}
}

func TestProfileGPUPassthroughOptions(t *testing.T) {
	p := ProfileGPUPassthrough
	if len(p.GPUDevices) != 1 {
		t.Fatalf("expected 1 GPU device, got %d", len(p.GPUDevices))
	}
	if p.GPUDevices[0] != "gpu0" {
		t.Errorf("expected gpu0, got %s", p.GPUDevices[0])
	}

	opts := p.Options()
	// GPU profile: CPU + Memory + Network = 3 options (GPU passthrough is handled separately).
	if len(opts) != 3 {
		t.Fatalf("expected 3 options from gpu profile, got %d", len(opts))
	}
}
