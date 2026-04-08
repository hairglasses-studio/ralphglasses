package fleet

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/observability"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tracing"
	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

const (
	DefaultPort         = 9473
	HeartbeatInterval   = 30 * time.Second
	StaleThreshold      = 90 * time.Second
	DisconnectThreshold = 5 * time.Minute
	ClaimTimeout        = 5 * time.Minute
)

// Coordinator manages the fleet from a single node.
type Coordinator struct {
	mu       sync.RWMutex
	nodeID   string
	hostname string
	port     int
	version  string

	workers map[string]*WorkerInfo // keyed by worker ID
	queue   *WorkQueue
	budget  GlobalBudget
	bus     *events.Bus
	sessMgr *session.Manager

	// Subsystems wired in Phase B1
	health     *HealthTracker
	budgetMgr  *BudgetManager
	router     Router
	retries    *RetryTracker
	autoscaler *AutoScaler

	// Cost prediction: sliding window burn-rate forecasting and anomaly detection.
	costPredictor *CostPredictor

	// Tailscale integration (nil-safe: auth middleware passes all when nil)
	tsClient TailscaleClient

	// queuePath is the file path used for queue persistence. Empty means no persistence.
	queuePath   string
	artifactDir string
	syncer      *WorktreeSync

	startedAt time.Time
	server    *http.Server
}

// NewCoordinator creates a coordinator node.
func NewCoordinator(nodeID, hostname string, port int, version string, bus *events.Bus, sessMgr *session.Manager) *Coordinator {
	return &Coordinator{
		nodeID:    nodeID,
		hostname:  hostname,
		port:      port,
		version:   version,
		workers:   make(map[string]*WorkerInfo),
		queue:     NewWorkQueue(),
		budget:    GlobalBudget{LimitUSD: 500},
		bus:       bus,
		sessMgr:   sessMgr,
		health:    NewHealthTracker(DefaultHealthConfig()),
		budgetMgr: NewBudgetManager(10.0),
		router: &ProviderAffinityRouter{
			PreferredProvider: string(session.ProviderCodex),
			Fallback:          &LeastLoadedRouter{},
		},
		retries:       NewRetryTracker(DefaultRetryPolicy()),
		autoscaler:    NewAutoScaler(DefaultAutoScalerConfig()),
		costPredictor: NewCostPredictor(0),
		tsClient:      DefaultTailscaleClient(),
		startedAt:     time.Now(),
	}
}

// NewCoordinatorWithPersistence creates a coordinator that loads the work queue
// from dataDir/.ralph/fleet-queue.json on startup and saves it on shutdown.
// dataDir is typically the scan-path root.
// If the file does not exist yet the queue starts empty (not an error).
func NewCoordinatorWithPersistence(nodeID, hostname string, port int, version string, bus *events.Bus, sessMgr *session.Manager, dataDir string) *Coordinator {
	c := NewCoordinator(nodeID, hostname, port, version, bus, sessMgr)

	ralphDir := filepath.Join(dataDir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0755); err != nil {
		slog.Warn("fleet: could not create .ralph dir for queue persistence", "path", ralphDir, "error", err)
		return c
	}

	c.queuePath = filepath.Join(ralphDir, "fleet-queue.json")
	c.artifactDir = filepath.Join(ralphDir, "fleet-artifacts")
	if err := os.MkdirAll(c.artifactDir, 0755); err != nil {
		slog.Warn("fleet: could not create artifact dir", "path", c.artifactDir, "error", err)
	} else {
		c.syncer = NewWorktreeSync(c.artifactDir)
	}
	if err := c.queue.LoadFrom(c.queuePath); err != nil {
		slog.Debug("fleet: no saved queue to restore", "path", c.queuePath, "error", err)
	} else {
		slog.Info("fleet: restored queue from disk", "path", c.queuePath)
	}

	return c
}

// SetBudgetLimit sets the fleet-wide budget ceiling.
func (c *Coordinator) SetBudgetLimit(limit float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.budget.LimitUSD = limit
}

// SetAutoScalerConfig replaces the autoscaler configuration.
func (c *Coordinator) SetAutoScalerConfig(cfg AutoScalerConfig) {
	c.autoscaler = NewAutoScaler(cfg)
}

// AutoScaler returns the coordinator's autoscaler instance.
func (c *Coordinator) AutoScaler() *AutoScaler {
	return c.autoscaler
}

// Start begins the HTTP server and maintenance goroutines.
func (c *Coordinator) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/register", c.handleRegister)
	mux.HandleFunc("POST /api/v1/heartbeat", c.handleHeartbeat)
	mux.HandleFunc("POST /api/v1/work/poll", c.handleWorkPoll)
	mux.HandleFunc("POST /api/v1/work/start", c.handleWorkStart)
	mux.HandleFunc("POST /api/v1/work/complete", c.handleWorkComplete)
	mux.HandleFunc("POST /api/v1/work/submit", c.handleWorkSubmit)
	mux.HandleFunc("GET /api/v1/work/{workID}", c.handleWorkStatus)
	mux.HandleFunc("POST /api/v1/work/{workID}/cancel", c.handleWorkCancel)
	mux.HandleFunc("POST /api/v1/work/{workID}/artifact", c.handleWorkArtifactUpload)
	mux.HandleFunc("GET /api/v1/work/{workID}/artifact", c.handleWorkArtifactGet)
	mux.HandleFunc("POST /api/v1/events/batch", c.handleEventBatch)
	mux.HandleFunc("GET /api/v1/events", c.handleEventStream)
	mux.HandleFunc("GET /api/v1/status", c.handleStatus)
	mux.HandleFunc("GET /api/v1/fleet", c.handleFleetState)
	mux.HandleFunc("GET /api/v1/sessions", c.handleSessions)
	mux.HandleFunc("GET /healthz", c.handleHealthz)

	// A2A AgentCard discovery endpoint (v1.0 spec path)
	mux.HandleFunc("GET "+AgentCardDiscoveryPath, c.handleAgentCard)

	// A2A task endpoints (v1.0 task send/receive)
	mux.HandleFunc("POST /api/v1/a2a/task/send", c.handleA2ATaskSend)
	mux.HandleFunc("GET /api/v1/a2a/task/{taskID}", c.handleA2ATaskGet)
	mux.HandleFunc("POST /api/v1/a2a/task/{taskID}/cancel", c.handleA2ATaskCancel)

	// Prometheus metrics endpoint
	if promRec, ok := tracing.Get().(*tracing.PrometheusRecorder); ok {
		mux.HandleFunc("GET /metrics", promRec.Handler())
	}

	// Wrap the mux with Tailscale auth middleware. When Tailscale is available
	// and responding, only peers with tag:ralph-fleet (or any authenticated
	// tailnet member) are allowed through. Health and status endpoints are
	// always open for monitoring.
	handler := TailscaleAuthMiddleware(c.tsClient, mux)

	c.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", c.port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		BaseContext:  func(_ net.Listener) context.Context { return ctx },
	}

	// Maintenance: expire stale workers and reclaim timed-out work
	go c.maintenanceLoop(ctx)

	util.Debug.Debugf("coordinator starting on :%d", c.port)
	if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop gracefully shuts down the coordinator.
func (c *Coordinator) Stop(ctx context.Context) error {
	if c.queuePath != "" {
		if err := c.queue.SaveTo(c.queuePath); err != nil {
			slog.Error("fleet: failed to save queue on shutdown", "path", c.queuePath, "error", err)
		} else {
			slog.Info("fleet: queue saved on shutdown", "path", c.queuePath)
		}
	}
	if c.server != nil {
		return c.server.Shutdown(ctx)
	}
	return nil
}

// maintenanceLoop periodically checks for stale workers and timed-out assignments.
func (c *Coordinator) maintenanceLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.expireWorkers()
			c.reclaimTimedOut()
			c.autoScaleCheck()           // Phase 10.5.4: evaluate worker pool scaling
			c.queue.ReapStale(time.Hour) // QW-11: clean phantom/stale tasks older than 1 hour
			c.queue.ReapPhantomRepos()   // QW-11: purge bare "001" placeholder repo entries
			if c.queuePath != "" {
				if err := c.queue.SaveTo(c.queuePath); err != nil {
					slog.Error("fleet: periodic queue checkpoint failed", "path", c.queuePath, "error", err)
				}
			}
		}
	}
}

func (c *Coordinator) expireWorkers() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for _, w := range c.workers {
		// Preserve manually-set statuses (paused, draining)
		if w.Status == WorkerPaused || w.Status == WorkerDraining {
			continue
		}
		since := now.Sub(w.LastHeartbeat)
		switch {
		case since > DisconnectThreshold:
			w.Status = WorkerDisconnected
		case since > StaleThreshold:
			w.Status = WorkerStale
		default:
			w.Status = WorkerOnline
		}
	}
}

func (c *Coordinator) reclaimTimedOut() {
	c.queue.ReclaimTimedOut(ClaimTimeout)
}

// SubmitWork adds a work item to the queue (for internal use).
func (c *Coordinator) SubmitWork(item *WorkItem) error {
	_, span := observability.StartFleetSpan(context.Background(), "task.submit")
	defer span.End()

	if item.ID == "" {
		item.ID = generateID()
	}
	if item.Status == "" {
		item.Status = WorkPending
	}
	item.SubmittedAt = time.Now()
	if item.MaxRetries == 0 {
		item.MaxRetries = 2
	}

	c.mu.RLock()
	avail := c.budget.AvailableBudget()
	c.mu.RUnlock()

	if item.MaxBudgetUSD > 0 && item.MaxBudgetUSD > avail {
		return fmt.Errorf("insufficient budget: need $%.2f, available $%.2f", item.MaxBudgetUSD, avail)
	}

	c.queue.Push(item)

	if item.MaxBudgetUSD > 0 {
		c.mu.Lock()
		c.budget.ReservedUSD += item.MaxBudgetUSD
		c.budget.LastUpdated = time.Now()
		c.mu.Unlock()
	}

	return nil
}

// GetFleetState returns a snapshot of the fleet state.
func (c *Coordinator) GetFleetState() FleetState {
	c.mu.RLock()
	workers := make([]WorkerInfo, 0, len(c.workers))
	for _, w := range c.workers {
		workers = append(workers, *w)
	}
	budget := c.budget
	c.mu.RUnlock()

	counts := c.queue.Counts()

	return FleetState{
		Workers:       workers,
		QueueDepth:    counts[WorkPending],
		ActiveWork:    counts[WorkAssigned] + counts[WorkRunning],
		CompletedWork: counts[WorkCompleted],
		FailedWork:    counts[WorkFailed],
		DLQDepth:      c.queue.DLQDepth(),
		TotalSpentUSD: budget.SpentUSD,
		BudgetUSD:     budget.LimitUSD,
		UpdatedAt:     time.Now(),
	}
}

// DeregisterWorker removes a worker from the fleet and reclaims its assigned work.
func (c *Coordinator) DeregisterWorker(workerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	_, ok := c.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %s not found", workerID)
	}

	// Reclaim assigned work items back to pending
	for _, item := range c.queue.All() {
		if item.AssignedTo == workerID && item.Status == WorkAssigned {
			item.Status = WorkPending
			item.AssignedTo = ""
			item.AssignedAt = nil
			c.queue.Update(item)
		}
	}

	delete(c.workers, workerID)

	// Clean up health tracking
	if c.health != nil {
		c.health.Remove(workerID)
	}

	return nil
}

// PauseWorker sets a worker's status to paused, preventing work assignment.
func (c *Coordinator) PauseWorker(workerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	w, ok := c.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %s not found", workerID)
	}
	w.Status = WorkerPaused
	return nil
}

// ResumeWorker sets a paused or draining worker's status back to online.
func (c *Coordinator) ResumeWorker(workerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	w, ok := c.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %s not found", workerID)
	}
	if w.Status != WorkerPaused && w.Status != WorkerDraining {
		return fmt.Errorf("worker %s is not paused or draining (status: %s)", workerID, w.Status)
	}
	w.Status = WorkerOnline
	return nil
}

// DrainWorker sets a worker's status to draining: no new work will be assigned,
// but existing active work continues to completion. Returns immediately (non-blocking).
func (c *Coordinator) DrainWorker(workerID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	w, ok := c.workers[workerID]
	if !ok {
		return fmt.Errorf("worker %s not found", workerID)
	}
	w.Status = WorkerDraining
	return nil
}

// IsWorkerDrained returns true when a worker is in draining status and has no
// active (assigned or running) work items remaining.
func (c *Coordinator) IsWorkerDrained(workerID string) bool {
	c.mu.RLock()
	w, ok := c.workers[workerID]
	c.mu.RUnlock()

	if !ok || w.Status != WorkerDraining {
		return false
	}

	for _, item := range c.queue.All() {
		if item.AssignedTo == workerID && (item.Status == WorkAssigned || item.Status == WorkRunning) {
			return false
		}
	}
	return true
}

// ListDLQ returns all items in the dead letter queue.
func (c *Coordinator) ListDLQ() []*WorkItem {
	return c.queue.ListDLQ()
}

// RetryFromDLQ moves an item from the dead letter queue back to the main queue.
func (c *Coordinator) RetryFromDLQ(itemID string) error {
	return c.queue.RetryFromDLQ(itemID)
}

// PurgeDLQ removes all items from the dead letter queue.
func (c *Coordinator) PurgeDLQ() int {
	return c.queue.PurgeDLQ()
}

// DLQDepth returns the number of items in the dead letter queue.
func (c *Coordinator) DLQDepth() int {
	return c.queue.DLQDepth()
}

// WorkItem returns a work item by ID from the queue.
func (c *Coordinator) WorkItem(id string) (*WorkItem, bool) {
	return c.queue.Lookup(id)
}

// CancelWork cancels a work item by ID, marking it as failed.
func (c *Coordinator) CancelWork(id string) error {
	item, ok := c.queue.Lookup(id)
	if !ok {
		return fmt.Errorf("work item %s not found", id)
	}
	if item.Status == WorkCompleted || item.Status == WorkFailed {
		return nil
	}

	now := time.Now()
	item.Status = WorkFailed
	item.Error = "cancelled"
	item.CompletedAt = &now
	item.AssignedTo = ""
	item.AssignedAt = nil
	if item.Source == WorkSourceStructuredCodexTeam {
		if item.Result == nil {
			item.Result = &WorkResult{}
		}
		if item.Result.TaskStatus == "" {
			item.Result.TaskStatus = session.TeamTaskCancelled
		}
		if item.Result.Summary == "" {
			item.Result.Summary = "cancelled"
		}
		if item.Result.ExitReason == "" {
			item.Result.ExitReason = "cancelled"
		}
	}
	c.queue.Update(item)

	if item.MaxBudgetUSD > 0 {
		c.mu.Lock()
		c.budget.ReservedUSD -= item.MaxBudgetUSD
		if c.budget.ReservedUSD < 0 {
			c.budget.ReservedUSD = 0
		}
		c.budget.LastUpdated = now
		c.mu.Unlock()
	}

	if c.bus != nil {
		c.bus.Publish(events.Event{
			Type:      "fleet.work_cancelled",
			SessionID: item.SessionID,
			RepoName:  item.RepoName,
			Data:      map[string]any{"work_item_id": item.ID},
		})
	}

	return nil
}

// SetTSClient replaces the coordinator's Tailscale client (useful for testing).
func (c *Coordinator) SetTSClient(tc TailscaleClient) {
	c.tsClient = tc
}

// FleetTag is the Tailscale ACL tag that fleet nodes should carry.
const FleetTag = "tag:ralph-fleet"

// tailscaleAuthExemptPaths are URL paths that bypass auth for health checks
// and monitoring.
var tailscaleAuthExemptPaths = []string{
	"/healthz",
	"/metrics",
	"/.well-known/agent.json",
}

// TailscaleAuthMiddleware verifies that incoming requests originate from an
// authenticated Tailscale peer. It calls WhoIs on the remote address; if the
// peer carries tag:ralph-fleet the request proceeds. If Tailscale is not
// available (WhoIs fails), the middleware is permissive and allows all requests
// for backward compatibility with non-Tailscale deployments.
//
// Health, metrics, and agent-card endpoints are always exempt.
func TailscaleAuthMiddleware(tsClient TailscaleClient, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always allow exempt paths (health checks, metrics, agent card).
		if slices.Contains(tailscaleAuthExemptPaths, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// If no Tailscale client is configured, pass through (backward compatible).
		if tsClient == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Determine the remote address for WhoIs lookup.
		remoteAddr := r.RemoteAddr
		if remoteAddr == "" {
			// No remote addr available (e.g. tests); allow through.
			next.ServeHTTP(w, r)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		who, err := tsClient.WhoIs(ctx, remoteAddr)
		if err != nil {
			// Tailscale not available or peer not on tailnet.
			// Check if the remote IP is a Tailscale IP (100.x.x.x range).
			// If it's NOT a Tailscale IP, allow through — this is a local/LAN
			// request and Tailscale auth doesn't apply.
			host, _, _ := net.SplitHostPort(remoteAddr)
			if host == "" {
				host = remoteAddr
			}
			if !isTailscaleIP(host) {
				next.ServeHTTP(w, r)
				return
			}
			// It IS a Tailscale IP but WhoIs failed — deny.
			util.Debug.Debugf("tailscale auth: WhoIs failed for %s: %v", remoteAddr, err)
			http.Error(w, "tailscale auth failed", http.StatusForbidden)
			return
		}

		// WhoIs succeeded — peer is on our tailnet. Check for fleet tag.
		if who.Node.HasTag(FleetTag) {
			next.ServeHTTP(w, r)
			return
		}

		// No fleet tag, but the peer is an authenticated tailnet member.
		// Allow access (the tag requirement can be tightened later via config).
		// Log a warning so operators know untagged peers are connecting.
		if who.UserProfile != nil {
			util.Debug.Debugf("tailscale auth: allowing untagged peer %s (user: %s)",
				who.Node.HostName, who.UserProfile.LoginName)
		}
		next.ServeHTTP(w, r)
	})
}

// isTailscaleIP returns true if the address falls in the Tailscale CGNAT
// range (100.64.0.0/10) or the Tailscale IPv6 range (fd7a:115c:a1e0::/48).
func isTailscaleIP(addr string) bool {
	ip := net.ParseIP(addr)
	if ip == nil {
		return false
	}
	// Tailscale IPv4 CGNAT: 100.64.0.0/10
	_, tsV4, _ := net.ParseCIDR("100.64.0.0/10")
	if tsV4 != nil && tsV4.Contains(ip) {
		return true
	}
	// Tailscale IPv6: fd7a:115c:a1e0::/48
	if strings.HasPrefix(ip.String(), "fd7a:115c:a1e0:") {
		return true
	}
	return false
}
