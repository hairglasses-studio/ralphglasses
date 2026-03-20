package session

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// buildCmd constructs the claude CLI command from LaunchOptions.
// Kept for backward compatibility with tests; delegates to buildClaudeCmd.
func buildCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	return buildClaudeCmd(ctx, opts)
}

// launch starts a new LLM CLI session and returns immediately.
// The session runs in a background goroutine that parses streaming output.
func launch(ctx context.Context, opts LaunchOptions) (*Session, error) {
	if opts.RepoPath == "" {
		return nil, fmt.Errorf("repo path required")
	}
	if opts.Prompt == "" && opts.Resume == "" && !opts.Continue {
		return nil, fmt.Errorf("prompt required (unless resuming)")
	}

	provider := opts.Provider
	if provider == "" {
		provider = ProviderClaude
	}

	sessionCtx, cancel := context.WithCancel(ctx)
	cmd, err := buildCmdForProvider(sessionCtx, opts)
	if err != nil {
		cancel()
		return nil, err
	}

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
		Provider:     provider,
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
		return nil, fmt.Errorf("start %s: %w", provider, err)
	}

	s.mu.Lock()
	s.Status = StatusRunning
	s.mu.Unlock()

	// Write prompt to stdin and close (Codex takes prompt as positional arg, skip stdin)
	go func() {
		if provider != ProviderCodex && opts.Prompt != "" {
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

// runSession reads streaming JSON from stdout/stderr and updates session state.
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

		event, err := normalizeEvent(s.Provider, line)
		if err != nil {
			continue
		}

		s.mu.Lock()
		s.LastActivity = time.Now()

		switch event.Type {
		case "system":
			if event.SessionID != "" {
				s.ProviderSessionID = event.SessionID
			}
		case "assistant":
			if event.Content != "" {
				s.LastOutput = truncateStr(event.Content, 4000)
			}
		case "result":
			s.LastOutput = truncateStr(event.Result, 4000)
			if event.CostUSD > 0 {
				// Assignment (not +=) because Claude emits cumulative cost in result events.
				// If Gemini/Codex emit per-event deltas instead, this needs to change to +=.
				s.SpentUSD = event.CostUSD
			}
			if event.NumTurns > 0 {
				s.TurnCount = event.NumTurns
			}
			if event.SessionID != "" {
				s.ProviderSessionID = event.SessionID
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
