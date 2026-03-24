package session

import (
	"encoding/json"
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

// FeedbackAnalyzer processes journal entries to build profiles for future decisions.
type FeedbackAnalyzer struct {
	mu               sync.Mutex
	promptProfiles   map[string]*PromptProfile   // keyed by task type
	providerProfiles map[string]*ProviderProfile // keyed by "provider:task_type"
	minSessions      int                         // minimum sessions before profiles are trusted
	stateDir         string
}

// NewFeedbackAnalyzer creates a feedback analyzer.
func NewFeedbackAnalyzer(stateDir string, minSessions int) *FeedbackAnalyzer {
	if minSessions <= 0 {
		minSessions = 5
	}
	fa := &FeedbackAnalyzer{
		promptProfiles:   make(map[string]*PromptProfile),
		providerProfiles: make(map[string]*ProviderProfile),
		minSessions:      minSessions,
		stateDir:         stateDir,
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

	for _, e := range entries {
		taskType := classifyTask(e.TaskFocus)
		byTask[taskType] = append(byTask[taskType], e)
		key := e.Provider + ":" + taskType
		byProviderTask[key] = append(byProviderTask[key], e)
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
	_ = os.MkdirAll(fa.stateDir, 0755)

	profiles := struct {
		Prompt   map[string]*PromptProfile   `json:"prompt_profiles"`
		Provider map[string]*ProviderProfile `json:"provider_profiles"`
	}{
		Prompt:   fa.promptProfiles,
		Provider: fa.providerProfiles,
	}

	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(fa.stateDir, "feedback_profiles.json"), data, 0644)
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
		Prompt   map[string]*PromptProfile   `json:"prompt_profiles"`
		Provider map[string]*ProviderProfile `json:"provider_profiles"`
	}
	if json.Unmarshal(data, &profiles) == nil {
		if profiles.Prompt != nil {
			fa.promptProfiles = profiles.Prompt
		}
		if profiles.Provider != nil {
			fa.providerProfiles = profiles.Provider
		}
	}
}
