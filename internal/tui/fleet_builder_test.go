package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestCountAlertsNoRepos(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	if got := m.countAlerts(); got != 0 {
		t.Errorf("countAlerts() = %d, want 0", got)
	}
}

func TestCountAlertsOpenCircuits(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{
		{Name: "healthy", Path: "/tmp/healthy"},
		{Name: "broken", Path: "/tmp/broken", Circuit: &model.CircuitBreakerState{State: "OPEN"}},
		{Name: "closed", Path: "/tmp/closed", Circuit: &model.CircuitBreakerState{State: "CLOSED"}},
		{Name: "broken2", Path: "/tmp/broken2", Circuit: &model.CircuitBreakerState{State: "OPEN"}},
	}
	if got := m.countAlerts(); got != 2 {
		t.Errorf("countAlerts() = %d, want 2", got)
	}
}

func TestCountAlertsErroredSessions(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	s1 := &session.Session{ID: "s1", Status: session.StatusErrored, Provider: "claude"}
	s2 := &session.Session{ID: "s2", Status: session.StatusRunning, Provider: "claude"}
	s3 := &session.Session{ID: "s3", Status: session.StatusErrored, Provider: "gemini"}
	mgr.AddSessionForTesting(s1)
	mgr.AddSessionForTesting(s2)
	mgr.AddSessionForTesting(s3)
	m.SessMgr = mgr

	if got := m.countAlerts(); got != 2 {
		t.Errorf("countAlerts() = %d, want 2 (2 errored sessions)", got)
	}
}

func TestCountAlertsCombined(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{
		{Name: "broken", Path: "/tmp/broken", Circuit: &model.CircuitBreakerState{State: "OPEN"}},
	}
	mgr := session.NewManager()
	s1 := &session.Session{ID: "s1", Status: session.StatusErrored, Provider: "claude"}
	mgr.AddSessionForTesting(s1)
	m.SessMgr = mgr

	if got := m.countAlerts(); got != 2 {
		t.Errorf("countAlerts() = %d, want 2 (1 circuit + 1 errored)", got)
	}
}

func TestClampFleetWindow(t *testing.T) {
	tests := []struct {
		name   string
		window int
		want   int
	}{
		{"zero", 0, 0},
		{"valid middle", 2, 2},
		{"valid last", len(fleetWindows) - 1, len(fleetWindows) - 1},
		{"negative", -1, 0},
		{"too large", len(fleetWindows), 0},
		{"way too large", 100, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewModel("/tmp/test", nil)
			m.Fleet.Window = tt.window
			if got := m.clampFleetWindow(); got != tt.want {
				t.Errorf("clampFleetWindow() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestSelectedFleetSection(t *testing.T) {
	tests := []struct {
		section int
		want    string
	}{
		{0, "repos"},
		{1, "sessions"},
		{2, "teams"},
		{-1, "repos"},
		{99, "repos"},
	}
	for _, tt := range tests {
		m := NewModel("/tmp/test", nil)
		m.Fleet.Section = tt.section
		if got := m.selectedFleetSection(); got != tt.want {
			t.Errorf("FleetSection=%d: selectedFleetSection() = %q, want %q", tt.section, got, tt.want)
		}
	}
}

func TestBuildFleetDataEmpty(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	data := m.buildFleetData()
	if data.TotalRepos != 0 {
		t.Errorf("TotalRepos = %d, want 0", data.TotalRepos)
	}
	if data.RunningLoops != 0 {
		t.Errorf("RunningLoops = %d, want 0", data.RunningLoops)
	}
	if len(data.Alerts) != 0 {
		t.Errorf("Alerts = %d, want 0", len(data.Alerts))
	}
}

func TestBuildFleetDataWithRepos(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{
		{Name: "alpha", Path: "/tmp/alpha", Status: &model.LoopStatus{Status: "running"}},
		{Name: "beta", Path: "/tmp/beta", Status: &model.LoopStatus{Status: "paused"}},
		{Name: "gamma", Path: "/tmp/gamma", Circuit: &model.CircuitBreakerState{State: "OPEN", Reason: "test failure"}},
	}
	data := m.buildFleetData()
	if data.TotalRepos != 3 {
		t.Errorf("TotalRepos = %d, want 3", data.TotalRepos)
	}
	if data.RunningLoops != 1 {
		t.Errorf("RunningLoops = %d, want 1", data.RunningLoops)
	}
	if data.PausedLoops != 1 {
		t.Errorf("PausedLoops = %d, want 1", data.PausedLoops)
	}
	if data.OpenCircuits != 1 {
		t.Errorf("OpenCircuits = %d, want 1", data.OpenCircuits)
	}
	// Open circuit generates a critical alert
	foundCritical := false
	for _, a := range data.Alerts {
		if a.Severity == "critical" {
			foundCritical = true
		}
	}
	if !foundCritical {
		t.Error("expected a critical alert for OPEN circuit")
	}
	// Repos should be sorted by name
	if len(data.Repos) >= 2 && data.Repos[0].Name > data.Repos[1].Name {
		t.Errorf("repos not sorted: %s > %s", data.Repos[0].Name, data.Repos[1].Name)
	}
}

func TestBuildFleetDataNoProgressAlert(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{
		{Name: "stuck", Path: "/tmp/stuck", Circuit: &model.CircuitBreakerState{State: "CLOSED", ConsecutiveNoProgress: 5}},
	}
	data := m.buildFleetData()
	foundWarning := false
	for _, a := range data.Alerts {
		if a.Severity == "warning" {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Error("expected warning alert for no-progress repo")
	}
}

func TestBuildFleetDataWithSessions(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	s1 := &session.Session{
		ID:         "session-001",
		Provider:   "claude",
		RepoPath:   "/tmp/alpha",
		RepoName:   "alpha",
		Status:     session.StatusRunning,
		SpentUSD:   1.50,
		BudgetUSD:  10.0,
		TurnCount:  5,
		LaunchedAt: time.Now(),
	}
	s2 := &session.Session{
		ID:         "session-002",
		Provider:   "gemini",
		RepoPath:   "/tmp/beta",
		RepoName:   "beta",
		Status:     session.StatusCompleted,
		SpentUSD:   0.75,
		TurnCount:  3,
		LaunchedAt: time.Now().Add(-time.Hour),
	}
	mgr.AddSessionForTesting(s1)
	mgr.AddSessionForTesting(s2)
	m.SessMgr = mgr

	data := m.buildFleetData()
	if data.TotalSessions != 2 {
		t.Errorf("TotalSessions = %d, want 2", data.TotalSessions)
	}
	if data.RunningSessions != 1 {
		t.Errorf("RunningSessions = %d, want 1", data.RunningSessions)
	}
	if data.TotalSpendUSD != 2.25 {
		t.Errorf("TotalSpendUSD = %.2f, want 2.25", data.TotalSpendUSD)
	}
	if data.TotalTurns != 8 {
		t.Errorf("TotalTurns = %d, want 8", data.TotalTurns)
	}
	// Check provider stats
	claudeStats, ok := data.Providers["claude"]
	if !ok {
		t.Fatal("missing claude provider stats")
	}
	if claudeStats.Sessions != 1 || claudeStats.Running != 1 {
		t.Errorf("claude stats: sessions=%d running=%d, want 1/1", claudeStats.Sessions, claudeStats.Running)
	}
	// Check cost-per-turn
	if cpt, ok := data.CostPerTurn["claude"]; !ok || cpt != 0.30 {
		t.Errorf("claude cost-per-turn = %.2f, want 0.30", cpt)
	}
}

func TestBuildFleetDataErroredSessionAlert(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	s := &session.Session{
		ID:         "error-session-12345678",
		Provider:   "claude",
		RepoPath:   "/tmp/repo",
		RepoName:   "repo",
		Status:     session.StatusErrored,
		LaunchedAt: time.Now(),
	}
	mgr.AddSessionForTesting(s)
	m.SessMgr = mgr

	data := m.buildFleetData()
	foundInfo := false
	for _, a := range data.Alerts {
		if a.Severity == "info" {
			foundInfo = true
		}
	}
	if !foundInfo {
		t.Error("expected info alert for errored session")
	}
}

func TestBuildFleetDataBudgetAlert(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	s := &session.Session{
		ID:         "budget-session-12345678",
		Provider:   "claude",
		RepoPath:   "/tmp/repo",
		RepoName:   "repo",
		Status:     session.StatusRunning,
		SpentUSD:   9.50,
		BudgetUSD:  10.0,
		LaunchedAt: time.Now(),
	}
	mgr.AddSessionForTesting(s)
	m.SessMgr = mgr

	data := m.buildFleetData()
	foundBudgetWarning := false
	for _, a := range data.Alerts {
		if a.Severity == "warning" {
			foundBudgetWarning = true
		}
	}
	if !foundBudgetWarning {
		t.Error("expected warning alert for session at >=90% budget")
	}
}

func TestBuildFleetCostHistoryNilEventBus(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.EventBus = nil
	result := m.buildFleetCostHistory()
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestBuildFleetCostHistoryNoEvents(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.EventBus = events.NewBus(100)
	result := m.buildFleetCostHistory()
	if result != nil {
		t.Errorf("expected nil for empty event bus, got %v", result)
	}
}

func TestBuildFleetCostHistoryWithCostEvents(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	bus := events.NewBus(100)
	m.EventBus = bus
	m.Fleet.Window = 4 // "all" window (Span=0)

	bus.Publish(events.Event{
		Type:      events.CostUpdate,
		SessionID: "s1",
		Timestamp: time.Now(),
		Data:      map[string]any{"spent_usd": 1.0},
	})
	bus.Publish(events.Event{
		Type:      events.CostUpdate,
		SessionID: "s2",
		Timestamp: time.Now(),
		Data:      map[string]any{"spent_usd": 2.0},
	})
	bus.Publish(events.Event{
		Type:      events.CostUpdate,
		SessionID: "s1",
		Timestamp: time.Now(),
		Data:      map[string]any{"spent_usd": 3.0},
	})

	result := m.buildFleetCostHistory()
	if len(result) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(result))
	}
	// After first event: s1=1.0 -> total=1.0
	if result[0] != 1.0 {
		t.Errorf("result[0] = %.1f, want 1.0", result[0])
	}
	// After second event: s1=1.0 + s2=2.0 -> total=3.0
	if result[1] != 3.0 {
		t.Errorf("result[1] = %.1f, want 3.0", result[1])
	}
	// After third event: s1=3.0 + s2=2.0 -> total=5.0
	if result[2] != 5.0 {
		t.Errorf("result[2] = %.1f, want 5.0", result[2])
	}
}

func TestBuildFleetCostHistorySkipsNonCostEvents(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	bus := events.NewBus(100)
	m.EventBus = bus
	m.Fleet.Window = 4 // "all"

	bus.Publish(events.Event{
		Type:      events.LoopStarted,
		SessionID: "s1",
		Timestamp: time.Now(),
		Data:      map[string]any{"message": "started"},
	})
	bus.Publish(events.Event{
		Type:      events.CostUpdate,
		SessionID: "s1",
		Timestamp: time.Now(),
		Data:      map[string]any{"spent_usd": 1.5},
	})

	result := m.buildFleetCostHistory()
	if len(result) != 1 {
		t.Errorf("expected 1 cost entry (non-cost events skipped), got %d", len(result))
	}
}

func TestFleetWindowLabels(t *testing.T) {
	expected := []string{"15m", "1h", "6h", "24h", "all"}
	if len(fleetWindows) != len(expected) {
		t.Fatalf("fleetWindows has %d entries, want %d", len(fleetWindows), len(expected))
	}
	for i, w := range fleetWindows {
		if w.Label != expected[i] {
			t.Errorf("fleetWindows[%d].Label = %q, want %q", i, w.Label, expected[i])
		}
	}
}

func TestMoveFleetCursor(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{
		{Name: "a", Path: "/a"},
		{Name: "b", Path: "/b"},
		{Name: "c", Path: "/c"},
	}
	data := m.buildFleetData()

	// Move down
	m.Fleet.Cursor = 0
	m.moveFleetCursor(data, 1)
	if m.Fleet.Cursor != 1 {
		t.Errorf("after +1: cursor = %d, want 1", m.Fleet.Cursor)
	}

	// Clamp at end
	m.Fleet.Cursor = 2
	m.moveFleetCursor(data, 1)
	if m.Fleet.Cursor != 2 {
		t.Errorf("after clamp at end: cursor = %d, want 2", m.Fleet.Cursor)
	}

	// Clamp at start
	m.Fleet.Cursor = 0
	m.moveFleetCursor(data, -1)
	if m.Fleet.Cursor != 0 {
		t.Errorf("after clamp at start: cursor = %d, want 0", m.Fleet.Cursor)
	}

	// Empty data
	m.Repos = nil
	data = m.buildFleetData()
	m.Fleet.Cursor = 5
	m.moveFleetCursor(data, 0)
	if m.Fleet.Cursor != 0 {
		t.Errorf("empty data: cursor = %d, want 0", m.Fleet.Cursor)
	}
}

func TestCycleFleetSection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	data := m.buildFleetData()

	m.Fleet.Section = 0
	m.cycleFleetSection(data, 1)
	if m.Fleet.Section != 1 {
		t.Errorf("after +1 from 0: section = %d, want 1", m.Fleet.Section)
	}

	m.cycleFleetSection(data, 1)
	if m.Fleet.Section != 2 {
		t.Errorf("after +1 from 1: section = %d, want 2", m.Fleet.Section)
	}

	// Wrap forward
	m.cycleFleetSection(data, 1)
	if m.Fleet.Section != 0 {
		t.Errorf("after wrap forward: section = %d, want 0", m.Fleet.Section)
	}

	// Wrap backward
	m.cycleFleetSection(data, -1)
	if m.Fleet.Section != 2 {
		t.Errorf("after wrap backward: section = %d, want 2", m.Fleet.Section)
	}
}

func TestFleetSectionLen(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	s := &session.Session{ID: "s1", Provider: "claude", Status: session.StatusRunning, LaunchedAt: time.Now()}
	mgr.AddSessionForTesting(s)
	m.SessMgr = mgr
	m.Repos = []*model.Repo{
		{Name: "a", Path: "/a"},
		{Name: "b", Path: "/b"},
	}
	data := m.buildFleetData()

	m.Fleet.Section = 0 // repos
	if got := m.fleetSectionLen(data); got != 2 {
		t.Errorf("repos section len = %d, want 2", got)
	}

	m.Fleet.Section = 1 // sessions
	if got := m.fleetSectionLen(data); got != 1 {
		t.Errorf("sessions section len = %d, want 1", got)
	}
}

func TestBuildFleetDataTopExpensiveCapped(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	for i := 0; i < 8; i++ {
		s := &session.Session{
			ID:         fmt.Sprintf("s%d", i),
			Provider:   "claude",
			RepoPath:   "/tmp/repo",
			RepoName:   "repo",
			Status:     session.StatusCompleted,
			SpentUSD:   float64(i) * 0.5,
			LaunchedAt: time.Now(),
		}
		mgr.AddSessionForTesting(s)
	}
	m.SessMgr = mgr

	data := m.buildFleetData()
	if len(data.TopExpensive) > 5 {
		t.Errorf("TopExpensive should be capped at 5, got %d", len(data.TopExpensive))
	}
	// Should be sorted descending by spend
	for i := 1; i < len(data.TopExpensive); i++ {
		if data.TopExpensive[i].SpendUSD > data.TopExpensive[i-1].SpendUSD {
			t.Errorf("TopExpensive not sorted: [%d]=%.2f > [%d]=%.2f", i, data.TopExpensive[i].SpendUSD, i-1, data.TopExpensive[i-1].SpendUSD)
		}
	}
}

// --- openFleetSelection ---

func TestOpenFleetSelectionRepos(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{
		{Name: "alpha", Path: "/tmp/alpha"},
		{Name: "beta", Path: "/tmp/beta"},
	}
	data := m.buildFleetData()
	m.Fleet.Section = 0 // repos
	m.Fleet.Cursor = 0

	m.openFleetSelection(data)
	if m.Nav.CurrentView != ViewRepoDetail {
		t.Errorf("expected ViewRepoDetail, got %d", m.Nav.CurrentView)
	}
	if m.Sel.RepoIdx < 0 {
		t.Error("SelectedIdx should be set")
	}
}

func TestOpenFleetSelectionReposOutOfBounds(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{{Name: "a", Path: "/tmp/a"}}
	data := m.buildFleetData()
	m.Fleet.Section = 0
	m.Fleet.Cursor = 99 // out of bounds

	before := m.Nav.CurrentView
	m.openFleetSelection(data)
	if m.Nav.CurrentView != before {
		t.Error("should not navigate when cursor out of bounds")
	}
}

func TestOpenFleetSelectionSessions(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	s := &session.Session{
		ID:         "sess-001",
		Provider:   "claude",
		RepoPath:   "/tmp/repo",
		RepoName:   "repo",
		Status:     session.StatusRunning,
		LaunchedAt: time.Now(),
	}
	mgr.AddSessionForTesting(s)
	m.SessMgr = mgr
	data := m.buildFleetData()

	m.Fleet.Section = 1 // sessions
	m.Fleet.Cursor = 0
	m.openFleetSelection(data)
	if m.Nav.CurrentView != ViewSessionDetail {
		t.Errorf("expected ViewSessionDetail, got %d", m.Nav.CurrentView)
	}
	if m.Sel.SessionID != "sess-001" {
		t.Errorf("SelectedSession = %q, want sess-001", m.Sel.SessionID)
	}
}

func TestOpenFleetSelectionTeams(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	mgr.AddTeamForTesting(&session.TeamStatus{
		Name:     "team-alpha",
		RepoPath: "/tmp/repo",
		Status:   session.StatusRunning,
	})
	m.SessMgr = mgr
	data := m.buildFleetData()

	m.Fleet.Section = 2 // teams
	m.Fleet.Cursor = 0
	m.openFleetSelection(data)
	if m.Nav.CurrentView != ViewTeamDetail {
		t.Errorf("expected ViewTeamDetail, got %d", m.Nav.CurrentView)
	}
	if m.Sel.TeamName != "team-alpha" {
		t.Errorf("SelectedTeam = %q, want team-alpha", m.Sel.TeamName)
	}
}

// --- diffFleetSelection ---

func TestDiffFleetSelectionRepos(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{
		{Name: "alpha", Path: "/tmp/alpha"},
	}
	data := m.buildFleetData()
	m.Fleet.Section = 0
	m.Fleet.Cursor = 0

	m.diffFleetSelection(data)
	if m.Nav.CurrentView != ViewDiff {
		t.Errorf("expected ViewDiff, got %d", m.Nav.CurrentView)
	}
}

func TestDiffFleetSelectionSessions(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{{Name: "repo", Path: "/tmp/repo"}}
	mgr := session.NewManager()
	s := &session.Session{
		ID: "s1", Provider: "claude", RepoPath: "/tmp/repo", RepoName: "repo",
		Status: session.StatusRunning, LaunchedAt: time.Now(),
	}
	mgr.AddSessionForTesting(s)
	m.SessMgr = mgr
	data := m.buildFleetData()

	m.Fleet.Section = 1 // sessions
	m.Fleet.Cursor = 0
	m.diffFleetSelection(data)
	if m.Nav.CurrentView != ViewDiff {
		t.Errorf("expected ViewDiff, got %d", m.Nav.CurrentView)
	}
}

func TestDiffFleetSelectionTeams(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{{Name: "repo", Path: "/tmp/repo"}}
	mgr := session.NewManager()
	mgr.AddTeamForTesting(&session.TeamStatus{
		Name: "team-1", RepoPath: "/tmp/repo", Status: session.StatusRunning,
	})
	m.SessMgr = mgr
	data := m.buildFleetData()

	m.Fleet.Section = 2 // teams
	m.Fleet.Cursor = 0
	m.diffFleetSelection(data)
	if m.Nav.CurrentView != ViewDiff {
		t.Errorf("expected ViewDiff, got %d", m.Nav.CurrentView)
	}
}

func TestDiffFleetSelectionOutOfBounds(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	data := m.buildFleetData()
	m.Fleet.Section = 0
	m.Fleet.Cursor = 5 // no repos
	before := m.Nav.CurrentView
	m.diffFleetSelection(data)
	if m.Nav.CurrentView != before {
		t.Error("should not navigate when cursor out of bounds")
	}
}

// --- timelineFleetSelection ---

func TestTimelineFleetSelectionRepos(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{{Name: "alpha", Path: "/tmp/alpha"}}
	data := m.buildFleetData()
	m.Fleet.Section = 0
	m.Fleet.Cursor = 0

	m.timelineFleetSelection(data)
	if m.Nav.CurrentView != ViewTimeline {
		t.Errorf("expected ViewTimeline, got %d", m.Nav.CurrentView)
	}
}

func TestTimelineFleetSelectionSessions(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{{Name: "repo", Path: "/tmp/repo"}}
	mgr := session.NewManager()
	s := &session.Session{
		ID: "s1", Provider: "claude", RepoPath: "/tmp/repo", RepoName: "repo",
		Status: session.StatusRunning, LaunchedAt: time.Now(),
	}
	mgr.AddSessionForTesting(s)
	m.SessMgr = mgr
	data := m.buildFleetData()

	m.Fleet.Section = 1
	m.Fleet.Cursor = 0
	m.timelineFleetSelection(data)
	if m.Nav.CurrentView != ViewTimeline {
		t.Errorf("expected ViewTimeline, got %d", m.Nav.CurrentView)
	}
}

func TestTimelineFleetSelectionTeams(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{{Name: "repo", Path: "/tmp/repo"}}
	mgr := session.NewManager()
	mgr.AddTeamForTesting(&session.TeamStatus{
		Name: "team-1", RepoPath: "/tmp/repo", Status: session.StatusRunning,
	})
	m.SessMgr = mgr
	data := m.buildFleetData()

	m.Fleet.Section = 2
	m.Fleet.Cursor = 0
	m.timelineFleetSelection(data)
	if m.Nav.CurrentView != ViewTimeline {
		t.Errorf("expected ViewTimeline, got %d", m.Nav.CurrentView)
	}
}

// --- stopFleetSelection ---

func TestStopFleetSelectionRepos(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{{Name: "alpha", Path: "/tmp/alpha"}}
	data := m.buildFleetData()
	m.Fleet.Section = 0
	m.Fleet.Cursor = 0

	m.stopFleetSelection(data)
	if m.Modals.ConfirmDialog == nil {
		t.Fatal("expected confirm dialog")
	}
	if m.Modals.ConfirmDialog.Action != "stopLoop" {
		t.Errorf("action = %q, want stopLoop", m.Modals.ConfirmDialog.Action)
	}
}

func TestStopFleetSelectionSessions(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	s := &session.Session{
		ID: "session-12345678", Provider: "claude", RepoPath: "/tmp/repo", RepoName: "repo",
		Status: session.StatusRunning, LaunchedAt: time.Now(),
	}
	mgr.AddSessionForTesting(s)
	m.SessMgr = mgr
	data := m.buildFleetData()

	m.Fleet.Section = 1
	m.Fleet.Cursor = 0
	m.stopFleetSelection(data)
	if m.Modals.ConfirmDialog == nil {
		t.Fatal("expected confirm dialog")
	}
	if m.Modals.ConfirmDialog.Action != "stopSession" {
		t.Errorf("action = %q, want stopSession", m.Modals.ConfirmDialog.Action)
	}
}

func TestStopFleetSelectionOutOfBounds(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	data := m.buildFleetData()
	m.Fleet.Section = 0
	m.Fleet.Cursor = 99
	m.stopFleetSelection(data)
	if m.Modals.ConfirmDialog != nil {
		t.Error("should not show confirm dialog when cursor out of bounds")
	}
}

// --- buildTimelineEntries ---

func TestBuildTimelineEntriesNilSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.SessMgr = nil
	got := m.buildTimelineEntries()
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestBuildTimelineEntriesAllSessions(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	now := time.Now()
	ended := now.Add(10 * time.Minute)
	s1 := &session.Session{
		ID: "s1", Provider: "claude", RepoPath: "/tmp/a", RepoName: "a",
		Status: session.StatusCompleted, LaunchedAt: now, EndedAt: &ended,
	}
	s2 := &session.Session{
		ID: "s2", Provider: "gemini", RepoPath: "/tmp/b", RepoName: "b",
		Status: session.StatusRunning, LaunchedAt: now,
	}
	mgr.AddSessionForTesting(s1)
	mgr.AddSessionForTesting(s2)
	m.SessMgr = mgr
	m.Sel.RepoIdx = -1 // no repo filter

	entries := m.buildTimelineEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Check one has end time
	foundEnded := false
	foundRunning := false
	for _, e := range entries {
		if e.EndTime != nil {
			foundEnded = true
		}
		if e.Status == string(session.StatusRunning) {
			foundRunning = true
		}
	}
	if !foundEnded {
		t.Error("expected at least one entry with EndTime")
	}
	if !foundRunning {
		t.Error("expected at least one running entry")
	}
}

func TestBuildTimelineEntriesFilteredByRepo(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	mgr := session.NewManager()
	now := time.Now()
	s1 := &session.Session{
		ID: "s1", Provider: "claude", RepoPath: "/tmp/a", RepoName: "a",
		Status: session.StatusRunning, LaunchedAt: now,
	}
	s2 := &session.Session{
		ID: "s2", Provider: "gemini", RepoPath: "/tmp/b", RepoName: "b",
		Status: session.StatusRunning, LaunchedAt: now,
	}
	mgr.AddSessionForTesting(s1)
	mgr.AddSessionForTesting(s2)
	m.SessMgr = mgr
	m.Repos = []*model.Repo{
		{Name: "a", Path: "/tmp/a"},
		{Name: "b", Path: "/tmp/b"},
	}
	m.Sel.RepoIdx = 0 // filter to /tmp/a

	entries := m.buildTimelineEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 filtered entry, got %d", len(entries))
	}
	if entries[0].ID != "s1" {
		t.Errorf("expected s1, got %s", entries[0].ID)
	}
}

// --- findRepoByPath ---

func TestFindRepoByPath(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{
		{Name: "a", Path: "/tmp/a"},
		{Name: "b", Path: "/tmp/b"},
		{Name: "c", Path: "/tmp/c"},
	}

	tests := []struct {
		path string
		want int
	}{
		{"/tmp/a", 0},
		{"/tmp/b", 1},
		{"/tmp/c", 2},
		{"/tmp/nonexistent", -1},
		{"", -1},
	}
	for _, tt := range tests {
		got := m.findRepoByPath(tt.path)
		if got != tt.want {
			t.Errorf("findRepoByPath(%q) = %d, want %d", tt.path, got, tt.want)
		}
	}
}
