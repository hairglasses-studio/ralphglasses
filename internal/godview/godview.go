// Package godview provides a non-interactive, read-only, maximum-throughput
// fleet monitoring dashboard for ralphglasses. It streams live agent output
// from Claude, Codex, and Gemini across all repos in a single terminal pane.
//
// Architecture: Raw ANSI rendering via bufio.NewWriter for sub-50ms updates.
// Uses main screen buffer (not alternate screen) to preserve scrollback.
package godview

import (
	"context"
	"sort"
	"sync"
	"time"
)

// State holds the complete God view snapshot rendered each frame.
type State struct {
	TotalRepos       int
	ReposOK          int
	ReposWarn        int
	ReposErr         int
	ActiveAgents     int
	AgentsByProvider map[string]int
	TotalCost        float64
	CostByProvider   map[string]float64
	CostRatePerHr    float64
	BudgetCap        float64
	Repos            []RepoStatus
	LiveLines        []LogLine
}

// RepoStatus represents one repo row in the table.
type RepoStatus struct {
	Name        string
	Provider    string
	Status      string
	Turns       int
	CostPerHr   float64
	TotalCost   float64
	CurrentTask string
	Progress    float64
	LastUpdate  time.Time
}

// LogLine is a single line of live output from an agent session.
type LogLine struct {
	Provider  string
	Repo      string
	Text      string
	Timestamp time.Time
}

// GodView is the main controller.
type GodView struct {
	renderer    *Renderer
	refreshRate time.Duration
	scanPath    string

	mu        sync.Mutex
	state     State
	liveLines []LogLine // Ring buffer
	maxLines  int
}

// New creates a GodView with the given refresh rate.
func New(scanPath string, refreshRate time.Duration) *GodView {
	if refreshRate <= 0 {
		refreshRate = 50 * time.Millisecond
	}
	return &GodView{
		renderer:    NewRenderer(),
		refreshRate: refreshRate,
		scanPath:    scanPath,
		liveLines:   make([]LogLine, 0, 200),
		maxLines:    200,
		state: State{
			AgentsByProvider: make(map[string]int),
			CostByProvider:   make(map[string]float64),
		},
	}
}

// AppendLine adds a live output line (thread-safe).
func (gv *GodView) AppendLine(line LogLine) {
	gv.mu.Lock()
	defer gv.mu.Unlock()
	gv.liveLines = append(gv.liveLines, line)
	if len(gv.liveLines) > gv.maxLines {
		gv.liveLines = gv.liveLines[len(gv.liveLines)-gv.maxLines:]
	}
}

// UpdateRepos replaces the repo status list (thread-safe).
func (gv *GodView) UpdateRepos(repos []RepoStatus) {
	gv.mu.Lock()
	defer gv.mu.Unlock()

	// Sort: active first, then by last update
	sort.Slice(repos, func(i, j int) bool {
		ai := activityScore(repos[i].Status)
		aj := activityScore(repos[j].Status)
		if ai != aj {
			return ai > aj
		}
		return repos[i].LastUpdate.After(repos[j].LastUpdate)
	})

	gv.state.Repos = repos

	// Recompute aggregates
	gv.state.TotalRepos = len(repos)
	gv.state.ReposOK = 0
	gv.state.ReposWarn = 0
	gv.state.ReposErr = 0
	gv.state.ActiveAgents = 0
	gv.state.AgentsByProvider = make(map[string]int)
	gv.state.CostByProvider = make(map[string]float64)
	gv.state.TotalCost = 0

	for _, r := range repos {
		switch r.Status {
		case "running", "ok", "completed", "done", "idle", "pending", "unknown":
			gv.state.ReposOK++
		case "warn", "degraded":
			gv.state.ReposWarn++
		case "error", "failed", "errored":
			gv.state.ReposErr++
		default:
			gv.state.ReposOK++
		}
		if r.Provider != "" && r.Status == "running" {
			gv.state.ActiveAgents++
			gv.state.AgentsByProvider[r.Provider]++
		}
		gv.state.TotalCost += r.TotalCost
		if r.Provider != "" {
			gv.state.CostByProvider[r.Provider] += r.TotalCost
		}
	}
}

// SetBudget sets fleet budget cap.
func (gv *GodView) SetBudget(cap, ratePerHr float64) {
	gv.mu.Lock()
	defer gv.mu.Unlock()
	gv.state.BudgetCap = cap
	gv.state.CostRatePerHr = ratePerHr
}

// Run starts the render loop. Blocks until ctx is cancelled.
func (gv *GodView) Run(ctx context.Context) error {
	defer gv.renderer.Cleanup()

	ticker := time.NewTicker(gv.refreshRate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			gv.mu.Lock()
			gv.state.LiveLines = gv.liveLines
			snap := gv.state // Copy for rendering
			gv.mu.Unlock()
			gv.renderer.Render(&snap)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func activityScore(status string) int {
	switch status {
	case "running":
		return 4
	case "error", "failed", "errored":
		return 3
	case "warn", "degraded":
		return 2
	case "completed", "done":
		return 1
	default:
		return 0
	}
}
