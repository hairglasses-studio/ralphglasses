package session

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func sanitizeLoopName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "loop"
	}
	return s
}

func truncateForPrompt(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return s[:limit-3] + "..."
}

func firstLine(text string) string {
	lines := nonEmptyLines(text)
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func nonEmptyLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// looksLikeJSON returns true if the string, after trimming whitespace,
// starts with '{' or '[', indicating it is likely JSON output rather than
// freeform text. Used as a fast pre-check before attempting full JSON parsing.
func looksLikeJSON(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) > 0 && (s[0] == '{' || s[0] == '[')
}

func sessionOutputSummary(s *Session) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var parts []string
	if len(s.OutputHistory) > 0 {
		parts = append(parts, strings.Join(s.OutputHistory, "\n"))
	}
	if s.LastOutput != "" {
		parts = append(parts, s.LastOutput)
	}
	if s.Error != "" {
		parts = append(parts, s.Error)
	}
	return strings.TrimSpace(strings.Join(dedupeStrings(parts), "\n"))
}

func writeLoopJournal(run *LoopRun, iter LoopIteration) error {
	entry := JournalEntry{
		Timestamp: time.Now(),
		SessionID: iter.WorkerSessionID,
		Provider:  string(run.Profile.WorkerProvider),
		RepoName:  run.RepoName,
		Model:     run.Profile.WorkerModel,
		TaskFocus: iter.Task.Title,
	}
	if iter.Status == "failed" {
		entry.Failed = []string{firstNonBlank(iter.Error, "loop iteration failed")}
	} else {
		entry.Worked = []string{firstNonBlank(iter.Task.Title, "loop iteration completed")}
	}
	return WriteJournalEntryManual(run.RepoPath, entry)
}

func joinOrPlaceholder(items []string, placeholder string) string {
	if len(items) == 0 {
		return placeholder
	}
	return strings.Join(items, "\n")
}

// sanitizeTaskTitle cleans up a planner-produced task title:
// - extracts .title/.Title from raw JSON objects
// - strips whitespace and newlines
// - truncates to 120 characters
func sanitizeTaskTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return title
	}

	// Strip markdown code fences (```json ... ```) before JSON parsing.
	if strings.HasPrefix(title, "```") {
		// Remove opening fence line
		if idx := strings.Index(title, "\n"); idx >= 0 {
			inner := title[idx+1:]
			// Remove closing fence
			if end := strings.LastIndex(inner, "```"); end >= 0 {
				inner = inner[:end]
			}
			title = strings.TrimSpace(inner)
		}
	}

	// If the title looks like a JSON object, try to extract a title field.
	if len(title) > 0 && (title[0] == '{' || title[0] == '[') {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(title), &obj); err == nil {
			for _, key := range []string{"title", "Title", "task", "name", "description"} {
				if v, ok := obj[key]; ok {
					if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
						title = strings.TrimSpace(s)
						break
					}
				}
			}
		}
	}

	// Reject worker output text that leaked into the title.
	outputPrefixes := []string{
		"all tests pass",
		"here's what",
		"i've completed",
		"i have completed",
		"the changes",
		"successfully",
		"done.",
		"completed.",
		"i updated",
		"i added",
		"i fixed",
		"i modified",
		"i removed",
		"i refactored",
		"i created",
		"created ",
		"created.",
		"no changes",
	}
	lower := strings.ToLower(title)
	for _, prefix := range outputPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return "self-improvement iteration"
		}
	}

	// Take only the first line if multiline.
	if idx := strings.IndexAny(title, "\n\r"); idx >= 0 {
		title = strings.TrimSpace(title[:idx])
	}

	// Truncate to 120 chars.
	if len(title) > 120 {
		title = title[:120]
	}

	return title
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func consecutiveLoopFailures(iterations []LoopIteration) int {
	failures := 0
	for i := len(iterations) - 1; i >= 0; i-- {
		if iterations[i].Status != "failed" {
			break
		}
		failures++
	}
	return failures
}

// enhanceForProvider runs hybrid prompt enhancement targeting the given session provider.
// Uses ModeAuto so LLM failures fall back to the local pipeline — never blocks the loop.
// Returns the enhanced prompt along with enhancement metadata (source, pre-score).
func (m *Manager) enhanceForProvider(ctx context.Context, prompt string, provider Provider) enhanceResult {
	// Score the raw prompt before enhancement
	analysis := enhancer.Analyze(prompt)
	preScore := 0
	if analysis.ScoreReport != nil {
		preScore = analysis.ScoreReport.Overall
	}

	target := mapProvider(provider)
	cfg := enhancer.Config{TargetProvider: target}
	result := enhancer.EnhanceHybrid(ctx, prompt, "", cfg, m.Enhancer, enhancer.ModeAuto, target)
	if result.Enhanced != prompt && m.bus != nil {
		m.bus.Publish(events.Event{
			Type: events.PromptEnhanced,
			Data: map[string]any{
				"target_provider": string(target),
				"source":          result.Source,
				"stages_run":      result.StagesRun,
				"pre_score":       preScore,
			},
		})
	}
	return enhanceResult{
		prompt:   result.Enhanced,
		source:   result.Source,
		preScore: preScore,
	}
}

// mapProvider converts a session Provider to the enhancer's ProviderName.
func mapProvider(p Provider) enhancer.ProviderName {
	switch p {
	case ProviderGemini:
		return enhancer.ProviderGemini
	case ProviderCodex:
		return enhancer.ProviderOpenAI
	default:
		return enhancer.ProviderClaude
	}
}

func (m *Manager) loopStateDir() string {
	if m.stateDir == "" {
		return ""
	}
	return filepath.Join(m.stateDir, "loops")
}

// LoopStateDir returns the directory where loop run JSON files are persisted.
// Exported for use by pruning tools.
func (m *Manager) LoopStateDir() string {
	return m.loopStateDir()
}

// PersistLoop writes loop state to disk.
func (m *Manager) PersistLoop(run *LoopRun) {
	dir := m.loopStateDir()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	run.mu.Lock()
	data, err := json.Marshal(run)
	run.mu.Unlock()
	if err != nil {
		return
	}

	if err := os.WriteFile(filepath.Join(dir, run.ID+".json"), data, 0644); err != nil {
		slog.Warn("failed to persist loop state", "loop", run.ID, "error", err)
	}

	// Also persist to Store if configured (durable persistence).
	if m.store != nil {
		if err := m.store.SaveLoopRun(context.Background(), run); err != nil {
			slog.Warn("store save loop run failed", "id", run.ID, "err", err)
		}
	}
}

// LoadExternalLoops merges loop runs persisted by other processes.
// It tries the Store first (authoritative), then fills gaps from JSON files.
func (m *Manager) LoadExternalLoops() {
	// Store-first: load from durable Store if available.
	if m.store != nil {
		runs, err := m.store.ListLoopRuns(context.Background(), LoopRunFilter{})
		if err == nil {
			m.workersMu.Lock()
			for _, run := range runs {
				if _, ok := m.loops[run.ID]; !ok {
					m.loops[run.ID] = run
				}
			}
			m.workersMu.Unlock()
		}
	}

	// Fall back to JSON files to fill gaps (other processes may have written them).
	dir := m.loopStateDir()
	if dir == "" {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	m.workersMu.Lock()
	defer m.workersMu.Unlock()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if _, ok := m.loops[id]; ok {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			continue
		}
		var run LoopRun
		if err := json.Unmarshal(data, &run); err != nil {
			continue
		}
		m.loops[id] = &run
	}
}

// hasGitChanges checks whether the given repo path has uncommitted or new
// changes relative to HEAD, indicating productive work despite a timeout.
func hasGitChanges(repoPath string) bool {
	cmd := exec.Command("git", "-C", repoPath, "diff", "--stat", "HEAD")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}

// EnrichObservationSummary augments an IterationSummary with provider-level
// cost breakdown from the observation set. This is the enrichment layer for
// Phase 0.6.2 — it fills fields that the base SummarizeObservations does not.
func EnrichObservationSummary(obs []LoopObservation) EnrichedSummary {
	base := SummarizeObservations(obs)
	enriched := EnrichedSummary{
		IterationSummary:  base,
		ProviderBreakdown: make(map[string]int),
		TotalCostUSD:      0,
		SuccessRate:       0,
	}
	if len(obs) == 0 {
		return enriched
	}

	var totalCost float64
	successes := 0
	for _, o := range obs {
		totalCost += o.TotalCostUSD
		if o.Status != "failed" && o.Error == "" {
			successes++
		}
		// Provider breakdown by worker (dominant cost driver).
		if o.WorkerProvider != "" {
			enriched.ProviderBreakdown[o.WorkerProvider]++
		} else if o.PlannerProvider != "" {
			enriched.ProviderBreakdown[o.PlannerProvider]++
		}
	}
	enriched.TotalCostUSD = totalCost
	enriched.SuccessRate = float64(successes) / float64(len(obs))
	return enriched
}

// EnrichedSummary extends IterationSummary with provider-level breakdown fields.
type EnrichedSummary struct {
	IterationSummary
	ProviderBreakdown map[string]int `json:"provider_breakdown"`
	TotalCostUSD      float64        `json:"total_cost_usd"`
	SuccessRate       float64        `json:"success_rate"`
}

func normalizeLoopProfile(profile LoopProfile) (LoopProfile, error) {
	def := DefaultLoopProfile()

	if profile.PlannerProvider == "" {
		profile.PlannerProvider = def.PlannerProvider
	}
	if profile.WorkerProvider == "" {
		profile.WorkerProvider = def.WorkerProvider
	}
	if profile.VerifierProvider == "" {
		profile.VerifierProvider = def.VerifierProvider
	}
	if profile.PlannerModel == "" {
		profile.PlannerModel = def.PlannerModel
	}
	if profile.WorkerModel == "" {
		profile.WorkerModel = def.WorkerModel
	}
	if profile.VerifierModel == "" {
		profile.VerifierModel = def.VerifierModel
	}
	if profile.MaxConcurrentWorkers <= 0 {
		profile.MaxConcurrentWorkers = def.MaxConcurrentWorkers
	}
	if profile.RetryLimit < 0 {
		return profile, fmt.Errorf("retry limit must be >= 0")
	}
	if len(profile.VerifyCommands) == 0 {
		profile.VerifyCommands = append([]string(nil), def.VerifyCommands...)
	}
	if profile.WorktreePolicy == "" {
		profile.WorktreePolicy = def.WorktreePolicy
	}
	if profile.MaxConcurrentWorkers > 8 {
		return profile, fmt.Errorf("max concurrent workers capped at 8, got %d", profile.MaxConcurrentWorkers)
	}
	if profile.WorktreePolicy != "git" {
		return profile, fmt.Errorf("unsupported worktree policy %q", profile.WorktreePolicy)
	}
	if profile.CompactionEnabled && profile.CompactionThreshold <= 0 {
		profile.CompactionThreshold = 10
	}

	for _, provider := range []Provider{
		profile.PlannerProvider,
		profile.WorkerProvider,
		profile.VerifierProvider,
	} {
		if providerBinary(provider) == "" {
			return profile, fmt.Errorf("unknown loop provider %q", provider)
		}
	}

	return profile, nil
}
