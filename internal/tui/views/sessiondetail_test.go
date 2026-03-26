package views

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestRenderSessionDetailNil(t *testing.T) {
	out := RenderSessionDetail(nil, 80, 40)
	if !strings.Contains(out, "No session selected") {
		t.Error("nil session should show 'No session selected'")
	}
}

func TestRenderSessionDetailFull(t *testing.T) {
	now := time.Now()
	s := &session.Session{
		ID:                "session-1234567890",
		Provider:          session.ProviderClaude,
		ProviderSessionID: "prov-abc",
		RepoName:          "test-repo",
		RepoPath:          "/tmp/test-repo",
		Status:            session.StatusRunning,
		Model:             "sonnet-4",
		AgentName:         "planner",
		TeamName:          "alpha",
		SpentUSD:          3.50,
		BudgetUSD:         10.0,
		TurnCount:         15,
		MaxTurns:          100,
		LaunchedAt:        now.Add(-5 * time.Minute),
		LastActivity:      now.Add(-10 * time.Second),
		LastEventType:     "assistant",
		StreamParseErrors: 1,
		CostHistory:       []float64{0.1, 0.2, 0.3, 0.5},
		OutputHistory:     []string{"line 1", "line 2"},
	}

	out := RenderSessionDetail(s, 120, 40)

	checks := []string{
		"session-12",  // truncated in title
		"Session Info",
		"claude",
		"prov-abc",
		"test-repo",
		"sonnet-4",
		"planner",
		"alpha",
		"Cost",
		"$3.50",
		"15/100",
		"$/turn",
		"Output History",
		"line 1",
		"Last Event",
		"assistant",
		"Parse Errors",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderSessionDetailWithError(t *testing.T) {
	now := time.Now()
	s := &session.Session{
		ID:         "err-sess-1",
		Provider:   session.ProviderGemini,
		RepoName:   "broken-repo",
		Status:     session.StatusErrored,
		Error:      "process exited with code 1",
		ExitReason: "error",
		LaunchedAt: now.Add(-time.Minute),
	}

	out := RenderSessionDetail(s, 80, 40)
	if !strings.Contains(out, "process exited with code 1") {
		t.Error("should show error message")
	}
	if !strings.Contains(out, "error") {
		t.Error("should show exit reason")
	}
}

func TestRenderSessionDetailMinimalFields(t *testing.T) {
	s := &session.Session{
		ID:         "minimal",
		Provider:   session.ProviderCodex,
		RepoName:   "repo",
		Status:     session.StatusStopped,
		LaunchedAt: time.Now(),
	}

	out := RenderSessionDetail(s, 80, 40)
	if !strings.Contains(out, "codex") {
		t.Error("should show provider")
	}
	// Should not contain budget section details since BudgetUSD is 0
	if strings.Contains(out, "Utilization") {
		t.Error("should not show budget utilization when budget is 0")
	}
}

func TestRenderBudgetBar(t *testing.T) {
	// Test various percentages don't panic
	for _, pct := range []float64{0, 25, 50, 75, 90, 100, 150} {
		bar := renderBudgetBar(pct, 30)
		if bar == "" {
			t.Errorf("renderBudgetBar(%.0f, 30) returned empty", pct)
		}
	}
}

func TestFormatStaleness(t *testing.T) {
	tests := []struct {
		dur    time.Duration
		expect string
	}{
		{30 * time.Second, "30s"},
		{90 * time.Second, "1m30s"},
	}
	for _, tt := range tests {
		got := formatStaleness(tt.dur)
		if got != tt.expect {
			t.Errorf("formatStaleness(%v) = %q, want %q", tt.dur, got, tt.expect)
		}
	}
}
