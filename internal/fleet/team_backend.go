package fleet

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// StructuredTeamBackend bridges session.StructuredTeamBackend onto the fleet queue.
type StructuredTeamBackend struct {
	coord  *Coordinator
	client *Client
}

// NewStructuredTeamBackend creates a fleet-backed structured team backend.
func NewStructuredTeamBackend(coord *Coordinator, client *Client) *StructuredTeamBackend {
	return &StructuredTeamBackend{coord: coord, client: client}
}

func (b *StructuredTeamBackend) Name() string {
	return session.TeamExecutionBackendFleet
}

func (b *StructuredTeamBackend) Submit(ctx context.Context, req session.TeamBackendSubmitRequest) (session.TeamWorkerHandle, error) {
	item := WorkItem{
		Type:             WorkTypeSession,
		Source:           WorkSourceStructuredCodexTeam,
		RepoName:         filepath.Base(req.RepoPath),
		RepoPath:         req.RepoPath,
		Prompt:           req.Prompt,
		Provider:         req.Provider,
		Model:            req.Model,
		TeamName:         req.TeamName,
		TeamTaskID:       req.TaskID,
		PlannerSessionID: req.PlannerSessionID,
		SessionName:      req.SessionName,
		PermissionMode:   req.PermissionMode,
		OutputSchema:     req.OutputSchema,
		WorktreePolicy:   req.WorktreePolicy,
		TargetBranch:     req.TargetBranch,
		HumanContext:     append([]string(nil), req.HumanContext...),
		MaxBudgetUSD:     req.MaxBudgetUSD,
		MaxTurns:         req.MaxTurns,
		MaxRetries:       2,
		Constraints: WorkConstraints{
			RequireLocal:    true,
			RequireProvider: req.Provider,
		},
	}

	if b.coord != nil {
		if err := b.coord.SubmitWork(&item); err != nil {
			return session.TeamWorkerHandle{}, err
		}
		return session.TeamWorkerHandle{WorkItemID: item.ID}, nil
	}
	if b.client == nil {
		return session.TeamWorkerHandle{}, fmt.Errorf("fleet backend is not configured")
	}
	id, err := b.client.SubmitWork(ctx, item)
	if err != nil {
		return session.TeamWorkerHandle{}, err
	}
	return session.TeamWorkerHandle{WorkItemID: id}, nil
}

func (b *StructuredTeamBackend) Poll(ctx context.Context, handle session.TeamWorkerHandle) (*session.TeamBackendPollResult, error) {
	if handle.WorkItemID == "" {
		return nil, fmt.Errorf("missing work item id")
	}

	var (
		item *WorkItem
		ok   bool
		err  error
	)
	if b.coord != nil {
		item, ok = b.coord.WorkItem(handle.WorkItemID)
		if !ok {
			return nil, fmt.Errorf("work item %s not found", handle.WorkItemID)
		}
	} else {
		if b.client == nil {
			return nil, fmt.Errorf("fleet backend is not configured")
		}
		item, err = b.client.WorkStatus(ctx, handle.WorkItemID)
		if err != nil {
			return nil, err
		}
	}

	result := &session.TeamBackendPollResult{
		Handle: session.TeamWorkerHandle{
			WorkItemID:     item.ID,
			SessionID:      item.SessionID,
			WorkerNodeID:   item.AssignedTo,
			WorktreePath:   handle.WorktreePath,
			WorktreeBranch: handle.WorktreeBranch,
			HeadSHA:        handle.HeadSHA,
			MergeBaseSHA:   handle.MergeBaseSHA,
		},
	}
	if item.Result != nil {
		result.Handle.SessionID = firstNonEmpty(item.Result.SessionID, result.Handle.SessionID)
		result.Handle.WorkerNodeID = firstNonEmpty(item.Result.WorkerNodeID, result.Handle.WorkerNodeID)
		result.Handle.WorktreePath = firstNonEmpty(item.Result.WorktreePath, result.Handle.WorktreePath)
		result.Handle.WorktreeBranch = firstNonEmpty(item.Result.WorktreeBranch, result.Handle.WorktreeBranch)
		result.Handle.HeadSHA = firstNonEmpty(item.Result.HeadSHA, result.Handle.HeadSHA)
		result.Handle.MergeBaseSHA = firstNonEmpty(item.Result.MergeBaseSHA, result.Handle.MergeBaseSHA)
	}

	switch item.Status {
	case WorkPending, WorkAssigned, WorkRunning:
		result.SessionStatus = session.StatusRunning
		result.Error = item.Error
		return result, nil
	case WorkCompleted:
		result.Terminal = true
		if item.Result != nil {
			result.WorkerResult = &session.TeamWorkerResult{
				TaskID:       item.TeamTaskID,
				Status:       firstNonEmpty(item.Result.TaskStatus, session.TeamTaskCompleted),
				Summary:      item.Result.Summary,
				Question:     item.Result.Question,
				ChangedFiles: append([]string(nil), item.Result.ChangedFiles...),
				Error:        item.Error,
			}
		}
		return result, nil
	case WorkFailed:
		result.Terminal = true
		result.Error = item.Error
		if item.Result != nil && item.Result.TaskStatus != "" {
			result.WorkerResult = &session.TeamWorkerResult{
				TaskID:       item.TeamTaskID,
				Status:       item.Result.TaskStatus,
				Summary:      item.Result.Summary,
				Question:     item.Result.Question,
				ChangedFiles: append([]string(nil), item.Result.ChangedFiles...),
				Error:        firstNonEmpty(item.Error, item.Result.ExitReason),
			}
		}
		return result, nil
	default:
		return result, nil
	}
}

func (b *StructuredTeamBackend) Stop(ctx context.Context, handle session.TeamWorkerHandle) error {
	if handle.WorkItemID == "" {
		return fmt.Errorf("missing work item id")
	}
	if b.coord != nil {
		return b.coord.CancelWork(handle.WorkItemID)
	}
	if b.client == nil {
		return fmt.Errorf("fleet backend is not configured")
	}
	return b.client.CancelWork(ctx, handle.WorkItemID)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
