package session

import (
	"testing"
	"time"
)

func TestSanitizeLoopName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"  My Loop  ", "my-loop"},
		{"my_loop_name", "my-loop-name"},
		{"UPPER CASE!", "upper-case"},
		{"", "loop"},
		{"  ", "loop"},
		{"special@chars#here", "special-chars-here"},
		{"---leading-trailing---", "leading-trailing"},
	}
	for _, tt := range tests {
		if got := sanitizeLoopName(tt.input); got != tt.want {
			t.Errorf("sanitizeLoopName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestTruncateForPrompt(t *testing.T) {
	tests := []struct {
		input string
		limit int
		want  string
	}{
		{"short", 100, "short"},
		{"hello world", 5, "he..."},
		{"abc", 0, "abc"},
		{"  padded  ", 100, "padded"},
		{"exactly10!", 10, "exactly10!"},
	}
	for _, tt := range tests {
		if got := truncateForPrompt(tt.input, tt.limit); got != tt.want {
			t.Errorf("truncateForPrompt(%q, %d) = %q, want %q", tt.input, tt.limit, got, tt.want)
		}
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"first\nsecond\nthird", "first"},
		{"only line", "only line"},
		{"", ""},
		{"\n\nthird", "third"},
	}
	for _, tt := range tests {
		if got := firstLine(tt.input); got != tt.want {
			t.Errorf("firstLine(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestJoinOrPlaceholder(t *testing.T) {
	if got := joinOrPlaceholder(nil, "default"); got != "default" {
		t.Errorf("nil items: got %q, want default", got)
	}
	if got := joinOrPlaceholder([]string{}, "default"); got != "default" {
		t.Errorf("empty items: got %q, want default", got)
	}
	if got := joinOrPlaceholder([]string{"a", "b"}, "default"); got != "a\nb" {
		t.Errorf("with items: got %q, want a\\nb", got)
	}
}

func TestFirstNonBlank(t *testing.T) {
	if got := firstNonBlank("", "  ", "hello"); got != "hello" {
		t.Errorf("got %q, want hello", got)
	}
	if got := firstNonBlank("", ""); got != "" {
		t.Errorf("all blank: got %q, want empty", got)
	}
	if got := firstNonBlank("first", "second"); got != "first" {
		t.Errorf("got %q, want first", got)
	}
}

func TestConsecutiveLoopFailures(t *testing.T) {
	iters := []LoopIteration{
		{Status: "completed"},
		{Status: "failed"},
		{Status: "failed"},
		{Status: "failed"},
	}
	if got := consecutiveLoopFailures(iters); got != 3 {
		t.Errorf("got %d, want 3", got)
	}

	// No failures
	iters2 := []LoopIteration{{Status: "completed"}}
	if got := consecutiveLoopFailures(iters2); got != 0 {
		t.Errorf("got %d, want 0", got)
	}

	// Empty
	if got := consecutiveLoopFailures(nil); got != 0 {
		t.Errorf("got %d, want 0", got)
	}
}

func TestMapProvider(t *testing.T) {
	tests := []struct {
		input Provider
		want  string
	}{
		{ProviderGemini, "gemini"},
		{ProviderCodex, "openai"},
		{ProviderClaude, "claude"},
		{"", "claude"},
	}
	for _, tt := range tests {
		got := mapProvider(tt.input)
		if string(got) != tt.want {
			t.Errorf("mapProvider(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSessionOutputSummary(t *testing.T) {
	s := &Session{
		OutputHistory: []string{"line1", "line2"},
		LastOutput:    "last",
		Error:         "err",
	}
	got := sessionOutputSummary(s)
	if got == "" {
		t.Error("expected non-empty summary")
	}
}

func TestSessionOutputSummary_Empty(t *testing.T) {
	s := &Session{}
	got := sessionOutputSummary(s)
	if got != "" {
		t.Errorf("expected empty summary, got %q", got)
	}
}

func TestLooksLikeJSON_EdgeCases(t *testing.T) {
	// Supplement existing TestLooksLikeJSON with edge cases.
	tests := []struct {
		input string
		want  bool
	}{
		{`{partial`, true},                     // only checks first char
		{`hello {"embedded": true}`, false},    // non-JSON prefix
		{"\t{\"tabbed\": true}", true},         // tab whitespace
		{"\n[1]", true},                        // newline whitespace
	}
	for _, tt := range tests {
		if got := looksLikeJSON(tt.input); got != tt.want {
			t.Errorf("looksLikeJSON(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestDedupeStrings_Ordering(t *testing.T) {
	// Supplement existing TestDedupeStrings with ordering validation.
	got := dedupeStrings([]string{"c", "b", "a", "b", "c"})
	want := []string{"c", "b", "a"}
	if len(got) != len(want) {
		t.Fatalf("dedupeStrings length = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("dedupeStrings[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNonEmptyLines(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"a\nb\nc", 3},
		{"\n\n\n", 0},
		{"  a  \n\n  b  ", 2},
		{"", 0},
	}
	for _, tt := range tests {
		got := nonEmptyLines(tt.input)
		if len(got) != tt.want {
			t.Errorf("nonEmptyLines(%q) = %d lines, want %d", tt.input, len(got), tt.want)
		}
	}
}

func TestSummarizeObservations_Empty(t *testing.T) {
	summary := SummarizeObservations(nil)
	if summary.TotalIterations != 0 {
		t.Errorf("total iterations = %d, want 0", summary.TotalIterations)
	}
}

func TestSummarizeObservations_WithData(t *testing.T) {
	now := time.Now()
	obs := []LoopObservation{
		{
			Timestamp:        now,
			TotalLatencyMs:   1000,
			TotalCostUSD:     0.50,
			WorkerProvider:   "claude",
			WorkerModelUsed:  "sonnet",
			PlannerModelUsed: "opus",
			AcceptancePath:   "auto_merge",
			Status:           "idle",
			GitDiffStat:      &DiffStat{FilesChanged: 2, Insertions: 10, Deletions: 3},
		},
		{
			Timestamp:        now,
			TotalLatencyMs:   2000,
			TotalCostUSD:     0.75,
			WorkerProvider:   "gemini",
			WorkerModelUsed:  "gemini-pro",
			PlannerModelUsed: "opus",
			AcceptancePath:   "pr",
			Status:           "idle",
			GitDiffStat:      &DiffStat{FilesChanged: 1, Insertions: 5, Deletions: 2},
		},
		{
			Timestamp:      now,
			TotalLatencyMs: 3000,
			TotalCostUSD:   1.00,
			WorkerProvider: "claude",
			Status:         "failed",
			Error:          "verify failed",
		},
		{
			Timestamp:      now,
			TotalLatencyMs: 4000,
			TotalCostUSD:   0.60,
			WorkerProvider: "claude",
			AcceptancePath: "auto_merge",
			Status:         "idle",
		},
		{
			Timestamp:      now,
			TotalLatencyMs: 5000,
			TotalCostUSD:   0.80,
			WorkerProvider: "claude",
			Status:         "idle",
		},
	}

	summary := SummarizeObservations(obs)

	if summary.TotalIterations != 5 {
		t.Errorf("total iterations = %d, want 5", summary.TotalIterations)
	}

	// 4 completed (idle), 1 failed
	if summary.CompletedCount != 4 {
		t.Errorf("completed = %d, want 4", summary.CompletedCount)
	}
	if summary.FailedCount != 1 {
		t.Errorf("failed = %d, want 1", summary.FailedCount)
	}

	// Acceptance paths
	if summary.AcceptanceCounts["auto_merge"] != 2 {
		t.Errorf("auto_merge count = %d, want 2", summary.AcceptanceCounts["auto_merge"])
	}

	// Git diff aggregation
	if summary.TotalFilesChanged != 3 {
		t.Errorf("files changed = %d, want 3", summary.TotalFilesChanged)
	}
	if summary.TotalInsertions != 15 {
		t.Errorf("insertions = %d, want 15", summary.TotalInsertions)
	}

	// Model usage
	if summary.ModelUsage["opus"] != 2 {
		t.Errorf("opus usage = %d, want 2", summary.ModelUsage["opus"])
	}

	// Latency P50 should be the median (~3s for 5 sorted values)
	if summary.LatencyP50 < 2.5 || summary.LatencyP50 > 3.5 {
		t.Errorf("latency p50 = %.1f, want ~3.0", summary.LatencyP50)
	}
}

func TestEnrichObservationSummary_Empty(t *testing.T) {
	enriched := EnrichObservationSummary(nil)
	if enriched.TotalCostUSD != 0 {
		t.Errorf("cost = %f, want 0", enriched.TotalCostUSD)
	}
	if enriched.SuccessRate != 0 {
		t.Errorf("success rate = %f, want 0", enriched.SuccessRate)
	}
}

func TestEnrichObservationSummary_WithData(t *testing.T) {
	now := time.Now()
	obs := []LoopObservation{
		{Timestamp: now, TotalLatencyMs: 1000, TotalCostUSD: 0.50, WorkerProvider: "claude", Status: "idle"},
		{Timestamp: now, TotalLatencyMs: 2000, TotalCostUSD: 0.75, WorkerProvider: "gemini", Status: "idle"},
		{Timestamp: now, TotalLatencyMs: 3000, TotalCostUSD: 1.00, WorkerProvider: "claude", Status: "failed", Error: "err"},
	}

	enriched := EnrichObservationSummary(obs)

	expectedCost := 0.50 + 0.75 + 1.00
	if enriched.TotalCostUSD < expectedCost-0.01 || enriched.TotalCostUSD > expectedCost+0.01 {
		t.Errorf("total cost = %.2f, want %.2f", enriched.TotalCostUSD, expectedCost)
	}

	// 2 out of 3 succeeded (one has error)
	if enriched.SuccessRate < 0.66 || enriched.SuccessRate > 0.67 {
		t.Errorf("success rate = %.2f, want ~0.67", enriched.SuccessRate)
	}

	// Provider breakdown
	if enriched.ProviderBreakdown["claude"] != 2 {
		t.Errorf("claude count = %d, want 2", enriched.ProviderBreakdown["claude"])
	}
	if enriched.ProviderBreakdown["gemini"] != 1 {
		t.Errorf("gemini count = %d, want 1", enriched.ProviderBreakdown["gemini"])
	}

	// Base summary should also be populated
	if enriched.TotalIterations != 3 {
		t.Errorf("total iterations = %d, want 3", enriched.TotalIterations)
	}
}
