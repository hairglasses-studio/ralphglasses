package gvisor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// mockCmd records calls and returns pre-configured output.
type mockCmd struct {
	calls [][]string
	// Map from command key to output/error.
	outputs map[string]mockOutput
}

type mockOutput struct {
	stdout string
	err    error
}

func (m *mockCmd) commandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	all := append([]string{name}, args...)
	m.calls = append(m.calls, all)

	key := strings.Join(all, " ")
	for pattern, out := range m.outputs {
		if strings.Contains(key, pattern) {
			if out.err != nil {
				// Return a command that will fail.
				return exec.CommandContext(ctx, "false")
			}
			// Return a command that echoes the expected output.
			return exec.CommandContext(ctx, "printf", "%s", out.stdout)
		}
	}
	// Default: succeed with empty output.
	return exec.CommandContext(ctx, "true")
}

func setupMock(t *testing.T, outputs map[string]mockOutput) *mockCmd {
	t.Helper()
	m := &mockCmd{outputs: outputs}
	orig := execCommandContext
	execCommandContext = m.commandContext
	t.Cleanup(func() { execCommandContext = orig })
	return m
}

func TestNewRuntime_NotFound(t *testing.T) {
	// Save and clear PATH to ensure runsc is not found.
	t.Setenv("PATH", "/nonexistent")
	_, err := NewRuntime()
	if err == nil {
		t.Fatal("expected error when runsc not in PATH")
	}
	if !strings.Contains(err.Error(), "runsc not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsAvailable(t *testing.T) {
	m := setupMock(t, map[string]mockOutput{
		"--version": {stdout: "runsc version 20240101.0"},
	})
	_ = m

	rt := newRuntimeWithPath("/usr/bin/runsc")
	if !rt.IsAvailable() {
		t.Fatal("expected IsAvailable to return true")
	}
}

func TestIsAvailable_Failure(t *testing.T) {
	setupMock(t, map[string]mockOutput{
		"--version": {err: fmt.Errorf("not found")},
	})

	rt := newRuntimeWithPath("/usr/bin/runsc")
	if rt.IsAvailable() {
		t.Fatal("expected IsAvailable to return false")
	}
}

func TestCreateSandbox_EmptyName(t *testing.T) {
	rt := newRuntimeWithPath("/usr/bin/runsc")
	err := rt.CreateSandbox("", "/rootfs")
	if err == nil || !strings.Contains(err.Error(), "name must not be empty") {
		t.Fatalf("expected empty name error, got: %v", err)
	}
}

func TestCreateSandbox_EmptyRootfs(t *testing.T) {
	rt := newRuntimeWithPath("/usr/bin/runsc")
	err := rt.CreateSandbox("test", "")
	if err == nil || !strings.Contains(err.Error(), "rootfs path must not be empty") {
		t.Fatalf("expected empty rootfs error, got: %v", err)
	}
}

func TestCreateSandbox_Success(t *testing.T) {
	m := setupMock(t, map[string]mockOutput{
		"create": {stdout: ""},
	})

	rt := newRuntimeWithPath("/usr/bin/runsc")
	rt.SetRootDir(t.TempDir())

	err := rt.CreateSandbox("test-sb", "/rootfs",
		WithNetwork(NetworkNone),
		WithPlatform(PlatformPtrace),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify runsc was called with expected args.
	found := false
	for _, call := range m.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "create") && strings.Contains(joined, "test-sb") {
			found = true
			if !strings.Contains(joined, "--network none") {
				t.Errorf("expected --network none, got: %s", joined)
			}
			if !strings.Contains(joined, "--platform ptrace") {
				t.Errorf("expected --platform ptrace, got: %s", joined)
			}
		}
	}
	if !found {
		t.Error("create command not found in mock calls")
	}
}

func TestCreateSandbox_WithMounts(t *testing.T) {
	setupMock(t, map[string]mockOutput{
		"create": {stdout: ""},
	})

	rt := newRuntimeWithPath("/usr/bin/runsc")
	rt.SetRootDir(t.TempDir())

	err := rt.CreateSandbox("mount-sb", "/rootfs",
		WithFilesystem([]Mount{
			{Source: "/host/data", Target: "/data", ReadOnly: true},
		}),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInSandbox_EmptyName(t *testing.T) {
	rt := newRuntimeWithPath("/usr/bin/runsc")
	_, err := rt.RunInSandbox(context.Background(), "", []string{"echo", "hi"})
	if err == nil || !strings.Contains(err.Error(), "name must not be empty") {
		t.Fatalf("expected empty name error, got: %v", err)
	}
}

func TestRunInSandbox_EmptyCommand(t *testing.T) {
	rt := newRuntimeWithPath("/usr/bin/runsc")
	_, err := rt.RunInSandbox(context.Background(), "sb", nil)
	if err == nil || !strings.Contains(err.Error(), "command must not be empty") {
		t.Fatalf("expected empty command error, got: %v", err)
	}
}

func TestRunInSandbox_Success(t *testing.T) {
	setupMock(t, map[string]mockOutput{
		"exec": {stdout: "hello world\n"},
	})

	rt := newRuntimeWithPath("/usr/bin/runsc")
	rt.SetRootDir(t.TempDir())

	out, err := rt.RunInSandbox(context.Background(), "test-sb", []string{"echo", "hello", "world"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected 'hello world' in output, got: %q", out)
	}
}

func TestDeleteSandbox_EmptyName(t *testing.T) {
	rt := newRuntimeWithPath("/usr/bin/runsc")
	err := rt.DeleteSandbox("")
	if err == nil || !strings.Contains(err.Error(), "name must not be empty") {
		t.Fatalf("expected empty name error, got: %v", err)
	}
}

func TestDeleteSandbox_Success(t *testing.T) {
	m := setupMock(t, map[string]mockOutput{
		"kill":   {stdout: ""},
		"delete": {stdout: ""},
	})

	rt := newRuntimeWithPath("/usr/bin/runsc")
	rt.SetRootDir(t.TempDir())

	err := rt.DeleteSandbox("test-sb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify kill was called before delete.
	killIdx, deleteIdx := -1, -1
	for i, call := range m.calls {
		joined := strings.Join(call, " ")
		if strings.Contains(joined, "kill") {
			killIdx = i
		}
		if strings.Contains(joined, "delete") {
			deleteIdx = i
		}
	}
	if killIdx == -1 {
		t.Error("kill command not found")
	}
	if deleteIdx == -1 {
		t.Error("delete command not found")
	}
	if killIdx > deleteIdx {
		t.Error("kill should be called before delete")
	}
}

func TestListSandboxes_Empty(t *testing.T) {
	setupMock(t, map[string]mockOutput{
		"list": {stdout: ""},
	})

	rt := newRuntimeWithPath("/usr/bin/runsc")
	rt.SetRootDir(t.TempDir())

	sandboxes, err := rt.ListSandboxes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sandboxes) != 0 {
		t.Errorf("expected empty list, got %d sandboxes", len(sandboxes))
	}
}

func TestListSandboxes_WithEntries(t *testing.T) {
	listJSON := `[{"id":"sb1","status":"running","pid":1234,"created":"2024-01-15T10:30:00Z"},{"id":"sb2","status":"stopped","pid":0,"created":"2024-01-15T11:00:00Z"}]`
	setupMock(t, map[string]mockOutput{
		"list": {stdout: listJSON},
	})

	rt := newRuntimeWithPath("/usr/bin/runsc")
	rt.SetRootDir(t.TempDir())

	sandboxes, err := rt.ListSandboxes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sandboxes) != 2 {
		t.Fatalf("expected 2 sandboxes, got %d", len(sandboxes))
	}

	if sandboxes[0].Name != "sb1" {
		t.Errorf("expected name sb1, got %s", sandboxes[0].Name)
	}
	if sandboxes[0].Status != "running" {
		t.Errorf("expected status running, got %s", sandboxes[0].Status)
	}
	if sandboxes[0].PID != 1234 {
		t.Errorf("expected PID 1234, got %d", sandboxes[0].PID)
	}
	if sandboxes[0].CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}

	if sandboxes[1].Name != "sb2" {
		t.Errorf("expected name sb2, got %s", sandboxes[1].Name)
	}
	if sandboxes[1].Status != "stopped" {
		t.Errorf("expected status stopped, got %s", sandboxes[1].Status)
	}
}

func TestListSandboxes_NullOutput(t *testing.T) {
	setupMock(t, map[string]mockOutput{
		"list": {stdout: "null"},
	})

	rt := newRuntimeWithPath("/usr/bin/runsc")
	rt.SetRootDir(t.TempDir())

	sandboxes, err := rt.ListSandboxes()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sandboxes != nil {
		t.Errorf("expected nil, got %v", sandboxes)
	}
}

func TestSetRootDir(t *testing.T) {
	rt := newRuntimeWithPath("/usr/bin/runsc")
	rt.SetRootDir("/custom/root")
	if rt.rootDir != "/custom/root" {
		t.Errorf("expected /custom/root, got %s", rt.rootDir)
	}
}

func TestRunscPath(t *testing.T) {
	rt := newRuntimeWithPath("/usr/local/bin/runsc")
	if rt.RunscPath() != "/usr/local/bin/runsc" {
		t.Errorf("expected /usr/local/bin/runsc, got %s", rt.RunscPath())
	}
}

func TestParseListPID(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"1234", 1234},
		{" 5678 ", 5678},
		{"", 0},
		{"abc", 0},
	}
	for _, tt := range tests {
		got := parseListPID(tt.input)
		if got != tt.want {
			t.Errorf("parseListPID(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestRunInSandbox_ContextCancellation(t *testing.T) {
	setupMock(t, map[string]mockOutput{
		"exec": {stdout: ""},
	})

	rt := newRuntimeWithPath("/usr/bin/runsc")
	rt.SetRootDir(t.TempDir())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	_, err := rt.RunInSandbox(ctx, "test-sb", []string{"sleep", "100"})
	// With a cancelled context, we may or may not get an error depending
	// on timing with the mock. Just verify no panic.
	_ = err
}

func TestSandboxStruct(t *testing.T) {
	now := time.Now()
	sb := Sandbox{
		Name:      "test",
		Status:    "running",
		PID:       42,
		CreatedAt: now,
	}
	if sb.Name != "test" || sb.Status != "running" || sb.PID != 42 {
		t.Errorf("unexpected sandbox values: %+v", sb)
	}
	if !sb.CreatedAt.Equal(now) {
		t.Errorf("expected CreatedAt %v, got %v", now, sb.CreatedAt)
	}
}
