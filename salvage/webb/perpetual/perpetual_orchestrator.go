// Package clients provides the implementation orchestrator for the perpetual engine.
package clients

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ImplementationOrchestrator converts proposals to DevTasks and manages their lifecycle
type ImplementationOrchestrator struct {
	devWorker       *DevWorkerClient
	engine          *PerpetualEngine
	activeProposals map[string]*ProposalTracking // taskID -> proposal tracking
	mu              sync.RWMutex
}

// ProposalTracking tracks the relationship between proposals and tasks
type ProposalTracking struct {
	Proposal   *PerpetualProposal
	TaskID     string
	StartedAt  time.Time
	Phase      string // research, development, review, consensus, pr_creation
	LastUpdate time.Time
}

// NewImplementationOrchestrator creates a new orchestrator
func NewImplementationOrchestrator(devWorker *DevWorkerClient, engine *PerpetualEngine) *ImplementationOrchestrator {
	return &ImplementationOrchestrator{
		devWorker:       devWorker,
		engine:          engine,
		activeProposals: make(map[string]*ProposalTracking),
	}
}

// ImplementProposal converts a proposal to a DevTask and queues it
func (o *ImplementationOrchestrator) ImplementProposal(ctx context.Context, proposal *PerpetualProposal) (*DevTask, error) {
	if o.devWorker == nil {
		return nil, fmt.Errorf("dev worker not initialized")
	}

	// Determine task scope based on proposal characteristics
	scope := o.determineScope(proposal)

	// Build task description with evidence
	description := o.buildTaskDescription(proposal)

	// Queue the task
	task, err := o.devWorker.QueueTask(scope, proposal.Title, description,
		WithDevSource("perpetual"),
		WithDevPriority(o.mapPriority(proposal.Score)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to queue task: %w", err)
	}

	// Track the proposal
	o.mu.Lock()
	o.activeProposals[task.ID] = &ProposalTracking{
		Proposal:   proposal,
		TaskID:     task.ID,
		StartedAt:  time.Now(),
		Phase:      "queued",
		LastUpdate: time.Now(),
	}
	o.mu.Unlock()

	// Update proposal status
	proposal.Status = "implementing"
	proposal.DevTaskID = task.ID

	// Fire callback
	if o.engine != nil && o.engine.onImplementStart != nil {
		o.engine.onImplementStart(proposal)
	}

	return task, nil
}

// ProcessProposalAsync queues a proposal and processes it asynchronously
func (o *ImplementationOrchestrator) ProcessProposalAsync(ctx context.Context, proposal *PerpetualProposal) (string, error) {
	task, err := o.ImplementProposal(ctx, proposal)
	if err != nil {
		return "", err
	}

	// Start processing in background with panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				o.handleTaskFailure(task.ID, fmt.Errorf("panic during task processing: %v", r))
			}
		}()

		if err := o.devWorker.ProcessTaskSync(task.ID); err != nil {
			o.handleTaskFailure(task.ID, err)
		} else {
			o.handleTaskSuccess(task.ID)
		}
	}()

	return task.ID, nil
}

// ProcessProposalSync queues and processes a proposal synchronously
func (o *ImplementationOrchestrator) ProcessProposalSync(ctx context.Context, proposal *PerpetualProposal) error {
	task, err := o.ImplementProposal(ctx, proposal)
	if err != nil {
		return err
	}

	// Process synchronously
	if err := o.devWorker.ProcessTaskSync(task.ID); err != nil {
		o.handleTaskFailure(task.ID, err)
		return err
	}

	o.handleTaskSuccess(task.ID)
	return nil
}

// GetActiveProposals returns all proposals currently being implemented
func (o *ImplementationOrchestrator) GetActiveProposals() []*ProposalTracking {
	o.mu.RLock()
	defer o.mu.RUnlock()

	result := make([]*ProposalTracking, 0, len(o.activeProposals))
	for _, tracking := range o.activeProposals {
		result = append(result, tracking)
	}
	return result
}

// GetProposalStatus returns the status of a specific proposal implementation
func (o *ImplementationOrchestrator) GetProposalStatus(taskID string) (*ProposalTracking, error) {
	o.mu.RLock()
	tracking, ok := o.activeProposals[taskID]
	o.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("task %s not found in active proposals", taskID)
	}

	// Update phase from task status
	if o.devWorker != nil {
		if task, err := o.devWorker.GetTask(taskID); err == nil {
			tracking.Phase = string(task.Cycle)
			tracking.LastUpdate = time.Now()
		}
	}

	return tracking, nil
}

// handleTaskSuccess processes a successful task completion
func (o *ImplementationOrchestrator) handleTaskSuccess(taskID string) {
	o.mu.Lock()
	tracking, ok := o.activeProposals[taskID]
	if ok {
		tracking.Phase = "completed"
		tracking.LastUpdate = time.Now()
		tracking.Proposal.Status = "completed"

		// Get PR info from task
		if o.devWorker != nil {
			if task, err := o.devWorker.GetTask(taskID); err == nil {
				tracking.Proposal.PRNumber = task.PRNumber

				// Fire PR created callback
				if o.engine != nil && o.engine.onPRCreated != nil && task.PRNumber > 0 {
					o.engine.onPRCreated(tracking.Proposal, task.PRNumber)
				}

				// Update metrics
				if o.engine != nil {
					o.engine.mu.Lock()
					o.engine.state.Metrics.ProposalsImplemented++
					if task.PRNumber > 0 {
						o.engine.state.Metrics.PRsCreated++
						o.engine.state.Metrics.DailyPRCount++
					}
					o.engine.mu.Unlock()
				}
			}
		}

		// Remove task from engine's active list
		if o.engine != nil {
			o.engine.RemoveTaskID(taskID)
		}

		delete(o.activeProposals, taskID)
	}
	o.mu.Unlock()
}

// handleTaskFailure processes a failed task
func (o *ImplementationOrchestrator) handleTaskFailure(taskID string, err error) {
	o.mu.Lock()
	tracking, ok := o.activeProposals[taskID]
	if ok {
		tracking.Phase = "failed"
		tracking.LastUpdate = time.Now()
		tracking.Proposal.Status = "failed"
		tracking.Proposal.FailureCount++
		tracking.Proposal.LastFailure = time.Now()

		// Re-queue proposal for retry after cooldown
		if o.engine != nil {
			o.engine.queue.Push(tracking.Proposal)
			o.engine.RemoveTaskID(taskID)
		}

		delete(o.activeProposals, taskID)
	}
	o.mu.Unlock()

	fmt.Printf("perpetual: task %s failed: %v\n", taskID, err)
}

// determineScope maps proposal characteristics to DevTaskScope
func (o *ImplementationOrchestrator) determineScope(proposal *PerpetualProposal) DevTaskScope {
	title := strings.ToLower(proposal.Title)
	desc := strings.ToLower(proposal.Description)

	// Check for MCP/tool integration keywords
	if strings.Contains(title, "mcp") || strings.Contains(title, "integrate") ||
		strings.Contains(desc, "mcp server") || strings.Contains(desc, "integration") {
		return ScopeTool
	}

	// Check for feature keywords
	if strings.Contains(title, "feature") || strings.Contains(title, "add") ||
		strings.Contains(desc, "new capability") {
		return ScopeFeature
	}

	// Default to task for smaller items
	if proposal.Effort == EffortSmall {
		return ScopeTask
	}

	return ScopeTool
}

// buildTaskDescription creates a detailed description for the DevTask
func (o *ImplementationOrchestrator) buildTaskDescription(proposal *PerpetualProposal) string {
	var sb strings.Builder

	sb.WriteString(proposal.Description)
	sb.WriteString("\n\n")

	// Add source information
	sb.WriteString(fmt.Sprintf("**Source:** %s\n", proposal.Source))
	sb.WriteString(fmt.Sprintf("**Impact Score:** %d/100\n", proposal.Impact))
	sb.WriteString(fmt.Sprintf("**Effort Level:** %s\n", proposal.Effort))
	sb.WriteString(fmt.Sprintf("**Priority Score:** %.2f\n", proposal.Score))

	// Add evidence
	if len(proposal.Evidence) > 0 {
		sb.WriteString("\n**Evidence:**\n")
		for _, e := range proposal.Evidence {
			sb.WriteString(fmt.Sprintf("- %s\n", e))
		}
	}

	return sb.String()
}

// mapPriority converts proposal score to DevTaskPriority
func (o *ImplementationOrchestrator) mapPriority(score float64) DevTaskPriority {
	switch {
	case score >= 50:
		return DevPriorityHigh
	case score >= 25:
		return DevPriorityNormal
	default:
		return DevPriorityLow
	}
}

// RecordPROutcome records the outcome of a PR (merged or rejected)
func (o *ImplementationOrchestrator) RecordPROutcome(proposal *PerpetualProposal, merged bool) {
	// Update metrics
	if o.engine != nil {
		o.engine.mu.Lock()
		if merged {
			o.engine.state.Metrics.PRsMerged++
		} else {
			o.engine.state.Metrics.PRsRejected++
		}
		o.engine.mu.Unlock()

		// Fire callback for learning
		if o.engine.onPROutcome != nil {
			o.engine.onPROutcome(proposal, merged)
		}
	}
}
