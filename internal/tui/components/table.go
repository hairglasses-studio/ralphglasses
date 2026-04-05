package components

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// Column defines a table column.
type Column struct {
	Title    string
	Width    int // base/min width (kept for backward compat)
	Sortable bool
	Grow     bool    // if true, this column expands to fill remaining width (legacy; prefer Flex)
	MinWidth int     // minimum width (0 = use Width as min)
	MaxWidth int     // maximum width (0 = unlimited)
	Flex     float64 // flex weight for proportional sizing (0 = fixed)
	Priority int     // 0 = always shown, 1 = hide when width < 100, 2 = hide when width < 140
}

// Row is a slice of styled cell strings.
type Row []string

// Table is a sortable, navigable table component.
type Table struct {
	Columns      []Column
	Rows         []Row
	Cursor       int
	SortCol      int
	SortAsc      bool
	Width        int
	Height       int
	Offset       int // scroll offset
	Filter       string
	filtered     []int                                       // indices into Rows matching filter
	EmptyMessage string                                      // shown when no rows match
	StatusColumn int                                         // column index for status-prefix filtering (-1 = disabled)
	MultiSelect  bool                                        // enable multi-select mode
	Selected     map[int]bool                                // selected row indices (into Rows)
	RowStyleFunc func(row Row, selected bool) lipgloss.Style // optional per-row styling
}

// NewTable creates a table with the given columns.
func NewTable(cols []Column) *Table {
	return &Table{
		Columns:      cols,
		SortAsc:      true,
		StatusColumn: -1,
	}
}

// SetRows replaces all rows and resets filter.
func (t *Table) SetRows(rows []Row) {
	t.Rows = rows
	t.applyFilter()
	if t.Cursor >= len(t.filtered) {
		t.Cursor = max(0, len(t.filtered)-1)
	}
}

// SetFilter sets the search filter and refilters rows.
func (t *Table) SetFilter(f string) {
	t.Filter = f
	t.applyFilter()
	t.Cursor = 0
	t.Offset = 0
}

func (t *Table) applyFilter() {
	t.filtered = nil
	f := strings.ToLower(t.Filter)
	prefix, remainder := parseStatusFilter(f)

	for i, row := range t.Rows {
		if f == "" {
			t.filtered = append(t.filtered, i)
			continue
		}

		// Status-prefix filtering (strip ANSI for matching)
		if prefix != "" && t.StatusColumn >= 0 && t.StatusColumn < len(row) {
			cell := strings.ToLower(StripAnsi(row[t.StatusColumn]))
			if !strings.Contains(cell, remainder) {
				continue
			}
			t.filtered = append(t.filtered, i)
			continue
		}

		// Standard text matching (strip ANSI for matching)
		for _, cell := range row {
			if strings.Contains(strings.ToLower(StripAnsi(cell)), f) {
				t.filtered = append(t.filtered, i)
				break
			}
		}
	}
}

// parseStatusFilter extracts a prefix operator from filter text.
// Prefixes: ! = status, @ = provider, # = circuit state
func parseStatusFilter(text string) (prefix, remainder string) {
	if len(text) < 2 {
		return "", text
	}
	switch text[0] {
	case '!', '@', '#':
		return string(text[0]), text[1:]
	}
	return "", text
}

// ToggleSelect toggles the selection state of the current row.
func (t *Table) ToggleSelect() {
	if len(t.filtered) == 0 || t.Cursor >= len(t.filtered) {
		return
	}
	if t.Selected == nil {
		t.Selected = make(map[int]bool)
	}
	idx := t.filtered[t.Cursor]
	if t.Selected[idx] {
		delete(t.Selected, idx)
	} else {
		t.Selected[idx] = true
	}
}

// SelectAll selects all visible (filtered) rows.
func (t *Table) SelectAll() {
	if t.Selected == nil {
		t.Selected = make(map[int]bool)
	}
	for _, idx := range t.filtered {
		t.Selected[idx] = true
	}
}

// ClearSelection clears all selections.
func (t *Table) ClearSelection() {
	t.Selected = nil
}

// SelectedRows returns all currently selected rows.
func (t *Table) SelectedRows() []Row {
	if len(t.Selected) == 0 {
		return nil
	}
	var rows []Row
	for idx := range t.Selected {
		if idx < len(t.Rows) {
			rows = append(rows, t.Rows[idx])
		}
	}
	return rows
}

// HasSelection returns true if any rows are selected.
func (t *Table) HasSelection() bool {
	return len(t.Selected) > 0
}

// SelectedRow returns the currently selected row, or nil.
func (t *Table) SelectedRow() Row {
	if len(t.filtered) == 0 || t.Cursor >= len(t.filtered) {
		return nil
	}
	return t.Rows[t.filtered[t.Cursor]]
}

// MoveUp moves the cursor up.
func (t *Table) MoveUp() {
	if t.Cursor > 0 {
		t.Cursor--
		if t.Cursor < t.Offset {
			t.Offset = t.Cursor
		}
	}
}

// MoveDown moves the cursor down.
func (t *Table) MoveDown() {
	if t.Cursor < len(t.filtered)-1 {
		t.Cursor++
		visibleRows := t.visibleRows()
		if t.Cursor >= t.Offset+visibleRows {
			t.Offset = t.Cursor - visibleRows + 1
		}
	}
}

func (t *Table) visibleRows() int {
	if t.Height <= 3 {
		return 5
	}
	return t.Height - 3 // header + separator + status
}

// CycleSort advances the sort column among sortable columns.
func (t *Table) CycleSort() {
	if len(t.Columns) == 0 {
		return
	}
	if t.SortCol >= len(t.Columns) {
		t.SortCol = len(t.Columns) - 1
	}
	start := t.SortCol
	for {
		t.SortCol = (t.SortCol + 1) % len(t.Columns)
		if t.Columns[t.SortCol].Sortable || t.SortCol == start {
			break
		}
	}
	if t.SortCol == start {
		t.SortAsc = !t.SortAsc
	} else {
		t.SortAsc = true
	}
	t.sortRows()
}

func (t *Table) sortRows() {
	if len(t.Columns) == 0 || len(t.Rows) == 0 {
		return
	}
	col := t.SortCol
	if col >= len(t.Columns) {
		col = len(t.Columns) - 1
		t.SortCol = col
	}
	asc := t.SortAsc
	sort.SliceStable(t.Rows, func(i, j int) bool {
		// Guard against rows with fewer cells than expected.
		var a, b string
		if col < len(t.Rows[i]) {
			a = StripAnsi(t.Rows[i][col])
		}
		if col < len(t.Rows[j]) {
			b = StripAnsi(t.Rows[j][col])
		}
		if asc {
			return a < b
		}
		return a > b
	})
	t.applyFilter()
}

// visibleColumnIndices returns the indices of columns visible at the current width.
// Columns with Priority 0 (default) are always shown. Priority 1 columns hide
// below 100 cols, Priority 2 columns hide below 140 cols.
func (t *Table) visibleColumnIndices() []int {
	var indices []int
	for i, col := range t.Columns {
		switch col.Priority {
		case 2:
			if t.Width >= 140 {
				indices = append(indices, i)
			}
		case 1:
			if t.Width >= 100 {
				indices = append(indices, i)
			}
		default:
			indices = append(indices, i)
		}
	}
	// Fallback: if all columns hidden, show priority-0 columns at minimum.
	if len(indices) == 0 {
		for i, col := range t.Columns {
			if col.Priority == 0 {
				indices = append(indices, i)
			}
		}
	}
	return indices
}

// effectiveWidths computes effective column widths based on Flex weights,
// MinWidth/MaxWidth constraints, and the table's available Width.
// Only visible columns (based on Priority) are included.
func (t *Table) effectiveWidths() []int {
	vis := t.visibleColumnIndices()
	n := len(vis)
	if n == 0 {
		return nil
	}

	widths := make([]int, n)
	flexWeights := make([]float64, n)

	// 1. Determine base width and flex weight for each visible column.
	for vi, ci := range vis {
		col := t.Columns[ci]
		base := col.Width
		if col.MinWidth > 0 && col.MinWidth > base {
			base = col.MinWidth
		}
		widths[vi] = base

		flex := col.Flex
		if col.Grow && flex == 0 {
			flex = 1.0
		}
		flexWeights[vi] = flex
	}

	if t.Width <= 0 {
		return widths
	}

	// 2. Calculate total fixed width + gaps (1 space between columns).
	gaps := n - 1
	totalFixed := gaps
	for _, w := range widths {
		totalFixed += w
	}

	remaining := t.Width - totalFixed
	if remaining <= 0 {
		return widths
	}

	// 3. Distribute remaining space proportionally by flex weight.
	capped := make([]bool, n)
	for range n {
		totalFlex := 0.0
		for vi, fw := range flexWeights {
			if fw > 0 && !capped[vi] {
				totalFlex += fw
			}
		}
		if totalFlex == 0 {
			break
		}

		overflow := 0
		allFit := true
		for vi, fw := range flexWeights {
			if fw <= 0 || capped[vi] {
				continue
			}
			extra := int(float64(remaining) * fw / totalFlex)
			proposed := widths[vi] + extra

			col := t.Columns[vis[vi]]
			if col.MaxWidth > 0 && proposed > col.MaxWidth {
				overflow += proposed - col.MaxWidth
				widths[vi] = col.MaxWidth
				capped[vi] = true
				allFit = false
			} else {
				widths[vi] = proposed
			}
		}

		if allFit || overflow == 0 {
			break
		}
		remaining = overflow
	}

	return widths
}

// effectiveColumns returns visible columns with widths resolved via effectiveWidths.
// The returned colMap maps visible column index → original Columns index (for row cell lookup).
func (t *Table) effectiveColumnsWithMap() ([]Column, []int) {
	vis := t.visibleColumnIndices()
	widths := t.effectiveWidths()

	cols := make([]Column, len(vis))
	for vi, ci := range vis {
		cols[vi] = t.Columns[ci]
		if vi < len(widths) {
			cols[vi].Width = widths[vi]
		}
	}
	return cols, vis
}

// effectiveColumns returns columns with widths resolved via effectiveWidths.
func (t *Table) effectiveColumns() []Column {
	cols, _ := t.effectiveColumnsWithMap()
	return cols
}

// View renders the table.
func (t *Table) View() string {
	var b strings.Builder
	cols, colMap := t.effectiveColumnsWithMap()

	// Clamp SortCol to valid range before rendering.
	if len(t.Columns) > 0 && t.SortCol >= len(t.Columns) {
		t.SortCol = 0
	}

	// Header
	var hdr []string
	for vi, col := range cols {
		title := col.Title
		origIdx := colMap[vi]
		if origIdx == t.SortCol && origIdx < len(t.Columns) && t.Columns[origIdx].Sortable {
			if t.SortAsc {
				title += " ▲"
			} else {
				title += " ▼"
			}
		}
		hdr = append(hdr, styles.HeaderStyle.Render(visualPad(title, col.Width)))
	}
	b.WriteString(strings.Join(hdr, " "))
	b.WriteRune('\n')

	// Separator
	for i, col := range cols {
		if i > 0 {
			b.WriteRune(' ')
		}
		b.WriteString(styles.InfoStyle.Render(strings.Repeat("─", col.Width)))
	}
	b.WriteRune('\n')

	// Rows
	visible := t.visibleRows()
	end := min(t.Offset+visible, len(t.filtered))

	for vi := t.Offset; vi < end; vi++ {
		idx := t.filtered[vi]
		row := t.Rows[idx]

		// Multi-select indicator
		if t.MultiSelect {
			if t.Selected != nil && t.Selected[idx] {
				b.WriteString(styles.StatusRunning.Render("[x]"))
			} else {
				b.WriteString(styles.InfoStyle.Render("[ ]"))
			}
		}

		isSelected := vi == t.Cursor
		var rowStyle lipgloss.Style
		hasRowStyle := false
		if t.RowStyleFunc != nil && !isSelected {
			rowStyle = t.RowStyleFunc(row, isSelected)
			hasRowStyle = true
		}

		var cells []string
		for vi, col := range cols {
			origIdx := colMap[vi]
			cell := ""
			if origIdx < len(row) {
				cell = row[origIdx]
			}
			padded := visualPad(cell, col.Width)
			if isSelected {
				padded = styles.SelectedStyle.Render(padded)
			} else if hasRowStyle {
				padded = rowStyle.Render(padded)
			}
			cells = append(cells, padded)
		}
		b.WriteString(strings.Join(cells, " "))
		b.WriteRune('\n')
	}

	if len(t.filtered) == 0 {
		msg := t.EmptyMessage
		if msg == "" {
			msg = "No items"
		}
		b.WriteString(styles.InfoStyle.Render("  " + msg))
		b.WriteRune('\n')
	}

	return b.String()
}

// RowCount returns the number of visible (filtered) rows.
func (t *Table) RowCount() int {
	return len(t.filtered)
}

func truncate(s string, maxW int) string {
	r := []rune(s)
	if len(r) <= maxW {
		return s
	}
	if maxW <= 3 {
		return string(r[:maxW])
	}
	return string(r[:maxW-1]) + "…"
}

// visualPad truncates (ANSI-aware) then pads to the given visual width.
func visualPad(s string, width int) string {
	vw := VisualWidth(s)
	if vw > width {
		s = VisualTruncate(s, width)
		vw = VisualWidth(s)
	}
	if vw < width {
		s += strings.Repeat(" ", width-vw)
	}
	return s
}

// HandleMouse processes a mouse event for the table.
// On left-click, it calculates which row was clicked based on the Y offset
// from the table header (2 rows: header + separator) and sets the cursor.
// Returns true if the click was handled (i.e., a valid row was clicked).
func (t *Table) HandleMouse(x, y int, button, action int) bool {
	// Only handle left-click press events.
	// button 1 = MouseButtonLeft, action 0 = MouseActionPress
	if button != 1 || action != 0 {
		return false
	}

	// The table renders: header (row 0), separator (row 1), then data rows.
	headerRows := 2
	clickedRow := y - headerRows

	if clickedRow < 0 {
		return false
	}

	// Convert to absolute row index (accounting for scroll offset).
	absIdx := t.Offset + clickedRow

	if absIdx < 0 || absIdx >= len(t.filtered) {
		return false
	}

	// Ensure within visible range.
	visible := t.visibleRows()
	if clickedRow >= visible {
		return false
	}

	t.Cursor = absIdx
	return true
}

// StyledCell wraps a cell value with a lipgloss style.
func StyledCell(style lipgloss.Style, val string) string {
	return style.Render(val)
}
