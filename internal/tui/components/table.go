package components

import (
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// Column defines a table column.
type Column struct {
	Title    string
	Width    int
	Sortable bool
	Grow     bool // if true, this column expands to fill remaining width
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
	filtered     []int        // indices into Rows matching filter
	EmptyMessage string       // shown when no rows match
	StatusColumn int          // column index for status-prefix filtering (-1 = disabled)
	MultiSelect  bool         // enable multi-select mode
	Selected     map[int]bool // selected row indices (into Rows)
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
	col := t.SortCol
	asc := t.SortAsc
	sort.SliceStable(t.Rows, func(i, j int) bool {
		a, b := StripAnsi(t.Rows[i][col]), StripAnsi(t.Rows[j][col])
		if asc {
			return a < b
		}
		return a > b
	})
	t.applyFilter()
}

// effectiveColumns returns columns with Grow widths resolved.
func (t *Table) effectiveColumns() []Column {
	cols := make([]Column, len(t.Columns))
	copy(cols, t.Columns)

	if t.Width <= 0 {
		return cols
	}

	growIdx := -1
	fixedWidth := 0
	for i, col := range cols {
		if col.Grow {
			growIdx = i
		} else {
			fixedWidth += col.Width
		}
	}
	if growIdx >= 0 {
		gaps := len(cols) - 1 // spaces between columns
		growWidth := t.Width - fixedWidth - gaps
		if growWidth < cols[growIdx].Width {
			growWidth = cols[growIdx].Width
		}
		cols[growIdx].Width = growWidth
	}
	return cols
}

// View renders the table.
func (t *Table) View() string {
	var b strings.Builder
	cols := t.effectiveColumns()

	// Header
	var hdr []string
	for i, col := range cols {
		title := col.Title
		if i == t.SortCol && t.Columns[i].Sortable {
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
	end := t.Offset + visible
	if end > len(t.filtered) {
		end = len(t.filtered)
	}

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

		var cells []string
		for ci, col := range cols {
			cell := ""
			if ci < len(row) {
				cell = row[ci]
			}
			padded := visualPad(cell, col.Width)
			if vi == t.Cursor {
				padded = styles.SelectedStyle.Render(padded)
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// StyledCell wraps a cell value with a lipgloss style.
func StyledCell(style lipgloss.Style, val string) string {
	return style.Render(val)
}
