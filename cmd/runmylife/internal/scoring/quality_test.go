package scoring

import (
	"strings"
	"testing"
)

func TestScoreText_ShortProfessional(t *testing.T) {
	text := "Hi Alex, the Q3 report is attached. Revenue grew 12% from $4.2M. Please review Section 3 by Friday."
	s := ScoreText(text)
	if s.Overall < 60 {
		t.Errorf("short professional text Overall = %.1f, want >= 60", s.Overall)
	}
	if s.Professionalism < 70 {
		t.Errorf("Professionalism = %.1f, want >= 70", s.Professionalism)
	}
	if s.Specificity < 60 {
		t.Errorf("Specificity = %.1f, want >= 60 (has numbers/names)", s.Specificity)
	}
}

func TestScoreText_FillerLaden(t *testing.T) {
	// Use actual filler phrases from scoreConciseness
	text := "In order to move forward, at the end of the day it goes without saying that " +
		"due to the fact that in the event that we proceed, as previously mentioned " +
		"it should be noted that needless to say with regard to this matter " +
		"in terms of results for what it's worth at this point in time we should act."
	s := ScoreText(text)
	if s.Conciseness > 60 {
		t.Errorf("filler-laden text Conciseness = %.1f, want <= 60", s.Conciseness)
	}
}

func TestScoreText_AllCaps(t *testing.T) {
	text := "THIS IS A VERY IMPORTANT MESSAGE PLEASE READ IMMEDIATELY"
	s := ScoreText(text)
	if s.Professionalism > 70 {
		t.Errorf("all-caps text Professionalism = %.1f, want <= 70", s.Professionalism)
	}
}

func TestScoreText_Empty(t *testing.T) {
	s := ScoreText("")
	if s == nil {
		t.Fatal("ScoreText should return non-nil for empty string")
	}
	if s.Overall != 0 {
		t.Errorf("empty text Overall = %.1f, want 0", s.Overall)
	}
}

func TestScoreText_QuestionEngagement(t *testing.T) {
	text := "What do you think about the new design? Could we meet Tuesday to discuss the timeline?"
	s := ScoreText(text)
	if s.Engagement < 60 {
		t.Errorf("question-heavy text Engagement = %.1f, want >= 60", s.Engagement)
	}
}

func TestScoreText_SpecificityWithNumbers(t *testing.T) {
	text := "We processed 1,234 orders on March 15. Server latency was 42ms. Revenue hit $89,000."
	s := ScoreText(text)
	if s.Specificity < 70 {
		t.Errorf("number-heavy text Specificity = %.1f, want >= 70", s.Specificity)
	}
}

func TestFormatScore(t *testing.T) {
	s := &QualityScore{
		Overall:         75.0,
		Clarity:         80,
		Conciseness:     70,
		Professionalism: 85,
		Specificity:     65,
		Engagement:      72,
		SlopScore:       5.0,
		Suggestions:     []string{"Be more specific"},
	}
	out := FormatScore(s)
	// FormatScore uses %.0f so 75.0 → "75"
	if !strings.Contains(out, "75") {
		t.Error("FormatScore should contain overall score")
	}
	if !strings.Contains(out, "Clarity") {
		t.Error("FormatScore should contain dimension names")
	}
	if !strings.Contains(out, "Be more specific") {
		t.Error("FormatScore should contain suggestions")
	}
}

func TestFormatScore_SlopMatches(t *testing.T) {
	s := &QualityScore{
		SlopMatches: []string{"delve into", "tapestry of"},
	}
	out := FormatScore(s)
	if !strings.Contains(out, "delve into") {
		t.Error("FormatScore should list slop matches")
	}
}

// Slop detection tests

func TestScoreSlop_Clean(t *testing.T) {
	text := "The migration finished at 14:32 UTC. Three rows failed validation on the phone_number column."
	score, matches := ScoreSlop(text)
	if score > 10 {
		t.Errorf("clean text slop score = %.1f, want <= 10; matches: %v", score, matches)
	}
}

func TestScoreSlop_HeavySlop(t *testing.T) {
	text := "I hope this email finds you well! I wanted to reach out because delve into this matter. " +
		"At the end of the day, it's important to note that we need to leverage synergies. " +
		"Let's circle back on this and navigate the holistic approach going forward."
	score, matches := ScoreSlop(text)
	if score < 20 {
		t.Errorf("slop-heavy text score = %.1f, want >= 20; matches: %v", score, matches)
	}
	if len(matches) < 3 {
		t.Errorf("expected >= 3 matches, got %d: %v", len(matches), matches)
	}
}

func TestScoreSlop_DensityScaling(t *testing.T) {
	short := "I hope this email finds you well. I wanted to reach out."
	long := "I hope this email finds you well. " + strings.Repeat("Normal professional content here. ", 50) + " I wanted to reach out."

	shortScore, _ := ScoreSlop(short)
	longScore, _ := ScoreSlop(long)

	if shortScore == 0 {
		t.Fatal("short text should detect slop patterns")
	}
	// Same patterns in much longer text → lower density → lower score
	if longScore >= shortScore {
		t.Errorf("density scaling: long score %.1f should be < short score %.1f", longScore, shortScore)
	}
}

func TestScoreSlop_CaseInsensitive(t *testing.T) {
	lower := "i hope this email finds you well"
	upper := "I Hope This Email Finds You Well"

	scoreLower, matchesLower := ScoreSlop(lower)
	scoreUpper, matchesUpper := ScoreSlop(upper)

	if scoreLower == 0 || len(matchesLower) == 0 {
		t.Error("lowercase slop not detected")
	}
	if scoreUpper == 0 || len(matchesUpper) == 0 {
		t.Error("titlecase slop not detected")
	}
}

func TestScoreSlop_AllPatterns_Compile(t *testing.T) {
	// Regression guard: ensure we have a substantial pattern set
	if len(slopPatterns) < 100 {
		t.Errorf("expected >= 100 slop patterns, got %d", len(slopPatterns))
	}
}

func TestScoreSlop_EmptyText(t *testing.T) {
	score, matches := ScoreSlop("")
	if score != 0 {
		t.Errorf("empty text slop score = %.1f, want 0", score)
	}
	if len(matches) != 0 {
		t.Errorf("empty text matches = %v, want nil", matches)
	}
}
