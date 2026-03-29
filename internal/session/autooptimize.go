package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// autonomyState is the JSON structure persisted for autonomy level.
type autonomyState struct {
	Level     int    `json:"level"`
	UpdatedAt string `json:"updated_at"`
}

// SaveAutonomyLevel persists the autonomy level to ralphDir/autonomy.json.
func SaveAutonomyLevel(ralphDir string, level int) error {
	if ralphDir == "" {
		return fmt.Errorf("empty ralph dir")
	}
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		return fmt.Errorf("create autonomy dir: %w", err)
	}
	state := autonomyState{
		Level:     level,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(ralphDir, "autonomy.json"), data, 0644)
}

// LoadAutonomyLevel reads the persisted autonomy level from ralphDir/autonomy.json.
// Returns 0 if the file does not exist.
func LoadAutonomyLevel(ralphDir string) (int, error) {
	data, err := os.ReadFile(filepath.Join(ralphDir, "autonomy.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var state autonomyState
	if err := json.Unmarshal(data, &state); err != nil {
		return 0, fmt.Errorf("parse autonomy.json: %w", err)
	}
	return state.Level, nil
}

// PersistAutonomyLevel is an alias for SaveAutonomyLevel for backward compatibility.
func PersistAutonomyLevel(level int, ralphDir string) error {
	return SaveAutonomyLevel(ralphDir, level)
}

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

// GatedChange records an autonomous change that was gated by E2E tests.
type GatedChange struct {
	DecisionID string         `json:"decision_id"`
	ChangeType string         `json:"change_type"` // "provider", "budget", "config"
	Before     map[string]any `json:"before"`
	After      map[string]any `json:"after"`
	Verdict    string         `json:"verdict"`     // "pass", "warn", "fail", "skip"
	RolledBack bool           `json:"rolled_back"`
}

// GateEnabled controls whether the auto-optimizer runs E2E gates before applying changes.
// When true, Level 2+ changes are validated against the test suite.
var GateEnabled bool

// RunTestGate is a pluggable function that runs the E2E gate.
// It's a variable so tests can replace it with a mock.
// Default implementation runs `go test ./... -count=1` with a 60s timeout.
var RunTestGate = defaultTestGate

func defaultTestGate(repoRoot string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "./...", "-count=1")
	cmd.Dir = repoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "fail", fmt.Errorf("test gate failed: %s: %w", string(out), err)
	}
	return "pass", nil
}

// GateChange evaluates a proposed change against the E2E test gate.
// Returns a GatedChange with verdict. If the gate fails, the change is marked for rollback.
func (ao *AutoOptimizer) GateChange(repoRoot string, change GatedChange) GatedChange {
	if !GateEnabled {
		change.Verdict = "skip"
		return change
	}

	verdict, err := RunTestGate(repoRoot)
	if err != nil {
		change.Verdict = "fail"
		change.RolledBack = true
		if ao.decisions != nil {
			ao.decisions.Propose(AutonomousDecision{
				Category:      DecisionSelfTest,
				RequiredLevel: LevelAutoOptimize,
				Rationale:     fmt.Sprintf("E2E gate failed for %s change: %v", change.ChangeType, err),
				Action:        "rollback change",
				Inputs: map[string]any{
					"change_type": change.ChangeType,
					"error":       err.Error(),
				},
			})
		}
		return change
	}

	change.Verdict = verdict
	if verdict == "fail" {
		change.RolledBack = true
	}

	if ao.decisions != nil {
		ao.decisions.Propose(AutonomousDecision{
			Category:      DecisionSelfTest,
			RequiredLevel: LevelAutoOptimize,
			Rationale:     fmt.Sprintf("E2E gate %s for %s change", verdict, change.ChangeType),
			Action:        fmt.Sprintf("gate verdict: %s", verdict),
			Inputs: map[string]any{
				"change_type": change.ChangeType,
				"verdict":     verdict,
			},
		})
	}

	return change
}

// ImprovementNote is a structured, actionable note generated from consolidated patterns.
type ImprovementNote struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"ts"`
	Category    string    `json:"category"`    // "config", "prompt", "code"
	Priority    int       `json:"priority"`    // 1-4 (1=highest)
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Source      string    `json:"source"`      // "pattern_consolidation", "gate_failure", "cost_anomaly"
	AutoApply   bool      `json:"auto_apply"`
	Status      string    `json:"status"`      // "pending", "applied", "rejected"
	DecisionID  string    `json:"decision_id,omitempty"`
}

const improvementNotesFile = ".ralph/improvement_notes.jsonl"

// GenerateNotes creates structured improvement notes from consolidated patterns.
func (ao *AutoOptimizer) GenerateNotes(patterns *ConsolidatedPatterns) []ImprovementNote {
	if patterns == nil {
		return nil
	}

	var notes []ImprovementNote

	for _, rule := range patterns.Rules {
		note := ao.ruleToNote(rule.Action)
		if note != nil {
			notes = append(notes, *note)
		}
	}

	// Generate notes from negative patterns with high counts
	for _, neg := range patterns.Negative {
		if neg.Count >= 3 {
			notes = append(notes, ImprovementNote{
				ID:          fmt.Sprintf("neg-%s-%d", neg.Category, neg.Count),
				Timestamp:   time.Now(),
				Category:    "config",
				Priority:    2,
				Title:       fmt.Sprintf("Recurring failure: %s", neg.Text),
				Description: fmt.Sprintf("Seen %d times (last: %s). Consider config changes to address.", neg.Count, neg.LastSeen.Format(time.DateOnly)),
				Source:      "pattern_consolidation",
				AutoApply:   false,
				Status:      "pending",
			})
		}
	}

	return notes
}

// ruleToNote converts a consolidated rule string into an improvement note, if actionable.
func (ao *AutoOptimizer) ruleToNote(rule string) *ImprovementNote {
	lower := strings.ToLower(rule)

	// Provider suggestion: "use X for Y tasks"
	if strings.Contains(lower, "use ") && strings.Contains(lower, " for ") {
		return &ImprovementNote{
			ID:          fmt.Sprintf("rule-provider-%d", time.Now().UnixNano()),
			Timestamp:   time.Now(),
			Category:    "config",
			Priority:    2,
			Title:       rule,
			Description: "Provider routing suggestion derived from journal patterns.",
			Source:      "pattern_consolidation",
			AutoApply:   true,
			Status:      "pending",
		}
	}

	// Budget suggestion
	if strings.Contains(lower, "budget") {
		return &ImprovementNote{
			ID:          fmt.Sprintf("rule-budget-%d", time.Now().UnixNano()),
			Timestamp:   time.Now(),
			Category:    "config",
			Priority:    3,
			Title:       rule,
			Description: "Budget adjustment suggested from session cost patterns.",
			Source:      "pattern_consolidation",
			AutoApply:   true,
			Status:      "pending",
		}
	}

	return nil
}

// ApplyPendingNotes attempts to apply all pending auto-applicable notes.
// Returns counts of applied and rejected notes.
func (ao *AutoOptimizer) ApplyPendingNotes(repoPath string) (applied, rejected int, err error) {
	notes, err := ReadPendingNotes(repoPath)
	if err != nil {
		return 0, 0, err
	}

	for i, note := range notes {
		if !note.AutoApply || note.Status != "pending" {
			continue
		}

		change := GatedChange{
			ChangeType: note.Category,
			Before:     map[string]any{"note": note.Title},
			After:      map[string]any{"action": "apply"},
		}

		gated := ao.GateChange(repoPath, change)
		if gated.RolledBack {
			notes[i].Status = "rejected"
			rejected++
		} else {
			notes[i].Status = "applied"
			applied++
		}

		if ao.decisions != nil {
			notes[i].DecisionID = gated.DecisionID
		}
	}

	// Write back updated notes
	if applied > 0 || rejected > 0 {
		if writeErr := writeImprovementNotes(repoPath, notes); writeErr != nil {
			return applied, rejected, writeErr
		}
	}

	return applied, rejected, nil
}

// ReadPendingNotes reads all improvement notes from the JSONL file.
func ReadPendingNotes(repoPath string) ([]ImprovementNote, error) {
	path := filepath.Join(repoPath, improvementNotesFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var notes []ImprovementNote
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var note ImprovementNote
		if json.Unmarshal([]byte(line), &note) == nil {
			notes = append(notes, note)
		}
	}
	return notes, nil
}

// WriteImprovementNote appends a single note to the improvement notes file.
func WriteImprovementNote(repoPath string, note ImprovementNote) error {
	path := filepath.Join(repoPath, improvementNotesFile)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create improvement notes dir: %w", err)
	}

	data, err := json.Marshal(note)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

func writeImprovementNotes(repoPath string, notes []ImprovementNote) error {
	path := filepath.Join(repoPath, improvementNotesFile)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create improvement notes dir: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, note := range notes {
		data, err := json.Marshal(note)
		if err != nil {
			continue
		}
		f.Write(append(data, '\n'))
	}
	return nil
}

// HandleSessionCompleteWithOutcome extends HandleSessionComplete to close
// decision outcome loops — records outcomes for any decisions associated
// with the completed session.
func (ao *AutoOptimizer) HandleSessionCompleteWithOutcome(ctx context.Context, s *Session) {
	// Run the standard handler
	ao.HandleSessionComplete(ctx, s)

	// Close decision outcomes
	if ao.decisions == nil || s == nil {
		return
	}

	s.Lock()
	status := s.Status
	sessionID := s.ID
	s.Unlock()

	success := status == StatusCompleted
	details := fmt.Sprintf("session %s ended with status %s", sessionID, status)

	// Find decisions associated with this session and record outcomes
	for _, d := range ao.decisions.Recent(100) {
		if d.SessionID == sessionID && d.Outcome == nil {
			ao.decisions.RecordOutcome(d.ID, DecisionOutcome{
				EvaluatedAt: time.Now(),
				Success:     success,
				Details:     details,
			})
		}
	}
}
