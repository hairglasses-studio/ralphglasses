package components

import (
	"testing"
)

// FuzzTableView exercises Table.View with random column counts, row data,
// widths, and sort configurations to catch panics and out-of-bounds.
func FuzzTableView(f *testing.F) {
	// Seed corpus.
	f.Add(3, 5, 80, 20, 0, true, "")
	f.Add(0, 0, 40, 10, 0, false, "")
	f.Add(1, 1, 120, 30, 0, true, "filter")
	f.Add(10, 100, 200, 50, 5, false, "x")

	f.Fuzz(func(t *testing.T, numCols, numRows, width, height, sortCol int, sortAsc bool, filter string) {
		if numCols < 0 || numCols > 50 {
			return
		}
		if numRows < 0 || numRows > 200 {
			return
		}
		if width < 0 {
			width = 0
		}
		if height < 0 {
			height = 0
		}

		cols := make([]Column, numCols)
		for i := range cols {
			cols[i] = Column{
				Title:    "Col",
				Width:    10,
				Sortable: true,
			}
		}

		rows := make([]Row, numRows)
		for i := range rows {
			row := make(Row, numCols)
			for j := range row {
				row[j] = "cell"
			}
			rows[i] = row
		}

		tbl := NewTable(cols)
		tbl.SetRows(rows)
		tbl.Width = width
		tbl.Height = height
		tbl.SortCol = sortCol
		tbl.SortAsc = sortAsc
		if filter != "" {
			tbl.SetFilter(filter)
		}

		// Must not panic.
		_ = tbl.View()
	})
}

// FuzzTableSort exercises sort with random SortCol values.
func FuzzTableSort(f *testing.F) {
	f.Add(3, 10, 0)
	f.Add(3, 10, 5)
	f.Add(0, 0, -1)

	f.Fuzz(func(t *testing.T, numCols, numRows, sortCol int) {
		if numCols < 0 || numCols > 20 || numRows < 0 || numRows > 50 {
			return
		}

		cols := make([]Column, numCols)
		for i := range cols {
			cols[i] = Column{Title: "C", Width: 8, Sortable: true}
		}

		rows := make([]Row, numRows)
		for i := range rows {
			row := make(Row, numCols)
			for j := range row {
				row[j] = "val"
			}
			rows[i] = row
		}

		tbl := NewTable(cols)
		tbl.SetRows(rows)
		tbl.SortCol = sortCol

		// CycleSort must not panic.
		tbl.CycleSort()
		_ = tbl.View()
	})
}
