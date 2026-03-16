package components

import (
	"fmt"
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
}

// Row is a slice of styled cell strings.
type Row []string

// Table is a sortable, navigable table component.
type Table struct {
	Columns   []Column
	Rows      []Row
	Cursor    int
	SortCol   int
	SortAsc   bool
	Width     int
	Height    int
	Offset    int // scroll offset
	Filter    string
	filtered  []int // indices into Rows matching filter
}

// NewTable creates a table with the given columns.
func NewTable(cols []Column) *Table {
	return &Table{
		Columns: cols,
		SortAsc: true,
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
	for i, row := range t.Rows {
		if f == "" {
			t.filtered = append(t.filtered, i)
			continue
		}
		for _, cell := range row {
			if strings.Contains(strings.ToLower(cell), f) {
				t.filtered = append(t.filtered, i)
				break
			}
		}
	}
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
		a, b := t.Rows[i][col], t.Rows[j][col]
		if asc {
			return a < b
		}
		return a > b
	})
	t.applyFilter()
}

// View renders the table.
func (t *Table) View() string {
	var b strings.Builder

	// Header
	var hdr []string
	for i, col := range t.Columns {
		title := col.Title
		if i == t.SortCol && col.Sortable {
			if t.SortAsc {
				title += " ▲"
			} else {
				title += " ▼"
			}
		}
		hdr = append(hdr, styles.HeaderStyle.Render(fmt.Sprintf("%-*s", col.Width, title)))
	}
	b.WriteString(strings.Join(hdr, " "))
	b.WriteRune('\n')

	// Separator
	for i, col := range t.Columns {
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
		var cells []string
		for ci, col := range t.Columns {
			cell := ""
			if ci < len(row) {
				cell = row[ci]
			}
			padded := fmt.Sprintf("%-*s", col.Width, truncate(cell, col.Width))
			if vi == t.Cursor {
				padded = styles.SelectedStyle.Render(padded)
			}
			cells = append(cells, padded)
		}
		b.WriteString(strings.Join(cells, " "))
		b.WriteRune('\n')
	}

	if len(t.filtered) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No repos found"))
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
