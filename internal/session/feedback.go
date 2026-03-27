package session

import (
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PromptProfile captures performance patterns by task type.
type PromptProfile struct {
	TaskType         string  `json:"task_type"`
	SampleCount      int     `json:"sample_count"`
	AvgCostUSD       float64 `json:"avg_cost_usd"`
	AvgTurns         int     `json:"avg_turns"`
	AvgDurationSec   float64 `json:"avg_duration_sec"`
	CompletionRate   float64 `json:"completion_rate"`
	BestProvider     string  `json:"best_provider"`
	BestModel        string  `json:"best_model"`
	SuggestedBudget  float64 `json:"suggested_budget_usd"`
	LastUpdated      time.Time `json:"last_updated"`
}

// ProviderProfile captures performance patterns by provider + task type.
type ProviderProfile struct {
	Provider         string  `json:"provider"`
	TaskType         string  `json:"task_type"`
	SampleCount      int     `json:"sample_count"`
	AvgCostUSD       float64 `json:"avg_cost_usd"`
	AvgTurns         int     `json:"avg_turns"`
	CompletionRate   float64 `json:"completion_rate"`
	CostPerTurn      float64 `json:"cost_per_turn"`
	LastUpdated      time.Time `json:"last_updated"`
}

// EnhancementProfile tracks enhancement mode effectiveness by source and task type.
type EnhancementProfile struct {
	Source         string  `json:"source"`          // "local", "llm"
	TaskType       string  `json:"task_type"`
	SampleCount    int     `json:"sample_count"`
	CompletionRate float64 `json:"completion_rate"`
	AvgCostUSD     float64 `json:"avg_cost_usd"`
	Effectiveness  float64 `json:"effectiveness"` // completion_rate / normalized_cost
}

// FeedbackAnalyzer processes journal entries to build profiles for future decisions.
type FeedbackAnalyzer struct {
	mu                    sync.Mutex
	promptProfiles        map[string]*PromptProfile      // keyed by task type
	providerProfiles      map[string]*ProviderProfile    // keyed by "provider:task_type"
	enhancementProfiles   map[string]*EnhancementProfile // keyed by "source:task_type"
	minSessions           int                            // minimum sessions before profiles are trusted
	stateDir              string
}

// NewFeedbackAnalyzer creates a feedback analyzer.
func NewFeedbackAnalyzer(stateDir string, minSessions int) *FeedbackAnalyzer {
	if minSessions <= 0 {
		minSessions = 5
	}
	fa := &FeedbackAnalyzer{
		promptProfiles:      make(map[string]*PromptProfile),
		providerProfiles:    make(map[string]*ProviderProfile),
		enhancementProfiles: make(map[string]*EnhancementProfile),
		minSessions:         minSessions,
		stateDir:            stateDir,
	}
	fa.load()
	return fa
}

// Ingest processes a batch of journal entries to update profiles.
func (fa *FeedbackAnalyzer) Ingest(entries []JournalEntry) {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	// Group by task type
	byTask := make(map[string][]JournalEntry)
	byProviderTask := make(map[string][]JournalEntry)
	byEnhancement := make(map[string][]JournalEntry)

	for _, e := range entries {
		taskType := classifyTask(e.TaskFocus)
		byTask[taskType] = append(byTask[taskType], e)
		key := e.Provider + ":" + taskType
		byProviderTask[key] = append(byProviderTask[key], e)

		// Group by enhancement source + task type
		if e.EnhancementSource != "" && e.EnhancementSource != "none" {
			enhKey := e.EnhancementSource + ":" + taskType
			byEnhancement[enhKey] = append(byEnhancement[enhKey], e)
		}
	}

	// Build prompt profiles
	for taskType, batch := range byTask {
		fa.promptProfiles[taskType] = buildPromptProfile(taskType, batch)
	}

	// Build provider profiles
	for key, batch := range byProviderTask {
		parts := strings.SplitN(key, ":", 2)
		fa.providerProfiles[key] = buildProviderProfile(parts[0], parts[1], batch)
	}

	// Build enhancement profiles
	for key, batch := range byEnhancement {
		parts := strings.SplitN(key, ":", 2)
		fa.enhancementProfiles[key] = buildEnhancementProfile(parts[0], parts[1], batch)
	}

	fa.save()
}

// GetPromptProfile returns the profile for a task type, if trusted.
func (fa *FeedbackAnalyzer) GetPromptProfile(taskType string) (*PromptProfile, bool) {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	p, ok := fa.promptProfiles[taskType]
	if !ok || p.SampleCount < fa.minSessions {
		return nil, false
	}
	return p, true
}

// GetProviderProfile returns the profile for a provider + task type, if trusted.
func (fa *FeedbackAnalyzer) GetProviderProfile(provider, taskType string) (*ProviderProfile, bool) {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	key := provider + ":" + taskType
	p, ok := fa.providerProfiles[key]
	if !ok || p.SampleCount < fa.minSessions {
		return nil, false
	}
	return p, true
}

// SuggestProvider returns the best provider for a task type based on profiles.
func (fa *FeedbackAnalyzer) SuggestProvider(taskType string) (Provider, bool) {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	p, ok := fa.promptProfiles[taskType]
	if !ok || p.SampleCount < fa.minSessions || p.BestProvider == "" {
		return "", false
	}
	return Provider(p.BestProvider), true
}

// SuggestBudget returns the suggested budget for a task type.
func (fa *FeedbackAnalyzer) SuggestBudget(taskType string) (float64, bool) {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	p, ok := fa.promptProfiles[taskType]
	if !ok || p.SampleCount < fa.minSessions {
		return 0, false
	}
	return p.SuggestedBudget, true
}

// AllPromptProfiles returns all prompt profiles.
func (fa *FeedbackAnalyzer) AllPromptProfiles() []PromptProfile {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	result := make([]PromptProfile, 0, len(fa.promptProfiles))
	for _, p := range fa.promptProfiles {
		result = append(result, *p)
	}
	return result
}

// AllProviderProfiles returns all provider profiles.
func (fa *FeedbackAnalyzer) AllProviderProfiles() []ProviderProfile {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	result := make([]ProviderProfile, 0, len(fa.providerProfiles))
	for _, p := range fa.providerProfiles {
		result = append(result, *p)
	}
	return result
}

// SuggestEnhancementMode returns "local", "llm", or "auto" based on which
// enhancement source has higher effectiveness for the given task type.
// Requires at least minSessions samples for each source to make a recommendation.
func (fa *FeedbackAnalyzer) SuggestEnhancementMode(taskType string) string {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	localKey := "local:" + taskType
	llmKey := "llm:" + taskType

	localP := fa.enhancementProfiles[localKey]
	llmP := fa.enhancementProfiles[llmKey]

	localReady := localP != nil && localP.SampleCount >= fa.minSessions
	llmReady := llmP != nil && llmP.SampleCount >= fa.minSessions

	if !localReady && !llmReady {
		return "auto"
	}
	if localReady && !llmReady {
		return "local"
	}
	if !localReady && llmReady {
		return "llm"
	}

	// Both ready — compare effectiveness
	if localP.Effectiveness > llmP.Effectiveness*1.1 {
		return "local"
	}
	if llmP.Effectiveness > localP.Effectiveness*1.1 {
		return "llm"
	}
	return "auto"
}

// AllEnhancementProfiles returns all enhancement profiles.
func (fa *FeedbackAnalyzer) AllEnhancementProfiles() []EnhancementProfile {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	result := make([]EnhancementProfile, 0, len(fa.enhancementProfiles))
	for _, p := range fa.enhancementProfiles {
		result = append(result, *p)
	}
	return result
}

func buildEnhancementProfile(source, taskType string, entries []JournalEntry) *EnhancementProfile {
	var totalCost float64
	var completed, total int

	for _, e := range entries {
		total++
		totalCost += e.SpentUSD
		if e.ExitReason == "" || e.ExitReason == "completed" || e.ExitReason == "normal" {
			completed++
		}
	}

	p := &EnhancementProfile{
		Source:      source,
		TaskType:    taskType,
		SampleCount: total,
	}

	if total > 0 {
		p.CompletionRate = float64(completed) / float64(total) * 100
		p.AvgCostUSD = totalCost / float64(total)
		if totalCost > 0 {
			p.Effectiveness = p.CompletionRate / (totalCost / float64(total))
		}
	}

	return p
}

func buildPromptProfile(taskType string, entries []JournalEntry) *PromptProfile {
	var totalCost, totalDuration float64
	var totalTurns, completed, total int

	providerScores := make(map[string]float64)
	providerCounts := make(map[string]int)

	for _, e := range entries {
		total++
		totalCost += e.SpentUSD
		totalTurns += e.TurnCount
		totalDuration += e.DurationSec

		if e.ExitReason == "" || e.ExitReason == "completed" || e.ExitReason == "normal" {
			completed++
		}

		// Score: completion weighted by inverse cost
		score := 0.0
		if e.SpentUSD > 0 {
			score = 1.0 / e.SpentUSD
		}
		if e.ExitReason == "" || e.ExitReason == "completed" || e.ExitReason == "normal" {
			score *= 2
		}
		providerScores[e.Provider] += score
		providerCounts[e.Provider]++
	}

	p := &PromptProfile{
		TaskType:    taskType,
		SampleCount: total,
		LastUpdated: time.Now(),
	}

	if total > 0 {
		p.AvgCostUSD = totalCost / float64(total)
		p.AvgTurns = totalTurns / total
		p.AvgDurationSec = totalDuration / float64(total)
		p.CompletionRate = float64(completed) / float64(total) * 100

		// Suggest budget: avg cost + 1 stddev, rounded up to nearest $0.50
		p.SuggestedBudget = math.Ceil(p.AvgCostUSD*2*2) / 2

		// Find best provider by normalized score
		var bestProvider string
		bestAvgScore := 0.0
		for provider, totalScore := range providerScores {
			avg := totalScore / float64(providerCounts[provider])
			if avg > bestAvgScore {
				bestAvgScore = avg
				bestProvider = provider
			}
		}
		p.BestProvider = bestProvider
	}

	return p
}

func buildProviderProfile(provider, taskType string, entries []JournalEntry) *ProviderProfile {
	var totalCost float64
	var totalTurns, completed, total int

	for _, e := range entries {
		total++
		totalCost += e.SpentUSD
		totalTurns += e.TurnCount
		if e.ExitReason == "" || e.ExitReason == "completed" || e.ExitReason == "normal" {
			completed++
		}
	}

	p := &ProviderProfile{
		Provider:    provider,
		TaskType:    taskType,
		SampleCount: total,
		LastUpdated: time.Now(),
	}

	if total > 0 {
		p.AvgCostUSD = totalCost / float64(total)
		p.AvgTurns = totalTurns / total
		p.CompletionRate = float64(completed) / float64(total) * 100
		if totalTurns > 0 {
			p.CostPerTurn = totalCost / float64(totalTurns)
		}
	}

	return p
}

// classifyTask maps a task focus string to a task type category.
// Categories are checked in priority order (first match wins).
func classifyTask(focus string) string {
	lower := strings.ToLower(focus)

	// Ordered list — more specific categories first to avoid ambiguity
	categories := []struct {
		name     string
		keywords []string
	}{
		{"refactor", []string{"refactor", "restructure", "reorganize", "extract"}},
		{"test", []string{"test", "spec", "coverage", "assert"}},
		{"docs", []string{"doc", "readme", "document"}},
		{"config", []string{"config", "setup", "install", "deploy", "ci/cd", "pipeline"}},
		{"review", []string{"review", "audit"}},
		{"optimization", []string{"optimize", "performance", "speed", "cache", "memory"}},
		{"bug_fix", []string{"fix", "bug", "error", "broken", "crash", "issue"}},
		{"feature", []string{"add", "implement", "create", "new", "feature", "build"}},
	}

	for _, cat := range categories {
		for _, kw := range cat.keywords {
			if strings.Contains(lower, kw) {
				return cat.name
			}
		}
	}
	return "general"
}

func (fa *FeedbackAnalyzer) save() {
	if fa.stateDir == "" {
		return
	}
	if err := os.MkdirAll(fa.stateDir, 0755); err != nil {
		slog.Warn("failed to create feedback state dir", "dir", fa.stateDir, "error", err)
		return
	}

	profiles := struct {
		Prompt      map[string]*PromptProfile      `json:"prompt_profiles"`
		Provider    map[string]*ProviderProfile    `json:"provider_profiles"`
		Enhancement map[string]*EnhancementProfile `json:"enhancement_profiles,omitempty"`
	}{
		Prompt:      fa.promptProfiles,
		Provider:    fa.providerProfiles,
		Enhancement: fa.enhancementProfiles,
	}

	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return
	}
	if err := os.WriteFile(filepath.Join(fa.stateDir, "feedback_profiles.json"), data, 0644); err != nil {
		slog.Warn("failed to write feedback profiles", "error", err)
	}
}

func (fa *FeedbackAnalyzer) load() {
	if fa.stateDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(fa.stateDir, "feedback_profiles.json"))
	if err != nil {
		return
	}

	var profiles struct {
		Prompt      map[string]*PromptProfile      `json:"prompt_profiles"`
		Provider    map[string]*ProviderProfile    `json:"provider_profiles"`
		Enhancement map[string]*EnhancementProfile `json:"enhancement_profiles"`
	}
	if json.Unmarshal(data, &profiles) == nil {
		if profiles.Prompt != nil {
			fa.promptProfiles = profiles.Prompt
		}
		if profiles.Provider != nil {
			fa.providerProfiles = profiles.Provider
		}
		if profiles.Enhancement != nil {
			fa.enhancementProfiles = profiles.Enhancement
		}
	}
}

// FeedbackProfiles is the aggregate result of seeding from observations.
type FeedbackProfiles struct {
	PromptProfiles   []PromptProfile   `json:"prompt_profiles"`
	ProviderProfiles []ProviderProfile `json:"provider_profiles"`
}

// SeedProfilesFromObservations builds feedback profiles from raw loop observations.
// It groups observations by provider and task type, computing aggregate statistics.
// Malformed or incomplete observations (missing provider/task_type) are skipped.
func SeedProfilesFromObservations(obs []LoopObservation) (*FeedbackProfiles, error) {
	result := &FeedbackProfiles{}
	if len(obs) == 0 {
		return result, nil
	}

	// --- Provider profiles ---
	type providerAccum struct {
		provider   string
		totalCost  float64
		totalLatMs int64
		completed  int
		total      int
		totalTurns int
	}
	providerMap := make(map[string]*providerAccum)

	for _, o := range obs {
		// Use worker provider as primary provider; fall back to planner provider.
		prov := o.WorkerProvider
		if prov == "" {
			prov = o.PlannerProvider
		}
		if prov == "" {
			continue // skip observations with no provider info
		}

		acc, ok := providerMap[prov]
		if !ok {
			acc = &providerAccum{provider: prov}
			providerMap[prov] = acc
		}
		acc.total++
		acc.totalCost += o.TotalCostUSD
		acc.totalLatMs += o.TotalLatencyMs
		if o.VerifyPassed {
			acc.completed++
		}
		// Use worker tokens as a proxy for turns.
		acc.totalTurns += int(o.WorkerTokensOut)
	}

	for _, acc := range providerMap {
		pp := ProviderProfile{
			Provider:    acc.provider,
			TaskType:    "all", // aggregate across task types
			SampleCount: acc.total,
			LastUpdated: time.Now(),
		}
		if acc.total > 0 {
			pp.AvgCostUSD = acc.totalCost / float64(acc.total)
			pp.AvgTurns = acc.totalTurns / acc.total
			pp.CompletionRate = float64(acc.completed) / float64(acc.total) * 100
			if acc.totalTurns > 0 {
				pp.CostPerTurn = acc.totalCost / float64(acc.totalTurns)
			}
		}
		result.ProviderProfiles = append(result.ProviderProfiles, pp)
	}

	// --- Task type (prompt) profiles ---
	type taskAccum struct {
		taskType     string
		totalCost    float64
		totalLatMs   int64
		totalTurns   int
		completed    int
		total        int
		providerHits map[string]int
	}
	taskMap := make(map[string]*taskAccum)

	for _, o := range obs {
		tt := o.TaskType
		if tt == "" {
			tt = "general"
		}

		acc, ok := taskMap[tt]
		if !ok {
			acc = &taskAccum{taskType: tt, providerHits: make(map[string]int)}
			taskMap[tt] = acc
		}
		acc.total++
		acc.totalCost += o.TotalCostUSD
		acc.totalLatMs += o.TotalLatencyMs
		acc.totalTurns += int(o.WorkerTokensOut)
		if o.VerifyPassed {
			acc.completed++
		}
		prov := o.WorkerProvider
		if prov == "" {
			prov = o.PlannerProvider
		}
		if prov != "" {
			acc.providerHits[prov]++
		}
	}

	for _, acc := range taskMap {
		pp := PromptProfile{
			TaskType:    acc.taskType,
			SampleCount: acc.total,
			LastUpdated: time.Now(),
		}
		if acc.total > 0 {
			pp.AvgCostUSD = acc.totalCost / float64(acc.total)
			pp.AvgTurns = acc.totalTurns / acc.total
			pp.AvgDurationSec = float64(acc.totalLatMs) / float64(acc.total) / 1000.0
			pp.CompletionRate = float64(acc.completed) / float64(acc.total) * 100
			pp.SuggestedBudget = math.Ceil(pp.AvgCostUSD*2*2) / 2

			// Find most-used provider.
			bestCount := 0
			for prov, count := range acc.providerHits {
				if count > bestCount {
					bestCount = count
					pp.BestProvider = prov
				}
			}
		}
		result.PromptProfiles = append(result.PromptProfiles, pp)
	}

	return result, nil
}

// SeedProfilesFromJSONL reads loop observations from a JSONL file and seeds profiles.
// Returns empty profiles if the file does not exist.
func SeedProfilesFromJSONL(path string) (*FeedbackProfiles, error) {
	obs, err := LoadObservations(path, time.Time{})
	if err != nil {
		return &FeedbackProfiles{}, nil // file missing or unreadable — return empty
	}
	return SeedProfilesFromObservations(obs)
}

// SeedFromObservations populates empty profile maps in the analyzer from observations.
// Only seeds profiles that are currently empty — does not overwrite existing data.
// This is idempotent: calling it multiple times produces the same result.
func (fa *FeedbackAnalyzer) SeedFromObservations(obs []LoopObservation) error {
	fa.mu.Lock()
	defer fa.mu.Unlock()

	// Only seed if profiles are empty.
	if len(fa.promptProfiles) > 0 || len(fa.providerProfiles) > 0 {
		return nil
	}

	seeded, err := SeedProfilesFromObservations(obs)
	if err != nil {
		return err
	}

	for i := range seeded.PromptProfiles {
		p := seeded.PromptProfiles[i]
		fa.promptProfiles[p.TaskType] = &p
	}
	for i := range seeded.ProviderProfiles {
		p := seeded.ProviderProfiles[i]
		key := p.Provider + ":" + p.TaskType
		fa.providerProfiles[key] = &p
	}

	fa.save()
	return nil
}

// IsEmpty returns true if there are no prompt or provider profiles.
func (fa *FeedbackAnalyzer) IsEmpty() bool {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	return len(fa.promptProfiles) == 0 && len(fa.providerProfiles) == 0
}
