package components

import "testing"

// --- Table.HandleMouse tests ---

func TestTableHandleMouse_ClickRow(t *testing.T) {
	tbl := makeTestTable()
	tbl.Width = 40
	tbl.Height = 20

	// Click on row 0 (y=2 is header offset: header=0, separator=1, first data row=2)
	// HandleMouse receives y relative to table start, so row 0 data = y=2
	if !tbl.HandleMouse(5, 2, 1, 0) {
		t.Error("expected click on row 0 to be handled")
	}
	if tbl.Cursor != 0 {
		t.Errorf("cursor = %d, want 0", tbl.Cursor)
	}

	// Click on row 1
	if !tbl.HandleMouse(5, 3, 1, 0) {
		t.Error("expected click on row 1 to be handled")
	}
	if tbl.Cursor != 1 {
		t.Errorf("cursor = %d, want 1", tbl.Cursor)
	}

	// Click on row 2 (last row)
	if !tbl.HandleMouse(5, 4, 1, 0) {
		t.Error("expected click on row 2 to be handled")
	}
	if tbl.Cursor != 2 {
		t.Errorf("cursor = %d, want 2", tbl.Cursor)
	}
}

func TestTableHandleMouse_ClickHeader(t *testing.T) {
	tbl := makeTestTable()
	tbl.Width = 40
	tbl.Height = 20

	// Click on header row (y=0)
	if tbl.HandleMouse(5, 0, 1, 0) {
		t.Error("click on header should not be handled")
	}

	// Click on separator row (y=1)
	if tbl.HandleMouse(5, 1, 1, 0) {
		t.Error("click on separator should not be handled")
	}
}

func TestTableHandleMouse_ClickOutOfBounds(t *testing.T) {
	tbl := makeTestTable()
	tbl.Width = 40
	tbl.Height = 20

	// Click beyond last row (3 rows, so y=5 is out of bounds)
	if tbl.HandleMouse(5, 5, 1, 0) {
		t.Error("click beyond last row should not be handled")
	}

	// Negative y
	if tbl.HandleMouse(5, -1, 1, 0) {
		t.Error("negative y should not be handled")
	}
}

func TestTableHandleMouse_RightClickIgnored(t *testing.T) {
	tbl := makeTestTable()
	tbl.Width = 40
	tbl.Height = 20

	// Right click (button=3) should be ignored
	if tbl.HandleMouse(5, 2, 3, 0) {
		t.Error("right click should not be handled")
	}
}

func TestTableHandleMouse_ReleaseIgnored(t *testing.T) {
	tbl := makeTestTable()
	tbl.Width = 40
	tbl.Height = 20

	// Mouse release (action=1) should be ignored
	if tbl.HandleMouse(5, 2, 1, 1) {
		t.Error("mouse release should not be handled")
	}
}

func TestTableHandleMouse_EmptyTable(t *testing.T) {
	tbl := NewTable([]Column{{Title: "A", Width: 5}})
	tbl.Width = 40
	tbl.Height = 20

	if tbl.HandleMouse(5, 2, 1, 0) {
		t.Error("click on empty table should not be handled")
	}
}

func TestTableHandleMouse_WithScrollOffset(t *testing.T) {
	tbl := makeTestTable()
	tbl.Width = 40
	tbl.Height = 6 // header + separator + status = 3 overhead, visible = 3
	tbl.Offset = 1 // scrolled down by 1

	// Click on first visible row (which is row index 1 due to offset)
	if !tbl.HandleMouse(5, 2, 1, 0) {
		t.Error("expected click to be handled")
	}
	if tbl.Cursor != 1 {
		t.Errorf("cursor = %d, want 1 (offset=1, clicked first visible)", tbl.Cursor)
	}
}

// --- TabBar.HandleMouse tests ---

func TestTabBarHandleMouse_ClickEachTab(t *testing.T) {
	tb := &TabBar{
		Tabs:   []string{"Repos", "Sessions", "Teams", "Fleet"},
		Active: 0,
	}

	// Tab layout with Padding(0,1): each tab = len(name) + 2
	// "Repos" = 7 chars (pos 0-6), gap at 7
	// "Sessions" = 10 chars (pos 8-17), gap at 18
	// "Teams" = 7 chars (pos 19-25), gap at 26
	// "Fleet" = 7 chars (pos 27-33)

	tests := []struct {
		x       int
		wantIdx int
		wantOk  bool
	}{
		{0, 0, true},   // start of "Repos"
		{3, 0, true},   // middle of "Repos"
		{6, 0, true},   // end of "Repos"
		{7, -1, false},  // gap between tabs
		{8, 1, true},   // start of "Sessions"
		{15, 1, true},  // middle of "Sessions"
		{17, 1, true},  // end of "Sessions"
		{18, -1, false}, // gap
		{19, 2, true},  // start of "Teams"
		{26, -1, false}, // gap
		{27, 3, true},  // start of "Fleet"
		{33, 3, true},  // end of "Fleet"
		{34, -1, false}, // past end
	}

	for _, tt := range tests {
		idx, ok := tb.HandleMouse(tt.x, 0, 1, 0)
		if ok != tt.wantOk {
			t.Errorf("x=%d: ok=%v, want %v", tt.x, ok, tt.wantOk)
		}
		if ok && idx != tt.wantIdx {
			t.Errorf("x=%d: idx=%d, want %d", tt.x, idx, tt.wantIdx)
		}
	}
}

func TestTabBarHandleMouse_WrongRow(t *testing.T) {
	tb := &TabBar{
		Tabs:   []string{"Repos", "Sessions"},
		Active: 0,
	}

	// Click not on y=0 should be ignored
	_, ok := tb.HandleMouse(3, 1, 1, 0)
	if ok {
		t.Error("click on wrong row should not be handled")
	}
}

func TestTabBarHandleMouse_RightClick(t *testing.T) {
	tb := &TabBar{
		Tabs:   []string{"Repos"},
		Active: 0,
	}

	_, ok := tb.HandleMouse(3, 0, 3, 0)
	if ok {
		t.Error("right click should not be handled")
	}
}

func TestTabBarHandleMouse_EmptyTabs(t *testing.T) {
	tb := &TabBar{
		Tabs:   nil,
		Active: 0,
	}

	_, ok := tb.HandleMouse(0, 0, 1, 0)
	if ok {
		t.Error("click on empty tab bar should not be handled")
	}
}

// --- ConfirmDialog.HandleMouse tests ---

func TestConfirmHandleMouse_ClickYes(t *testing.T) {
	d := &ConfirmDialog{
		Title:   "Confirm",
		Message: "Do it?",
		Action:  "test",
		Active:  true,
	}

	// "Yes" button starts at x=2, width=5 (x: 2-6)
	result, ok := d.HandleMouse(3, 4, 1, 0)
	if !ok {
		t.Fatal("expected click on Yes to be handled")
	}
	if result.Result != ConfirmYes {
		t.Errorf("result = %d, want ConfirmYes", result.Result)
	}
	if result.Action != "test" {
		t.Errorf("action = %q, want test", result.Action)
	}
}

func TestConfirmHandleMouse_ClickNo(t *testing.T) {
	d := &ConfirmDialog{
		Title:   "Confirm",
		Message: "Do it?",
		Action:  "test",
		Active:  true,
	}

	// "No" button starts at x=9 (2+5+2), width=4 (x: 9-12)
	result, ok := d.HandleMouse(10, 4, 1, 0)
	if !ok {
		t.Fatal("expected click on No to be handled")
	}
	if result.Result != ConfirmNo {
		t.Errorf("result = %d, want ConfirmNo", result.Result)
	}
}

func TestConfirmHandleMouse_ClickCancel(t *testing.T) {
	d := &ConfirmDialog{
		Title:   "Confirm",
		Message: "Do it?",
		Action:  "test",
		Active:  true,
	}

	// "Cancel" button starts at x=15 (2+5+2+4+2), width=8 (x: 15-22)
	result, ok := d.HandleMouse(17, 4, 1, 0)
	if !ok {
		t.Fatal("expected click on Cancel to be handled")
	}
	if result.Result != ConfirmCancel {
		t.Errorf("result = %d, want ConfirmCancel", result.Result)
	}
}

func TestConfirmHandleMouse_ClickOutside(t *testing.T) {
	d := &ConfirmDialog{
		Title:   "Confirm",
		Message: "Do it?",
		Action:  "test",
		Active:  true,
	}

	// Click outside button area
	_, ok := d.HandleMouse(50, 4, 1, 0)
	if ok {
		t.Error("click outside buttons should not be handled")
	}
	if !d.Active {
		t.Error("dialog should remain active after click outside buttons")
	}
}

func TestConfirmHandleMouse_Inactive(t *testing.T) {
	d := &ConfirmDialog{
		Title:  "Confirm",
		Active: false,
	}

	_, ok := d.HandleMouse(3, 4, 1, 0)
	if ok {
		t.Error("inactive dialog should not handle clicks")
	}
}

func TestConfirmHandleMouse_RightClick(t *testing.T) {
	d := &ConfirmDialog{
		Title:  "Confirm",
		Active: true,
	}

	_, ok := d.HandleMouse(3, 4, 3, 0)
	if ok {
		t.Error("right click should not be handled")
	}
}
