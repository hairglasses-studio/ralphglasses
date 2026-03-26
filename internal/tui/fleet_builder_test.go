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
			m.FleetWindow = tt.window
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
		m.FleetSection = tt.section
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
	m.FleetWindow = 4 // "all" window (Span=0)

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
	m.FleetWindow = 4 // "all"

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
	m.FleetCursor = 0
	m.moveFleetCursor(data, 1)
	if m.FleetCursor != 1 {
		t.Errorf("after +1: cursor = %d, want 1", m.FleetCursor)
	}

	// Clamp at end
	m.FleetCursor = 2
	m.moveFleetCursor(data, 1)
	if m.FleetCursor != 2 {
		t.Errorf("after clamp at end: cursor = %d, want 2", m.FleetCursor)
	}

	// Clamp at start
	m.FleetCursor = 0
	m.moveFleetCursor(data, -1)
	if m.FleetCursor != 0 {
		t.Errorf("after clamp at start: cursor = %d, want 0", m.FleetCursor)
	}

	// Empty data
	m.Repos = nil
	data = m.buildFleetData()
	m.FleetCursor = 5
	m.moveFleetCursor(data, 0)
	if m.FleetCursor != 0 {
		t.Errorf("empty data: cursor = %d, want 0", m.FleetCursor)
	}
}

func TestCycleFleetSection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	data := m.buildFleetData()

	m.FleetSection = 0
	m.cycleFleetSection(data, 1)
	if m.FleetSection != 1 {
		t.Errorf("after +1 from 0: section = %d, want 1", m.FleetSection)
	}

	m.cycleFleetSection(data, 1)
	if m.FleetSection != 2 {
		t.Errorf("after +1 from 1: section = %d, want 2", m.FleetSection)
	}

	// Wrap forward
	m.cycleFleetSection(data, 1)
	if m.FleetSection != 0 {
		t.Errorf("after wrap forward: section = %d, want 0", m.FleetSection)
	}

	// Wrap backward
	m.cycleFleetSection(data, -1)
	if m.FleetSection != 2 {
		t.Errorf("after wrap backward: section = %d, want 2", m.FleetSection)
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

	m.FleetSection = 0 // repos
	if got := m.fleetSectionLen(data); got != 2 {
		t.Errorf("repos section len = %d, want 2", got)
	}

	m.FleetSection = 1 // sessions
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
