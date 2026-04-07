package session

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/tracing"
)

// buildCmd constructs the provider CLI command from LaunchOptions.
// Kept for backward compatibility with tests; delegates to buildClaudeCmd.
func buildCmd(ctx context.Context, opts LaunchOptions) *exec.Cmd {
	return buildClaudeCmd(ctx, opts)
}

// launch starts a new LLM CLI session and returns immediately.
// The session runs in a background goroutine that parses streaming output.
func launch(ctx context.Context, opts LaunchOptions, bus ...*events.Bus) (*Session, error) {
	if opts.RepoPath == "" {
		return nil, ErrRepoPathRequired
	}
	// Validate launch preconditions: repo must exist and be a git repo.
	if info, err := os.Stat(opts.RepoPath); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrRepoNotExist, opts.RepoPath)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrRepoNotExist, opts.RepoPath)
	}
	if _, err := os.Stat(filepath.Join(opts.RepoPath, ".git")); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrRepoNotGit, opts.RepoPath)
	}
	if opts.Prompt == "" && opts.Resume == "" && !opts.Continue {
		return nil, fmt.Errorf("prompt required (unless resuming)")
	}

	provider := opts.Provider
	if provider == "" {
		provider = DefaultPrimaryProvider()
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

	// When launching into a self-test target, propagate RALPH_SELF_TEST=1
	// to prevent recursive self-test loops.
	if IsSelfTestTarget(opts.RepoPath) {
		env := cmd.Env
		if env == nil {
			env = os.Environ()
		}
		cmd.Env = SetSelfTestEnv(env)
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
		ID:             uuid.New().String(),
		Provider:       provider,
		RepoPath:       opts.RepoPath,
		RepoName:       filepath.Base(opts.RepoPath),
		Status:         StatusLaunching,
		Prompt:         opts.Prompt,
		Model:          opts.Model,
		AgentName:      opts.Agent,
		TeamName:       opts.TeamName,
		SweepID:        opts.SweepID,
		PermissionMode: opts.PermissionMode,
		Resumed:        opts.Resume != "" || opts.Continue,
		BudgetUSD:      opts.MaxBudgetUSD,
		MaxTurns:       opts.MaxTurns,
		LaunchedAt:     now,
		LastActivity:   now,
		cmd:            cmd,
		cancel:         cancel,
		doneCh:         make(chan struct{}),
		OutputCh:       make(chan string, 100),
		bus:            sessionBus,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start %s: %w", provider, err)
	}

	s.mu.Lock()
	s.Pid = cmd.Process.Pid
	s.ChildPids = collectSessionChildPIDs(cmd.Process.Pid)
	s.Status = StatusRunning
	s.mu.Unlock()

	// Write active state for starship integration (fire-and-forget).
	_ = WriteActiveState(s)

	// Start tracing span
	rec := tracing.Get()
	_, span := rec.StartSessionSpan(sessionCtx, s.ID, string(provider), opts.Model, s.RepoName)

	// Write prompt to stdin and close.
	// Gemini and Codex take prompt as a CLI argument, not stdin.
	go func() {
		if provider == ProviderClaude && opts.Prompt != "" {
			_, _ = io.WriteString(stdin, opts.Prompt)
		}
		stdin.Close()
	}()

	// Create replay recorder if recording is enabled.
	if opts.RecordingEnabled {
		replayDir := opts.RecordingDir
		if replayDir == "" {
			replayDir = filepath.Join(opts.RepoPath, ".ralph", "replays")
		}
		if err := os.MkdirAll(replayDir, 0755); err != nil {
			slog.Warn("session: failed to create replay dir, recording disabled",
				"dir", replayDir, "error", err)
		} else {
			replayPath := filepath.Join(replayDir, s.ID+".jsonl")
			s.recorder = NewRecorder(s.ID, replayPath)
			// Record the initial input event (the prompt).
			if opts.Prompt != "" {
				_ = s.recorder.Record(ReplayEvent{Type: ReplayInput, Data: opts.Prompt})
			}
			// Emit recording started event.
			if sessionBus != nil {
				sessionBus.Publish(events.Event{
					Type:      events.RecordingStarted,
					SessionID: s.ID,
					RepoPath:  s.RepoPath,
					RepoName:  s.RepoName,
					Provider:  string(s.Provider),
					Data:      map[string]any{"replay_path": replayPath},
				})
			}
		}
	}

	// Open log file for persisting session output to disk so the `logs`
	// MCP tool can read it via process.ReadFullLog.
	var logFile *os.File
	if opts.RepoPath != "" {
		if lf, err := process.OpenLogFile(opts.RepoPath); err != nil {
			slog.Warn("session: failed to open log file, output will not be persisted to disk",
				"repo", opts.RepoPath, "error", err)
		} else {
			logFile = lf
		}
	}

	// Background goroutine: parse streaming JSON output
	go func() {
		defer cancel()
		defer close(s.OutputCh)
		if logFile != nil {
			defer logFile.Close()
		}
		runSession(sessionCtx, s, stdout, stderr, span, logFile)
	}()

	// Startup probe: wait up to 5 seconds for the process to stabilize.
	// If it exits during this window, return the error immediately instead
	// of reporting a fake success (FINDING-160).
	if err := startupProbe(s, 5*time.Second); err != nil {
		return nil, err
	}

	return s, nil
}

// startupProbe waits for the process to produce first output or survive the
// probe window. If the process exits during the window, it returns the error.
func startupProbe(s *Session, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-s.doneCh:
			// Process exited during startup. Give the runner a moment to
			// set the final status and error fields.
			time.Sleep(100 * time.Millisecond)
			s.mu.Lock()
			status := s.Status
			errMsg := s.Error
			exitReason := s.ExitReason
			s.mu.Unlock()

			if status == StatusErrored || status == StatusStopped {
				detail := errMsg
				if detail == "" {
					detail = exitReason
				}
				if detail == "" {
					detail = "process exited during startup"
				}
				return fmt.Errorf("session startup failed: %s", detail)
			}
			// Completed normally (unlikely for a long-running session, but valid)
			return nil
		case <-ticker.C:
			// Check if the session has produced any output (non-destructive).
			s.mu.Lock()
			hasOutput := s.TotalOutputCount > 0
			s.mu.Unlock()
			if hasOutput {
				return nil
			}
		case <-timer.C:
			// Process survived the probe window without exiting — success
			return nil
		}
	}
}

// runSession reads streaming JSON from stdout/stderr and updates session state.
// If logFile is non-nil, output lines are also written to it for disk persistence.
func runSession(ctx context.Context, s *Session, stdout, stderr io.Reader, span *tracing.SessionSpan, logFile *os.File) {
	// Read stderr in background for error capture
	var stderrBuf strings.Builder
	stderrDone := make(chan struct{})
	go func() {
		defer close(stderrDone)
		_, _ = io.Copy(&stderrBuf, stderr)
	}()

	runSessionOutput(ctx, s, stdout, logFile)

	// Wait for stderr collection
	<-stderrDone

	// Wait for process exit
	err := s.cmd.Wait()

	// Signal that the process has exited — killWithEscalation watches this.
	if s.doneCh != nil {
		close(s.doneCh)
	}

	s.mu.Lock()

	now := time.Now()
	s.EndedAt = &now
	s.LastActivity = now

	if err != nil {
		if s.Status == StatusStopped {
			if s.ExitReason == "" {
				s.ExitReason = "stopped by user"
			}
		} else {
			s.Status = StatusErrored
			// Classify kill signals for better observability and retry decisions.
			if IsKillSignalError(err.Error()) {
				reason := ClassifyExitSignal(err, s.Pid, false)
				s.ExitReason = FormatKillError(err, reason)
				slog.Warn("session killed by signal",
					"session_id", s.ID,
					"pid", s.Pid,
					"reason", reason.String(),
					"provider", string(s.Provider),
					"repo", s.RepoName,
					"spent_usd", s.SpentUSD,
					"turns", s.TurnCount)
			} else {
				s.ExitReason = err.Error()
			}
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

	// Detect provider quota exhaustion. The CLI may exit 0 while still
	// being unable to continue, so classify these as terminal errors.
	if signal := detectQuotaSignal(s.Provider, s.OutputHistory, s.LastOutput, s.Error, time.Now(), time.Local); signal.Exhausted {
		s.Status = StatusErrored
		s.ExitReason = signal.Reason
		if s.Error == "" {
			switch s.Provider {
			case ProviderClaude:
				s.Error = "Claude Extra Usage quota exhausted"
			case ProviderCodex:
				s.Error = "Codex subscription usage exhausted"
			default:
				s.Error = "provider quota exhausted"
			}
		}
	}

	// Fallback: if no cost was captured from structured output, try stderr patterns
	if s.SpentUSD == 0 {
		if cost, ok := ParseProviderCostFromStderr(s.Provider, stderrBuf.String()); ok && cost > 0 {
			s.SpentUSD = cost
			s.CostSource = "stderr"
			s.CostHistory = append(s.CostHistory, cost)
		}
	}

	// Fallback: if no output was captured from stdout events, use cleaned stderr
	if s.LastOutput == "" && s.Error == "" {
		if cleaned := cleanProviderOutput(s.Provider, stderrBuf.String()); cleaned != "" {
			s.LastOutput = truncateStr(cleaned, 4000)
		}
	}

	// Close replay recorder and emit recording ended event.
	if s.recorder != nil {
		_ = s.recorder.Record(ReplayEvent{
			Type: ReplayStatus,
			Data: fmt.Sprintf("session ended: %s (%s)", s.Status, s.ExitReason),
		})
		_ = s.recorder.Close()
		if s.bus != nil {
			s.bus.Publish(events.Event{
				Type:      events.RecordingEnded,
				SessionID: s.ID,
				RepoPath:  s.RepoPath,
				RepoName:  s.RepoName,
				Provider:  string(s.Provider),
				Data:      map[string]any{"status": string(s.Status)},
			})
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
				"cost_source": s.CostSource,
			},
		})
	}

	// Record tracing
	rec := tracing.Get()
	if span != nil {
		if s.Status == StatusErrored {
			rec.RecordError(span, s.Error)
		}
		rec.EndSessionSpan(span, s.SpentUSD, s.TurnCount, s.ExitReason)
		rec.RecordCostMetric(context.Background(), string(s.Provider), s.RepoName, s.SpentUSD)
	}

	// Capture onComplete before releasing the lock.
	// PersistSession and WriteJournalEntry both acquire s.mu, so they must
	// not be called while we hold it (Go mutexes are not reentrant).
	onComplete := s.onComplete
	s.mu.Unlock()

	// Write active state for starship integration (fire-and-forget).
	// Terminal sessions see final status; RemoveActiveState is called
	// by the manager when the last session finishes.
	_ = WriteActiveState(s)

	// Persist final state to disk (acquires s.mu internally — safe now).
	if onComplete != nil {
		onComplete(s)
	}

	// Write improvement journal entry (fire-and-forget with timeout).
	go func() {
		// Use a bounded context so the goroutine cannot hang indefinitely.
		writeCtx, writeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer writeCancel()
		select {
		case <-writeCtx.Done():
			return
		default:
		}
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
// If logFile is non-nil, parsed output is also written to it for disk persistence.
func runSessionOutput(ctx context.Context, s *Session, stdout io.Reader, logFile *os.File) {
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
					appendSessionOutput(s, msg, logFile)
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
					appendSessionOutput(s, eventText, logFile)
				}
			case "assistant":
				if eventText != "" {
					appendSessionOutput(s, eventText, logFile)
				}
			case "result":
				if eventText != "" {
					s.LastOutput = truncateStr(eventText, 4000)
					appendSessionOutput(s, eventText, logFile)
				}
				if event.NumTurns > 0 {
					s.TurnCount = event.NumTurns
				}
				if event.IsError {
					s.Error = truncateStr(firstNonEmpty(event.Error, event.Result, event.Text), 2000)
					// Do NOT persist session ID from error responses — using a
					// bad ID on resume causes infinite retry loops.
				} else if event.SessionID != "" {
					s.ProviderSessionID = event.SessionID
				}
			default:
				if eventText != "" {
					appendSessionOutput(s, eventText, logFile)
				}
				if event.IsError {
					s.Error = truncateStr(firstNonEmpty(event.Error, eventText), 2000)
				}
			}

			// Cost tracking — runs for ALL event types, not just "result".
			// Claude CLI with API key auth may emit cost/usage data on non-result
			// events (e.g. system, message_stop). Max accounts emit cost_usd on
			// result events. This catches both paths.
			if event.CostUSD > 0 {
				prevSpent := s.SpentUSD
				if s.Provider == ProviderClaude {
					s.SpentUSD = event.CostUSD // Claude emits cumulative cost
				} else {
					s.SpentUSD += event.CostUSD // Gemini/Codex emit per-event cost
				}
				if event.CostSource != "" {
					s.CostSource = event.CostSource
				}
				s.CostHistory = append(s.CostHistory, s.SpentUSD)
				// Record turn metric for tracing
				if delta := s.SpentUSD - prevSpent; delta > 0 {
					tracing.Get().RecordTurnMetric(ctx, string(s.Provider), s.Model, s.ID, 0, 0, delta, 0)
				}
				// Publish cost update event
				if s.bus != nil && s.SpentUSD != prevSpent {
					s.bus.Publish(events.Event{
						Type:      events.CostUpdate,
						SessionID: s.ID,
						RepoPath:  s.RepoPath,
						RepoName:  s.RepoName,
						Provider:  string(s.Provider),
						Data:      map[string]any{"spent_usd": s.SpentUSD, "turns": s.TurnCount, "cost_source": s.CostSource},
					})
				}
				// Emit budget threshold alerts at 50%, 75%, 90%.
				if s.bus != nil && s.BudgetUSD > 0 {
					ratio := s.SpentUSD / s.BudgetUSD
					if s.budgetAlertsEmitted == nil {
						s.budgetAlertsEmitted = make(map[string]bool)
					}
					for _, t := range DefaultBudgetThresholds {
						if ratio >= t.Pct && !s.budgetAlertsEmitted[t.Label] {
							s.budgetAlertsEmitted[t.Label] = true
							s.bus.Publish(events.Event{
								Type:      events.BudgetAlert,
								SessionID: s.ID,
								RepoPath:  s.RepoPath,
								RepoName:  s.RepoName,
								Provider:  string(s.Provider),
								Data:      map[string]any{"threshold": t.Label, "spent_usd": s.SpentUSD, "budget_usd": s.BudgetUSD},
							})
						}
					}
					// Full budget exceeded
					if s.SpentUSD >= s.BudgetUSD {
						s.bus.Publish(events.Event{
							Type:      events.BudgetExceeded,
							SessionID: s.ID,
							RepoPath:  s.RepoPath,
							RepoName:  s.RepoName,
							Provider:  string(s.Provider),
							Data:      map[string]any{"spent_usd": s.SpentUSD, "budget_usd": s.BudgetUSD},
						})
						// Claude enforces budget via --max-budget-usd CLI flag.
						// Non-Claude providers lack CLI budget enforcement, so
						// kill the session here to prevent runaway spend.
						if s.Provider != ProviderClaude {
							s.cancel() // terminate the underlying CLI process immediately
							s.Status = StatusStopped
							s.ExitReason = fmt.Sprintf("budget exceeded: $%.2f of $%.2f", s.SpentUSD, s.BudgetUSD)
							s.mu.Unlock()
							return
						}
					}
				}
			}

			if event.CacheReadTokens > 0 {
				s.CacheReadTokens = event.CacheReadTokens
			}
			if event.CacheWriteTokens > 0 {
				s.CacheWriteTokens = event.CacheWriteTokens
			}
			if s.Provider == ProviderClaude && s.Resumed && s.CacheWriteTokens > 0 && s.CacheReadTokens == 0 && s.CacheAnomaly == "" {
				s.CacheAnomaly = "resumed Claude session wrote prompt cache entries without any cache reads; treating prompt-cache savings as unsafe"
				if s.bus != nil {
					s.bus.Publish(events.Event{
						Type:      events.SessionError,
						SessionID: s.ID,
						RepoPath:  s.RepoPath,
						RepoName:  s.RepoName,
						Provider:  string(s.Provider),
						Data: map[string]any{
							"error":              s.CacheAnomaly,
							"cache_read_tokens":  s.CacheReadTokens,
							"cache_write_tokens": s.CacheWriteTokens,
						},
					})
				}
			}

			// Record replay event if recorder is active.
			if s.recorder != nil && eventText != "" {
				replayType := ReplayOutput
				switch {
				case event.Type == "system":
					replayType = ReplayStatus
				case strings.HasPrefix(event.Type, "tool"):
					replayType = ReplayTool
				}
				_ = s.recorder.Record(ReplayEvent{Type: replayType, Data: eventText})
			}

			s.mu.Unlock()
		}
	}
}

// recordReplayEvent is a no-op-safe helper that records a single replay event.
func recordReplayEvent(rec *Recorder, typ ReplayEventType, data string) {
	if rec == nil || data == "" {
		return
	}
	_ = rec.Record(ReplayEvent{Type: typ, Data: data})
}

func appendSessionOutput(s *Session, text string, logFile *os.File) {
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
	// Persist to disk log file for the `logs` MCP tool.
	if logFile != nil {
		_, _ = fmt.Fprintln(logFile, text)
	}
}

// isExtraUsageExhausted checks session output for Claude's secondary quota
// exhaustion message. When detected, the session should not be retried.
func isExtraUsageExhausted(s *Session) bool {
	for _, line := range s.OutputHistory {
		if strings.Contains(strings.ToLower(line), "out of extra usage") {
			return true
		}
	}
	if strings.Contains(strings.ToLower(s.Error), "out of extra usage") {
		return true
	}
	if strings.Contains(strings.ToLower(s.LastOutput), "out of extra usage") {
		return true
	}
	return false
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}
