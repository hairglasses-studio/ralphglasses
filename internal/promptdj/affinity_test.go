package promptdj

import (
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestAffinityMatrix_AllCellsPopulated(t *testing.T) {
	m := NewAffinityMatrix()

	taskTypes := []enhancer.TaskType{
		enhancer.TaskTypeCode, enhancer.TaskTypeAnalysis,
		enhancer.TaskTypeTroubleshooting, enhancer.TaskTypeCreative,
		enhancer.TaskTypeWorkflow, enhancer.TaskTypeGeneral,
	}
	qualityTiers := []QualityTier{QualityHigh, QualityMedium, QualityLow}

	for _, tt := range taskTypes {
		for _, qt := range qualityTiers {
			entries := m.Lookup(tt, qt)
			if len(entries) == 0 {
				t.Errorf("empty affinity for %s/%s", tt, qt)
			}
			// Each cell should have at least 2 providers
			if len(entries) < 2 {
				t.Errorf("expected >= 2 entries for %s/%s, got %d", tt, qt, len(entries))
			}
			// Weights should be in (0, 1]
			for _, e := range entries {
				if e.Weight <= 0 || e.Weight > 1 {
					t.Errorf("invalid weight %.2f for %s/%s/%s", e.Weight, tt, qt, e.Provider)
				}
			}
		}
	}
}

func TestAffinityMatrix_LowQualityFavorsClaude(t *testing.T) {
	m := NewAffinityMatrix()

	// Key design insight: Claude Opus should dominate low-quality tiers
	for _, tt := range []enhancer.TaskType{
		enhancer.TaskTypeCode, enhancer.TaskTypeAnalysis,
		enhancer.TaskTypeTroubleshooting, enhancer.TaskTypeCreative,
	} {
		top, ok := m.TopProvider(tt, QualityLow)
		if !ok {
			t.Errorf("no top provider for %s/low", tt)
			continue
		}
		if top.Provider != session.ProviderClaude {
			t.Errorf("expected Claude for %s/low, got %s (weight %.2f)", tt, top.Provider, top.Weight)
		}
	}
}

func TestDomainBoosts(t *testing.T) {
	m := NewAffinityMatrix()
	entries := m.Lookup(enhancer.TaskTypeCode, QualityHigh)

	// Apply Go domain boost
	boosted := applyDomainBoosts(entries, []string{"go"})

	// Claude should get +0.10 boost for Go
	for _, e := range boosted {
		if e.Provider == session.ProviderClaude {
			original := entries[0] // Claude was first in code/high
			if e.Weight <= original.Weight {
				t.Errorf("expected Go boost for Claude, original=%.2f boosted=%.2f", original.Weight, e.Weight)
			}
			break
		}
	}
}

func TestQualityTierFromScore(t *testing.T) {
	tests := []struct {
		score int
		want  QualityTier
	}{
		{95, QualityHigh},
		{80, QualityHigh},
		{79, QualityMedium},
		{50, QualityMedium},
		{49, QualityLow},
		{0, QualityLow},
	}
	for _, tc := range tests {
		got := QualityTierFromScore(tc.score)
		if got != tc.want {
			t.Errorf("QualityTierFromScore(%d) = %s, want %s", tc.score, got, tc.want)
		}
	}
}
