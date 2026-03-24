package session

import (
	"math"
	"strings"
)

// ConfidenceSignals captures multiple indicators of session output quality.
type ConfidenceSignals struct {
	Overall        float64 `json:"overall"`         // 0.0-1.0
	TurnEfficiency float64 `json:"turn_efficiency"` // actual / expected turns (1.0 = exactly as expected)
	ErrorFree      bool    `json:"error_free"`
	VerifyPassed   bool    `json:"verify_passed"`
	OutputLength   int     `json:"output_length"`  // total chars of output
	HedgeCount     int     `json:"hedge_count"`    // hedging language count
	QuestionCount  int     `json:"question_count"` // questions in output
}

// ConfidenceThresholds configures high/low confidence cutoffs.
type ConfidenceThresholds struct {
	HighConfidence float64 `json:"high_confidence"` // above this = high confidence (default 0.85)
	LowConfidence  float64 `json:"low_confidence"`  // below this = low confidence (default 0.3)
	SkipVerifyMin  float64 `json:"skip_verify_min"` // minimum to skip verification (default 0.95)
}

// hedgeWords are phrases indicating uncertainty in output.
var hedgeWords = []string{
	"might",
	"maybe",
	"could be",
	"possibly",
	"not sure",
	"uncertain",
	"i think",
	"perhaps",
}

// DefaultConfidenceThresholds returns sensible defaults for confidence classification.
func DefaultConfidenceThresholds() ConfidenceThresholds {
	return ConfidenceThresholds{
		HighConfidence: 0.85,
		LowConfidence:  0.3,
		SkipVerifyMin:  0.95,
	}
}

// ExtractConfidence computes confidence signals from a session's state.
// expectedTurns is the number of turns the task was expected to take.
// verification is the list of verification results from the loop iteration.
func ExtractConfidence(s *Session, expectedTurns int, verification []LoopVerification) ConfidenceSignals {
	var sig ConfidenceSignals

	// Turn efficiency
	if expectedTurns > 0 {
		sig.TurnEfficiency = 1.0 - math.Abs(float64(s.TurnCount)-float64(expectedTurns))/float64(expectedTurns)
		if sig.TurnEfficiency < 0 {
			sig.TurnEfficiency = 0
		}
		if sig.TurnEfficiency > 1 {
			sig.TurnEfficiency = 1
		}
	} else {
		sig.TurnEfficiency = 0.5
	}

	// Error free
	sig.ErrorFree = s.Error == ""

	// Verify passed
	if len(verification) > 0 {
		sig.VerifyPassed = true
		for _, v := range verification {
			if v.Status != "passed" && v.ExitCode != 0 {
				sig.VerifyPassed = false
				break
			}
		}
	}

	// Output length and joined output
	var totalLen int
	var joined strings.Builder
	for i, o := range s.OutputHistory {
		totalLen += len(o)
		if i > 0 {
			joined.WriteByte('\n')
		}
		joined.WriteString(o)
	}
	sig.OutputLength = totalLen
	output := joined.String()

	// Hedge count
	lower := strings.ToLower(output)
	for _, word := range hedgeWords {
		sig.HedgeCount += strings.Count(lower, word)
	}

	// Question count — use existing DetectQuestions
	_, sig.QuestionCount = DetectQuestions(output)

	// Overall weighted score
	verifyScore := 0.0
	if sig.VerifyPassed {
		verifyScore = 1.0
	}
	errorScore := 0.0
	if sig.ErrorFree {
		errorScore = 1.0
	}
	hedgeNorm := math.Min(float64(sig.HedgeCount)/10.0, 1.0)
	questionNorm := math.Min(float64(sig.QuestionCount)/5.0, 1.0)

	sig.Overall = 0.30*verifyScore + 0.25*(1-hedgeNorm) + 0.20*sig.TurnEfficiency + 0.15*errorScore + 0.10*(1-questionNorm)

	// Clamp
	if sig.Overall < 0 {
		sig.Overall = 0
	}
	if sig.Overall > 1 {
		sig.Overall = 1
	}

	return sig
}

// ShouldSkipVerification returns true when confidence is high enough to skip
// the verification step in a loop iteration.
func ShouldSkipVerification(signals ConfidenceSignals, thresholds ConfidenceThresholds) bool {
	return signals.Overall >= thresholds.SkipVerifyMin && signals.ErrorFree && signals.HedgeCount == 0
}

// ShouldTriggerReflexion returns true when confidence is low enough that the
// session should enter a reflexion/retry cycle.
func ShouldTriggerReflexion(signals ConfidenceSignals, thresholds ConfidenceThresholds) bool {
	return signals.Overall < thresholds.LowConfidence
}

// ConfidenceLevel returns a human-readable classification: "high", "medium", or "low".
func ConfidenceLevel(signals ConfidenceSignals, thresholds ConfidenceThresholds) string {
	if signals.Overall >= thresholds.HighConfidence {
		return "high"
	}
	if signals.Overall < thresholds.LowConfidence {
		return "low"
	}
	return "medium"
}
