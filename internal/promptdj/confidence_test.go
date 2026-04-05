package promptdj

import (
	"testing"
)

func TestComputeConfidence_HighQuality(t *testing.T) {
	c := ComputeConfidence(ConfidenceComponents{
		ClassificationConf: 0.95,
		QualityScore:       0.90,
		AffinityStrength:   0.85,
		HistoricalSuccess:  0.80,
		LatencyHealth:      1.00,
		DomainSpecificity:  1.00,
	}, true, false, 0)

	if c < 0.8 {
		t.Errorf("expected high confidence for high-quality components, got %.3f", c)
	}
	if c > 1.0 {
		t.Errorf("confidence should not exceed 1.0, got %.3f", c)
	}
}

func TestComputeConfidence_ColdStartPenalty(t *testing.T) {
	base := ConfidenceComponents{
		ClassificationConf: 0.80,
		QualityScore:       0.70,
		AffinityStrength:   0.75,
		HistoricalSuccess:  0.50,
		LatencyHealth:      1.00,
		DomainSpecificity:  0.50,
	}

	warm := ComputeConfidence(base, true, false, 0)
	cold := ComputeConfidence(base, false, false, 0)

	if cold >= warm {
		t.Errorf("cold-start should have lower confidence: warm=%.3f cold=%.3f", warm, cold)
	}
	// Cold should be ~85% of warm
	ratio := cold / warm
	if ratio < 0.84 || ratio > 0.86 {
		t.Errorf("expected cold/warm ratio ~0.85, got %.3f", ratio)
	}
}

func TestComputeConfidence_EnhancementPenalty(t *testing.T) {
	base := ConfidenceComponents{
		ClassificationConf: 0.80,
		QualityScore:       0.60,
		AffinityStrength:   0.70,
		HistoricalSuccess:  0.60,
		LatencyHealth:      1.00,
		DomainSpecificity:  1.00,
	}

	noEnhance := ComputeConfidence(base, true, false, 0)
	badEnhance := ComputeConfidence(base, true, true, 5) // enhanced but only 5 point improvement

	if badEnhance >= noEnhance {
		t.Errorf("poor enhancement should reduce confidence: no=%.3f bad=%.3f", noEnhance, badEnhance)
	}
}

func TestComputeConfidence_Bounds(t *testing.T) {
	// All zeros
	low := ComputeConfidence(ConfidenceComponents{}, false, true, 0)
	if low < 0 {
		t.Errorf("confidence should not be negative, got %.3f", low)
	}

	// All ones
	high := ComputeConfidence(ConfidenceComponents{
		ClassificationConf: 1.0,
		QualityScore:       1.0,
		AffinityStrength:   1.0,
		HistoricalSuccess:  1.0,
		LatencyHealth:      1.0,
		DomainSpecificity:  1.0,
	}, true, false, 0)
	if high > 1.0 {
		t.Errorf("confidence should not exceed 1.0, got %.3f", high)
	}
}

func TestConfidenceLevelFromScore(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{0.95, "high"},
		{0.80, "high"},
		{0.79, "medium"},
		{0.50, "medium"},
		{0.49, "low"},
		{0.0, "low"},
	}
	for _, tc := range tests {
		got := ConfidenceLevelFromScore(tc.score)
		if got != tc.want {
			t.Errorf("ConfidenceLevelFromScore(%.2f) = %s, want %s", tc.score, got, tc.want)
		}
	}
}

func TestDomainSpecificityScore(t *testing.T) {
	if DomainSpecificityScore(nil) != 0.5 {
		t.Error("nil tags should return 0.5")
	}
	if DomainSpecificityScore([]string{"general"}) != 0.5 {
		t.Error("general tag should return 0.5")
	}
	if DomainSpecificityScore([]string{"go", "mcp"}) != 1.0 {
		t.Error("specific tags should return 1.0")
	}
}
