package fleet

import (
	"context"
	"net"
	"path/filepath"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

// WorkerAgent runs on each worker node, handling registration, heartbeat, and work execution.
type WorkerAgent struct {
	nodeID   string
	hostname string
	port     int
	version  string
	scanPath string

	client    *Client
	sessMgr   *session.Manager
	bus       *events.Bus
	startedAt time.Time

	// eventCursor tracks the last forwarded event for batching
	eventCursor int
}

// NewWorkerAgent creates a worker agent that connects to a coordinator.
func NewWorkerAgent(coordinatorURL string, hostname string, port int, version string, scanPath string, bus *events.Bus, sessMgr *session.Manager) *WorkerAgent {
	return &WorkerAgent{
		hostname:  hostname,
		port:      port,
		version:   version,
		scanPath:  scanPath,
		client:    NewClient(coordinatorURL),
		sessMgr:   sessMgr,
		bus:       bus,
		startedAt: time.Now(),
	}
}

// Run starts the worker's registration, heartbeat, and poll loops.
// Blocks until ctx is cancelled.
func (w *WorkerAgent) Run(ctx context.Context) error {
	// Discover local repos and providers
	repos := w.discoverRepos(ctx)
	providers := w.discoverProviders()

	tsIP := DiscoverTailscaleIP()

	// Register with coordinator
	workerID, err := w.client.Register(ctx, RegisterPayload{
		Hostname:    w.hostname,
		TailscaleIP: tsIP,
		Port:        w.port,
		Providers:   providers,
		Repos:       repos,
		MaxSessions: 4,
		Version:     w.version,
	})
	if err != nil {
		return err
	}
	w.nodeID = workerID
	util.Debug.Debugf("registered as worker %s", workerID)

	// Run heartbeat, poll, and event forwarding concurrently
	go w.heartbeatLoop(ctx, repos, providers)
	go w.pollLoop(ctx)
	go w.eventForwardLoop(ctx)

	<-ctx.Done()
	return ctx.Err()
}

func (w *WorkerAgent) heartbeatLoop(ctx context.Context, repos []string, providers []session.Provider) {
	ticker := time.NewTicker(HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sessions := w.sessMgr.List("")
			active := 0
			var spent float64
			for _, s := range sessions {
				s.Lock()
				if s.Status == session.StatusRunning || s.Status == session.StatusLaunching {
					active++
				}
				spent += s.SpentUSD
				s.Unlock()
			}

			_ = w.client.Heartbeat(ctx, HeartbeatPayload{
				WorkerID:       w.nodeID,
				ActiveSessions: active,
				SpentUSD:       spent,
				AvailableSlots: 4 - active,
				Repos:          repos,
				Providers:      providers,
				Load:           float64(active) / 4.0,
			})
		}
	}
}

func (w *WorkerAgent) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			item, err := w.client.PollWork(ctx, w.nodeID)
			if err != nil {
				util.Debug.Debugf("poll error: %v", err)
				continue
			}
			if item == nil {
				continue
			}
			go w.executeWork(ctx, item)
		}
	}
}

func (w *WorkerAgent) executeWork(ctx context.Context, item *WorkItem) {
	util.Debug.Debugf("executing work %s: %s", item.ID, item.RepoName)

	opts := session.LaunchOptions{
		Provider:     item.Provider,
		RepoPath:     item.RepoPath,
		Prompt:       item.Prompt,
		Model:        item.Model,
		Agent:        item.Agent,
		MaxBudgetUSD: item.MaxBudgetUSD,
		MaxTurns:     item.MaxTurns,
	}

	if opts.Provider == "" {
		opts.Provider = session.DefaultPrimaryProvider()
	}

	sess, err := w.sessMgr.Launch(ctx, opts)
	if err != nil {
		_ = w.client.CompleteWork(ctx, WorkCompletePayload{
			WorkItemID: item.ID,
			Status:     WorkFailed,
			Error:      err.Error(),
		})
		return
	}

	// Wait for session to complete
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sess.Lock()
			status := sess.Status
			spent := sess.SpentUSD
			turns := sess.TurnCount
			exitReason := sess.ExitReason
			lastOutput := sess.LastOutput
			launched := sess.LaunchedAt
			sess.Unlock()

			switch status {
			case session.StatusCompleted:
				_ = w.client.CompleteWork(ctx, WorkCompletePayload{
					WorkItemID: item.ID,
					Status:     WorkCompleted,
					Result: &WorkResult{
						SessionID:  sess.ID,
						SpentUSD:   spent,
						TurnCount:  turns,
						DurationS:  time.Since(launched).Seconds(),
						ExitReason: exitReason,
						Output:     lastOutput,
					},
				})
				return
			case session.StatusErrored, session.StatusStopped:
				_ = w.client.CompleteWork(ctx, WorkCompletePayload{
					WorkItemID: item.ID,
					Status:     WorkFailed,
					Error:      exitReason,
					Result: &WorkResult{
						SessionID:  sess.ID,
						SpentUSD:   spent,
						TurnCount:  turns,
						DurationS:  time.Since(launched).Seconds(),
						ExitReason: exitReason,
					},
				})
				return
			}
		}
	}
}

func (w *WorkerAgent) eventForwardLoop(ctx context.Context) {
	if w.bus == nil {
		return
	}

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			evts, cursor := w.bus.HistoryAfterCursor(w.eventCursor, 100)
			w.eventCursor = cursor

			if len(evts) == 0 {
				continue
			}

			batch := EventBatch{
				WorkerID: w.nodeID,
				Events:   make([]FleetEvent, len(evts)),
			}
			for i, e := range evts {
				batch.Events[i] = FleetEvent{
					NodeID:    w.nodeID,
					Type:      string(e.Type),
					Timestamp: e.Timestamp,
					RepoName:  e.RepoName,
					SessionID: e.SessionID,
					Provider:  e.Provider,
					Data:      e.Data,
				}
			}
			_ = w.client.SendEvents(ctx, batch)
		}
	}
}

func (w *WorkerAgent) discoverRepos(ctx context.Context) []string {
	if w.scanPath == "" {
		return nil
	}
	repos, err := discovery.Scan(ctx, w.scanPath)
	if err != nil {
		return nil
	}
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = filepath.Base(r.Path)
	}
	return names
}

func (w *WorkerAgent) discoverProviders() []session.Provider {
	var providers []session.Provider
	for _, p := range []session.Provider{session.ProviderCodex, session.ProviderGemini, session.ProviderClaude} {
		if err := session.ValidateProvider(p); err == nil {
			providers = append(providers, p)
		}
	}
	if len(providers) == 0 {
		providers = []session.Provider{session.DefaultPrimaryProvider()}
	}
	return providers
}

// NodeID returns the worker's assigned node ID (empty before registration).
func (w *WorkerAgent) NodeID() string {
	return w.nodeID
}

// DiscoverTailscaleIP gets the node's Tailscale IP, or empty string if unavailable.
// It queries the Tailscale status (via LocalAPI or CLI) and extracts the first
// IPv4 address from the self node.
func DiscoverTailscaleIP() string {
	status, err := GetTailscaleStatus()
	if err != nil {
		util.Debug.Debugf("DiscoverTailscaleIP: tailscale unavailable: %v", err)
		return ""
	}

	// Prefer the first IPv4 address from the self node.
	for _, ip := range status.Self.TailscaleIPs {
		parsed := net.ParseIP(ip)
		if parsed != nil && parsed.To4() != nil {
			return ip
		}
	}

	// Fall back to the first address of any family.
	if len(status.Self.TailscaleIPs) > 0 {
		return status.Self.TailscaleIPs[0]
	}
	return ""
}
