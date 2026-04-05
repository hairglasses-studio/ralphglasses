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

	// Header (1 line + separator)
	r.renderHeader(state, w)
	r.writeHeavySep(w)

	// Calculate layout: give table exactly enough for repos, rest to logs
	repoCount := len(state.Repos)
	if repoCount > 20 {
		repoCount = 20 // Cap table at 20 rows
	}
	tableRows := repoCount + 1 // +1 for column header
	logRows := h - tableRows - 7 // header(1) + hsep(1) + sep(1) + logheader(1) + sep(1) + cost(1) + padding(1)
	if logRows < 4 {
		logRows = 4
	}
	if tableRows < 3 {
		tableRows = 3
	}
	r.renderTable(state, w, tableRows)

	// Separator
	r.writeSep(w)

	// Live output area
	r.renderLiveOutput(state, w, logRows)

	// Heavy separator before cost
	r.writeHeavySep(w)

	// Cost footer (1 line)
	r.renderCostBar(state, w)

	r.buf.Flush()
}

func (r *Renderer) renderHeader(s *State, w int) {
	now := time.Now().Format("15:04:05")

	// Single header line — bold, high contrast
	r.buf.WriteString(ClearLine)
	fmt.Fprintf(r.buf, "%s%s ⚡ RALPH GODVIEW %s", Reverse, Header, Reset)
	fmt.Fprintf(r.buf, "  %s%d%s repos ", Bold, s.TotalRepos, Reset)
	fmt.Fprintf(r.buf, "%s✓%d%s ", StatusOK, s.ReposOK, Reset)
	if s.ReposWarn > 0 {
		fmt.Fprintf(r.buf, "%s⚠%d%s ", StatusWarn, s.ReposWarn, Reset)
	}
	if s.ReposErr > 0 {
		fmt.Fprintf(r.buf, "%s✗%d%s ", StatusErr, s.ReposErr, Reset)
	}
	fmt.Fprintf(r.buf, " │  %s%d%s agents ", Bold, s.ActiveAgents, Reset)
	for prov, count := range s.AgentsByProvider {
		if count > 0 {
			fmt.Fprintf(r.buf, "%s%d×%s%s ", ProviderColor(prov), count, prov, Reset)
		}
	}
	fmt.Fprintf(r.buf, " │  %s%s%s%s",
		Bold, costColor(s.TotalCost), FormatCost(s.TotalCost), Reset)
	// Right-align time
	fmt.Fprintf(r.buf, "  %s%s%s\n", Dim, now, Reset)
}

func (r *Renderer) renderTable(s *State, w, maxRows int) {
	// Column header row — dim, underlined feel
	r.buf.WriteString(ClearLine)
	fmt.Fprintf(r.buf, " %s%-18s %-8s  %-3s %5s %8s %8s  %-28s %s%s\n",
		Dim, "REPO", "AGENT", "ST", "TURN", "RATE", "COST", "TASK", "PROGRESS", Reset)

	shown := 0
	for _, repo := range s.Repos {
		if shown >= maxRows-1 { // -1 for header
			break
		}
		r.buf.WriteString(ClearLine)

		provCol := ProviderColor(repo.Provider)
		statCol := StatusColor(repo.Status)
		icon := StatusIcon(repo.Status)

		provider := repo.Provider
		if provider == "" {
			provider = "·"
			provCol = StatusIdle
		}

		turns := "  ·"
		if repo.Turns > 0 {
			turns = fmt.Sprintf("%3d", repo.Turns)
		}

		rate := FormatRate(repo.CostPerHr)
		cost := FormatCost(repo.TotalCost)
		taskWidth := 28
		if w > 120 {
			taskWidth = w - 85 // Scale task column with terminal width
		}
		task := Truncate(repo.CurrentTask, taskWidth)
		if task == "" {
			task = Dim + "·" + Reset
		}

		progress := "   ·"
		if repo.Progress > 0 {
			barWidth := 8
			if w > 120 {
				barWidth = 12
			}
			progress = fmt.Sprintf("%s %3.0f%%", ProgressBar(repo.Progress, barWidth), repo.Progress)
		}
		if repo.Status == "completed" || repo.Status == "done" {
			progress = StatusDone + "✓done" + Reset
		}
		if repo.Status == "error" || repo.Status == "failed" || repo.Status == "errored" {
			progress = StatusErr + "✗fail" + Reset
		}

		fmt.Fprintf(r.buf, " %-18s %s%-8s%s %s%s%s  %5s %8s %8s  %-*s %s\n",
			Truncate(repo.Name, 18),
			provCol, PadRight(provider, 8), Reset,
			statCol, icon, Reset,
			turns,
			PadLeft(rate, 8),
			PadLeft(cost, 8),
			taskWidth, task,
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
	fmt.Fprintf(r.buf, " %s%s▸ LIVE OUTPUT%s\n", Bold, Header, Reset)

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
	fmt.Fprintf(r.buf, " %s%sCOST%s ", Bold, Header, Reset)

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

func (r *Renderer) writeHeavySep(w int) {
	r.buf.WriteString(ClearLine)
	r.buf.WriteString(Header)
	r.buf.WriteString(strings.Repeat("━", w))
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
