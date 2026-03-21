package views

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestRenderSessionDetailIncludesProviderMetadata(t *testing.T) {
	now := time.Now()
	view := RenderSessionDetail(&session.Session{
		ID:                "session-123456",
		Provider:          session.ProviderCodex,
		ProviderSessionID: "provider-789",
		RepoName:          "repo",
		RepoPath:          "/tmp/repo",
		Status:            session.StatusRunning,
		Model:             "gpt-5.4-xhigh",
		LastActivity:      now,
		LaunchedAt:        now.Add(-time.Minute),
		LastEventType:     "assistant",
		StreamParseErrors: 2,
		OutputHistory:     []string{"line 1", "line 2"},
	}, 100, 40)

	for _, want := range []string{"Provider ID:", "provider-789", "Last Event:", "assistant", "Parse Errors:", "Output History"} {
		if !strings.Contains(view, want) {
			t.Fatalf("RenderSessionDetail missing %q\n%s", want, view)
		}
	}
}

func TestRenderTeamDetailIncludesNavigationHints(t *testing.T) {
	view := RenderTeamDetail(&session.TeamStatus{
		Name:      "alpha-team",
		RepoPath:  "/tmp/repo",
		LeadID:    "lead-123",
		Status:    session.StatusRunning,
		CreatedAt: time.Now(),
		Tasks: []session.TeamTask{
			{Description: "ship it", Status: "pending"},
		},
	}, &session.Session{
		ID:       "lead-123",
		Provider: session.ProviderClaude,
		Status:   session.StatusRunning,
		Model:    "sonnet",
	}, 100)

	for _, want := range []string{"Lead Session", "Enter: lead session", "timeline"} {
		if !strings.Contains(strings.ToLower(view), strings.ToLower(want)) {
			t.Fatalf("RenderTeamDetail missing %q\n%s", want, view)
		}
	}
}

func TestRenderFleetDashboardShowsSelectionAndWindow(t *testing.T) {
	now := time.Now()
	view := RenderFleetDashboard(FleetData{
		TotalRepos:      1,
		TotalSessions:   1,
		RunningSessions: 1,
		TotalSpendUSD:   2.5,
		TotalTurns:      4,
		Providers:       map[string]ProviderStat{"codex": {Sessions: 1, Running: 1, SpendUSD: 2.5}},
		Repos:           []*model.Repo{{Name: "repo1", Path: "/tmp/repo1"}},
		Sessions:        []*session.Session{{ID: "session-123456", Provider: session.ProviderCodex, RepoName: "repo1", Status: session.StatusRunning, SpentUSD: 2.5, LaunchedAt: now}},
		Teams:           []*session.TeamStatus{{Name: "team-a", Status: session.StatusRunning, Tasks: []session.TeamTask{{Description: "a"}}}},
		CostHistory:     []float64{1, 2, 3},
		CostWindowLabel: "1h",
		SelectedSection: "sessions",
		SelectedCursor:  0,
		CostPerTurn:     map[string]float64{"codex": 0.625},
		TopExpensive:    []ExpensiveSession{{ID: "session-123456", Provider: "codex", RepoName: "repo1", SpendUSD: 2.5, Status: "running"}},
	}, 140, 40)

	for _, want := range []string{"Cost Trend (1h)", "Teams", "Tab/", "session-"} {
		if !strings.Contains(view, want) {
			t.Fatalf("RenderFleetDashboard missing %q\n%s", want, view)
		}
	}
}
