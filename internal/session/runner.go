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

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// buildCmd constructs the claude CLI command from LaunchOptions.
// Kept for backward compatibility with tests; delegates to buildClaudeCmd.
func buildCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	return buildClaudeCmd(ctx, opts)
}

// launch starts a new LLM CLI session and returns immediately.
// The session runs in a background goroutine that parses streaming output.
func launch(ctx context.Context, opts LaunchOptions, bus ...*events.Bus) (*Session, error) {
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
	opts.Provider = provider
	if opts.Model == "" {
		opts.Model = ProviderDefaults(provider)
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

	var sessionBus *events.Bus
	if len(bus) > 0 {
		sessionBus = bus[0]
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
		OutputCh:     make(chan string, 100),
		bus:          sessionBus,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start %s: %w", provider, err)
	}

	s.mu.Lock()
	s.Status = StatusRunning
	s.mu.Unlock()

	// Write prompt to stdin and close.
	// Gemini and Codex take prompt as a CLI argument, not stdin.
	go func() {
		if provider == ProviderClaude && opts.Prompt != "" {
			_, _ = io.WriteString(stdin, opts.Prompt)
		}
		stdin.Close()
	}()

	// Background goroutine: parse streaming JSON output
	go func() {
		defer cancel()
		defer close(s.OutputCh)
		runSession(sessionCtx, s, stdout, stderr)
	}()

	return s, nil
}

// runSession reads streaming JSON from stdout/stderr and updates session state.
func runSession(ctx context.Context, s *Session, stdout, stderr io.Reader) {
	// Read stderr in background for error capture
	var stderrBuf strings.Builder
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		_, _ = io.Copy(&stderrBuf, stderr)
	}()

	runSessionOutput(ctx, s, stdout)

	// Wait for stderr collection
	<-stderrDone

	// Wait for process exit
	err := s.cmd.Wait()

	s.mu.Lock()

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
				s.Error = truncateStr(sanitizeStderr(s.Provider, errStr), 2000)
			}
		}
	} else {
		if s.Status == StatusRunning {
			s.Status = StatusCompleted
			s.ExitReason = "completed normally"
		}
	}

	// Fallback: if no output was captured from stdout events, use cleaned stderr
	if s.LastOutput == "" && s.Error == "" {
		if cleaned := cleanProviderOutput(s.Provider, stderrBuf.String()); cleaned != "" {
			s.LastOutput = truncateStr(cleaned, 4000)
		}
	}

	// Publish session ended event
	if s.bus != nil {
		s.bus.Publish(events.Event{
			Type:      events.SessionEnded,
			SessionID: s.ID,
			RepoPath:  s.RepoPath,
			RepoName:  s.RepoName,
			Provider:  string(s.Provider),
			Data: map[string]any{
				"status":      string(s.Status),
				"exit_reason": s.ExitReason,
				"spent_usd":   s.SpentUSD,
				"turns":       s.TurnCount,
			},
		})
	}

	// Capture onComplete before releasing the lock.
	// PersistSession and WriteJournalEntry both acquire s.mu, so they must
	// not be called while we hold it (Go mutexes are not reentrant).
	onComplete := s.onComplete
	s.mu.Unlock()

	// Persist final state to disk (acquires s.mu internally — safe now).
	if onComplete != nil {
		onComplete(s)
	}

	// Write improvement journal entry (fire-and-forget).
	go func() {
		if err := WriteJournalEntry(s); err == nil && s.bus != nil {
			s.bus.Publish(events.Event{
				Type:      events.JournalWritten,
				SessionID: s.ID,
				RepoPath:  s.RepoPath,
				RepoName:  s.RepoName,
				Provider:  string(s.Provider),
			})
		}
	}()
}

// runSessionOutput parses streaming JSON lines from stdout and updates session state.
// It selects on ctx.Done() so it returns promptly when the session is cancelled,
// without waiting for the next line from the provider process.
func runSessionOutput(ctx context.Context, s *Session, stdout io.Reader) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	// scanCh receives lines from the scanner goroutine. It is closed when the
	// scanner reaches EOF or its own ctx.Done() check fires.
	scanCh := make(chan []byte, 16)
	go func() {
		defer close(scanCh)
		for scanner.Scan() {
			b := make([]byte, len(scanner.Bytes()))
			copy(b, scanner.Bytes())
			select {
			case scanCh <- b:
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-scanCh:
			if !ok {
				return
			}
			if len(line) == 0 {
				continue
			}

			event, err := normalizeEvent(s.Provider, line)
			if err != nil {
				s.mu.Lock()
				s.StreamParseErrors++
				s.LastEventType = "parse_error"
				if msg := strings.TrimSpace(string(line)); msg != "" {
					appendSessionOutput(s, msg)
				}
				if s.Error == "" {
					s.Error = truncateStr(err.Error(), 2000)
				}
				s.mu.Unlock()
				continue
			}

			s.mu.Lock()
			s.LastActivity = time.Now()
			s.LastEventType = event.Type
			eventText := firstNonEmpty(event.Content, event.Text, event.Result)

			switch event.Type {
			case "system":
				if event.SessionID != "" {
					s.ProviderSessionID = event.SessionID
				}
				if eventText != "" {
					appendSessionOutput(s, eventText)
				}
			case "assistant":
				if eventText != "" {
					appendSessionOutput(s, eventText)
				}
			case "result":
				if eventText != "" {
					s.LastOutput = truncateStr(eventText, 4000)
				}
				if event.CostUSD > 0 {
					prevSpent := s.SpentUSD
					s.SpentUSD = event.CostUSD
					s.CostHistory = append(s.CostHistory, event.CostUSD)
					// Publish cost update event
					if s.bus != nil && event.CostUSD != prevSpent {
						s.bus.Publish(events.Event{
							Type:      events.CostUpdate,
							SessionID: s.ID,
							RepoPath:  s.RepoPath,
							RepoName:  s.RepoName,
							Provider:  string(s.Provider),
							Data:      map[string]any{"spent_usd": event.CostUSD, "turns": s.TurnCount},
						})
					}
					// Check budget exceeded
					if s.bus != nil && s.BudgetUSD > 0 && s.SpentUSD >= s.BudgetUSD {
						s.bus.Publish(events.Event{
							Type:      events.BudgetExceeded,
							SessionID: s.ID,
							RepoPath:  s.RepoPath,
							RepoName:  s.RepoName,
							Provider:  string(s.Provider),
							Data:      map[string]any{"spent_usd": s.SpentUSD, "budget_usd": s.BudgetUSD},
						})
					}
				}
				if event.NumTurns > 0 {
					s.TurnCount = event.NumTurns
				}
				if event.SessionID != "" {
					s.ProviderSessionID = event.SessionID
				}
				if event.IsError {
					s.Error = truncateStr(firstNonEmpty(event.Error, event.Result, event.Text), 2000)
				}
			default:
				if eventText != "" {
					appendSessionOutput(s, eventText)
				}
				if event.IsError {
					s.Error = truncateStr(firstNonEmpty(event.Error, eventText), 2000)
				}
			}

			s.mu.Unlock()
		}
	}
}

func appendSessionOutput(s *Session, text string) {
	s.TotalOutputCount++
	s.LastOutput = truncateStr(text, 4000)
	s.OutputHistory = append(s.OutputHistory, text)
	if len(s.OutputHistory) > 100 {
		s.OutputHistory = s.OutputHistory[len(s.OutputHistory)-100:]
	}
	select {
	case s.OutputCh <- text:
	default:
	}
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}
