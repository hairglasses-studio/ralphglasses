package remote

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

// SSHOption configures an SSHClient.
type SSHOption func(*SSHClient)

// WithTimeout sets the connection/command timeout for the SSH client.
func WithTimeout(d time.Duration) SSHOption {
	return func(c *SSHClient) {
		c.timeout = d
	}
}

// SSHClient executes commands on a remote host via the ssh binary.
type SSHClient struct {
	host    *Host
	timeout time.Duration
}

// NewSSHClient creates an SSH client for the given host.
func NewSSHClient(host *Host, opts ...SSHOption) *SSHClient {
	c := &SSHClient{
		host:    host,
		timeout: 30 * time.Second,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Command returns an SSHCommand that can be executed with Run.
func (c *SSHClient) Command(ctx context.Context, cmd string) *SSHCommand {
	return &SSHCommand{
		host:    c.host,
		cmd:     cmd,
		timeout: c.timeout,
		ctx:     ctx,
	}
}

// SSHCommand represents a single command to run on a remote host.
type SSHCommand struct {
	host    *Host
	cmd     string
	timeout time.Duration
	ctx     context.Context
}

// SSHArgs builds the argument list for the ssh binary (excluding the binary
// name itself). The resulting slice is suitable for exec.Command("ssh", args...).
func (sc *SSHCommand) SSHArgs() []string {
	var args []string

	// Strict host key checking.
	args = append(args, "-o", "StrictHostKeyChecking=accept-new")

	// Port.
	if sc.host.Port != 0 && sc.host.Port != 22 {
		args = append(args, "-p", strconv.Itoa(sc.host.Port))
	}

	// Identity file.
	if sc.host.KeyPath != "" {
		args = append(args, "-i", sc.host.KeyPath)
	}

	// user@host
	target := sc.host.Address
	if sc.host.User != "" {
		target = sc.host.User + "@" + sc.host.Address
	}
	args = append(args, target)

	// Remote command.
	args = append(args, sc.cmd)

	return args
}

// CommandResult holds the output of a completed remote command.
type CommandResult struct {
	Stdout   string        `json:"stdout"`
	Stderr   string        `json:"stderr"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
}

// Run executes the SSH command and returns its result.
func (sc *SSHCommand) Run() (*CommandResult, error) {
	ctx := sc.ctx
	if sc.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, sc.timeout)
		defer cancel()
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "ssh", sc.SSHArgs()...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	err := cmd.Run()
	dur := time.Since(start)

	exitCode := 0
	if err != nil {
		if cmd.ProcessState != nil {
			exitCode = cmd.ProcessState.ExitCode()
		} else {
			return nil, fmt.Errorf("ssh command failed: %w", err)
		}
	}

	return &CommandResult{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: dur,
	}, nil
}
