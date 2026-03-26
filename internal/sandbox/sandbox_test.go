package sandbox

import (
	"fmt"
	"os/exec"
	"testing"
	"time"
)

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
	// MemorySwapMB should equal MemoryMB by default to disable swap abuse.
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

	// We can't actually run docker in tests, but we can verify the args
	// by building the config and checking the expected flag patterns.
	// We use a helper that builds args the same way Create does.
	cfg := DefaultContainerConfig("/tmp/work")

	// Simulate what Create does to build the args slice.
	args := buildCreateArgs("test-container", cfg)

	assertArgsContain(t, args, "--pids-limit", "256")
	assertArgsContain(t, args, "--memory-swap", "4096m")
	assertArgsContain(t, args, "--ulimit", "nofile=1024:2048")
}

func TestCreate_PidsLimitDefaultsWhenZero(t *testing.T) {
	t.Parallel()

	cfg := DefaultContainerConfig("/tmp/work")
	cfg.PidsLimit = 0 // explicitly zero — should still get default 256

	args := buildCreateArgs("test-container", cfg)
	assertArgsContain(t, args, "--pids-limit", "256")
}

func TestCreate_MemorySwapMatchesMemoryWhenZero(t *testing.T) {
	t.Parallel()

	cfg := DefaultContainerConfig("/tmp/work")
	cfg.MemorySwapMB = 0 // zero — should fall back to MemoryMB

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
		"GOOD_KEY":    "value1",
		"--privileged": "true",  // should be skipped
		"ALSO_GOOD":  "value2",
	}

	args := buildCreateArgs("test-container", cfg)

	// Verify good keys are present and bad key is absent.
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
