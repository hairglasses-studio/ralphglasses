package views

import (
	"fmt"
	"path/filepath"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// LoopListColumns defines the loop list table structure.
var LoopListColumns = []components.Column{
	{Title: "ID", Width: 10, Sortable: false, MinWidth: 9},
	{Title: "Repo", Width: 20, Sortable: true, MinWidth: 12, Flex: 2.0},
	{Title: "Phase", Width: 12, Sortable: true, MinWidth: 8, Flex: 0.5},
	{Title: "Iters", Width: 6, Sortable: true, MinWidth: 5},
	{Title: "Status", Width: 10, Sortable: true, MinWidth: 8},
}

// NewLoopListTable creates a table pre-configured for the loop list.
func NewLoopListTable() *components.Table {
	t := components.NewTable(LoopListColumns)
	t.EmptyMessage = "No active loops"
	t.StatusColumn = 4
	return t
}

// LoopRunsToRows converts loop runs to table rows with styled cells.
func LoopRunsToRows(loops []*session.LoopRun, tickFrame int) []components.Row {
	rows := make([]components.Row, 0, len(loops))
	for _, l := range loops {
		l.Lock()
		id := l.ID
		if len(id) > 8 {
			id = id[:8]
		}
		repo := l.RepoName
		if repo == "" {
			repo = filepath.Base(l.RepoPath)
		}
		status := l.Status
		paused := l.Paused
		iterCount := len(l.Iterations)
		phase := "-"
		if iterCount > 0 {
			phase = l.Iterations[iterCount-1].Status
		}
		l.Unlock()

		if paused && status == "running" {
			status = "paused"
		}
		isActive := status == "running"
		statusCell := fmt.Sprintf("%s%s %s",
			components.ActivityDot(isActive, tickFrame),
			styles.StatusIcon(status),
			status)

		rows = append(rows, components.Row{
			id,
			repo,
			phase,
			fmt.Sprintf("%d", iterCount),
			statusCell,
		})
	}
	return rows
}
