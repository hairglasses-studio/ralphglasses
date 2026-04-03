package session

import (
	"log/slog"
)

// PromptEvolutionBridge connects the PromptEvolution subsystem to the session
// lifecycle, enabling automatic variant selection and trial recording.

// SelectEvolvedPrompt checks if a better prompt variant exists for the given
// task type and returns it. Falls back to the original prompt if no variant
// is ready or evolution is not configured.
func (m *Manager) SelectEvolvedPrompt(taskType, originalPrompt string) (prompt string, variantID string) {
	if m.promptEvolution == nil {
		return originalPrompt, ""
	}

	template, vid := m.promptEvolution.SelectBest(taskType)
	if template == "" || vid == "" {
		return originalPrompt, ""
	}

	slog.Debug("prompt evolution: selected variant",
		"task_type", taskType,
		"variant_id", vid,
	)
	return template, vid
}

// RecordPromptTrial records the outcome of a session that used an evolved
// prompt variant. This feeds back into the bandit-based selection.
func (m *Manager) RecordPromptTrial(variantID string, success bool, costUSD, durationSec float64) {
	if m.promptEvolution == nil || variantID == "" {
		return
	}

	m.promptEvolution.RecordTrial(PromptTrialResult{
		VariantID: variantID,
		Success:   success,
		CostUSD:   costUSD,
		DurSec:    durationSec,
	})

	slog.Debug("prompt evolution: recorded trial",
		"variant_id", variantID,
		"success", success,
		"cost_usd", costUSD,
	)
}

// EvolvePrompts triggers mutation for a task type, creating new variants
// from the current best performers. Called periodically by the supervisor.
func (m *Manager) EvolvePrompts(taskType string) string {
	if m.promptEvolution == nil {
		return ""
	}

	mutant := m.promptEvolution.Mutate(taskType)
	if mutant != "" {
		slog.Info("prompt evolution: created mutant",
			"task_type", taskType,
		)
	}
	return mutant
}
