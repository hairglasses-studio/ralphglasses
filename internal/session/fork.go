package session

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
)

// ForkOptions configures a session fork.
type ForkOptions struct {
	// Prompt is the new prompt for the forked session.
	// If empty, the parent's prompt is reused with a fork context prefix.
	Prompt string

	// Model overrides the parent's model. If empty, inherits from parent.
	Model string

	// Provider overrides the parent's provider. If empty, inherits from parent.
	Provider Provider

	// MaxBudgetUSD overrides the parent's budget. If 0, inherits from parent.
	MaxBudgetUSD float64

	// InjectParentContext adds a summary of the parent session's output
	// history to the forked session's prompt.
	InjectParentContext bool
}

// Fork creates a new session from an existing session's state.
// The forked session inherits the parent's repo, provider, model, and budget,
// and optionally includes a summary of the parent's output history as context.
func (m *Manager) Fork(ctx context.Context, parentID string, opts ForkOptions) (*Session, error) {
	m.sessionsMu.RLock()
	parent, ok := m.sessions[parentID]
	m.sessionsMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("session %s: %w", parentID, ErrSessionNotFound)
	}

	parent.mu.Lock()
	parentProvider := parent.Provider
	parentModel := parent.Model
	parentRepo := parent.RepoPath
	parentBudget := parent.BudgetUSD
	parentPrompt := parent.Prompt
	parentTurnCount := parent.TurnCount
	parentAgent := parent.AgentName
	parentTeam := parent.TeamName

	// Capture recent output for context injection
	var parentContext string
	if opts.InjectParentContext && len(parent.OutputHistory) > 0 {
		parentContext = buildParentContextSummary(parent)
	}
	parent.mu.Unlock()

	// Determine fork parameters
	provider := opts.Provider
	if provider == "" {
		provider = parentProvider
	}
	model := opts.Model
	if model == "" {
		model = parentModel
	}
	budgetUSD := opts.MaxBudgetUSD
	if budgetUSD == 0 {
		budgetUSD = parentBudget
	}

	// Build the fork prompt
	prompt := opts.Prompt
	if prompt == "" {
		prompt = parentPrompt
	}
	if parentContext != "" {
		prompt = parentContext + "\n\n---\n\n" + prompt
	}

	// Launch the forked session
	forkOpts := LaunchOptions{
		Provider:     provider,
		RepoPath:     parentRepo,
		Prompt:       prompt,
		Model:        model,
		MaxBudgetUSD: budgetUSD,
		Agent:        parentAgent,
		TeamName:     parentTeam,
		SessionName:  "fork-" + parentID[:8],
	}

	child, err := m.Launch(ctx, forkOpts)
	if err != nil {
		return nil, fmt.Errorf("fork session %s: %w", parentID, err)
	}

	// Set fork lineage
	child.mu.Lock()
	child.ParentID = parentID
	child.ForkPoint = parentTurnCount
	child.mu.Unlock()

	// Update parent's child list
	parent.mu.Lock()
	parent.ChildIDs = append(parent.ChildIDs, child.ID)
	parent.mu.Unlock()

	slog.Info("session forked",
		"parent", parentID,
		"child", child.ID,
		"fork_point", parentTurnCount,
		"provider", provider,
		"model", model,
	)

	return child, nil
}

// buildParentContextSummary creates a concise summary of the parent session's
// recent output to inject into the forked session's prompt.
func buildParentContextSummary(parent *Session) string {
	var sb strings.Builder
	sb.WriteString("## Parent Session Context\n\n")
	sb.WriteString(fmt.Sprintf("This session was forked from session %s at turn %d.\n", parent.ID, parent.TurnCount))
	sb.WriteString(fmt.Sprintf("Provider: %s, Model: %s\n", parent.Provider, parent.Model))

	if parent.SpentUSD > 0 {
		sb.WriteString(fmt.Sprintf("Parent spent: $%.4f across %d turns\n", parent.SpentUSD, parent.TurnCount))
	}

	// Include recent output (last 5 entries, truncated)
	history := parent.OutputHistory
	start := 0
	if len(history) > 5 {
		start = len(history) - 5
	}
	if len(history) > 0 {
		sb.WriteString("\n### Recent Parent Output\n")
		for i := start; i < len(history); i++ {
			entry := history[i]
			if len(entry) > 500 {
				entry = entry[:500] + "..."
			}
			sb.WriteString(fmt.Sprintf("\n**Turn %d:**\n%s\n", i+1, entry))
		}
	}

	return sb.String()
}

func init() {
	// Ensure uuid is used (prevent import cycle warnings in minimal builds)
	_ = uuid.New
}
