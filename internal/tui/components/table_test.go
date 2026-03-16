package components

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func makeTestTable() *Table {
	cols := []Column{
		{Title: "Name", Width: 10, Sortable: true},
		{Title: "Status", Width: 8, Sortable: true},
	}
	t := NewTable(cols)
	t.SetRows([]Row{
		{"alpha", "running"},
		{"beta", "stopped"},
		{"gamma", "running"},
	})
	return t
}

func TestNewTable(t *testing.T) {
	tbl := NewTable([]Column{{Title: "A", Width: 5}})
	if !tbl.SortAsc {
		t.Error("default sort should be ascending")
	}
}

func TestSelectedRow(t *testing.T) {
	tbl := makeTestTable()
	row := tbl.SelectedRow()
	if row[0] != "alpha" {
		t.Errorf("first selected = %q", row[0])
	}
}

func TestSelectedRowEmpty(t *testing.T) {
	tbl := NewTable([]Column{{Title: "A", Width: 5}})
	if tbl.SelectedRow() != nil {
		t.Error("expected nil for empty table")
	}
}

func TestMoveDownUp(t *testing.T) {
	tbl := makeTestTable()
	tbl.MoveDown()
	if tbl.Cursor != 1 {
		t.Errorf("cursor = %d, want 1", tbl.Cursor)
	}
	tbl.MoveDown()
	tbl.MoveDown() // past end
	if tbl.Cursor != 2 {
		t.Errorf("cursor = %d, want 2", tbl.Cursor)
	}
	tbl.MoveUp()
	tbl.MoveUp()
	tbl.MoveUp() // past start
	if tbl.Cursor != 0 {
		t.Errorf("cursor = %d, want 0", tbl.Cursor)
	}
}

func TestFilter(t *testing.T) {
	tbl := makeTestTable()
	tbl.SetFilter("alpha")
	if tbl.RowCount() != 1 {
		t.Errorf("filtered = %d, want 1", tbl.RowCount())
	}
	tbl.SetFilter("running")
	if tbl.RowCount() != 2 {
		t.Errorf("filtered running = %d, want 2", tbl.RowCount())
	}
	tbl.SetFilter("")
	if tbl.RowCount() != 3 {
		t.Errorf("unfiltered = %d, want 3", tbl.RowCount())
	}
}

func TestFilterCaseInsensitive(t *testing.T) {
	tbl := makeTestTable()
	tbl.SetFilter("ALPHA")
	if tbl.RowCount() != 1 {
		t.Errorf("case insensitive = %d, want 1", tbl.RowCount())
	}
}

func TestCycleSort(t *testing.T) {
	tbl := makeTestTable()
	tbl.CycleSort()
	if tbl.SortCol != 1 {
		t.Errorf("after cycle: sort col = %d, want 1", tbl.SortCol)
	}
}

func TestView(t *testing.T) {
	tbl := makeTestTable()
	tbl.Width = 40
	tbl.Height = 20
	view := tbl.View()
	if !strings.Contains(view, "Name") {
		t.Error("view should contain header 'Name'")
	}
	if !strings.Contains(view, "alpha") {
		t.Error("view should contain row 'alpha'")
	}
}

func TestViewEmpty(t *testing.T) {
	tbl := NewTable([]Column{{Title: "A", Width: 5}})
	tbl.Width = 40
	tbl.Height = 20
	view := tbl.View()
	if !strings.Contains(view, "No repos found") {
		t.Error("empty view should show 'No repos found'")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("no truncate: %q", got)
	}
	if got := truncate("hello world", 5); got != "hell…" {
		t.Errorf("truncated: %q", got)
	}
	if got := truncate("abc", 2); got != "ab" {
		t.Errorf("short truncate: %q", got)
	}
}

func TestStyledCell(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	result := StyledCell(style, "test")
	if result == "" {
		t.Error("StyledCell returned empty")
	}
}
