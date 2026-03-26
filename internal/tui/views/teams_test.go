package views

import (
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestNewTeamsTable(t *testing.T) {
	tbl := NewTeamsTable()
	if tbl == nil {
		t.Fatal("nil table")
	}
	if len(tbl.Columns) != len(TeamColumns) {
		t.Errorf("expected %d columns, got %d", len(TeamColumns), len(tbl.Columns))
	}
	if tbl.EmptyMessage == "" {
		t.Error("empty message should be set")
	}
}

func TestTeamColumns(t *testing.T) {
	expected := []string{"Name", "Repo", "Status", "Lead", "Progress", "Tasks"}
	if len(TeamColumns) != len(expected) {
		t.Fatalf("expected %d columns, got %d", len(expected), len(TeamColumns))
	}
	for i, col := range TeamColumns {
		if col.Title != expected[i] {
			t.Errorf("column %d title = %q, want %q", i, col.Title, expected[i])
		}
	}
}

func TestTeamsToRows(t *testing.T) {
	teams := []*session.TeamStatus{
		{
			Name:     "alpha-team",
			RepoPath: "/path/to/my-repo",
			LeadID:   "lead-1234567890",
			Status:   session.StatusRunning,
			Tasks: []session.TeamTask{
				{Description: "task 1", Status: "completed"},
				{Description: "task 2", Status: "pending"},
				{Description: "task 3", Status: "completed"},
			},
		},
		{
			Name:     "beta-team",
			RepoPath: "/path/to/other-repo",
			LeadID:   "short",
			Status:   session.StatusStopped,
			Tasks:    nil,
		},
	}

	rows := TeamsToRows(teams)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// First team: name
	if rows[0][0] != "alpha-team" {
		t.Errorf("row[0] name = %q, want alpha-team", rows[0][0])
	}
	// Repo is filepath.Base
	if rows[0][1] != "my-repo" {
		t.Errorf("row[0] repo = %q, want my-repo", rows[0][1])
	}
	// Status cell contains "running"
	if !strings.Contains(rows[0][2], "running") {
		t.Errorf("row[0] status = %q, want contains running", rows[0][2])
	}
	// Lead truncated to 8
	if rows[0][3] != "lead-123" {
		t.Errorf("row[0] lead = %q, want lead-123", rows[0][3])
	}
	// Tasks count
	if rows[0][5] != "3" {
		t.Errorf("row[0] tasks = %q, want 3", rows[0][5])
	}

	// Second team: no tasks, progress should be "-"
	if rows[1][4] != "-" {
		t.Errorf("row[1] progress = %q, want -", rows[1][4])
	}
	if rows[1][5] != "0" {
		t.Errorf("row[1] tasks = %q, want 0", rows[1][5])
	}
}

func TestTeamsToRowsEmpty(t *testing.T) {
	rows := TeamsToRows(nil)
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}
