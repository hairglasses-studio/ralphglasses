package promptdj

import (
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// AffinityEntry is a weighted provider preference for a (task type, quality tier) pair.
type AffinityEntry struct {
	Provider session.Provider
	Model    string
	Weight   float64 // 0.0-1.0, higher = stronger preference
}

// affinityKey is a lookup key for the matrix.
type affinityKey struct {
	TaskType    enhancer.TaskType
	QualityTier QualityTier
}

// AffinityMatrix maps (task type, quality tier) to ranked provider preferences.
// Populated from the prompt-router-design.md specification.
type AffinityMatrix struct {
	entries map[affinityKey][]AffinityEntry
}

// NewAffinityMatrix returns the default static affinity matrix.
// Key insight from design: Claude Opus dominates low-quality tiers because
// ambiguous prompts need the strongest reasoning model to compensate.
func NewAffinityMatrix() *AffinityMatrix {
	m := &AffinityMatrix{entries: make(map[affinityKey][]AffinityEntry)}

	// code
	m.set(enhancer.TaskTypeCode, QualityHigh, []AffinityEntry{
		{session.ProviderClaude, "claude-opus", 0.85},
		{session.ProviderCodex, "", 0.70},
		{session.ProviderGemini, "", 0.30},
	})
	m.set(enhancer.TaskTypeCode, QualityMedium, []AffinityEntry{
		{session.ProviderCodex, "", 0.75},
		{session.ProviderClaude, "claude-opus", 0.70},
		{session.ProviderGemini, "", 0.35},
	})
	m.set(enhancer.TaskTypeCode, QualityLow, []AffinityEntry{
		{session.ProviderClaude, "claude-opus", 0.90},
		{session.ProviderCodex, "", 0.60},
		{session.ProviderGemini, "", 0.15},
	})

	// analysis
	m.set(enhancer.TaskTypeAnalysis, QualityHigh, []AffinityEntry{
		{session.ProviderClaude, "claude-opus", 0.90},
		{session.ProviderGemini, "", 0.50},
		{session.ProviderCodex, "", 0.45},
	})
	m.set(enhancer.TaskTypeAnalysis, QualityMedium, []AffinityEntry{
		{session.ProviderClaude, "claude-opus", 0.80},
		{session.ProviderCodex, "", 0.55},
		{session.ProviderGemini, "", 0.45},
	})
	m.set(enhancer.TaskTypeAnalysis, QualityLow, []AffinityEntry{
		{session.ProviderClaude, "claude-opus", 0.95},
		{session.ProviderCodex, "", 0.40},
		{session.ProviderGemini, "", 0.20},
	})

	// troubleshooting
	m.set(enhancer.TaskTypeTroubleshooting, QualityHigh, []AffinityEntry{
		{session.ProviderCodex, "", 0.75},
		{session.ProviderClaude, "claude-opus", 0.70},
		{session.ProviderGemini, "", 0.40},
	})
	m.set(enhancer.TaskTypeTroubleshooting, QualityMedium, []AffinityEntry{
		{session.ProviderClaude, "claude-opus", 0.75},
		{session.ProviderCodex, "", 0.70},
		{session.ProviderGemini, "", 0.35},
	})
	m.set(enhancer.TaskTypeTroubleshooting, QualityLow, []AffinityEntry{
		{session.ProviderClaude, "claude-opus", 0.85},
		{session.ProviderCodex, "", 0.55},
		{session.ProviderGemini, "", 0.20},
	})

	// creative
	m.set(enhancer.TaskTypeCreative, QualityHigh, []AffinityEntry{
		{session.ProviderGemini, "", 0.70},
		{session.ProviderClaude, "claude-opus", 0.65},
		{session.ProviderCodex, "", 0.40},
	})
	m.set(enhancer.TaskTypeCreative, QualityMedium, []AffinityEntry{
		{session.ProviderClaude, "claude-opus", 0.70},
		{session.ProviderGemini, "", 0.65},
		{session.ProviderCodex, "", 0.35},
	})
	m.set(enhancer.TaskTypeCreative, QualityLow, []AffinityEntry{
		{session.ProviderClaude, "claude-opus", 0.80},
		{session.ProviderGemini, "", 0.55},
		{session.ProviderCodex, "", 0.25},
	})

	// workflow
	m.set(enhancer.TaskTypeWorkflow, QualityHigh, []AffinityEntry{
		{session.ProviderGemini, "", 0.75},
		{session.ProviderCodex, "", 0.65},
		{session.ProviderClaude, "claude-sonnet", 0.50},
	})
	m.set(enhancer.TaskTypeWorkflow, QualityMedium, []AffinityEntry{
		{session.ProviderGemini, "", 0.70},
		{session.ProviderCodex, "", 0.60},
		{session.ProviderClaude, "claude-sonnet", 0.55},
	})
	m.set(enhancer.TaskTypeWorkflow, QualityLow, []AffinityEntry{
		{session.ProviderClaude, "claude-opus", 0.75},
		{session.ProviderGemini, "", 0.50},
		{session.ProviderCodex, "", 0.45},
	})

	// general
	m.set(enhancer.TaskTypeGeneral, QualityHigh, []AffinityEntry{
		{session.ProviderGemini, "", 0.70},
		{session.ProviderCodex, "", 0.60},
		{session.ProviderClaude, "claude-sonnet", 0.55},
	})
	m.set(enhancer.TaskTypeGeneral, QualityMedium, []AffinityEntry{
		{session.ProviderCodex, "", 0.65},
		{session.ProviderClaude, "claude-sonnet", 0.60},
		{session.ProviderGemini, "", 0.55},
	})
	m.set(enhancer.TaskTypeGeneral, QualityLow, []AffinityEntry{
		{session.ProviderClaude, "claude-opus", 0.80},
		{session.ProviderCodex, "", 0.50},
		{session.ProviderGemini, "", 0.25},
	})

	return m
}

func (m *AffinityMatrix) set(tt enhancer.TaskType, qt QualityTier, entries []AffinityEntry) {
	m.entries[affinityKey{tt, qt}] = entries
}

// Lookup returns the ranked provider preferences for a task type and quality tier.
// Returns nil if no entry exists.
func (m *AffinityMatrix) Lookup(taskType enhancer.TaskType, qualityTier QualityTier) []AffinityEntry {
	return m.entries[affinityKey{taskType, qualityTier}]
}

// TopProvider returns the highest-weighted provider for a task type and quality tier.
func (m *AffinityMatrix) TopProvider(taskType enhancer.TaskType, qualityTier QualityTier) (AffinityEntry, bool) {
	entries := m.Lookup(taskType, qualityTier)
	if len(entries) == 0 {
		return AffinityEntry{}, false
	}
	return entries[0], true
}

// DomainBoosts returns provider weight adjustments for domain-specific tags.
var DomainBoosts = map[string]map[session.Provider]float64{
	"go":         {session.ProviderClaude: 0.10, session.ProviderCodex: 0.05},
	"mcp":        {session.ProviderClaude: 0.10},
	"shader":     {session.ProviderGemini: 0.10},
	"terminal":   {session.ProviderClaude: 0.05},
	"rice":       {session.ProviderGemini: 0.05, session.ProviderClaude: 0.05},
	"agents":     {session.ProviderClaude: 0.10},
	"tui":        {session.ProviderClaude: 0.05},
	"security":   {session.ProviderClaude: 0.10},
	"testing":    {session.ProviderCodex: 0.05, session.ProviderClaude: 0.05},
	"deployment": {session.ProviderGemini: 0.05},
}
