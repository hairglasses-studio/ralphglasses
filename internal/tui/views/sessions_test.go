package views

import (
	"strings"
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
		t.Error("empty message should be set")
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

func TestSessionsToRows(t *testing.T) {
	now := time.Now()
	sessions := []*session.Session{
		{
			ID:       "session-1234567890",
			Provider: session.ProviderClaude,
			RepoName: "my-repo",
			Status:   session.StatusRunning,
			SpentUSD: 1.50,
			BudgetUSD: 5.0,
			TurnCount: 10,
			MaxTurns:  50,
			AgentName: "planner",
			TeamName:  "alpha",
			LaunchedAt: now.Add(-5 * time.Minute),
		},
		{
			ID:         "short-id",
			Provider:   session.ProviderGemini,
			RepoName:   "other-repo",
			Status:     session.StatusStopped,
			SpentUSD:   0.25,
			TurnCount:  3,
			LaunchedAt: now.Add(-time.Hour),
		},
	}

	rows := SessionsToRows(sessions, 0)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// First row: ID truncated to 8 chars
	if rows[0][0] != "session-" {
		t.Errorf("row[0] ID = %q, want %q", rows[0][0], "session-")
	}
	// Provider cell contains provider name
	if !strings.Contains(rows[0][1], "claude") {
		t.Errorf("row[0] provider = %q, want contains claude", rows[0][1])
	}
	// Repo
	if rows[0][2] != "my-repo" {
		t.Errorf("row[0] repo = %q, want my-repo", rows[0][2])
	}
	// Status contains "running"
	if !strings.Contains(rows[0][3], "running") {
		t.Errorf("row[0] status = %q, want contains running", rows[0][3])
	}
	// Turns with max
	if rows[0][6] != "10/50" {
		t.Errorf("row[0] turns = %q, want 10/50", rows[0][6])
	}
	// Agent
	if rows[0][7] != "planner" {
		t.Errorf("row[0] agent = %q, want planner", rows[0][7])
	}
	// Team
	if rows[0][8] != "alpha" {
		t.Errorf("row[0] team = %q, want alpha", rows[0][8])
	}

	// Second row: short ID unchanged
	if rows[1][0] != "short-id" {
		t.Errorf("row[1] ID = %q, want short-id", rows[1][0])
	}
	// Turns without max
	if rows[1][6] != "3" {
		t.Errorf("row[1] turns = %q, want 3", rows[1][6])
	}
}

func TestSessionsToRowsEmpty(t *testing.T) {
	rows := SessionsToRows(nil, 0)
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name   string
		since  time.Time
		expect string
	}{
		{"zero", time.Time{}, "-"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatDuration(tt.since)
			if got != tt.expect {
				t.Errorf("formatDuration = %q, want %q", got, tt.expect)
			}
		})
	}

	// Recent time should produce a duration string containing 's' (seconds)
	got := formatDuration(time.Now().Add(-5 * time.Second))
	if !strings.Contains(got, "s") {
		t.Errorf("recent duration = %q, want contains 's'", got)
	}
}
