package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// buildCmd constructs the claude CLI command from LaunchOptions.
func buildCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	args := []string{"-p"}

	// Prompt is passed via stdin, not as an argument
	args = append(args, "--output-format", "stream-json")

	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", fmt.Sprintf("%.2f", opts.MaxBudgetUSD))
	}
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
	}
	if opts.Agent != "" {
		args = append(args, "--agent", opts.Agent)
	}
	if len(opts.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(opts.AllowedTools, ","))
	}
	if opts.SystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.SystemPrompt)
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	} else if opts.Continue {
		args = append(args, "--continue")
	}
	if opts.Worktree != "" {
		if opts.Worktree == "true" {
			args = append(args, "-w")
		} else {
			args = append(args, "-w", opts.Worktree)
		}
	}
	if opts.SessionName != "" {
		args = append(args, "-n", opts.SessionName)
	}

	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = opts.RepoPath
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	return cmd
}

// launch starts a new Claude Code session and returns immediately.
// The session runs in a background goroutine that parses streaming output.
func launch(ctx context.Context, opts LaunchOptions) (*Session, error) {
	if opts.RepoPath == "" {
		return nil, fmt.Errorf("repo path required")
	}
	if opts.Prompt == "" && opts.Resume == "" && !opts.Continue {
		return nil, fmt.Errorf("prompt required (unless resuming)")
	}

	// Verify claude binary exists
	if _, err := exec.LookPath("claude"); err != nil {
		return nil, fmt.Errorf("claude binary not found on PATH: %w", err)
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	cmd := buildCmd(sessionCtx, opts)

	// Pipe prompt via stdin
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	now := time.Now()
	s := &Session{
		ID:           uuid.New().String(),
		RepoPath:     opts.RepoPath,
		RepoName:     filepath.Base(opts.RepoPath),
		Status:       StatusLaunching,
		Prompt:       opts.Prompt,
		Model:        opts.Model,
		AgentName:    opts.Agent,
		TeamName:     opts.TeamName,
		BudgetUSD:    opts.MaxBudgetUSD,
		MaxTurns:     opts.MaxTurns,
		LaunchedAt:   now,
		LastActivity: now,
		cmd:          cmd,
		cancel:       cancel,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start claude: %w", err)
	}

	s.mu.Lock()
	s.Status = StatusRunning
	s.mu.Unlock()

	// Write prompt to stdin and close
	go func() {
		if opts.Prompt != "" {
			_, _ = io.WriteString(stdin, opts.Prompt)
		}
		stdin.Close()
	}()

	// Background goroutine: parse streaming JSON output
	go func() {
		defer cancel()
		runSession(s, stdout, stderr)
	}()

	return s, nil
}

// runSession reads streaming JSON from claude stdout/stderr and updates session state.
func runSession(s *Session, stdout, stderr io.Reader) {
	// Read stderr in background for error capture
	var stderrBuf strings.Builder
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		_, _ = io.Copy(&stderrBuf, stderr)
	}()

	runSessionOutput(s, stdout)

	// Wait for stderr collection
	<-stderrDone

	// Wait for process exit
	err := s.cmd.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.EndedAt = &now
	s.LastActivity = now

	if err != nil {
		if s.Status == StatusStopped {
			s.ExitReason = "stopped by user"
		} else {
			s.Status = StatusErrored
			s.ExitReason = err.Error()
			if errStr := stderrBuf.String(); errStr != "" && s.Error == "" {
				s.Error = truncateStr(errStr, 2000)
			}
		}
	} else {
		if s.Status == StatusRunning {
			s.Status = StatusCompleted
			s.ExitReason = "completed normally"
		}
	}
}

// runSessionOutput parses streaming JSON lines from stdout and updates session state.
func runSessionOutput(s *Session, stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var event StreamEvent
		if err := json.Unmarshal(line, &event); err != nil {
			continue
		}
		event.Raw = json.RawMessage(append([]byte(nil), line...))

		s.mu.Lock()
		s.LastActivity = time.Now()

		switch event.Type {
		case "system":
			if event.SessionID != "" {
				s.ClaudeID = event.SessionID
			}
		case "assistant":
			if event.Content != "" {
				s.LastOutput = truncateStr(event.Content, 4000)
			}
		case "result":
			s.LastOutput = truncateStr(event.Result, 4000)
			if event.CostUSD > 0 {
				s.SpentUSD = event.CostUSD
			}
			if event.NumTurns > 0 {
				s.TurnCount = event.NumTurns
			}
			if event.SessionID != "" {
				s.ClaudeID = event.SessionID
			}
			if event.IsError {
				s.Error = event.Result
			}
		}

		s.mu.Unlock()
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}
