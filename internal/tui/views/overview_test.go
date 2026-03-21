package views

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

func TestNewOverviewTable(t *testing.T) {
	tbl := NewOverviewTable()
	if tbl == nil {
		t.Fatal("nil table")
	}
	if len(tbl.Columns) != len(OverviewColumns) {
		t.Errorf("expected %d columns, got %d", len(OverviewColumns), len(tbl.Columns))
	}
}

func TestOverviewColumns(t *testing.T) {
	expected := []string{"Name", "Status", "Loop", "Calls", "Budget", "Progress", "CB", "Updated"}
	if len(OverviewColumns) != len(expected) {
		t.Fatalf("expected %d columns, got %d", len(expected), len(OverviewColumns))
	}
	for i, col := range OverviewColumns {
		if col.Title != expected[i] {
			t.Errorf("column %d title = %q, want %q", i, col.Title, expected[i])
		}
	}
}

func TestReposToRows(t *testing.T) {
	repos := []*model.Repo{
		{
			Name: "test-repo",
			Status: &model.LoopStatus{
				Status:          "running",
				LoopCount:       42,
				CallsMadeThisHr: 10,
				MaxCallsPerHour: 80,
				LastAction:      "ran tests",
				Timestamp:       time.Now(),
			},
			Circuit: &model.CircuitBreakerState{State: "CLOSED"},
		},
		{
			Name: "empty-repo",
		},
	}

	rows := ReposToRows(repos, 0)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	if rows[0][0] != "test-repo" {
		t.Errorf("row[0][0] = %q, want test-repo", rows[0][0])
	}
	if rows[0][2] != "42" {
		t.Errorf("loop count = %q, want 42", rows[0][2])
	}
	if rows[1][2] != "-" {
		t.Errorf("empty loop count = %q, want -", rows[1][2])
	}
}

func TestReposToRowsEmpty(t *testing.T) {
	rows := ReposToRows(nil, 0)
	if len(rows) != 0 {
		t.Errorf("expected 0 rows, got %d", len(rows))
	}
}
