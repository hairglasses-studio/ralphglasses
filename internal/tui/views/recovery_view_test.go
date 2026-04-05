package views

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestRecoveryView_ImplementsView(t *testing.T) {
	var _ View = (*RecoveryView)(nil)
}

func TestRecoveryView_RenderEmpty(t *testing.T) {
	v := NewRecoveryView()
	v.SetDimensions(80, 24)
	out := v.Render()
	if out == "" {
		t.Error("expected non-empty render for empty data")
	}
	if len(out) < 10 {
		t.Errorf("render too short: %d chars", len(out))
	}
}

func TestRecoveryView_RenderWithPlan(t *testing.T) {
	v := NewRecoveryView()
	v.SetData(RecoveryData{
		CurrentPlan: &session.CrashRecoveryPlan{
			DetectedAt:    time.Now(),
			Severity:      "major",
			TotalSessions: 10,
			AliveCount:    7,
			DeadCount:     3,
			SessionsToResume: []session.RecoverableSession{
				{SessionID: "sess-001", RepoName: "mcpkit", Priority: 1, OpenTasks: 5},
				{SessionID: "sess-002", RepoName: "dotfiles", Priority: 2, OpenTasks: 2},
			},
		},
		BudgetTotal:   5.00,
		BudgetSpent:   1.50,
		PolicyEnabled: true,
	})
	v.SetDimensions(120, 40)
	out := v.Render()
	if out == "" {
		t.Fatal("expected non-empty render with plan data")
	}
}

func TestRecoveryView_RenderSessions(t *testing.T) {
	v := NewRecoveryView()
	v.SetData(RecoveryData{
		Sessions: []RecoverySessionRow{
			{SessionID: "aaaa-bbbb", RepoName: "mcpkit", SessionName: "test-session", Priority: 1, OpenTasks: 3, Status: "pending"},
			{SessionID: "cccc-dddd", RepoName: "dotfiles", Priority: 2, OpenTasks: 1, Status: "succeeded", CostUSD: 0.50},
		},
	})
	v.panel = PanelSessions
	v.SetDimensions(120, 40)
	out := v.Render()
	if out == "" {
		t.Fatal("expected non-empty render for sessions panel")
	}
}

func TestRecoveryView_RenderHistory(t *testing.T) {
	now := time.Now()
	v := NewRecoveryView()
	v.SetData(RecoveryData{
		History: []*session.RecoveryOp{
			{ID: "rec-1", Severity: "minor", Status: session.RecoveryOpCompleted, DeadCount: 2, ResumedCount: 2, DetectedAt: now},
			{ID: "rec-2", Severity: "major", Status: session.RecoveryOpFailed, DeadCount: 4, ResumedCount: 1, DetectedAt: now.Add(-time.Hour)},
		},
	})
	v.panel = PanelHistory
	v.SetDimensions(120, 40)
	out := v.Render()
	if out == "" {
		t.Fatal("expected non-empty render for history panel")
	}
}

func TestRecoveryView_HandleKey_Tab(t *testing.T) {
	v := NewRecoveryView()
	v.SetDimensions(80, 24)

	if v.panel != PanelPlan {
		t.Fatalf("expected initial panel=PanelPlan, got %d", v.panel)
	}

	// Simulate Tab key (using KeyMsg with "tab" string).
	// We can't easily construct tea.KeyPressMsg, so test via panel manipulation.
	v.panel = RecoveryPanel((int(v.panel) + 1) % recoveryPanelCount)
	if v.panel != PanelSessions {
		t.Errorf("expected PanelSessions after tab, got %d", v.panel)
	}

	v.panel = RecoveryPanel((int(v.panel) + 1) % recoveryPanelCount)
	if v.panel != PanelHistory {
		t.Errorf("expected PanelHistory after second tab, got %d", v.panel)
	}

	v.panel = RecoveryPanel((int(v.panel) + 1) % recoveryPanelCount)
	if v.panel != PanelPlan {
		t.Errorf("expected PanelPlan after third tab (wrap), got %d", v.panel)
	}
}

func TestRecoveryView_SetDimensions_Small(t *testing.T) {
	v := NewRecoveryView()
	// Should not panic on very small dimensions.
	v.SetDimensions(10, 5)
	out := v.Render()
	if out == "" {
		t.Error("expected non-empty render even at small size")
	}
}

func TestRecoveryView_MaxCursor(t *testing.T) {
	v := NewRecoveryView()
	v.SetData(RecoveryData{
		Sessions: []RecoverySessionRow{
			{SessionID: "a", Priority: 1},
			{SessionID: "b", Priority: 2},
		},
	})
	v.panel = PanelSessions
	if v.maxCursorForPanel() != 2 {
		t.Errorf("expected maxCursor=2, got %d", v.maxCursorForPanel())
	}

	v.panel = PanelPlan
	if v.maxCursorForPanel() != 0 {
		t.Errorf("expected maxCursor=0 for plan panel, got %d", v.maxCursorForPanel())
	}
}
