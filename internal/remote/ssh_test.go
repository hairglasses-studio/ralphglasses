package remote

import (
	"context"
	"testing"
	"time"
)

func TestSSHClient_SSHArgs(t *testing.T) {
	host := &Host{Address: "10.0.0.1", User: "admin", Port: 22}
	client := NewSSHClient(host)
	cmd := client.Command(context.Background(), "uptime")

	args := cmd.SSHArgs()

	// Should contain StrictHostKeyChecking, user@host, and command.
	expected := []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"admin@10.0.0.1",
		"uptime",
	}
	assertArgsEqual(t, expected, args)
}

func TestSSHClient_SSHArgs_CustomPort(t *testing.T) {
	host := &Host{Address: "10.0.0.1", User: "deploy", Port: 2222}
	client := NewSSHClient(host)
	cmd := client.Command(context.Background(), "ls /")

	args := cmd.SSHArgs()

	expected := []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"-p", "2222",
		"deploy@10.0.0.1",
		"ls /",
	}
	assertArgsEqual(t, expected, args)
}

func TestSSHClient_SSHArgs_WithKey(t *testing.T) {
	host := &Host{
		Address: "10.0.0.5",
		User:    "root",
		Port:    22,
		KeyPath: "/home/user/.ssh/id_ed25519",
	}
	client := NewSSHClient(host)
	cmd := client.Command(context.Background(), "hostname")

	args := cmd.SSHArgs()

	expected := []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"-i", "/home/user/.ssh/id_ed25519",
		"root@10.0.0.5",
		"hostname",
	}
	assertArgsEqual(t, expected, args)
}

func TestSSHClient_SSHArgs_NoUser(t *testing.T) {
	host := &Host{Address: "10.0.0.1", Port: 22}
	client := NewSSHClient(host)
	cmd := client.Command(context.Background(), "whoami")

	args := cmd.SSHArgs()

	// Without a user, target should be just the address.
	expected := []string{
		"-o", "StrictHostKeyChecking=accept-new",
		"10.0.0.1",
		"whoami",
	}
	assertArgsEqual(t, expected, args)
}

func TestSSHClient_WithTimeout(t *testing.T) {
	host := &Host{Address: "10.0.0.1", User: "admin", Port: 22}
	client := NewSSHClient(host, WithTimeout(5*time.Minute))

	if client.timeout != 5*time.Minute {
		t.Fatalf("expected timeout 5m, got %v", client.timeout)
	}
}

func TestCommandResult(t *testing.T) {
	r := &CommandResult{
		Stdout:   "hello\n",
		Stderr:   "",
		ExitCode: 0,
		Duration: 150 * time.Millisecond,
	}
	if r.Stdout != "hello\n" {
		t.Fatalf("unexpected stdout: %q", r.Stdout)
	}
	if r.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", r.ExitCode)
	}
	if r.Duration != 150*time.Millisecond {
		t.Fatalf("unexpected duration: %v", r.Duration)
	}
}

// assertArgsEqual is a test helper that compares two string slices.
func assertArgsEqual(t *testing.T, expected, got []string) {
	t.Helper()
	if len(expected) != len(got) {
		t.Fatalf("arg count mismatch: expected %d, got %d\nexpected: %v\ngot:      %v",
			len(expected), len(got), expected, got)
	}
	for i := range expected {
		if expected[i] != got[i] {
			t.Fatalf("arg[%d] mismatch: expected %q, got %q\nexpected: %v\ngot:      %v",
				i, expected[i], got[i], expected, got)
		}
	}
}
