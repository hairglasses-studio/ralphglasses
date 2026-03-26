package views

import (
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestNewLoopListTable(t *testing.T) {
	tbl := NewLoopListTable()
	if tbl == nil {
		t.Fatal("nil table")
	}
	if len(tbl.Columns) != len(LoopListColumns) {
		t.Errorf("expected %d columns, got %d", len(LoopListColumns), len(tbl.Columns))
	}
}

func TestLoopListColumns(t *testing.T) {
	expected := []string{"ID", "Repo", "Phase", "Iters", "Status"}
	if len(LoopListColumns) != len(expected) {
		t.Fatalf("expected %d columns, got %d", len(expected), len(LoopListColumns))
	}
	for i, col := range LoopListColumns {
		if col.Title != expected[i] {
			t.Errorf("column %d title = %q, want %q", i, col.Title, expected[i])
		}
	}
}

func TestLoopRunsToRows(t *testing.T) {
	loops := []*session.LoopRun{
		{
			ID:       "abcdef12-1234-5678-abcd-ef1234567890",
			RepoName: "my-repo",
			RepoPath: "/path/to/my-repo",
			Status:   "running",
			Iterations: []session.LoopIteration{
				{Number: 1, Status: "executing"},
				{Number: 2, Status: "verifying"},
			},
		},
		{
			ID:       "00000000-0000-0000-0000-000000000001",
			RepoName: "",
			RepoPath: "/path/to/other-repo",
			Status:   "stopped",
		},
	}

	rows := LoopRunsToRows(loops, 0)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// ID is truncated to 8 chars
	if rows[0][0] != "abcdef12" {
		t.Errorf("row[0] ID = %q, want %q", rows[0][0], "abcdef12")
	}
	// Repo name from RepoName field
	if rows[0][1] != "my-repo" {
		t.Errorf("row[0] repo = %q, want my-repo", rows[0][1])
	}
	// Phase from last iteration
	if rows[0][2] != "verifying" {
		t.Errorf("row[0] phase = %q, want verifying", rows[0][2])
	}
	// Iteration count
	if rows[0][3] != "2" {
		t.Errorf("row[0] iters = %q, want 2", rows[0][3])
	}
	// Status cell contains status string
	if !strings.Contains(rows[0][4], "running") {
		t.Errorf("row[0] status = %q, want contains running", rows[0][4])
	}

	// Second row: RepoName empty, falls back to filepath.Base(RepoPath)
	if rows[1][1] != "other-repo" {
		t.Errorf("row[1] repo = %q, want other-repo", rows[1][1])
	}
	// No iterations — phase is "-"
	if rows[1][2] != "-" {
		t.Errorf("row[1] phase = %q, want -", rows[1][2])
	}
	if rows[1][3] != "0" {
		t.Errorf("row[1] iters = %q, want 0", rows[1][3])
	}
	if !strings.Contains(rows[1][4], "stopped") {
		t.Errorf("row[1] status = %q, want contains stopped", rows[1][4])
	}
}

func TestLoopRunsToRowsEmpty(t *testing.T) {
	rows := LoopRunsToRows(nil, 0)
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}

func TestLoopRunsToRowsPausedStatus(t *testing.T) {
	loops := []*session.LoopRun{
		{
			ID:       "aabbccdd-0000-0000-0000-000000000001",
			RepoName: "paused-repo",
			RepoPath: "/path/to/paused-repo",
			Status:   "running",
			Paused:   true,
		},
		{
			ID:       "aabbccdd-0000-0000-0000-000000000002",
			RepoName: "running-repo",
			RepoPath: "/path/to/running-repo",
			Status:   "running",
			Paused:   false,
		},
	}

	rows := LoopRunsToRows(loops, 0)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// Paused loop should show "paused" in status cell
	if !strings.Contains(rows[0][4], "paused") {
		t.Errorf("paused loop status cell = %q, want contains 'paused'", rows[0][4])
	}

	// Running loop should show "running" in status cell
	if !strings.Contains(rows[1][4], "running") {
		t.Errorf("running loop status cell = %q, want contains 'running'", rows[1][4])
	}
}
