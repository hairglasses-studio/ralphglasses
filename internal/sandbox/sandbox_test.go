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
