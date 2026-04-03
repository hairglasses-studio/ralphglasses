package session

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// MultiRepoSupervisor coordinates autonomous R&D cycles across multiple
// repositories in an organization. It wraps per-repo Supervisors with
// cross-repo budget enforcement and health-based cycle prioritization.
//
// Informed by:
// - Scaling Agent Systems (2512.08296): centralized coordination
// - Hyperagents (2603.19461): recursive self-modification at Level 3
type MultiRepoSupervisor struct {
	mu          sync.Mutex
	supervisors map[string]*Supervisor // repo path -> supervisor
	managers    map[string]*Manager    // repo path -> manager
	orgName     string
	bus         *events.Bus

	running bool
	cancel  context.CancelFunc
	done    chan struct{}

	TickInterval    time.Duration // default 120s
	MaxConcurrent   int           // max repos with active cycles (default 3)
	CooldownBetween time.Duration // between launching cycles on different repos (default 5m)

	// Cross-repo state
	repoHealth      map[string]float64 // repo path -> health score (0-1, lower = needs work)
	lastCycleRepo   string
	lastCycleLaunch time.Time
	tickCount       int

	// Budget
	totalBudgetUSD float64
	spentUSD       float64
}

// MultiRepoStatus is a snapshot of the multi-repo supervisor state.
type MultiRepoStatus struct {
	Running       bool                      `json:"running"`
	RepoCount     int                       `json:"repo_count"`
	ActiveCycles  int                       `json:"active_cycles"`
	TotalBudget   float64                   `json:"total_budget_usd"`
	TotalSpent    float64                   `json:"total_spent_usd"`
	TickCount     int                       `json:"tick_count"`
	RepoHealth    map[string]float64        `json:"repo_health"`
	RepoStatuses  map[string]SupervisorState `json:"repo_statuses"`
}

// NewMultiRepoSupervisor creates a coordinator for multiple repository supervisors.
func NewMultiRepoSupervisor(orgName string, bus *events.Bus) *MultiRepoSupervisor {
	return &MultiRepoSupervisor{
		supervisors:     make(map[string]*Supervisor),
		managers:        make(map[string]*Manager),
		orgName:         orgName,
		bus:             bus,
		repoHealth:      make(map[string]float64),
		TickInterval:    120 * time.Second,
		MaxConcurrent:   3,
		CooldownBetween: 5 * time.Minute,
		done:            make(chan struct{}),
	}
}

// AddRepo registers a repository with its manager. Creates and starts a
// per-repo Supervisor.
func (ms *MultiRepoSupervisor) AddRepo(repoPath string, mgr *Manager) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, exists := ms.supervisors[repoPath]; exists {
		return fmt.Errorf("repo %s already registered", repoPath)
	}

	sup := NewSupervisor(mgr, repoPath)
	sup.SetBus(ms.bus)

	ms.supervisors[repoPath] = sup
	ms.managers[repoPath] = mgr
	ms.repoHealth[repoPath] = 0.5 // neutral starting health

	slog.Info("multi_supervisor: added repo", "repo", repoPath, "org", ms.orgName)
	return nil
}

// RemoveRepo stops the supervisor for a repo and unregisters it.
func (ms *MultiRepoSupervisor) RemoveRepo(repoPath string) {
	ms.mu.Lock()
	sup, ok := ms.supervisors[repoPath]
	if ok {
		delete(ms.supervisors, repoPath)
		delete(ms.managers, repoPath)
		delete(ms.repoHealth, repoPath)
	}
	ms.mu.Unlock()

	if ok && sup.Running() {
		sup.Stop()
	}
}

// Start begins the multi-repo coordination loop.
func (ms *MultiRepoSupervisor) Start(ctx context.Context) error {
	ms.mu.Lock()
	if ms.running {
		ms.mu.Unlock()
		return fmt.Errorf("multi_supervisor: already running")
	}
	ms.running = true
	ctx, ms.cancel = context.WithCancel(ctx)
	ms.mu.Unlock()

	go ms.run(ctx)
	return nil
}

// Stop halts the multi-repo supervisor and all per-repo supervisors.
func (ms *MultiRepoSupervisor) Stop() {
	ms.mu.Lock()
	if !ms.running {
		ms.mu.Unlock()
		return
	}
	ms.running = false
	ms.cancel()
	ms.mu.Unlock()

	<-ms.done

	// Stop all per-repo supervisors
	ms.mu.Lock()
	for _, sup := range ms.supervisors {
		if sup.Running() {
			sup.Stop()
		}
	}
	ms.mu.Unlock()
}

// RepoCount returns the number of registered repos.
func (ms *MultiRepoSupervisor) RepoCount() int {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return len(ms.supervisors)
}

// ActiveCycles returns how many repos currently have running cycles.
func (ms *MultiRepoSupervisor) ActiveCycles() int {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	active := 0
	for _, sup := range ms.supervisors {
		if sup.Running() {
			active++
		}
	}
	return active
}

// SetMaxBudget sets the global fleet budget cap.
func (ms *MultiRepoSupervisor) SetMaxBudget(usd float64) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.totalBudgetUSD = usd
}

// Status returns an aggregate status snapshot.
func (ms *MultiRepoSupervisor) Status() MultiRepoStatus {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	health := make(map[string]float64, len(ms.repoHealth))
	for k, v := range ms.repoHealth {
		health[k] = v
	}

	statuses := make(map[string]SupervisorState, len(ms.supervisors))
	for path, sup := range ms.supervisors {
		statuses[path] = sup.Status()
	}

	return MultiRepoStatus{
		Running:      ms.running,
		RepoCount:    len(ms.supervisors),
		ActiveCycles: ms.countActiveLocked(),
		TotalBudget:  ms.totalBudgetUSD,
		TotalSpent:   ms.spentUSD,
		TickCount:    ms.tickCount,
		RepoHealth:   health,
		RepoStatuses: statuses,
	}
}

func (ms *MultiRepoSupervisor) run(ctx context.Context) {
	defer close(ms.done)

	ticker := time.NewTicker(ms.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ms.tick(ctx)
		}
	}
}

func (ms *MultiRepoSupervisor) tick(ctx context.Context) {
	ms.mu.Lock()
	ms.tickCount++
	tickCount := ms.tickCount

	// 1. Check global budget
	if ms.totalBudgetUSD > 0 && ms.spentUSD >= ms.totalBudgetUSD {
		slog.Warn("multi_supervisor: global budget exhausted",
			"spent", ms.spentUSD, "budget", ms.totalBudgetUSD)
		ms.mu.Unlock()
		return
	}

	// 2. Collect health scores and find the repo most needing work
	type repoScore struct {
		path   string
		health float64
		active bool
	}
	var repos []repoScore
	for path, sup := range ms.supervisors {
		health := ms.repoHealth[path]
		repos = append(repos, repoScore{
			path:   path,
			health: health,
			active: sup.Running(),
		})
	}

	activeCycles := ms.countActiveLocked()
	cooldownOK := time.Since(ms.lastCycleLaunch) >= ms.CooldownBetween
	maxConcurrent := ms.MaxConcurrent
	ms.mu.Unlock()

	// 3. Sort by health (lowest first = most needs work)
	sort.Slice(repos, func(i, j int) bool {
		return repos[i].health < repos[j].health
	})

	// 4. Try to launch a cycle on the neediest repo
	if cooldownOK && activeCycles < maxConcurrent {
		for _, r := range repos {
			if r.active {
				continue // already has a cycle running
			}

			ms.mu.Lock()
			sup := ms.supervisors[r.path]
			ms.mu.Unlock()

			if sup == nil {
				continue
			}

			if err := sup.Start(ctx); err != nil {
				slog.Debug("multi_supervisor: failed to start cycle",
					"repo", r.path, "error", err)
				continue
			}

			ms.mu.Lock()
			ms.lastCycleRepo = r.path
			ms.lastCycleLaunch = time.Now()
			ms.mu.Unlock()

			slog.Info("multi_supervisor: launched cycle on neediest repo",
				"repo", r.path, "health", r.health)
			break
		}
	}

	// 5. Run feedback loops on each supervisor every 10 ticks
	if tickCount%10 == 0 {
		ms.mu.Lock()
		sups := make(map[string]*Supervisor, len(ms.supervisors))
		for k, v := range ms.supervisors {
			sups[k] = v
		}
		ms.mu.Unlock()

		for path, sup := range sups {
			if sup.Running() {
				sup.RunFeedbackLoop()
				slog.Debug("multi_supervisor: ran feedback loop", "repo", path)
			}
		}
	}
}

// UpdateRepoHealth sets the health score for a repo (0-1, lower = needs work).
func (ms *MultiRepoSupervisor) UpdateRepoHealth(repoPath string, health float64) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if health < 0 {
		health = 0
	}
	if health > 1 {
		health = 1
	}
	ms.repoHealth[repoPath] = health
}

func (ms *MultiRepoSupervisor) countActiveLocked() int {
	active := 0
	for _, sup := range ms.supervisors {
		if sup.Running() {
			active++
		}
	}
	return active
}
