package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

const antigravityExitReason = "external_interactive_handoff"

type antigravityLoopRecord struct {
	SessionID           string     `json:"session_id"`
	Provider            Provider   `json:"provider"`
	RepoPath            string     `json:"repo_path"`
	RepoName            string     `json:"repo_name"`
	WorkflowID          string     `json:"workflow_id"`
	OriginalPrompt      string     `json:"original_prompt"`
	CompletionContract  string     `json:"completion_contract"`
	IterationCap        int        `json:"iteration_cap"`
	Status              string     `json:"status"`
	LastValidationNote  string     `json:"last_validation_note"`
	LaunchedAt          time.Time  `json:"launched_at"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
	ExternalSessionHint string     `json:"external_session_hint,omitempty"`
}

func launchAntigravityHandoff(ctx context.Context, opts LaunchOptions, bus ...*events.Bus) (*Session, error) {
	sessionCtx, cancel := context.WithCancel(ctx)
	cmd, err := buildCmdForProvider(sessionCtx, opts)
	if err != nil {
		cancel()
		return nil, err
	}

	var sessionBus *events.Bus
	if len(bus) > 0 {
		sessionBus = bus[0]
	}

	now := time.Now()
	s := &Session{
		ID:               uuid.New().String(),
		Provider:         ProviderAntigravity,
		RepoPath:         opts.RepoPath,
		RepoName:         filepath.Base(opts.RepoPath),
		Status:           StatusLaunching,
		Prompt:           opts.Prompt,
		Model:            "",
		AgentName:        opts.Agent,
		TeamName:         opts.TeamName,
		SweepID:          opts.SweepID,
		PermissionMode:   opts.PermissionMode,
		BudgetUSD:        opts.MaxBudgetUSD,
		MaxTurns:         opts.MaxTurns,
		LaunchedAt:       now,
		LastActivity:     now,
		LastOutput:       "Opened Antigravity interactive session",
		OutputHistory:    []string{"Opened Antigravity interactive session"},
		TotalOutputCount: 1,
		CtxBudget:        NewContextBudget(ModelLimitForProvider(DefaultPrimaryProvider())),
		cmd:              cmd,
		cancel:           cancel,
		doneCh:           make(chan struct{}),
		OutputCh:         make(chan string, 1),
		bus:              sessionBus,
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start %s: %w", opts.Provider, err)
	}

	finishedAt := time.Now()

	s.mu.Lock()
	s.Pid = cmd.Process.Pid
	s.ChildPids = nil
	s.Status = StatusCompleted
	s.ExitReason = antigravityExitReason
	s.EndedAt = &finishedAt
	s.LastActivity = finishedAt
	s.LastOutput = "Antigravity interactive handoff opened"
	s.OutputHistory = []string{s.LastOutput}
	s.TotalOutputCount = 1
	s.mu.Unlock()

	// Antigravity is an external interactive handoff. Once the launcher starts
	// successfully, Ralph should record completion immediately rather than
	// pretending it can manage an ongoing streamed session lifecycle.
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}

	select {
	case s.OutputCh <- s.LastOutput:
	default:
	}
	close(s.OutputCh)
	close(s.doneCh)

	_ = WriteActiveState(s)
	_ = persistAntigravityLoopRecord(s, opts, &finishedAt)
	cancel()

	if sessionBus != nil {
		sessionBus.Publish(events.Event{
			Type:      events.SessionEnded,
			SessionID: s.ID,
			RepoPath:  s.RepoPath,
			RepoName:  s.RepoName,
			Provider:  string(s.Provider),
			Data: map[string]any{
				"status":      s.Status,
				"exit_reason": s.ExitReason,
			},
		})
	}

	if onComplete := s.onComplete; onComplete != nil {
		onComplete(s)
	}

	return s, nil
}

func persistAntigravityLoopRecord(s *Session, opts LaunchOptions, completedAt *time.Time) error {
	loopDir := filepath.Join(opts.RepoPath, ".ralph", "loops")
	if err := os.MkdirAll(loopDir, 0o755); err != nil {
		return err
	}

	record := antigravityLoopRecord{
		SessionID:          s.ID,
		Provider:           ProviderAntigravity,
		RepoPath:           opts.RepoPath,
		RepoName:           filepath.Base(opts.RepoPath),
		WorkflowID:         antigravityWorkflowID(opts),
		OriginalPrompt:     opts.Prompt,
		CompletionContract: antigravityExitReason,
		IterationCap:       1,
		Status:             "launched",
		LastValidationNote: "Antigravity is launched as an external interactive handoff with reduced telemetry in ralphglasses.",
		LaunchedAt:         s.LaunchedAt,
	}
	if completedAt != nil {
		record.Status = "opened"
		record.CompletedAt = completedAt
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(filepath.Join(loopDir, "antigravity-"+s.ID+".json"), data, 0o644)
}

func antigravityWorkflowID(opts LaunchOptions) string {
	for _, candidate := range []string{opts.SessionName, opts.Agent} {
		if candidate != "" {
			return candidate
		}
	}
	return "manual"
}
