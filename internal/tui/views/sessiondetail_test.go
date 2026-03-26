package views

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestRenderSessionDetail_Nil(t *testing.T) {
	out := RenderSessionDetail(nil, 100, 40)
	if !strings.Contains(out, "No session selected") {
		t.Errorf("nil session: expected 'No session selected', got: %q", out)
	}
}

func TestRenderSessionDetail_BasicFields(t *testing.T) {
	now := time.Now()
	s := &session.Session{
		ID:         "abcdef1234567890",
		Provider:   session.ProviderClaude,
		RepoName:   "test-repo",
		RepoPath:   "/home/user/test-repo",
		Status:     session.StatusRunning,
		Model:      "opus-4",
		LaunchedAt: now.Add(-10 * time.Minute),
		SpentUSD:   3.50,
		BudgetUSD:  10.00,
		TurnCount:  15,
		MaxTurns:   50,
	}

	out := RenderSessionDetail(s, 120, 50)

	checks := []string{
		"abcdef1234567890",
		"claude",
		"test-repo",
		"/home/user/test-repo",
		"opus-4",
		"Session Info",
		"Cost",
		"$3.50",
		"15",
		"Esc: back",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderSessionDetail_WithError(t *testing.T) {
	s := &session.Session{
		ID:         "err-sess",
		Provider:   session.ProviderGemini,
		Status:     session.StatusErrored,
		Error:      "rate limit exceeded",
		LaunchedAt: time.Now(),
	}

	out := RenderSessionDetail(s, 100, 40)
	if !strings.Contains(out, "rate limit exceeded") {
		t.Error("should show error message")
	}
	if !strings.Contains(out, "Error") {
		t.Error("should show Error section header")
	}
}

func TestRenderSessionDetail_WithExitReason(t *testing.T) {
	s := &session.Session{
		ID:         "exit-sess",
		Provider:   session.ProviderCodex,
		Status:     session.StatusStopped,
		ExitReason: "budget exhausted",
		LaunchedAt: time.Now(),
	}

	out := RenderSessionDetail(s, 100, 40)
	if !strings.Contains(out, "budget exhausted") {
		t.Error("should show exit reason")
	}
}

func TestRenderSessionDetail_WithAgent(t *testing.T) {
	s := &session.Session{
		ID:         "agent-ses",
		Provider:   session.ProviderClaude,
		Status:     session.StatusRunning,
		AgentName:  "coder",
		TeamName:   "backend",
		LaunchedAt: time.Now(),
	}

	out := RenderSessionDetail(s, 100, 40)
	if !strings.Contains(out, "coder") {
		t.Error("should show agent name")
	}
	if !strings.Contains(out, "backend") {
		t.Error("should show team name")
	}
}

func TestRenderSessionDetail_WithOutputHistory(t *testing.T) {
	s := &session.Session{
		ID:            "hist-sess",
		Provider:      session.ProviderClaude,
		Status:        session.StatusRunning,
		OutputHistory: []string{"line one", "line two", "line three"},
		LaunchedAt:    time.Now(),
	}

	out := RenderSessionDetail(s, 100, 50)
	if !strings.Contains(out, "Output History") {
		t.Error("should show Output History header")
	}
	if !strings.Contains(out, "line one") {
		t.Error("should show output history lines")
	}
}

func TestRenderSessionDetail_LastOutputFallback(t *testing.T) {
	s := &session.Session{
		ID:         "last-sess",
		Provider:   session.ProviderClaude,
		Status:     session.StatusCompleted,
		LastOutput: "final output line",
		LaunchedAt: time.Now(),
	}

	out := RenderSessionDetail(s, 100, 50)
	if !strings.Contains(out, "final output line") {
		t.Error("should show last output when output history is empty")
	}
}

func TestRenderSessionDetail_CostPerTurn(t *testing.T) {
	s := &session.Session{
		ID:         "cpt-sess",
		Provider:   session.ProviderClaude,
		Status:     session.StatusRunning,
		SpentUSD:   5.0,
		TurnCount:  10,
		LaunchedAt: time.Now(),
	}

	out := RenderSessionDetail(s, 100, 40)
	if !strings.Contains(out, "$/turn") {
		t.Error("should show cost per turn")
	}
}

func TestRenderSessionDetail_CostSparkline(t *testing.T) {
	s := &session.Session{
		ID:          "spark-ses",
		Provider:    session.ProviderClaude,
		Status:      session.StatusRunning,
		CostHistory: []float64{0.1, 0.2, 0.15, 0.3, 0.25},
		LaunchedAt:  time.Now(),
	}

	out := RenderSessionDetail(s, 100, 40)
	if !strings.Contains(out, "Cost trend") {
		t.Error("should show cost trend sparkline")
	}
}

func TestRenderSessionDetail_ParseErrors(t *testing.T) {
	s := &session.Session{
		ID:                "parse-ses",
		Provider:          session.ProviderCodex,
		Status:            session.StatusRunning,
		StreamParseErrors: 5,
		LaunchedAt:        time.Now(),
	}

	out := RenderSessionDetail(s, 100, 40)
	if !strings.Contains(out, "Parse Errors") {
		t.Error("should show parse errors count")
	}
}

func TestRenderSessionDetail_ProviderSessionID(t *testing.T) {
	s := &session.Session{
		ID:                "prov-sess",
		Provider:          session.ProviderCodex,
		ProviderSessionID: "ext-provider-456",
		Status:            session.StatusRunning,
		LaunchedAt:        time.Now(),
	}

	out := RenderSessionDetail(s, 100, 40)
	if !strings.Contains(out, "Provider ID") {
		t.Error("should show Provider ID label")
	}
	if !strings.Contains(out, "ext-provider-456") {
		t.Error("should show provider session ID value")
	}
}

func TestRenderSessionDetail_NoBudget(t *testing.T) {
	s := &session.Session{
		ID:        "nobudget",
		Provider:  session.ProviderClaude,
		Status:    session.StatusRunning,
		SpentUSD:  1.25,
		BudgetUSD: 0,
		LaunchedAt: time.Now(),
	}

	out := RenderSessionDetail(s, 100, 40)
	if !strings.Contains(out, "$1.25") {
		t.Error("should show spend amount even without budget")
	}
}
