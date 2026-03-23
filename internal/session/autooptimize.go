package session

import (
	"context"
	"fmt"
	"time"
)

// AutoOptimizer implements Level 2 (auto-optimize) decision engines.
// It wires FeedbackAnalyzer profiles and CostNorm into launch decisions,
// making the system learn from every session.
type AutoOptimizer struct {
	feedback  *FeedbackAnalyzer
	decisions *DecisionLog
	hitl      *HITLTracker
	recovery  *AutoRecovery
}

// NewAutoOptimizer creates an auto-optimizer with all required dependencies.
func NewAutoOptimizer(feedback *FeedbackAnalyzer, decisions *DecisionLog, hitl *HITLTracker, recovery *AutoRecovery) *AutoOptimizer {
	return &AutoOptimizer{
		feedback:  feedback,
		decisions: decisions,
		hitl:      hitl,
		recovery:  recovery,
	}
}

// OptimizedLaunchOptions adjusts LaunchOptions based on feedback profiles.
// Called by Manager.Launch when autonomy level >= 2.
// Returns the adjusted options and whether any changes were made.
func (ao *AutoOptimizer) OptimizedLaunchOptions(opts LaunchOptions) (LaunchOptions, bool) {
	if ao.feedback == nil || ao.decisions == nil {
		return opts, false
	}

	taskType := classifyTask(opts.Prompt)
	changed := false

	// Provider selection: use FeedbackAnalyzer.SuggestProvider if no explicit provider
	if opts.Provider == "" || opts.Provider == ProviderClaude {
		if suggested, ok := ao.feedback.SuggestProvider(taskType); ok && suggested != "" {
			decision := AutonomousDecision{
				Category:      DecisionProviderSelect,
				RequiredLevel: LevelAutoOptimize,
				Rationale:     fmt.Sprintf("FeedbackAnalyzer suggests %s for %s tasks", suggested, taskType),
				Action:        fmt.Sprintf("switch provider from %s to %s", opts.Provider, suggested),
				Inputs: map[string]any{
					"task_type":          taskType,
					"original_provider":  string(opts.Provider),
					"suggested_provider": string(suggested),
				},
			}
			if ao.decisions.Propose(decision) {
				opts.Provider = suggested
				changed = true
			}
		}
	}

	// Budget adjustment: use FeedbackAnalyzer.SuggestBudget if no explicit budget
	if opts.MaxBudgetUSD <= 0 {
		if suggested, ok := ao.feedback.SuggestBudget(taskType); ok && suggested > 0 {
			decision := AutonomousDecision{
				Category:      DecisionBudgetAdjust,
				RequiredLevel: LevelAutoOptimize,
				Rationale:     fmt.Sprintf("FeedbackAnalyzer suggests $%.2f budget for %s tasks", suggested, taskType),
				Action:        fmt.Sprintf("set budget to $%.2f", suggested),
				Inputs: map[string]any{
					"task_type":        taskType,
					"suggested_budget": suggested,
				},
			}
			if ao.decisions.Propose(decision) {
				opts.MaxBudgetUSD = suggested
				changed = true
			}
		}
	}

	return opts, changed
}

// HandleSessionComplete processes a completed session for feedback ingestion
// and auto-recovery on failure.
func (ao *AutoOptimizer) HandleSessionComplete(ctx context.Context, s *Session) {
	if s == nil {
		return
	}

	s.Lock()
	status := s.Status
	sessionID := s.ID
	repoName := s.RepoName
	s.Unlock()

	// Record HITL metrics
	if ao.hitl != nil {
		switch status {
		case StatusCompleted:
			ao.hitl.RecordAuto(MetricSessionCompleted, sessionID, repoName, "session completed normally")
		case StatusErrored:
			ao.hitl.RecordAuto(MetricSessionErrored, sessionID, repoName, "session errored")
		}
	}

	// Auto-recovery for errored sessions
	if status == StatusErrored && ao.recovery != nil {
		ao.recovery.HandleSessionError(ctx, s)
	}

	// Clear retry state on successful completion
	if status == StatusCompleted && ao.recovery != nil {
		ao.recovery.ClearRetryState(sessionID)
	}
}

// BuildSmartFailoverChain returns a FailoverChain ordered by FeedbackAnalyzer
// profiles for the given task type, falling back to the default static chain.
func (ao *AutoOptimizer) BuildSmartFailoverChain(prompt string) FailoverChain {
	if ao.feedback == nil {
		return DefaultFailoverChain()
	}

	taskType := classifyTask(prompt)

	// Score each provider based on feedback profiles
	type scored struct {
		provider Provider
		score    float64
	}

	candidates := []Provider{ProviderClaude, ProviderGemini, ProviderCodex}
	var scores []scored

	for _, p := range candidates {
		profile, ok := ao.feedback.GetProviderProfile(string(p), taskType)
		if !ok {
			// No data — assign neutral score
			scores = append(scores, scored{p, 0.5})
			continue
		}

		// Score = completion_rate * (1 / normalized_cost_per_turn)
		score := profile.CompletionRate / 100.0
		if profile.CostPerTurn > 0 {
			// Use CostNorm to normalize to Claude baseline
			norm := NormalizeProviderCost(p, profile.CostPerTurn, 0, 0)
			if norm.NormalizedUSD > 0 {
				costFactor := profile.CostPerTurn / norm.NormalizedUSD
				if costFactor > 0 {
					score *= (1.0 / costFactor)
				}
			}
		}
		scores = append(scores, scored{p, score})
	}

	// Sort by score descending (simple insertion sort for 3 elements)
	for i := 1; i < len(scores); i++ {
		for j := i; j > 0 && scores[j].score > scores[j-1].score; j-- {
			scores[j], scores[j-1] = scores[j-1], scores[j]
		}
	}

	chain := FailoverChain{Providers: make([]Provider, len(scores))}
	for i, s := range scores {
		chain.Providers[i] = s.provider
	}
	return chain
}

// IngestSessionJournal reads the journal entry from a completed session and
// feeds it to the FeedbackAnalyzer for profile updates.
func (ao *AutoOptimizer) IngestSessionJournal(s *Session) {
	if ao.feedback == nil || s == nil {
		return
	}

	s.Lock()
	entry := JournalEntry{
		Timestamp:  time.Now(),
		SessionID:  s.ID,
		Provider:   string(s.Provider),
		RepoName:   s.RepoName,
		Model:      s.Model,
		SpentUSD:   s.SpentUSD,
		TurnCount:  s.TurnCount,
		ExitReason: s.ExitReason,
		TaskFocus:  s.Prompt,
	}
	if s.LaunchedAt.IsZero() {
		entry.DurationSec = 0
	} else if s.EndedAt != nil {
		entry.DurationSec = s.EndedAt.Sub(s.LaunchedAt).Seconds()
	}
	s.Unlock()

	ao.feedback.Ingest([]JournalEntry{entry})
}

// ProviderRecommendation holds a recommendation for a task.
type ProviderRecommendation struct {
	Provider        Provider `json:"provider"`
	Model           string   `json:"model"`
	EstimatedBudget float64  `json:"estimated_budget_usd"`
	Confidence      string   `json:"confidence"` // "high", "medium", "low"
	TaskType        string   `json:"task_type"`
	Rationale       string   `json:"rationale"`
	NormalizedCost  float64  `json:"normalized_cost_usd,omitempty"`
}

// RecommendProvider returns a provider recommendation for the given task.
func (ao *AutoOptimizer) RecommendProvider(prompt string) ProviderRecommendation {
	taskType := classifyTask(prompt)

	rec := ProviderRecommendation{
		Provider:   ProviderClaude,
		Model:      ProviderDefaults(ProviderClaude),
		TaskType:   taskType,
		Confidence: "low",
		Rationale:  "default: no feedback data available",
	}

	if ao.feedback == nil {
		return rec
	}

	// Check prompt profile for overall best provider
	profile, hasTrusted := ao.feedback.GetPromptProfile(taskType)
	if !hasTrusted {
		rec.Rationale = fmt.Sprintf("insufficient data for %s tasks (need %d+ samples)", taskType, ao.feedback.minSessions)
		return rec
	}

	// Use suggested provider
	if profile.BestProvider != "" {
		rec.Provider = Provider(profile.BestProvider)
		rec.Model = ProviderDefaults(rec.Provider)
		rec.Confidence = "medium"
		rec.Rationale = fmt.Sprintf("best provider for %s tasks: %.0f%% completion, $%.3f avg cost",
			taskType, profile.CompletionRate, profile.AvgCostUSD)
	}

	// Use suggested budget
	if profile.SuggestedBudget > 0 {
		rec.EstimatedBudget = profile.SuggestedBudget
	}

	// Check provider-specific profile for more confidence
	provProfile, hasProvData := ao.feedback.GetProviderProfile(string(rec.Provider), taskType)
	if hasProvData {
		rec.Confidence = "high"
		rec.Rationale = fmt.Sprintf("%s for %s: %.0f%% completion, $%.4f/turn, %d samples",
			rec.Provider, taskType, provProfile.CompletionRate, provProfile.CostPerTurn, provProfile.SampleCount)

		// Add normalized cost
		if provProfile.CostPerTurn > 0 {
			norm := NormalizeProviderCost(rec.Provider, provProfile.CostPerTurn, 0, 0)
			rec.NormalizedCost = norm.NormalizedUSD
		}
	}

	return rec
}
