package awesome

import "testing"

func TestRateValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		stars   int
		lang    string
		matches int
		want    Rating
	}{
		{"high: 3+ matches, Go", 200, "Go", 5, RatingHigh},
		{"high: 3+ matches, >100 stars", 200, "Rust", 4, RatingHigh},
		{"medium: 1-2 matches", 50, "Python", 2, RatingMedium},
		{"medium: >500 stars, Go", 600, "Go", 0, RatingMedium},
		{"low: Go, 0 matches", 30, "Go", 0, RatingLow},
		{"none: 0 matches, non-Go, few stars", 5, "JavaScript", 0, RatingNone},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			got := rateValue(tc.stars, tc.lang, tc.matches)
			if got != tc.want {
				t.Errorf("rateValue(%d, %q, %d) = %q, want %q", tc.stars, tc.lang, tc.matches, got, tc.want)
			}
		})
	}
}

func TestRateComplexity(t *testing.T) {
	t.Parallel()
	got := rateComplexity("Go", "this is an mcp server with hooks")
	if got != "drop-in" {
		t.Errorf("expected drop-in for Go MCP server, got %q", got)
	}

	got = rateComplexity("Rust", "a complex system")
	if got != "moderate" {
		t.Errorf("expected moderate for Rust, got %q", got)
	}
}

func TestBuildRationale(t *testing.T) {
	t.Parallel()
	ae := AnalysisEntry{
		Stars:             100,
		Language:          "Go",
		CapabilityMatches: 3,
	}
	r := buildRationale(ae)
	if r == "" {
		t.Error("expected non-empty rationale")
	}
}
