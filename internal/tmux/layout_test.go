package tmux

import (
	"fmt"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Layout.String / ParseLayout
// ---------------------------------------------------------------------------

func TestLayoutString(t *testing.T) {
	tests := []struct {
		l    Layout
		want string
	}{
		{LayoutSingle, "single"},
		{LayoutDual, "dual"},
		{LayoutQuad, "quad"},
		{LayoutFleet, "fleet"},
		{Layout(99), "layout(99)"},
	}
	for _, tt := range tests {
		if got := tt.l.String(); got != tt.want {
			t.Errorf("Layout(%d).String() = %q, want %q", int(tt.l), got, tt.want)
		}
	}
}

func TestParseLayout(t *testing.T) {
	good := []struct {
		input string
		want  Layout
	}{
		{"single", LayoutSingle},
		{"DUAL", LayoutDual},
		{" Quad ", LayoutQuad},
		{"fleet", LayoutFleet},
	}
	for _, tt := range good {
		got, err := ParseLayout(tt.input)
		if err != nil {
			t.Errorf("ParseLayout(%q) unexpected error: %v", tt.input, err)
		}
		if got != tt.want {
			t.Errorf("ParseLayout(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}

	if _, err := ParseLayout("unknown"); err == nil {
		t.Error("ParseLayout(\"unknown\") expected error, got nil")
	}
}

// ---------------------------------------------------------------------------
// Pane helpers
// ---------------------------------------------------------------------------

func TestPaneEdges(t *testing.T) {
	p := Pane{X: 10, Y: 5, W: 80, H: 24}
	if p.Right() != 90 {
		t.Errorf("Right() = %d, want 90", p.Right())
	}
	if p.Bottom() != 29 {
		t.Errorf("Bottom() = %d, want 29", p.Bottom())
	}
}

// ---------------------------------------------------------------------------
// gridDimensions
// ---------------------------------------------------------------------------

func TestGridDimensions(t *testing.T) {
	tests := []struct {
		n        int
		wantR    int
		wantC    int
	}{
		{0, 0, 0},
		{1, 1, 1},
		{2, 1, 2},
		{3, 2, 2},
		{4, 2, 2},
		{5, 2, 3},
		{6, 2, 3},
		{7, 3, 3},
		{8, 3, 3},
		{9, 3, 3},
		{10, 3, 4},
		{16, 4, 4},
	}
	for _, tt := range tests {
		r, c := gridDimensions(tt.n)
		if r != tt.wantR || c != tt.wantC {
			t.Errorf("gridDimensions(%d) = (%d, %d), want (%d, %d)",
				tt.n, r, c, tt.wantR, tt.wantC)
		}
	}
}

// ---------------------------------------------------------------------------
// ComputeGeometry — error cases
// ---------------------------------------------------------------------------

func TestComputeGeometry_Errors(t *testing.T) {
	if _, err := ComputeGeometry(LayoutSingle, 0, 50, 0); err == nil {
		t.Error("expected error for zero width")
	}
	if _, err := ComputeGeometry(LayoutSingle, 200, -1, 0); err == nil {
		t.Error("expected error for negative height")
	}
	if _, err := ComputeGeometry(LayoutFleet, 200, 50, 0); err == nil {
		t.Error("expected error for fleet with n=0")
	}
	if _, err := ComputeGeometry(Layout(99), 200, 50, 1); err == nil {
		t.Error("expected error for unknown layout")
	}
}

// ---------------------------------------------------------------------------
// ComputeGeometry — Single
// ---------------------------------------------------------------------------

func TestGeometry_Single(t *testing.T) {
	geo, err := ComputeGeometry(LayoutSingle, 200, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(geo.Panes) != 1 {
		t.Fatalf("single layout: got %d panes, want 1", len(geo.Panes))
	}
	p := geo.Panes[0]
	if p.X != 0 || p.Y != 0 {
		t.Errorf("single pane origin = (%d,%d), want (0,0)", p.X, p.Y)
	}
	if p.W != 200 || p.H != 50 {
		t.Errorf("single pane size = %dx%d, want 200x50", p.W, p.H)
	}
	if p.Label != "session-0" {
		t.Errorf("single pane label = %q, want %q", p.Label, "session-0")
	}
}

// ---------------------------------------------------------------------------
// ComputeGeometry — Dual
// ---------------------------------------------------------------------------

func TestGeometry_Dual(t *testing.T) {
	geo, err := ComputeGeometry(LayoutDual, 200, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(geo.Panes) != 2 {
		t.Fatalf("dual layout: got %d panes, want 2", len(geo.Panes))
	}

	left := geo.Panes[0]
	right := geo.Panes[1]

	// Left pane starts at origin.
	if left.X != 0 || left.Y != 0 {
		t.Errorf("left origin = (%d,%d), want (0,0)", left.X, left.Y)
	}
	// Right pane starts after left + separator.
	expectedRightX := left.W + separatorWidth
	if right.X != expectedRightX {
		t.Errorf("right X = %d, want %d", right.X, expectedRightX)
	}
	// Both panes occupy full height.
	if left.H != 50 || right.H != 50 {
		t.Errorf("heights = %d, %d, want 50, 50", left.H, right.H)
	}
	// Total width should equal terminal width.
	totalW := left.W + separatorWidth + right.W
	if totalW != 200 {
		t.Errorf("total width = %d, want 200", totalW)
	}
	// Left pane should be ~60 %.
	ratio := float64(left.W) / float64(left.W+right.W)
	if ratio < 0.55 || ratio > 0.65 {
		t.Errorf("left/right ratio = %.2f, want ~0.60", ratio)
	}

	if left.Label != "session-0" {
		t.Errorf("left label = %q, want %q", left.Label, "session-0")
	}
	if right.Label != "logs" {
		t.Errorf("right label = %q, want %q", right.Label, "logs")
	}
}

// ---------------------------------------------------------------------------
// ComputeGeometry — Quad
// ---------------------------------------------------------------------------

func TestGeometry_Quad(t *testing.T) {
	geo, err := ComputeGeometry(LayoutQuad, 200, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(geo.Panes) != 4 {
		t.Fatalf("quad layout: got %d panes, want 4", len(geo.Panes))
	}

	// Verify total coverage equals terminal area minus separators.
	tl := geo.Panes[0] // top-left
	tr := geo.Panes[1] // top-right
	bl := geo.Panes[2] // bottom-left
	br := geo.Panes[3] // bottom-right

	totalW := tl.W + separatorWidth + tr.W
	totalH := tl.H + separatorWidth + bl.H
	if totalW != 200 {
		t.Errorf("total width = %d, want 200", totalW)
	}
	if totalH != 50 {
		t.Errorf("total height = %d, want 50", totalH)
	}

	// Top-left at origin.
	if tl.X != 0 || tl.Y != 0 {
		t.Errorf("TL origin = (%d,%d)", tl.X, tl.Y)
	}
	// Top-right to the right of TL.
	if tr.X != tl.W+separatorWidth {
		t.Errorf("TR.X = %d, want %d", tr.X, tl.W+separatorWidth)
	}
	// Bottom-left below TL.
	if bl.Y != tl.H+separatorWidth {
		t.Errorf("BL.Y = %d, want %d", bl.Y, tl.H+separatorWidth)
	}
	// Bottom-right correct corner.
	if br.X != bl.W+separatorWidth || br.Y != tr.H+separatorWidth {
		t.Errorf("BR = (%d,%d), want (%d,%d)",
			br.X, br.Y, bl.W+separatorWidth, tr.H+separatorWidth)
	}

	// Labels.
	for i, want := range []string{"session-0", "session-1", "session-2", "session-3"} {
		if geo.Panes[i].Label != want {
			t.Errorf("pane %d label = %q, want %q", i, geo.Panes[i].Label, want)
		}
	}
}

// ---------------------------------------------------------------------------
// ComputeGeometry — Fleet
// ---------------------------------------------------------------------------

func TestGeometry_Fleet_SingleFallback(t *testing.T) {
	geo, err := ComputeGeometry(LayoutFleet, 200, 50, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(geo.Panes) != 1 {
		t.Fatalf("fleet n=1: got %d panes, want 1", len(geo.Panes))
	}
	p := geo.Panes[0]
	if p.W != 200 || p.H != 50 {
		t.Errorf("fleet n=1 size = %dx%d, want 200x50", p.W, p.H)
	}
}

func TestGeometry_Fleet_Six(t *testing.T) {
	geo, err := ComputeGeometry(LayoutFleet, 200, 50, 6)
	if err != nil {
		t.Fatal(err)
	}
	// 6 sessions -> 2 rows x 3 cols
	if len(geo.Panes) != 6 {
		t.Fatalf("fleet n=6: got %d panes, want 6", len(geo.Panes))
	}

	rows, cols := gridDimensions(6)
	if rows != 2 || cols != 3 {
		t.Fatalf("grid 6 = %dx%d, want 2x3", rows, cols)
	}

	// Every pane should have positive dimensions.
	for i, p := range geo.Panes {
		if p.W <= 0 || p.H <= 0 {
			t.Errorf("pane %d has non-positive size %dx%d", i, p.W, p.H)
		}
	}

	// All panes in the same row should share Y coordinate and height.
	row0Y := geo.Panes[0].Y
	for _, p := range geo.Panes[:3] {
		if p.Y != row0Y {
			t.Errorf("row-0 pane %q Y=%d, expected %d", p.Label, p.Y, row0Y)
		}
	}
	row1Y := geo.Panes[3].Y
	for _, p := range geo.Panes[3:] {
		if p.Y != row1Y {
			t.Errorf("row-1 pane %q Y=%d, expected %d", p.Label, p.Y, row1Y)
		}
	}

	// Total width of first row should equal terminal width.
	firstRowW := geo.Panes[0].W + separatorWidth + geo.Panes[1].W + separatorWidth + geo.Panes[2].W
	if firstRowW != 200 {
		t.Errorf("first row total width = %d, want 200", firstRowW)
	}

	// Total height should equal terminal height.
	totalH := geo.Panes[0].H + separatorWidth + geo.Panes[3].H
	if totalH != 50 {
		t.Errorf("total height = %d, want 50", totalH)
	}
}

func TestGeometry_Fleet_Odd(t *testing.T) {
	// 5 sessions -> 2 rows x 3 cols, last row has only 2 panes.
	geo, err := ComputeGeometry(LayoutFleet, 200, 50, 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(geo.Panes) != 5 {
		t.Fatalf("fleet n=5: got %d panes, want 5", len(geo.Panes))
	}
	// Verify labels are sequential.
	for i, p := range geo.Panes {
		want := fmt.Sprintf("session-%d", i)
		if p.Label != want {
			t.Errorf("pane %d label = %q, want %q", i, p.Label, want)
		}
	}
}

func TestGeometry_Fleet_Large(t *testing.T) {
	// 16 sessions -> 4x4 grid
	geo, err := ComputeGeometry(LayoutFleet, 320, 100, 16)
	if err != nil {
		t.Fatal(err)
	}
	if len(geo.Panes) != 16 {
		t.Fatalf("fleet n=16: got %d panes, want 16", len(geo.Panes))
	}

	rows, cols := gridDimensions(16)
	if rows != 4 || cols != 4 {
		t.Fatalf("grid 16 = %dx%d, want 4x4", rows, cols)
	}

	// No pane should exceed terminal bounds.
	for i, p := range geo.Panes {
		if p.Right() > 320 {
			t.Errorf("pane %d right edge %d > terminal width 320", i, p.Right())
		}
		if p.Bottom() > 100 {
			t.Errorf("pane %d bottom edge %d > terminal height 100", i, p.Bottom())
		}
	}
}

// ---------------------------------------------------------------------------
// Geometry: no pane overlap
// ---------------------------------------------------------------------------

func TestGeometry_NoOverlap(t *testing.T) {
	cases := []struct {
		layout Layout
		w, h   int
		n      int
	}{
		{LayoutSingle, 200, 50, 0},
		{LayoutDual, 200, 50, 0},
		{LayoutQuad, 200, 50, 0},
		{LayoutFleet, 200, 50, 3},
		{LayoutFleet, 200, 50, 6},
		{LayoutFleet, 200, 50, 9},
		{LayoutFleet, 320, 100, 12},
	}
	for _, tc := range cases {
		name := fmt.Sprintf("%s_%dx%d_n%d", tc.layout, tc.w, tc.h, tc.n)
		t.Run(name, func(t *testing.T) {
			geo, err := ComputeGeometry(tc.layout, tc.w, tc.h, tc.n)
			if err != nil {
				t.Fatal(err)
			}
			for i := 0; i < len(geo.Panes); i++ {
				for j := i + 1; j < len(geo.Panes); j++ {
					a, b := geo.Panes[i], geo.Panes[j]
					if overlaps(a, b) {
						t.Errorf("pane %d (%v) overlaps pane %d (%v)", i, a, j, b)
					}
				}
			}
		})
	}
}

// overlaps returns true if two panes share any interior area.
func overlaps(a, b Pane) bool {
	if a.X >= b.Right() || b.X >= a.Right() {
		return false
	}
	if a.Y >= b.Bottom() || b.Y >= a.Bottom() {
		return false
	}
	return true
}

// ---------------------------------------------------------------------------
// Geometry: panes within bounds
// ---------------------------------------------------------------------------

func TestGeometry_WithinBounds(t *testing.T) {
	cases := []struct {
		layout Layout
		w, h   int
		n      int
	}{
		{LayoutSingle, 80, 24, 0},
		{LayoutDual, 160, 40, 0},
		{LayoutQuad, 200, 50, 0},
		{LayoutFleet, 200, 50, 2},
		{LayoutFleet, 200, 50, 7},
		{LayoutFleet, 300, 80, 15},
	}
	for _, tc := range cases {
		name := fmt.Sprintf("%s_%dx%d_n%d", tc.layout, tc.w, tc.h, tc.n)
		t.Run(name, func(t *testing.T) {
			geo, err := ComputeGeometry(tc.layout, tc.w, tc.h, tc.n)
			if err != nil {
				t.Fatal(err)
			}
			for i, p := range geo.Panes {
				if p.X < 0 || p.Y < 0 {
					t.Errorf("pane %d has negative origin (%d,%d)", i, p.X, p.Y)
				}
				if p.Right() > tc.w {
					t.Errorf("pane %d right=%d exceeds width=%d", i, p.Right(), tc.w)
				}
				if p.Bottom() > tc.h {
					t.Errorf("pane %d bottom=%d exceeds height=%d", i, p.Bottom(), tc.h)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ApplyLayout with mock Commander
// ---------------------------------------------------------------------------

// mockCommander records calls and returns canned responses.
type mockCommander struct {
	calls [][]string
	// nextPaneID is incremented to generate fake pane IDs.
	nextPaneID int
}

func (m *mockCommander) Run(args ...string) (string, error) {
	m.calls = append(m.calls, args)
	if len(args) > 0 && args[0] == "display-message" {
		return "%0", nil
	}
	if len(args) > 0 && args[0] == "split-window" {
		m.nextPaneID++
		return fmt.Sprintf("%%%d", m.nextPaneID), nil
	}
	return "", nil
}

func TestApplyLayout_Single(t *testing.T) {
	geo, _ := ComputeGeometry(LayoutSingle, 200, 50, 0)
	mc := &mockCommander{}
	ids, err := ApplyLayout(mc, "ralph:0", geo)
	if err != nil {
		t.Fatal(err)
	}
	// Single layout: no splits needed, just the initial pane.
	if len(ids) != 1 {
		t.Fatalf("got %d pane IDs, want 1", len(ids))
	}
	if ids[0] != "%0" {
		t.Errorf("first pane ID = %q, want %%0", ids[0])
	}
	// Should have exactly one call: display-message to get initial pane ID.
	if len(mc.calls) != 1 {
		t.Errorf("got %d calls, want 1", len(mc.calls))
	}
}

func TestApplyLayout_Dual(t *testing.T) {
	geo, _ := ComputeGeometry(LayoutDual, 200, 50, 0)
	mc := &mockCommander{}
	ids, err := ApplyLayout(mc, "ralph:0", geo)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("got %d pane IDs, want 2", len(ids))
	}
	// First call: display-message, second call: split-window.
	if len(mc.calls) != 2 {
		t.Fatalf("got %d calls, want 2", len(mc.calls))
	}
	splitArgs := strings.Join(mc.calls[1], " ")
	if !strings.Contains(splitArgs, "split-window") {
		t.Errorf("second call should be split-window, got %q", splitArgs)
	}
	if !strings.Contains(splitArgs, "-h") {
		t.Errorf("dual split should be horizontal (-h), got %q", splitArgs)
	}
}

func TestApplyLayout_Quad(t *testing.T) {
	geo, _ := ComputeGeometry(LayoutQuad, 200, 50, 0)
	mc := &mockCommander{}
	ids, err := ApplyLayout(mc, "ralph:0", geo)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 4 {
		t.Fatalf("got %d pane IDs, want 4", len(ids))
	}
	// 1 display-message + 3 split-window = 4 calls.
	if len(mc.calls) != 4 {
		t.Fatalf("got %d calls, want 4", len(mc.calls))
	}
}

func TestApplyLayout_Fleet(t *testing.T) {
	geo, _ := ComputeGeometry(LayoutFleet, 200, 50, 6)
	mc := &mockCommander{}
	ids, err := ApplyLayout(mc, "ralph:0", geo)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 6 {
		t.Fatalf("got %d pane IDs, want 6", len(ids))
	}
	// 1 display-message + 5 splits = 6 calls.
	if len(mc.calls) != 6 {
		t.Fatalf("got %d calls, want 6", len(mc.calls))
	}
}

func TestApplyLayout_EmptyGeometry(t *testing.T) {
	geo := &LayoutGeometry{Panes: nil}
	mc := &mockCommander{}
	_, err := ApplyLayout(mc, "ralph:0", geo)
	if err == nil {
		t.Fatal("expected error for empty geometry")
	}
}

// ---------------------------------------------------------------------------
// ApplyLayout — verify split directions
// ---------------------------------------------------------------------------

func TestApplyLayout_SplitDirections(t *testing.T) {
	// Quad layout: pane 1 is same row as 0 (horizontal), pane 2 is new row
	// (vertical), pane 3 is same row as 2 (horizontal).
	geo, _ := ComputeGeometry(LayoutQuad, 200, 50, 0)
	mc := &mockCommander{}
	_, err := ApplyLayout(mc, "test:0", geo)
	if err != nil {
		t.Fatal(err)
	}

	// Calls: [display-message, split(1), split(2), split(3)]
	wantDirs := []string{"-h", "-v", "-h"}
	for i, want := range wantDirs {
		call := mc.calls[i+1] // skip display-message
		found := false
		for _, arg := range call {
			if arg == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("split %d: expected direction %s in args %v", i+1, want, call)
		}
	}
}
