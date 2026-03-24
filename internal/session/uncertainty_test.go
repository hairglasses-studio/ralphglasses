package session

import (
	"testing"
)

func TestExtractConfidence_HighConfidence(t *testing.T) {
	s := &Session{
		TurnCount:     5,
		Error:         "",
		OutputHistory: []string{"All tasks completed successfully.", "No issues found."},
	}
	verifications := []LoopVerification{
		{Status: "passed", ExitCode: 0},
		{Status: "passed", ExitCode: 0},
	}

	sig := ExtractConfidence(s, 5, verifications)

	if sig.Overall <= 0.8 {
		t.Errorf("Overall = %f, want > 0.8", sig.Overall)
	}
	if !sig.VerifyPassed {
		t.Error("VerifyPassed = false, want true")
	}
	if !sig.ErrorFree {
		t.Error("ErrorFree = false, want true")
	}
	if sig.TurnEfficiency != 1.0 {
		t.Errorf("TurnEfficiency = %f, want 1.0", sig.TurnEfficiency)
	}
}

func TestExtractConfidence_LowConfidence(t *testing.T) {
	s := &Session{
		TurnCount: 20,
		Error:     "build failed",
		OutputHistory: []string{
			"I think this might work, maybe not, possibly could be wrong.",
			"I'm not sure, perhaps we should try something else?",
			"Should I try a different approach? Would you like me to change?",
			"Could be uncertain about this. Maybe reconsider?",
		},
	}
	verifications := []LoopVerification{
		{Status: "failed", ExitCode: 1},
	}

	sig := ExtractConfidence(s, 5, verifications)

	if sig.Overall >= 0.4 {
		t.Errorf("Overall = %f, want < 0.4", sig.Overall)
	}
}

func TestExtractConfidence_ZeroExpectedTurns(t *testing.T) {
	s := &Session{
		TurnCount:     3,
		OutputHistory: []string{"done"},
	}

	sig := ExtractConfidence(s, 0, nil)

	if sig.TurnEfficiency != 0.5 {
		t.Errorf("TurnEfficiency = %f, want 0.5", sig.TurnEfficiency)
	}
}

func TestShouldSkipVerification(t *testing.T) {
	thresholds := DefaultConfidenceThresholds()

	// High confidence, error free, no hedging -> skip
	high := ConfidenceSignals{Overall: 0.96, ErrorFree: true, HedgeCount: 0}
	if !ShouldSkipVerification(high, thresholds) {
		t.Error("expected skip verification for high confidence signals")
	}

	// Medium confidence -> no skip
	medium := ConfidenceSignals{Overall: 0.6, ErrorFree: true, HedgeCount: 0}
	if ShouldSkipVerification(medium, thresholds) {
		t.Error("expected no skip for medium confidence")
	}

	// High confidence but has hedging -> no skip
	hedgy := ConfidenceSignals{Overall: 0.96, ErrorFree: true, HedgeCount: 2}
	if ShouldSkipVerification(hedgy, thresholds) {
		t.Error("expected no skip when hedge count > 0")
	}
}

func TestShouldTriggerReflexion(t *testing.T) {
	thresholds := DefaultConfidenceThresholds()

	low := ConfidenceSignals{Overall: 0.2}
	if !ShouldTriggerReflexion(low, thresholds) {
		t.Error("expected reflexion trigger for low confidence")
	}

	medium := ConfidenceSignals{Overall: 0.5}
	if ShouldTriggerReflexion(medium, thresholds) {
		t.Error("expected no reflexion for medium confidence")
	}
}

func TestConfidenceLevel(t *testing.T) {
	thresholds := DefaultConfidenceThresholds()

	tests := []struct {
		overall float64
		want    string
	}{
		{0.90, "high"},
		{0.50, "medium"},
		{0.20, "low"},
		{0.85, "high"},
		{0.30, "medium"}, // 0.30 is not < 0.30, so medium
		{0.29, "low"},
	}

	for _, tt := range tests {
		sig := ConfidenceSignals{Overall: tt.overall}
		got := ConfidenceLevel(sig, thresholds)
		if got != tt.want {
			t.Errorf("ConfidenceLevel(overall=%f) = %q, want %q", tt.overall, got, tt.want)
		}
	}
}

func TestHedgeWordCounting(t *testing.T) {
	s := &Session{
		TurnCount:     3,
		OutputHistory: []string{"This might work, maybe it will, perhaps not."},
	}

	sig := ExtractConfidence(s, 3, nil)

	if sig.HedgeCount != 3 {
		t.Errorf("HedgeCount = %d, want 3", sig.HedgeCount)
	}

	// Clean output
	s2 := &Session{
		TurnCount:     3,
		OutputHistory: []string{"All tasks completed successfully."},
	}

	sig2 := ExtractConfidence(s2, 3, nil)

	if sig2.HedgeCount != 0 {
		t.Errorf("HedgeCount = %d, want 0", sig2.HedgeCount)
	}
}
