package components

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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
	if !strings.Contains(view, "No items") {
		t.Error("empty view should show 'No items'")
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

func TestSortColOutOfBounds(t *testing.T) {
	tbl := makeTestTable()
	tbl.Width = 40
	tbl.Height = 20
	// Set SortCol beyond column count
	tbl.SortCol = 99

	// CycleSort should clamp and not panic
	tbl.CycleSort()
	if tbl.SortCol >= len(tbl.Columns) {
		t.Errorf("CycleSort did not clamp SortCol: got %d, columns=%d", tbl.SortCol, len(tbl.Columns))
	}

	// Reset to out-of-bounds and verify View clamps
	tbl.SortCol = 99
	view := tbl.View()
	if view == "" {
		t.Error("View should not be empty with out-of-bounds SortCol")
	}
	if tbl.SortCol >= len(tbl.Columns) {
		t.Errorf("View did not clamp SortCol: got %d", tbl.SortCol)
	}

	// sortRows should also clamp
	tbl.SortCol = 50
	tbl.sortRows()
	if tbl.SortCol >= len(tbl.Columns) {
		t.Errorf("sortRows did not clamp SortCol: got %d", tbl.SortCol)
	}
}

func TestSortColOutOfBoundsEmptyColumns(t *testing.T) {
	tbl := NewTable(nil)
	tbl.SortCol = 5
	// Should not panic
	tbl.CycleSort()
	tbl.sortRows()
	_ = tbl.View()
}

func TestStyledCell(t *testing.T) {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	result := StyledCell(style, "test")
	if result == "" {
		t.Error("StyledCell returned empty")
	}
}

func TestToggleSelect(t *testing.T) {
	tbl := makeTestTable()
	tbl.MultiSelect = true

	tbl.ToggleSelect()
	if !tbl.Selected[0] {
		t.Error("row 0 should be selected")
	}

	tbl.ToggleSelect() // deselect
	if tbl.Selected[0] {
		t.Error("row 0 should be deselected")
	}
}

func TestToggleSelectEmpty(t *testing.T) {
	tbl := NewTable([]Column{{Title: "A", Width: 5}})
	tbl.ToggleSelect() // should not panic
}

func TestSelectAll(t *testing.T) {
	tbl := makeTestTable()
	tbl.SelectAll()
	if len(tbl.Selected) != 3 {
		t.Errorf("selected = %d, want 3", len(tbl.Selected))
	}
}

func TestClearSelection(t *testing.T) {
	tbl := makeTestTable()
	tbl.SelectAll()
	tbl.ClearSelection()
	if tbl.HasSelection() {
		t.Error("expected no selection after clear")
	}
}

func TestSelectedRows(t *testing.T) {
	tbl := makeTestTable()
	// No selection
	if rows := tbl.SelectedRows(); rows != nil {
		t.Errorf("expected nil, got %d rows", len(rows))
	}

	tbl.SelectAll()
	rows := tbl.SelectedRows()
	if len(rows) != 3 {
		t.Errorf("selected rows = %d, want 3", len(rows))
	}
}

func TestHasSelection(t *testing.T) {
	tbl := makeTestTable()
	if tbl.HasSelection() {
		t.Error("should have no selection initially")
	}
	tbl.ToggleSelect()
	if !tbl.HasSelection() {
		t.Error("should have selection after toggle")
	}
}

func TestMaxFunc(t *testing.T) {
	tests := []struct {
		a, b, want int
	}{
		{1, 2, 2},
		{5, 3, 5},
		{0, 0, 0},
		{-1, 1, 1},
	}
	for _, tt := range tests {
		if got := max(tt.a, tt.b); got != tt.want {
			t.Errorf("max(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestViewWithGrowColumn(t *testing.T) {
	cols := []Column{
		{Title: "Name", Width: 10, Sortable: true},
		{Title: "Desc", Width: 0, Grow: true},
	}
	tbl := NewTable(cols)
	tbl.SetRows([]Row{{"alpha", "some description"}})
	tbl.Width = 60
	tbl.Height = 10
	view := tbl.View()
	if !strings.Contains(view, "alpha") {
		t.Error("view should contain row data")
	}
}

func TestEffectiveWidths_NarrowTerminal(t *testing.T) {
	cols := []Column{
		{Title: "Name", Width: 18, MinWidth: 12, Flex: 2.0},
		{Title: "Status", Width: 10, MinWidth: 8},
		{Title: "Loop", Width: 6, MinWidth: 5},
		{Title: "Calls", Width: 12, MinWidth: 5, Flex: 0.5},
		{Title: "Budget", Width: 12, MinWidth: 8, Flex: 1.0},
	}
	tbl := NewTable(cols)
	tbl.Width = 60 // narrow — no extra space to distribute

	widths := tbl.effectiveWidths()
	if len(widths) != 5 {
		t.Fatalf("expected 5 widths, got %d", len(widths))
	}
	// At 60 cols: base widths are 18+10+6+12+12=58, gaps=4, total=62 > 60
	// So remaining <= 0, all columns stay at base width
	expected := []int{18, 10, 6, 12, 12}
	for i, w := range widths {
		if w != expected[i] {
			t.Errorf("col %d: got %d, want %d", i, w, expected[i])
		}
	}
}

func TestEffectiveWidths_StandardTerminal(t *testing.T) {
	cols := []Column{
		{Title: "Name", Width: 12, MinWidth: 12, Flex: 2.0},
		{Title: "Status", Width: 8, MinWidth: 8},
		{Title: "Budget", Width: 8, MinWidth: 8, Flex: 1.0},
	}
	tbl := NewTable(cols)
	tbl.Width = 120

	widths := tbl.effectiveWidths()
	if len(widths) != 3 {
		t.Fatalf("expected 3 widths, got %d", len(widths))
	}
	// Base: 12+8+8=28, gaps=2, total=30, remaining=90
	// Flex total=3.0; Name gets 2/3*90=60, Budget gets 1/3*90=30
	// Name=12+60=72, Status=8, Budget=8+30=38
	if widths[0] != 72 {
		t.Errorf("Name: got %d, want 72", widths[0])
	}
	if widths[1] != 8 {
		t.Errorf("Status: got %d, want 8", widths[1])
	}
	if widths[2] != 38 {
		t.Errorf("Budget: got %d, want 38", widths[2])
	}
}

func TestEffectiveWidths_WideTerminal(t *testing.T) {
	cols := []Column{
		{Title: "Name", Width: 12, MinWidth: 12, Flex: 2.0, MaxWidth: 40},
		{Title: "Status", Width: 8, MinWidth: 8},
		{Title: "Budget", Width: 8, MinWidth: 8, Flex: 1.0, MaxWidth: 30},
	}
	tbl := NewTable(cols)
	tbl.Width = 240

	widths := tbl.effectiveWidths()
	if len(widths) != 3 {
		t.Fatalf("expected 3 widths, got %d", len(widths))
	}
	// Name should be capped at MaxWidth=40
	if widths[0] > 40 {
		t.Errorf("Name exceeded MaxWidth: got %d, want <= 40", widths[0])
	}
	// Budget should be capped at MaxWidth=30
	if widths[2] > 30 {
		t.Errorf("Budget exceeded MaxWidth: got %d, want <= 30", widths[2])
	}
	// Status stays fixed
	if widths[1] != 8 {
		t.Errorf("Status: got %d, want 8", widths[1])
	}
}

func TestEffectiveWidths_BackwardCompat(t *testing.T) {
	cols := []Column{
		{Title: "Name", Width: 10},
		{Title: "Desc", Width: 5, Grow: true}, // Grow:true, no Flex → treated as Flex:1.0
	}
	tbl := NewTable(cols)
	tbl.Width = 60

	widths := tbl.effectiveWidths()
	if len(widths) != 2 {
		t.Fatalf("expected 2 widths, got %d", len(widths))
	}
	// Base: 10+5=15, gaps=1, total=16, remaining=44
	// Only Desc has flex (1.0 from Grow), so it gets all 44
	// Desc=5+44=49
	if widths[0] != 10 {
		t.Errorf("Name: got %d, want 10", widths[0])
	}
	if widths[1] != 49 {
		t.Errorf("Desc: got %d, want 49", widths[1])
	}
}

func TestEffectiveWidths_NoFlex(t *testing.T) {
	cols := []Column{
		{Title: "A", Width: 10},
		{Title: "B", Width: 20},
		{Title: "C", Width: 15},
	}
	tbl := NewTable(cols)
	tbl.Width = 120

	widths := tbl.effectiveWidths()
	// No flex columns — all stay at base width
	expected := []int{10, 20, 15}
	for i, w := range widths {
		if w != expected[i] {
			t.Errorf("col %d: got %d, want %d", i, w, expected[i])
		}
	}
}

func TestEffectiveWidths_ZeroWidth(t *testing.T) {
	cols := []Column{
		{Title: "Name", Width: 10, Flex: 1.0},
	}
	tbl := NewTable(cols)
	tbl.Width = 0 // no terminal width set

	widths := tbl.effectiveWidths()
	if widths[0] != 10 {
		t.Errorf("with zero Width: got %d, want 10", widths[0])
	}
}

func TestEffectiveWidths_Empty(t *testing.T) {
	tbl := NewTable(nil)
	widths := tbl.effectiveWidths()
	if widths != nil {
		t.Errorf("expected nil for empty columns, got %v", widths)
	}
}

func TestViewWithRowStyleFunc(t *testing.T) {
	tbl := makeTestTable()
	tbl.Width = 40
	tbl.Height = 20
	tbl.RowStyleFunc = func(row Row, selected bool) lipgloss.Style {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	}
	view := tbl.View()
	if !strings.Contains(view, "alpha") {
		t.Error("view should contain row data with custom style")
	}
}
