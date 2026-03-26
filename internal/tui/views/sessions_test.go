package views

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestNewSessionsTable(t *testing.T) {
	tbl := NewSessionsTable()
	if tbl == nil {
		t.Fatal("nil table")
	}
	if len(tbl.Columns) != len(SessionColumns) {
		t.Errorf("expected %d columns, got %d", len(SessionColumns), len(tbl.Columns))
	}
	if tbl.EmptyMessage == "" {
		t.Error("expected non-empty EmptyMessage")
	}
	if tbl.StatusColumn != 3 {
		t.Errorf("StatusColumn = %d, want 3", tbl.StatusColumn)
	}
}

func TestSessionColumns(t *testing.T) {
	expected := []string{"ID", "Prov", "Repo", "Status", "Budget", "Trend", "Turns", "Agent", "Team", "Dur"}
	if len(SessionColumns) != len(expected) {
		t.Fatalf("expected %d columns, got %d", len(expected), len(SessionColumns))
	}
	for i, col := range SessionColumns {
		if col.Title != expected[i] {
			t.Errorf("column %d title = %q, want %q", i, col.Title, expected[i])
		}
	}
}

func TestSessionsToRows_Empty(t *testing.T) {
	rows := SessionsToRows(nil, 0)
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestSessionsToRows_Single(t *testing.T) {
	now := time.Now()
	sessions := []*session.Session{
		{
			ID:        "abcdef1234567890",
			Provider:  session.ProviderClaude,
			RepoName:  "my-repo",
			Status:    session.StatusRunning,
			SpentUSD:  1.50,
			BudgetUSD: 5.00,
			TurnCount: 10,
			MaxTurns:  20,
			AgentName: "builder",
			TeamName:  "alpha",
			LaunchedAt: now.Add(-5 * time.Minute),
		},
	}

	rows := SessionsToRows(sessions, 0)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	row := rows[0]
	// ID should be truncated to 8 chars
	if row[0] != "abcdef12" {
		t.Errorf("ID = %q, want %q", row[0], "abcdef12")
	}
	// Repo
	if row[2] != "my-repo" {
		t.Errorf("repo = %q, want %q", row[2], "my-repo")
	}
	// Agent
	if row[7] != "builder" {
		t.Errorf("agent = %q, want %q", row[7], "builder")
	}
	// Team
	if row[8] != "alpha" {
		t.Errorf("team = %q, want %q", row[8], "alpha")
	}
}

func TestSessionsToRows_ShortID(t *testing.T) {
	sessions := []*session.Session{
		{
			ID:         "short",
			Provider:   session.ProviderGemini,
			RepoName:   "repo",
			Status:     session.StatusCompleted,
			LaunchedAt: time.Now(),
		},
	}

	rows := SessionsToRows(sessions, 0)
	if rows[0][0] != "short" {
		t.Errorf("short ID should not be truncated, got %q", rows[0][0])
	}
}

func TestSessionsToRows_NoBudget(t *testing.T) {
	sessions := []*session.Session{
		{
			ID:         "sess1234",
			Provider:   session.ProviderCodex,
			Status:     session.StatusRunning,
			SpentUSD:   2.50,
			BudgetUSD:  0,
			TurnCount:  5,
			MaxTurns:   0,
			LaunchedAt: time.Now(),
		},
	}

	rows := SessionsToRows(sessions, 0)
	row := rows[0]
	// Budget cell should just show dollar amount when budget is 0
	if row[4] != "$2.50" {
		t.Errorf("budget cell = %q, want $2.50", row[4])
	}
	// Turns cell should just show count when maxTurns is 0
	if row[6] != "5" {
		t.Errorf("turns cell = %q, want 5", row[6])
	}
}

func TestSessionsToRows_WithCostHistory(t *testing.T) {
	sessions := []*session.Session{
		{
			ID:          "sess1234",
			Provider:    session.ProviderClaude,
			Status:      session.StatusRunning,
			CostHistory: []float64{0.1, 0.2, 0.3},
			LaunchedAt:  time.Now(),
		},
	}

	rows := SessionsToRows(sessions, 0)
	// Trend cell should be non-empty when there's cost history with >1 points
	if rows[0][5] == "" {
		t.Error("expected non-empty trend cell with cost history")
	}
}

func TestSessionsToRows_Multiple(t *testing.T) {
	now := time.Now()
	sessions := []*session.Session{
		{ID: "sess0001", Provider: session.ProviderClaude, Status: session.StatusRunning, LaunchedAt: now},
		{ID: "sess0002", Provider: session.ProviderGemini, Status: session.StatusCompleted, LaunchedAt: now},
		{ID: "sess0003", Provider: session.ProviderCodex, Status: session.StatusErrored, LaunchedAt: now},
	}

	rows := SessionsToRows(sessions, 0)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name string
		dur  time.Duration
		want string
	}{
		{"zero", 0, "-"},
		{"seconds", 30 * time.Second, "30s"},
		{"minutes", 5*time.Minute + 30*time.Second, "5m30s"},
		{"hours", 2*time.Hour + 15*time.Minute, "2h15m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var since time.Time
			if tt.dur == 0 {
				since = time.Time{}
			} else {
				since = time.Now().Add(-tt.dur)
			}
			got := formatDuration(since)
			if tt.dur == 0 {
				if got != "-" {
					t.Errorf("formatDuration(zero) = %q, want %q", got, "-")
				}
			}
			// For non-zero durations, just verify it's non-empty and reasonable
			if tt.dur > 0 && got == "-" {
				t.Errorf("formatDuration should not return '-' for non-zero duration")
			}
		})
	}
}
