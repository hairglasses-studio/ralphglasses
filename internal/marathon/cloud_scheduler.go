package marathon

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// VMSpec describes the virtual machine configuration for a cloud marathon run.
type VMSpec struct {
	Provider    string            `json:"provider"`     // "aws" or "gcp"
	MachineType string           `json:"machine_type"`  // e.g. "n2-standard-8", "m5.2xlarge"
	Region      string           `json:"region"`
	Image       string           `json:"image"`         // AMI ID or GCP image family
	DiskGB      int              `json:"disk_gb"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// VMInfo holds runtime information about a provisioned VM.
type VMInfo struct {
	ID        string    `json:"id"`
	PublicIP  string    `json:"public_ip,omitempty"`
	Status    string    `json:"status"` // "provisioning", "running", "terminated", "error"
	CreatedAt time.Time `json:"created_at"`
}

// VMProvider abstracts cloud VM lifecycle operations.
// Implementations use exec-based CLI calls (aws/gcloud) rather than SDK imports.
type VMProvider interface {
	// Provision creates a new VM with the given spec. Returns VM info on success.
	Provision(ctx context.Context, spec VMSpec) (*VMInfo, error)

	// Status returns current VM info. Returns error if VM not found.
	Status(ctx context.Context, vmID string) (*VMInfo, error)

	// Execute runs a command on the VM via SSH/exec. Returns combined output.
	Execute(ctx context.Context, vmID string, command string) ([]byte, error)

	// Terminate destroys the VM. Idempotent — no error if already terminated.
	Terminate(ctx context.Context, vmID string) error

	// CostSoFar returns the estimated cost in USD for the given VM since creation.
	CostSoFar(ctx context.Context, vmID string) (float64, error)
}

// CloudTaskState represents the lifecycle state of a scheduled cloud task.
type CloudTaskState string

const (
	CloudTaskPending    CloudTaskState = "pending"
	CloudTaskProvisioning CloudTaskState = "provisioning"
	CloudTaskRunning    CloudTaskState = "running"
	CloudTaskCollecting CloudTaskState = "collecting"
	CloudTaskCompleted  CloudTaskState = "completed"
	CloudTaskFailed     CloudTaskState = "failed"
	CloudTaskCancelled  CloudTaskState = "cancelled"
)

// CloudTask represents a scheduled marathon run on a cloud VM.
type CloudTask struct {
	ID          string         `json:"id"`
	Marathon    Config         `json:"marathon_config"`
	VM          VMSpec         `json:"vm_spec"`
	State       CloudTaskState `json:"state"`
	VMInfo      *VMInfo        `json:"vm_info,omitempty"`
	Result      *CloudResult   `json:"result,omitempty"`
	Error       string         `json:"error,omitempty"`
	ScheduledAt time.Time      `json:"scheduled_at"`
	StartedAt   time.Time      `json:"started_at,omitempty"`
	FinishedAt  time.Time      `json:"finished_at,omitempty"`

	cancel context.CancelFunc
}

// CloudResult holds the outcome of a cloud marathon run, including
// marathon stats and VM cost.
type CloudResult struct {
	Stats     *Stats  `json:"stats"`
	VMCostUSD float64 `json:"vm_cost_usd"`
}

// CloudSchedulerConfig configures the cloud scheduler.
type CloudSchedulerConfig struct {
	// PollInterval controls how often the scheduler checks task state.
	// Defaults to 30s.
	PollInterval time.Duration

	// MaxConcurrent limits how many tasks run simultaneously. Zero means unlimited.
	MaxConcurrent int

	// ResultCommand is the CLI command template to run on the VM to retrieve
	// marathon results as JSON. The string "%s" is replaced with the repo path.
	// Defaults to "cat %s/.ralph/marathon/checkpoints/latest.json".
	ResultCommand string

	// MarathonBinary is the path to the ralphglasses binary on the VM.
	// Defaults to "ralphglasses".
	MarathonBinary string
}

// CloudScheduler manages marathon runs on cloud VMs. It handles the full
// lifecycle: provision VM, start marathon, poll for completion, collect
// results, and terminate the VM.
type CloudScheduler struct {
	cfg      CloudSchedulerConfig
	provider VMProvider

	mu    sync.Mutex
	tasks map[string]*CloudTask
	seq   int
}

// NewCloudScheduler creates a CloudScheduler with the given VM provider.
func NewCloudScheduler(provider VMProvider, cfg CloudSchedulerConfig) *CloudScheduler {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}
	if cfg.MarathonBinary == "" {
		cfg.MarathonBinary = "ralphglasses"
	}
	if cfg.ResultCommand == "" {
		cfg.ResultCommand = "cat %s/.ralph/marathon/checkpoints/latest.json"
	}
	return &CloudScheduler{
		cfg:      cfg,
		provider: provider,
		tasks:    make(map[string]*CloudTask),
	}
}

// Schedule queues a marathon run on a cloud VM. It returns the task ID
// immediately. The task is executed asynchronously — use TaskStatus to poll.
func (cs *CloudScheduler) Schedule(ctx context.Context, marathon Config, vm VMSpec) (string, error) {
	if err := marathon.Validate(); err != nil {
		return "", fmt.Errorf("invalid marathon config: %w", err)
	}
	if vm.Provider == "" {
		return "", fmt.Errorf("vm provider must be set")
	}

	cs.mu.Lock()
	cs.seq++
	id := fmt.Sprintf("cloud-%d-%d", time.Now().Unix(), cs.seq)

	// Enforce concurrency limit.
	if cs.cfg.MaxConcurrent > 0 {
		active := 0
		for _, t := range cs.tasks {
			if t.State == CloudTaskProvisioning || t.State == CloudTaskRunning || t.State == CloudTaskCollecting {
				active++
			}
		}
		if active >= cs.cfg.MaxConcurrent {
			cs.mu.Unlock()
			return "", fmt.Errorf("concurrency limit reached (%d/%d)", active, cs.cfg.MaxConcurrent)
		}
	}

	task := &CloudTask{
		ID:          id,
		Marathon:    marathon,
		VM:          vm,
		State:       CloudTaskPending,
		ScheduledAt: time.Now(),
	}
	cs.tasks[id] = task
	cs.mu.Unlock()

	// Run the full lifecycle in the background.
	taskCtx, cancel := context.WithCancel(ctx)
	task.cancel = cancel

	go cs.runTask(taskCtx, task)

	slog.Info("cloud_scheduler: task scheduled",
		"task_id", id,
		"provider", vm.Provider,
		"machine_type", vm.MachineType,
		"budget_usd", marathon.BudgetUSD,
		"duration", marathon.Duration,
	)

	return id, nil
}

// Cancel cancels a running or pending task. The VM is terminated if provisioned.
func (cs *CloudScheduler) Cancel(ctx context.Context, taskID string) error {
	cs.mu.Lock()
	task, ok := cs.tasks[taskID]
	cs.mu.Unlock()

	if !ok {
		return fmt.Errorf("task %q not found", taskID)
	}

	// Signal cancellation.
	if task.cancel != nil {
		task.cancel()
	}

	// Terminate VM if it exists.
	if task.VMInfo != nil && task.VMInfo.ID != "" {
		if err := cs.provider.Terminate(ctx, task.VMInfo.ID); err != nil {
			slog.Warn("cloud_scheduler: VM termination failed during cancel",
				"task_id", taskID, "vm_id", task.VMInfo.ID, "error", err)
		}
	}

	cs.mu.Lock()
	task.State = CloudTaskCancelled
	task.FinishedAt = time.Now()
	cs.mu.Unlock()

	slog.Info("cloud_scheduler: task cancelled", "task_id", taskID)
	return nil
}

// TaskStatus returns the current state of a task.
func (cs *CloudScheduler) TaskStatus(taskID string) (*CloudTask, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	task, ok := cs.tasks[taskID]
	if !ok {
		return nil, fmt.Errorf("task %q not found", taskID)
	}

	// Return a copy to avoid races.
	clone := *task
	if task.VMInfo != nil {
		vmCopy := *task.VMInfo
		clone.VMInfo = &vmCopy
	}
	if task.Result != nil {
		resCopy := *task.Result
		clone.Result = &resCopy
	}
	return &clone, nil
}

// ListTasks returns all tasks, optionally filtered by state.
// Pass nil to return all tasks.
func (cs *CloudScheduler) ListTasks(states []CloudTaskState) []*CloudTask {
	stateSet := make(map[CloudTaskState]bool, len(states))
	for _, s := range states {
		stateSet[s] = true
	}

	cs.mu.Lock()
	defer cs.mu.Unlock()

	var result []*CloudTask
	for _, t := range cs.tasks {
		if len(stateSet) > 0 && !stateSet[t.State] {
			continue
		}
		clone := *t
		result = append(result, &clone)
	}
	return result
}

// TotalCostUSD returns the aggregate VM + marathon spend across all tasks.
func (cs *CloudScheduler) TotalCostUSD() float64 {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	var total float64
	for _, t := range cs.tasks {
		if t.Result != nil {
			total += t.Result.VMCostUSD
			if t.Result.Stats != nil {
				total += t.Result.Stats.TotalSpentUSD
			}
		}
	}
	return total
}

// runTask executes the full VM lifecycle for a cloud task.
func (cs *CloudScheduler) runTask(ctx context.Context, task *CloudTask) {
	defer func() {
		if task.cancel != nil {
			task.cancel()
		}
	}()

	// Phase 1: Provision VM.
	cs.setTaskState(task, CloudTaskProvisioning)
	vmInfo, err := cs.provider.Provision(ctx, task.VM)
	if err != nil {
		cs.failTask(task, fmt.Errorf("provision: %w", err))
		return
	}

	cs.mu.Lock()
	task.VMInfo = vmInfo
	task.StartedAt = time.Now()
	cs.mu.Unlock()

	// Ensure cleanup: terminate VM when done regardless of outcome.
	defer func() {
		termCtx, termCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer termCancel()
		if err := cs.provider.Terminate(termCtx, vmInfo.ID); err != nil {
			slog.Warn("cloud_scheduler: VM termination failed",
				"task_id", task.ID, "vm_id", vmInfo.ID, "error", err)
		}
	}()

	// Phase 2: Start marathon on VM.
	cs.setTaskState(task, CloudTaskRunning)
	marathonCmd := cs.buildMarathonCommand(task)
	_, err = cs.provider.Execute(ctx, vmInfo.ID, marathonCmd)
	if err != nil {
		// Check if cancelled before treating as failure.
		if ctx.Err() != nil {
			return
		}
		cs.failTask(task, fmt.Errorf("marathon execution: %w", err))
		return
	}

	// Phase 3: Collect results.
	cs.setTaskState(task, CloudTaskCollecting)
	result, err := cs.collectResults(ctx, task)
	if err != nil {
		cs.failTask(task, fmt.Errorf("result collection: %w", err))
		return
	}

	// Fetch final VM cost.
	vmCost, costErr := cs.provider.CostSoFar(ctx, vmInfo.ID)
	if costErr != nil {
		slog.Warn("cloud_scheduler: cost retrieval failed",
			"task_id", task.ID, "error", costErr)
	}
	result.VMCostUSD = vmCost

	cs.mu.Lock()
	task.Result = result
	task.State = CloudTaskCompleted
	task.FinishedAt = time.Now()
	cs.mu.Unlock()

	slog.Info("cloud_scheduler: task completed",
		"task_id", task.ID,
		"vm_cost_usd", vmCost,
		"marathon_cost_usd", result.Stats.TotalSpentUSD,
		"cycles", result.Stats.CyclesCompleted,
	)
}

// buildMarathonCommand constructs the CLI command to run a marathon on the VM.
func (cs *CloudScheduler) buildMarathonCommand(task *CloudTask) string {
	return fmt.Sprintf(
		"%s marathon --budget %.2f --duration %s --sessions %d --repo %s",
		cs.cfg.MarathonBinary,
		task.Marathon.BudgetUSD,
		task.Marathon.Duration.String(),
		task.Marathon.SessionCount,
		task.Marathon.RepoPath,
	)
}

// collectResults retrieves marathon output from the VM.
func (cs *CloudScheduler) collectResults(ctx context.Context, task *CloudTask) (*CloudResult, error) {
	cmd := fmt.Sprintf(cs.cfg.ResultCommand, task.Marathon.RepoPath)
	output, err := cs.provider.Execute(ctx, task.VMInfo.ID, cmd)
	if err != nil {
		return nil, fmt.Errorf("execute result command: %w", err)
	}

	// Parse checkpoint JSON to extract stats.
	stats, err := parseStatsFromOutput(output)
	if err != nil {
		return nil, fmt.Errorf("parse results: %w", err)
	}

	return &CloudResult{Stats: stats}, nil
}

// parseStatsFromOutput extracts Stats from checkpoint JSON output.
func parseStatsFromOutput(output []byte) (*Stats, error) {
	// The output is a checkpoint JSON. Extract what we need.
	var cp Checkpoint
	if err := jsonUnmarshal(output, &cp); err != nil {
		// Fall back: maybe it's raw stats JSON.
		var s Stats
		if err2 := jsonUnmarshal(output, &s); err2 != nil {
			return nil, fmt.Errorf("could not parse output as checkpoint or stats: %w", err)
		}
		return &s, nil
	}
	return &Stats{
		CyclesCompleted: cp.CyclesCompleted,
		TotalSpentUSD:   cp.SpentUSD,
	}, nil
}

func (cs *CloudScheduler) setTaskState(task *CloudTask, state CloudTaskState) {
	cs.mu.Lock()
	task.State = state
	cs.mu.Unlock()
}

func (cs *CloudScheduler) failTask(task *CloudTask, err error) {
	cs.mu.Lock()
	task.State = CloudTaskFailed
	task.Error = err.Error()
	task.FinishedAt = time.Now()
	cs.mu.Unlock()

	slog.Warn("cloud_scheduler: task failed", "task_id", task.ID, "error", err)
}

// jsonUnmarshal is a package-level variable so tests can override parsing behavior.
var jsonUnmarshal = json.Unmarshal
