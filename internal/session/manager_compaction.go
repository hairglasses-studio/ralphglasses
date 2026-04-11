package session

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
)

func (m *Manager) applyResumeCompaction(ctx context.Context, opts *LaunchOptions) {
	if opts == nil || opts.Resume == "" {
		return
	}
	provider := normalizeSessionProvider(opts.Provider)
	if provider == ProviderClaude {
		return
	}

	summary, applied, _ := m.resumeCompactionPrompt(ctx, *opts)
	if !applied {
		return
	}

	var sb strings.Builder
	sb.WriteString(summary)
	sb.WriteString("\n\nContinue with the task.\n")
	opts.SystemPrompt = sb.String() + "\n" + opts.SystemPrompt
}

func (m *Manager) resumeCompactionPrompt(ctx context.Context, opts LaunchOptions) (string, bool, bool) {
	history := m.resumeHistory(opts.Resume)
	if len(history) == 0 {
		return "", false, false
	}

	cfg := DefaultCompactionConfig()
	compactor := NewContextCompactor(cfg)
	processed := history
	attempted := false

	clearedHistory, cleared := compactor.ClearOldToolResults(history, cfg.KeepRecentTurns)
	if cleared > 0 {
		attempted = true
		processed = clearedHistory
		if !compactor.NeedsCompaction(processed) {
			m.resetResumeCompactionFailures(opts.Resume)
			slog.Info("applied resume tool-result compaction", "session", opts.Resume, "cleared", cleared)
			return renderCompactionSummary(processed), true, attempted
		}
	}

	failures := m.resumeCompactionFailureCount(opts.Resume)
	if failures >= maxConsecutiveCompactFailures {
		slog.Warn("resume compaction circuit open; skipping", "session", opts.Resume, "failures", failures)
		return "", false, false
	}
	if !compactor.NeedsCompaction(processed) {
		return "", false, attempted
	}

	if cfg.Strategy == StrategyLLMSummarize {
		summary, ok := m.tryResumeLLMSummary(ctx, opts, processed)
		if ok {
			m.resetResumeCompactionFailures(opts.Resume)
			slog.Info("performed LLM-powered marathon context compaction", "session", opts.Resume)
			return summary, true, true
		}
	}

	compactor.consecutiveFailures = failures
	result, compacted := compactor.AutoCompactIfNeeded(processed)
	attempted = true
	if !compacted {
		m.setResumeCompactionFailures(opts.Resume, compactor.ConsecutiveFailures())
		slog.Warn("resume compaction attempt did not reduce context",
			"session", opts.Resume,
			"failures", compactor.ConsecutiveFailures(),
			"broken", compactor.CircuitBroken(),
		)
		return "", false, attempted
	}

	m.resetResumeCompactionFailures(opts.Resume)
	slog.Info("performed heuristic marathon context compaction",
		"session", opts.Resume,
		"reduction", result.Reduction,
		"tool_outputs_dropped", result.ToolOutputsDropped,
	)
	return renderCompactionSummary(result.Messages), true, attempted
}

func (m *Manager) resumeHistory(sessionID string) []Message {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	m.sessionsMu.RLock()
	prev := m.sessions[sessionID]
	m.sessionsMu.RUnlock()
	if prev == nil {
		return nil
	}

	prev.mu.Lock()
	defer prev.mu.Unlock()
	return copyMessages(prev.MessageHistory)
}

func (m *Manager) tryResumeLLMSummary(ctx context.Context, opts LaunchOptions, history []Message) (string, bool) {
	summarizerPrompt := "Summarize the following conversation history into a concise narrative, preserving all key decisions, implemented changes, and remaining tasks. Respond ONLY with the summary.\n\n"
	var historyText strings.Builder
	for _, msg := range history {
		historyText.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}
	summarizerOpts := LaunchOptions{
		SessionName:  "summarizer-" + opts.Resume,
		Provider:     opts.Provider,
		RepoPath:     opts.RepoPath,
		Prompt:       summarizerPrompt + historyText.String(),
		MaxBudgetUSD: 0.50,
		Bare:         true,
	}
	sumSess, err := m.launchWorkflowSession(ctx, summarizerOpts)
	if err != nil {
		return "", false
	}
	if waitErr := m.waitForSession(ctx, sumSess); waitErr != nil {
		return "", false
	}
	return sumSess.LastOutput, strings.TrimSpace(sumSess.LastOutput) != ""
}

func renderCompactionSummary(messages []Message) string {
	var sb strings.Builder
	sb.WriteString("Previous conversation summary (compacted):\n")
	for _, msg := range messages {
		sb.WriteString(fmt.Sprintf("[%s]: %s\n", msg.Role, msg.Content))
	}
	return sb.String()
}

func (m *Manager) resumeCompactionFailureCount(sessionID string) int {
	m.compactionFailuresMu.Lock()
	defer m.compactionFailuresMu.Unlock()
	return m.resumeCompactionFailures[sessionID]
}

func (m *Manager) setResumeCompactionFailures(sessionID string, count int) {
	m.compactionFailuresMu.Lock()
	defer m.compactionFailuresMu.Unlock()
	if count <= 0 {
		delete(m.resumeCompactionFailures, sessionID)
		return
	}
	m.resumeCompactionFailures[sessionID] = count
}

func (m *Manager) resetResumeCompactionFailures(sessionID string) {
	m.setResumeCompactionFailures(sessionID, 0)
}
