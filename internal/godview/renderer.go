package godview

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/term"
)

// Renderer handles raw ANSI output for maximum throughput.
// Uses main screen buffer (not alternate screen) to preserve scrollback.
type Renderer struct {
	buf    *bufio.Writer
	width  int
	height int
	mu     sync.Mutex
}

// NewRenderer creates a renderer writing to stdout.
func NewRenderer() *Renderer {
	r := &Renderer{
		buf: bufio.NewWriterSize(os.Stdout, 32*1024), // 32KB buffer
	}
	r.updateSize()

	// Handle terminal resize
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, syscall.SIGWINCH)
	go func() {
		for range sigwinch {
			r.mu.Lock()
			r.updateSize()
			r.mu.Unlock()
		}
	}()

	return r
}

func (r *Renderer) updateSize() {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		r.width, r.height = 80, 24
		return
	}
	r.width, r.height = w, h
}

// Size returns the current terminal dimensions.
func (r *Renderer) Size() (width, height int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.width, r.height
}

// Render draws the complete God view frame.
func (r *Renderer) Render(state *State) {
	r.mu.Lock()
	defer r.mu.Unlock()

	w, h := r.width, r.height
	r.buf.WriteString(CursorHome)
	r.buf.WriteString(CursorHide)

	// Header (2 lines)
	r.renderHeader(state, w)

	// Separator
	r.writeSep(w)

	// Table (adaptive rows)
	tableRows := h - 8 // header(2) + sep(1) + footer(1) + sep(2) + log area
	logRows := 6
	if h > 40 {
		tableRows = (h - 6) / 2
		logRows = h - 6 - tableRows
	}
	if tableRows < 3 {
		tableRows = 3
	}
	if logRows < 2 {
		logRows = 2
	}
	r.renderTable(state, w, tableRows)

	// Separator
	r.writeSep(w)

	// Live output area
	r.renderLiveOutput(state, w, logRows)

	// Separator
	r.writeSep(w)

	// Cost footer (1 line)
	r.renderCostBar(state, w)

	r.buf.Flush()
}

func (r *Renderer) renderHeader(s *State, w int) {
	now := time.Now().Format("15:04:05")

	// Line 1: summary
	r.buf.WriteString(ClearLine)
	fmt.Fprintf(r.buf, "%s RALPH GODVIEW%s │ ", Header, Reset)
	fmt.Fprintf(r.buf, "%s■%s %d repos  ", Bold, Reset, s.TotalRepos)
	fmt.Fprintf(r.buf, "%s✓%s%d ok  ", StatusOK, Reset, s.ReposOK)
	if s.ReposWarn > 0 {
		fmt.Fprintf(r.buf, "%s⚠%s%d warn  ", StatusWarn, Reset, s.ReposWarn)
	}
	if s.ReposErr > 0 {
		fmt.Fprintf(r.buf, "%s✗%s%d err  ", StatusErr, Reset, s.ReposErr)
	}
	fmt.Fprintf(r.buf, "│ %d agents ", s.ActiveAgents)
	for prov, count := range s.AgentsByProvider {
		if count > 0 {
			fmt.Fprintf(r.buf, "%s%d×%s%s ", ProviderColor(prov), count, prov, Reset)
		}
	}
	fmt.Fprintf(r.buf, "│ %s%s%s │ %s\n",
		costColor(s.TotalCost), FormatCost(s.TotalCost), Reset, now)
}

func (r *Renderer) renderTable(s *State, w, maxRows int) {
	// Header row
	r.buf.WriteString(ClearLine)
	fmt.Fprintf(r.buf, "%s%-16s %-8s %-7s %5s %7s %7s %-30s %s%s\n",
		Dim, "REPO", "AGENT", "STATUS", "TURNS", "$/HR", "COST", "TASK", "PROGRESS", Reset)

	shown := 0
	for _, repo := range s.Repos {
		if shown >= maxRows {
			break
		}
		r.buf.WriteString(ClearLine)

		provCol := ProviderColor(repo.Provider)
		statCol := StatusColor(repo.Status)
		icon := StatusIcon(repo.Status)

		provider := repo.Provider
		if provider == "" {
			provider = "--"
			provCol = StatusIdle
		}

		turns := "--"
		if repo.Turns > 0 {
			turns = fmt.Sprintf("%d", repo.Turns)
		}

		rate := FormatRate(repo.CostPerHr)
		cost := FormatCost(repo.TotalCost)
		task := Truncate(repo.CurrentTask, 30)
		if task == "" {
			task = "--"
		}

		progress := "--"
		if repo.Progress > 0 {
			progress = fmt.Sprintf("%s %3.0f%%", ProgressBar(repo.Progress, 5), repo.Progress)
		}
		if repo.Status == "completed" || repo.Status == "done" {
			progress = StatusDone + "done" + Reset
		}

		fmt.Fprintf(r.buf, "%-16s %s%-8s%s %s%s%-6s%s %5s %7s %7s %-30s %s\n",
			Truncate(repo.Name, 16),
			provCol, PadRight(provider, 8), Reset,
			statCol, icon, PadRight(repo.Status, 6), Reset,
			PadLeft(turns, 5),
			PadLeft(rate, 7),
			PadLeft(cost, 7),
			task,
			progress,
		)
		shown++
	}

	// Fill remaining rows
	for i := shown; i < maxRows; i++ {
		r.buf.WriteString(ClearLine)
		r.buf.WriteByte('\n')
	}
}

func (r *Renderer) renderLiveOutput(s *State, w, maxLines int) {
	r.buf.WriteString(ClearLine)
	fmt.Fprintf(r.buf, "%sLIVE OUTPUT%s\n", Dim, Reset)

	start := len(s.LiveLines) - maxLines + 1
	if start < 0 {
		start = 0
	}
	shown := 0
	for i := start; i < len(s.LiveLines) && shown < maxLines-1; i++ {
		line := s.LiveLines[i]
		r.buf.WriteString(ClearLine)

		provCol := ProviderColor(line.Provider)
		ts := line.Timestamp.Format("15:04:05")
		text := Truncate(line.Text, w-30)

		fmt.Fprintf(r.buf, "%s[%s/%s %s]%s %s\n",
			provCol, line.Provider, Truncate(line.Repo, 12), ts, Reset, text)
		shown++
	}

	// Fill remaining
	for i := shown; i < maxLines-1; i++ {
		r.buf.WriteString(ClearLine)
		r.buf.WriteByte('\n')
	}
}

func (r *Renderer) renderCostBar(s *State, w int) {
	r.buf.WriteString(ClearLine)
	fmt.Fprintf(r.buf, "%sCOST:%s ", Dim, Reset)

	for prov, cost := range s.CostByProvider {
		if cost > 0 {
			pct := 0.0
			if s.TotalCost > 0 {
				pct = cost / s.TotalCost * 100
			}
			col := ProviderColor(prov)
			fmt.Fprintf(r.buf, "%s%s %s (%.0f%%)%s │ ", col, prov, FormatCost(cost), pct, Reset)
		}
	}
	fmt.Fprintf(r.buf, "rate: %s │ cap: %s",
		FormatRate(s.CostRatePerHr), FormatCost(s.BudgetCap))
	r.buf.WriteByte('\n')
}

func (r *Renderer) writeSep(w int) {
	r.buf.WriteString(ClearLine)
	r.buf.WriteString(Border)
	r.buf.WriteString(strings.Repeat("─", w))
	r.buf.WriteString(Reset)
	r.buf.WriteByte('\n')
}

// Cleanup restores terminal state.
func (r *Renderer) Cleanup() {
	r.buf.WriteString(CursorShow)
	r.buf.Flush()
}

func costColor(usd float64) string {
	switch {
	case usd > 20:
		return CostHi
	case usd > 5:
		return CostMid
	default:
		return CostLo
	}
}
