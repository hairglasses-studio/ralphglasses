package sandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
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
	// Find the "--" separator that separates test flags from the command args.
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

	// args[0] is "docker", args[1:] are the docker subcommand and flags.
	if args[0] != "docker" {
		fmt.Fprintf(os.Stderr, "unexpected command: %s\n", args[0])
		os.Exit(1)
	}

	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "no docker subcommand")
		os.Exit(1)
	}

	subcmd := args[1]
	behavior := os.Getenv("MOCK_BEHAVIOR")

	switch subcmd {
	case "info":
		if behavior == "unavailable" {
			fmt.Fprintln(os.Stderr, "Cannot connect to the Docker daemon")
			os.Exit(1)
		}
		if behavior == "empty_version" {
			fmt.Fprintln(os.Stdout, "")
			return
		}
		fmt.Fprintln(os.Stdout, "24.0.7")

	case "create":
		if behavior == "create_fail" {
			fmt.Fprintln(os.Stderr, "Error: image not found")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "abc123def456")

	case "start":
		if behavior == "start_fail" {
			fmt.Fprintln(os.Stderr, "Error: container not found")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, args[2]) // echo container ID

	case "exec":
		if behavior == "exec_fail" {
			fmt.Fprintln(os.Stderr, "Error: container not running")
			os.Exit(1)
		}
		if behavior == "exec_exit_1" {
			fmt.Fprintln(os.Stdout, "command failed")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, "exec output here")

	case "stop":
		if behavior == "stop_fail" {
			fmt.Fprintln(os.Stderr, "Error: no such container")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, args[len(args)-1])

	case "rm":
		if behavior == "rm_fail" {
			fmt.Fprintln(os.Stderr, "Error: removal in progress")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stdout, args[len(args)-1])

	case "inspect":
		if behavior == "inspect_fail" {
			fmt.Fprintln(os.Stderr, "Error: no such container")
			os.Exit(1)
		}
		if behavior == "inspect_bad_json" {
			fmt.Fprintln(os.Stdout, "not json at all{{{")
			return
		}
		if behavior == "inspect_empty_array" {
			fmt.Fprintln(os.Stdout, "[]")
			return
		}
		fmt.Fprintln(os.Stdout, `[{"Id":"abc123","State":{"Status":"running"}}]`)

	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n", subcmd)
		os.Exit(1)
	}
}

// mockExecCommandContext returns a function that creates exec.Cmd pointing to the
// test binary's TestHelperProcess, with the given mock behavior.
func mockExecCommandContext(behavior string) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", name}
		cs = append(cs, args...)
		cmd := exec.CommandContext(ctx, os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_TEST_HELPER_PROCESS=1",
			"MOCK_BEHAVIOR="+behavior,
		)
		return cmd
	}
}

// withMockExec swaps execCommandContext for the duration of a test and restores it via Cleanup.
func withMockExec(t *testing.T, behavior string) {
	t.Helper()
	orig := execCommandContext
	execCommandContext = mockExecCommandContext(behavior)
	t.Cleanup(func() { execCommandContext = orig })
}

// ========================================================================
// Existing tests (kept as-is)
// ========================================================================

func TestDefaultContainerConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		workDir string
	}{
		{name: "typical path", workDir: "/home/user/project"},
		{name: "empty path", workDir: ""},
		{name: "path with spaces", workDir: "/home/user/my project"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := DefaultContainerConfig(tt.workDir)

			if cfg.Image != "ubuntu:24.04" {
				t.Errorf("Image = %q, want %q", cfg.Image, "ubuntu:24.04")
			}
			if cfg.WorkDir != tt.workDir {
				t.Errorf("WorkDir = %q, want %q", cfg.WorkDir, tt.workDir)
			}
			if cfg.MountPath != "/workspace" {
				t.Errorf("MountPath = %q, want %q", cfg.MountPath, "/workspace")
			}
			if cfg.CPUs != 2.0 {
				t.Errorf("CPUs = %f, want 2.0", cfg.CPUs)
			}
			if cfg.MemoryMB != 4096 {
				t.Errorf("MemoryMB = %d, want 4096", cfg.MemoryMB)
			}
			if cfg.NetworkMode != "none" {
				t.Errorf("NetworkMode = %q, want %q", cfg.NetworkMode, "none")
			}
			if cfg.Timeout != time.Hour {
				t.Errorf("Timeout = %v, want %v", cfg.Timeout, time.Hour)
			}
			if cfg.ReadOnly {
				t.Error("ReadOnly = true, want false")
			}
		})
	}
}

func TestContainerConfigZeroValue(t *testing.T) {
	t.Parallel()

	var cfg ContainerConfig
	if cfg.Image != "" {
		t.Errorf("zero-value Image = %q, want empty", cfg.Image)
	}
	if cfg.CPUs != 0 {
		t.Errorf("zero-value CPUs = %f, want 0", cfg.CPUs)
	}
	if cfg.MemoryMB != 0 {
		t.Errorf("zero-value MemoryMB = %d, want 0", cfg.MemoryMB)
	}
	if cfg.ReadOnly {
		t.Error("zero-value ReadOnly = true, want false")
	}
}

func TestContainerStructFields(t *testing.T) {
	t.Parallel()

	now := time.Now()
	later := now.Add(time.Minute)

	c := Container{
		ID:        "abc123",
		Name:      "test-sandbox",
		Config:    DefaultContainerConfig("/tmp/work"),
		Status:    "running",
		CreatedAt: now,
		StartedAt: &later,
		ExitCode:  0,
	}

	if c.ID != "abc123" {
		t.Errorf("ID = %q, want %q", c.ID, "abc123")
	}
	if c.Name != "test-sandbox" {
		t.Errorf("Name = %q, want %q", c.Name, "test-sandbox")
	}
	if c.Status != "running" {
		t.Errorf("Status = %q, want %q", c.Status, "running")
	}
	if c.StartedAt == nil || !c.StartedAt.Equal(later) {
		t.Errorf("StartedAt = %v, want %v", c.StartedAt, later)
	}
}

func TestContainerStatusTransitions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		status string
	}{
		{name: "created state", status: "created"},
		{name: "running state", status: "running"},
		{name: "exited state", status: "exited"},
		{name: "removed state", status: "removed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := Container{Status: tt.status}
			if c.Status != tt.status {
				t.Errorf("Status = %q, want %q", c.Status, tt.status)
			}
		})
	}
}

func TestIsExitErrorWithPlainError(t *testing.T) {
	t.Parallel()

	plainErr := fmt.Errorf("not an exit error")
	var target *exec.ExitError
	if isExitError(plainErr, &target) {
		t.Error("isExitError returned true for plain error, want false")
	}
	if target != nil {
		t.Error("target was set for plain error, want nil")
	}
}

func TestIsExitErrorWithNil(t *testing.T) {
	t.Parallel()

	var target *exec.ExitError
	if isExitError(nil, &target) {
		t.Error("isExitError returned true for nil error, want false")
	}
}

func TestDefaultContainerConfig_HasPidsLimit(t *testing.T) {
	t.Parallel()

	cfg := DefaultContainerConfig("/tmp/work")

	if cfg.PidsLimit != 256 {
		t.Errorf("PidsLimit = %d, want 256", cfg.PidsLimit)
	}
	if cfg.MemorySwapMB != 4096 {
		t.Errorf("MemorySwapMB = %d, want 4096", cfg.MemorySwapMB)
	}
	if cfg.UlimitNofile != "1024:2048" {
		t.Errorf("UlimitNofile = %q, want %q", cfg.UlimitNofile, "1024:2048")
	}
	if cfg.MemorySwapMB != cfg.MemoryMB {
		t.Errorf("MemorySwapMB (%d) != MemoryMB (%d), should match by default", cfg.MemorySwapMB, cfg.MemoryMB)
	}
}

func TestValidateEnvKey_Valid(t *testing.T) {
	t.Parallel()

	validKeys := []string{
		"HOME",
		"PATH",
		"_PRIVATE",
		"MY_VAR_123",
		"a",
		"_",
		"A1B2C3",
	}

	for _, key := range validKeys {
		t.Run(key, func(t *testing.T) {
			t.Parallel()
			if err := ValidateEnvKey(key); err != nil {
				t.Errorf("ValidateEnvKey(%q) = %v, want nil", key, err)
			}
		})
	}
}

func TestValidateEnvKey_Invalid(t *testing.T) {
	t.Parallel()

	invalidKeys := []struct {
		name string
		key  string
	}{
		{name: "starts with digit", key: "1VAR"},
		{name: "contains dash", key: "MY-VAR"},
		{name: "contains space", key: "MY VAR"},
		{name: "contains equals", key: "MY=VAR"},
		{name: "docker flag injection", key: "--privileged"},
		{name: "empty string", key: ""},
		{name: "contains dot", key: "my.var"},
		{name: "contains newline", key: "MY\nVAR"},
	}

	for _, tt := range invalidKeys {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateEnvKey(tt.key); err == nil {
				t.Errorf("ValidateEnvKey(%q) = nil, want error", tt.key)
			}
		})
	}
}

func TestValidateEnvValue_NullByte(t *testing.T) {
	t.Parallel()

	if err := validateEnvValue("normal value"); err != nil {
		t.Errorf("validateEnvValue(normal) = %v, want nil", err)
	}
	if err := validateEnvValue("has\x00null"); err == nil {
		t.Error("validateEnvValue(null byte) = nil, want error")
	}
	if err := validateEnvValue(""); err != nil {
		t.Errorf("validateEnvValue(empty) = %v, want nil", err)
	}
}

func TestCreate_IncludesPidsLimit(t *testing.T) {
	t.Parallel()

	cfg := DefaultContainerConfig("/tmp/work")
	args := buildCreateArgs("test-container", cfg)

	assertArgsContain(t, args, "--pids-limit", "256")
	assertArgsContain(t, args, "--memory-swap", "4096m")
	assertArgsContain(t, args, "--ulimit", "nofile=1024:2048")
}

func TestCreate_PidsLimitDefaultsWhenZero(t *testing.T) {
	t.Parallel()

	cfg := DefaultContainerConfig("/tmp/work")
	cfg.PidsLimit = 0

	args := buildCreateArgs("test-container", cfg)
	assertArgsContain(t, args, "--pids-limit", "256")
}

func TestCreate_MemorySwapMatchesMemoryWhenZero(t *testing.T) {
	t.Parallel()

	cfg := DefaultContainerConfig("/tmp/work")
	cfg.MemorySwapMB = 0

	args := buildCreateArgs("test-container", cfg)
	assertArgsContain(t, args, "--memory-swap", "4096m")
}

func TestCreate_CustomPidsLimit(t *testing.T) {
	t.Parallel()

	cfg := DefaultContainerConfig("/tmp/work")
	cfg.PidsLimit = 512

	args := buildCreateArgs("test-container", cfg)
	assertArgsContain(t, args, "--pids-limit", "512")
}

func TestCreate_EnvKeyValidation(t *testing.T) {
	t.Parallel()

	cfg := DefaultContainerConfig("/tmp/work")
	cfg.Env = map[string]string{
		"GOOD_KEY":     "value1",
		"--privileged": "true",
		"ALSO_GOOD":    "value2",
	}

	args := buildCreateArgs("test-container", cfg)

	foundGood := false
	foundBad := false
	for _, a := range args {
		if a == "GOOD_KEY=value1" || a == "ALSO_GOOD=value2" {
			foundGood = true
		}
		if a == "--privileged=true" {
			foundBad = true
		}
	}
	if !foundGood {
		t.Error("valid env keys were not included in args")
	}
	if foundBad {
		t.Error("invalid env key '--privileged' should have been filtered out")
	}
}

// ========================================================================
// Docker lifecycle tests using mock exec
// ========================================================================

func TestDockerAvailable_Success(t *testing.T) {
	withMockExec(t, "success")
	if err := DockerAvailable(); err != nil {
		t.Errorf("DockerAvailable() = %v, want nil", err)
	}
}

func TestDockerAvailable_Unavailable(t *testing.T) {
	withMockExec(t, "unavailable")
	err := DockerAvailable()
	if err == nil {
		t.Fatal("DockerAvailable() = nil, want error")
	}
	if !strings.Contains(err.Error(), "docker not available") {
		t.Errorf("error = %q, want to contain 'docker not available'", err)
	}
}

func TestDockerAvailable_EmptyVersion(t *testing.T) {
	withMockExec(t, "empty_version")
	err := DockerAvailable()
	if err == nil {
		t.Fatal("DockerAvailable() = nil, want error")
	}
	if !strings.Contains(err.Error(), "docker daemon not running") {
		t.Errorf("error = %q, want to contain 'docker daemon not running'", err)
	}
}

func TestCreate_Success(t *testing.T) {
	withMockExec(t, "success")
	ctx := context.Background()
	cfg := DefaultContainerConfig("/tmp/work")

	c, err := Create(ctx, "test-sandbox", cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if c == nil {
		t.Fatal("Create() returned nil container")
	}
	if c.ID != "abc123def456" {
		t.Errorf("ID = %q, want %q", c.ID, "abc123def456")
	}
	if c.Name != "test-sandbox" {
		t.Errorf("Name = %q, want %q", c.Name, "test-sandbox")
	}
	if c.Status != "created" {
		t.Errorf("Status = %q, want %q", c.Status, "created")
	}
	if c.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
}

func TestCreate_Failure(t *testing.T) {
	withMockExec(t, "create_fail")
	ctx := context.Background()
	cfg := DefaultContainerConfig("/tmp/work")

	c, err := Create(ctx, "test-sandbox", cfg)
	if err == nil {
		t.Fatal("Create() = nil error, want error")
	}
	if c != nil {
		t.Errorf("Create() returned non-nil container on error: %+v", c)
	}
	if !strings.Contains(err.Error(), "docker create") {
		t.Errorf("error = %q, want to contain 'docker create'", err)
	}
}

func TestCreate_DefaultsApplied(t *testing.T) {
	withMockExec(t, "success")
	ctx := context.Background()

	// Provide minimal config — defaults should fill in.
	cfg := ContainerConfig{
		WorkDir: "/tmp/work",
	}

	c, err := Create(ctx, "test-defaults", cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if c.Config.Image != "ubuntu:24.04" {
		t.Errorf("Config.Image = %q, want %q", c.Config.Image, "ubuntu:24.04")
	}
	if c.Config.MountPath != "/workspace" {
		t.Errorf("Config.MountPath = %q, want %q", c.Config.MountPath, "/workspace")
	}
	if c.Config.Timeout != time.Hour {
		t.Errorf("Config.Timeout = %v, want %v", c.Config.Timeout, time.Hour)
	}
}

func TestCreate_ReadOnlyConfig(t *testing.T) {
	withMockExec(t, "success")
	ctx := context.Background()
	cfg := DefaultContainerConfig("/tmp/work")
	cfg.ReadOnly = true

	c, err := Create(ctx, "test-readonly", cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !c.Config.ReadOnly {
		t.Error("Config.ReadOnly = false, want true")
	}
}

func TestCreate_NoWorkDir(t *testing.T) {
	withMockExec(t, "success")
	ctx := context.Background()
	cfg := DefaultContainerConfig("")
	cfg.WorkDir = ""

	c, err := Create(ctx, "test-no-workdir", cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if c == nil {
		t.Fatal("Create() returned nil container")
	}
}

func TestCreate_WithEnvVars(t *testing.T) {
	withMockExec(t, "success")
	ctx := context.Background()
	cfg := DefaultContainerConfig("/tmp/work")
	cfg.Env = map[string]string{
		"FOO": "bar",
		"BAZ": "qux",
	}

	c, err := Create(ctx, "test-env", cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if c == nil {
		t.Fatal("Create() returned nil container")
	}
}

func TestCreate_ContextCancelled(t *testing.T) {
	withMockExec(t, "success")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cfg := DefaultContainerConfig("/tmp/work")
	_, err := Create(ctx, "test-cancelled", cfg)
	if err == nil {
		t.Fatal("Create() with cancelled context should return error")
	}
}

func TestStart_Success(t *testing.T) {
	withMockExec(t, "success")
	ctx := context.Background()

	c := &Container{
		ID:     "abc123",
		Name:   "test-sandbox",
		Status: "created",
	}

	err := Start(ctx, c)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if c.Status != "running" {
		t.Errorf("Status = %q, want %q", c.Status, "running")
	}
	if c.StartedAt == nil {
		t.Error("StartedAt is nil after Start()")
	}
}

func TestStart_Failure(t *testing.T) {
	withMockExec(t, "start_fail")
	ctx := context.Background()

	c := &Container{
		ID:     "abc123",
		Name:   "test-sandbox",
		Status: "created",
	}

	err := Start(ctx, c)
	if err == nil {
		t.Fatal("Start() = nil error, want error")
	}
	if !strings.Contains(err.Error(), "docker start") {
		t.Errorf("error = %q, want to contain 'docker start'", err)
	}
	// Status should not have changed on failure.
	if c.Status != "created" {
		t.Errorf("Status = %q after failed Start, want %q", c.Status, "created")
	}
}

func TestExec_Success(t *testing.T) {
	withMockExec(t, "success")
	ctx := context.Background()

	out, exitCode, err := Exec(ctx, "abc123", []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if exitCode != 0 {
		t.Errorf("exitCode = %d, want 0", exitCode)
	}
	if !strings.Contains(out, "exec output here") {
		t.Errorf("output = %q, want to contain 'exec output here'", out)
	}
}

func TestExec_NonZeroExit(t *testing.T) {
	withMockExec(t, "exec_exit_1")
	ctx := context.Background()

	out, exitCode, err := Exec(ctx, "abc123", []string{"false"})
	if err != nil {
		t.Fatalf("Exec() error = %v (non-zero exit should not be an error)", err)
	}
	if exitCode == 0 {
		t.Errorf("exitCode = 0, want non-zero")
	}
	if !strings.Contains(out, "command failed") {
		t.Errorf("output = %q, want to contain 'command failed'", out)
	}
}

func TestExec_CommandFailure(t *testing.T) {
	withMockExec(t, "exec_fail")
	ctx := context.Background()

	// exec_fail causes a non-ExitError failure path since the mock writes to stderr and exits 1,
	// but the exec_fail case in the helper exits with code 1 which IS an ExitError.
	// To test the non-ExitError branch, we use a cancelled context.
	ctx2, cancel := context.WithCancel(ctx)
	cancel()

	_, exitCode, err := Exec(ctx2, "abc123", []string{"echo", "hello"})
	if err == nil {
		t.Fatal("Exec() with cancelled context should return error")
	}
	if exitCode != -1 {
		t.Errorf("exitCode = %d, want -1 for non-ExitError", exitCode)
	}
	if !strings.Contains(err.Error(), "docker exec") {
		t.Errorf("error = %q, want to contain 'docker exec'", err)
	}
}

func TestStop_Success(t *testing.T) {
	withMockExec(t, "success")
	ctx := context.Background()

	c := &Container{
		ID:     "abc123",
		Name:   "test-sandbox",
		Status: "running",
	}

	err := Stop(ctx, c, 10)
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if c.Status != "exited" {
		t.Errorf("Status = %q, want %q", c.Status, "exited")
	}
}

func TestStop_Failure(t *testing.T) {
	withMockExec(t, "stop_fail")
	ctx := context.Background()

	c := &Container{
		ID:     "abc123",
		Name:   "test-sandbox",
		Status: "running",
	}

	err := Stop(ctx, c, 10)
	if err == nil {
		t.Fatal("Stop() = nil error, want error")
	}
	if !strings.Contains(err.Error(), "docker stop") {
		t.Errorf("error = %q, want to contain 'docker stop'", err)
	}
	// Status should not have changed on failure.
	if c.Status != "running" {
		t.Errorf("Status = %q after failed Stop, want %q", c.Status, "running")
	}
}

func TestRemove_Success(t *testing.T) {
	withMockExec(t, "success")
	ctx := context.Background()

	c := &Container{
		ID:     "abc123",
		Name:   "test-sandbox",
		Status: "exited",
	}

	err := Remove(ctx, c)
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if c.Status != "removed" {
		t.Errorf("Status = %q, want %q", c.Status, "removed")
	}
}

func TestRemove_Failure(t *testing.T) {
	withMockExec(t, "rm_fail")
	ctx := context.Background()

	c := &Container{
		ID:     "abc123",
		Name:   "test-sandbox",
		Status: "exited",
	}

	err := Remove(ctx, c)
	if err == nil {
		t.Fatal("Remove() = nil error, want error")
	}
	if !strings.Contains(err.Error(), "docker rm") {
		t.Errorf("error = %q, want to contain 'docker rm'", err)
	}
	if c.Status != "exited" {
		t.Errorf("Status = %q after failed Remove, want %q", c.Status, "exited")
	}
}

func TestInspect_Success(t *testing.T) {
	withMockExec(t, "success")
	ctx := context.Background()

	result, err := Inspect(ctx, "abc123")
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if result == nil {
		t.Fatal("Inspect() returned nil map")
	}
	id, ok := result["Id"]
	if !ok {
		t.Fatal("Inspect() result missing 'Id' key")
	}
	if id != "abc123" {
		t.Errorf("Id = %v, want %q", id, "abc123")
	}
}

func TestInspect_Failure(t *testing.T) {
	withMockExec(t, "inspect_fail")
	ctx := context.Background()

	result, err := Inspect(ctx, "nonexistent")
	if err == nil {
		t.Fatal("Inspect() = nil error, want error")
	}
	if result != nil {
		t.Errorf("Inspect() returned non-nil map on error: %v", result)
	}
	if !strings.Contains(err.Error(), "docker inspect") {
		t.Errorf("error = %q, want to contain 'docker inspect'", err)
	}
}

func TestInspect_BadJSON(t *testing.T) {
	withMockExec(t, "inspect_bad_json")
	ctx := context.Background()

	result, err := Inspect(ctx, "abc123")
	if err == nil {
		t.Fatal("Inspect() = nil error for bad JSON, want error")
	}
	if result != nil {
		t.Errorf("Inspect() returned non-nil map for bad JSON: %v", result)
	}
	if !strings.Contains(err.Error(), "parse inspect") {
		t.Errorf("error = %q, want to contain 'parse inspect'", err)
	}
}

func TestInspect_EmptyArray(t *testing.T) {
	withMockExec(t, "inspect_empty_array")
	ctx := context.Background()

	result, err := Inspect(ctx, "abc123")
	if err == nil {
		t.Fatal("Inspect() = nil error for empty array, want error")
	}
	if result != nil {
		t.Errorf("Inspect() returned non-nil map for empty array: %v", result)
	}
	if !strings.Contains(err.Error(), "container not found") {
		t.Errorf("error = %q, want to contain 'container not found'", err)
	}
}

// ========================================================================
// Full lifecycle test
// ========================================================================

func TestFullLifecycle(t *testing.T) {
	withMockExec(t, "success")
	ctx := context.Background()

	// 1. Create
	cfg := DefaultContainerConfig("/tmp/work")
	c, err := Create(ctx, "lifecycle-test", cfg)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if c.Status != "created" {
		t.Fatalf("after Create: Status = %q, want %q", c.Status, "created")
	}

	// 2. Start
	if err := Start(ctx, c); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if c.Status != "running" {
		t.Fatalf("after Start: Status = %q, want %q", c.Status, "running")
	}
	if c.StartedAt == nil {
		t.Fatal("after Start: StartedAt is nil")
	}

	// 3. Exec
	out, exitCode, err := Exec(ctx, c.ID, []string{"echo", "hello"})
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Exec() exitCode = %d, want 0", exitCode)
	}
	if out == "" {
		t.Error("Exec() output is empty")
	}

	// 4. Inspect
	info, err := Inspect(ctx, c.ID)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if info == nil {
		t.Fatal("Inspect() returned nil")
	}

	// 5. Stop
	if err := Stop(ctx, c, 10); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if c.Status != "exited" {
		t.Fatalf("after Stop: Status = %q, want %q", c.Status, "exited")
	}

	// 6. Remove
	if err := Remove(ctx, c); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if c.Status != "removed" {
		t.Fatalf("after Remove: Status = %q, want %q", c.Status, "removed")
	}
}

// ========================================================================
// Table-driven tests for Create config variations
// ========================================================================

func TestCreate_ConfigVariations(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*ContainerConfig)
	}{
		{
			name:   "custom image",
			modify: func(c *ContainerConfig) { c.Image = "alpine:3.19" },
		},
		{
			name:   "custom mount path",
			modify: func(c *ContainerConfig) { c.MountPath = "/app" },
		},
		{
			name:   "host network",
			modify: func(c *ContainerConfig) { c.NetworkMode = "host" },
		},
		{
			name:   "bridge network",
			modify: func(c *ContainerConfig) { c.NetworkMode = "bridge" },
		},
		{
			name:   "no network mode",
			modify: func(c *ContainerConfig) { c.NetworkMode = "" },
		},
		{
			name:   "read only",
			modify: func(c *ContainerConfig) { c.ReadOnly = true },
		},
		{
			name:   "custom memory",
			modify: func(c *ContainerConfig) { c.MemoryMB = 8192; c.MemorySwapMB = 8192 },
		},
		{
			name:   "custom cpus",
			modify: func(c *ContainerConfig) { c.CPUs = 4.0 },
		},
		{
			name:   "zero cpus",
			modify: func(c *ContainerConfig) { c.CPUs = 0 },
		},
		{
			name:   "zero memory",
			modify: func(c *ContainerConfig) { c.MemoryMB = 0 },
		},
		{
			name:   "custom timeout",
			modify: func(c *ContainerConfig) { c.Timeout = 30 * time.Minute },
		},
		{
			name:   "custom ulimit",
			modify: func(c *ContainerConfig) { c.UlimitNofile = "2048:4096" },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withMockExec(t, "success")
			ctx := context.Background()
			cfg := DefaultContainerConfig("/tmp/work")
			tt.modify(&cfg)

			c, err := Create(ctx, "test-"+tt.name, cfg)
			if err != nil {
				t.Fatalf("Create() error = %v", err)
			}
			if c == nil {
				t.Fatal("Create() returned nil container")
			}
			if c.Status != "created" {
				t.Errorf("Status = %q, want %q", c.Status, "created")
			}
		})
	}
}

// ========================================================================
// buildCreateArgs tests (kept from original)
// ========================================================================

// buildCreateArgs mirrors the arg-building logic from Create without executing docker.
// This allows testing the flag construction in isolation.
func buildCreateArgs(name string, config ContainerConfig) []string {
	if config.Image == "" {
		config.Image = "ubuntu:24.04"
	}
	if config.MountPath == "" {
		config.MountPath = "/workspace"
	}

	args := []string{"create", "--name", name}

	if config.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%.1f", config.CPUs))
	}
	if config.MemoryMB > 0 {
		args = append(args, "--memory", fmt.Sprintf("%dm", config.MemoryMB))
		swapMB := config.MemorySwapMB
		if swapMB == 0 {
			swapMB = config.MemoryMB
		}
		args = append(args, "--memory-swap", fmt.Sprintf("%dm", swapMB))
	}

	pidsLimit := config.PidsLimit
	if pidsLimit == 0 {
		pidsLimit = 256
	}
	args = append(args, "--pids-limit", fmt.Sprintf("%d", pidsLimit))

	ulimitNofile := config.UlimitNofile
	if ulimitNofile == "" {
		ulimitNofile = "1024:2048"
	}
	args = append(args, "--ulimit", fmt.Sprintf("nofile=%s", ulimitNofile))

	if config.NetworkMode != "" {
		args = append(args, "--network", config.NetworkMode)
	}

	if config.ReadOnly {
		args = append(args, "--read-only")
	}
	args = append(args, "--security-opt", "no-new-privileges")

	if config.WorkDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:%s", config.WorkDir, config.MountPath))
		args = append(args, "-w", config.MountPath)
	}

	for k, v := range config.Env {
		if err := ValidateEnvKey(k); err != nil {
			continue
		}
		if err := validateEnvValue(v); err != nil {
			continue
		}
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	args = append(args, config.Image)
	return args
}

// assertArgsContain checks that flag appears in args followed by the expected value.
func assertArgsContain(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == value {
			return
		}
	}
	t.Errorf("args do not contain %s %s; args: %v", flag, value, args)
}

// ========================================================================
// Additional buildCreateArgs edge-case tests
// ========================================================================

func TestBuildCreateArgs_EmptyImage(t *testing.T) {
	t.Parallel()

	cfg := ContainerConfig{WorkDir: "/tmp/work"}
	args := buildCreateArgs("test", cfg)

	// Should default to ubuntu:24.04 as the last element.
	last := args[len(args)-1]
	if last != "ubuntu:24.04" {
		t.Errorf("last arg = %q, want %q", last, "ubuntu:24.04")
	}
}

func TestBuildCreateArgs_EmptyMountPath(t *testing.T) {
	t.Parallel()

	cfg := ContainerConfig{WorkDir: "/tmp/work", Image: "alpine:3.19"}
	args := buildCreateArgs("test", cfg)

	// MountPath should default to /workspace.
	found := false
	for i, a := range args {
		if a == "-w" && i+1 < len(args) && args[i+1] == "/workspace" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("args missing -w /workspace; args: %v", args)
	}
}

func TestBuildCreateArgs_ReadOnly(t *testing.T) {
	t.Parallel()

	cfg := DefaultContainerConfig("/tmp/work")
	cfg.ReadOnly = true
	args := buildCreateArgs("test", cfg)

	found := slices.Contains(args, "--read-only")
	if !found {
		t.Error("args missing --read-only flag")
	}
}

func TestBuildCreateArgs_SecurityOpt(t *testing.T) {
	t.Parallel()

	cfg := DefaultContainerConfig("/tmp/work")
	args := buildCreateArgs("test", cfg)

	assertArgsContain(t, args, "--security-opt", "no-new-privileges")
}

func TestBuildCreateArgs_NullByteEnvValue(t *testing.T) {
	t.Parallel()

	cfg := DefaultContainerConfig("/tmp/work")
	cfg.Env = map[string]string{
		"GOOD":    "value",
		"BAD_VAL": "has\x00null",
	}
	args := buildCreateArgs("test", cfg)

	// BAD_VAL should be filtered out.
	for _, a := range args {
		if strings.Contains(a, "BAD_VAL") {
			t.Error("args contain BAD_VAL with null byte, should be filtered")
		}
	}
}
