package clients

import (
	"context"
	"fmt"
	"os"
	"time"

	"google.golang.org/api/option"
	tasks "google.golang.org/api/tasks/v1"
)

// GTasksClient wraps Google's Tasks API.
type GTasksClient struct {
	service *tasks.Service
}

// GTaskList represents a Google Tasks list.
type GTaskList struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at"`
}

// GTask represents a Google Task.
type GTask struct {
	ID        string `json:"id"`
	ListID    string `json:"list_id"`
	Title     string `json:"title"`
	Notes     string `json:"notes"`
	Status    string `json:"status"` // needsAction or completed
	Due       string `json:"due"`
	Completed string `json:"completed"`
	Parent    string `json:"parent"`
	Position  string `json:"position"`
	UpdatedAt string `json:"updated_at"`
}

// NewGTasksClient creates a Google Tasks API client using shared Google OAuth.
func NewGTasksClient(ctx context.Context, account string) (*GTasksClient, error) {
	credPath := GoogleCredentialsPathForAccount(account)
	if account != "" && account != "personal" {
		if _, err := os.Stat(credPath); os.IsNotExist(err) {
			credPath = GoogleCredentialsPathForAccount("")
		}
	}

	oauthConfig, err := LoadGoogleCredentials(credPath, DefaultTasksScopes()...)
	if err != nil {
		return nil, fmt.Errorf("load credentials for tasks: %w", err)
	}

	token, err := LoadGoogleTokenForAccount(account)
	if err != nil {
		return nil, err
	}

	ts := oauthConfig.TokenSource(ctx, token)
	savingTS := &savingTokenSource{base: ts, lastToken: token, account: account}

	svc, err := tasks.NewService(ctx, option.WithTokenSource(savingTS))
	if err != nil {
		return nil, fmt.Errorf("create tasks service: %w", err)
	}
	return &GTasksClient{service: svc}, nil
}

// ListTaskLists returns all task lists.
func (g *GTasksClient) ListTaskLists(ctx context.Context) ([]GTaskList, error) {
	resp, err := g.service.Tasklists.List().Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("list task lists: %w", err)
	}
	var lists []GTaskList
	for _, item := range resp.Items {
		lists = append(lists, GTaskList{ID: item.Id, Title: item.Title, UpdatedAt: item.Updated})
	}
	return lists, nil
}

// ListTasks returns tasks in a task list.
func (g *GTasksClient) ListTasks(ctx context.Context, taskListID string, showCompleted bool) ([]GTask, error) {
	if taskListID == "" {
		taskListID = "@default"
	}
	call := g.service.Tasks.List(taskListID).Context(ctx).MaxResults(100)
	if showCompleted {
		call = call.ShowCompleted(true)
	}
	resp, err := call.Do()
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	var result []GTask
	for _, item := range resp.Items {
		result = append(result, GTask{
			ID: item.Id, ListID: taskListID, Title: item.Title, Notes: item.Notes,
			Status: item.Status, Due: item.Due, Completed: derefStr(item.Completed),
			Parent: item.Parent, Position: item.Position, UpdatedAt: item.Updated,
		})
	}
	return result, nil
}

// CreateTask creates a new task.
func (g *GTasksClient) CreateTask(ctx context.Context, taskListID, title, notes, due string) (*GTask, error) {
	if taskListID == "" {
		taskListID = "@default"
	}
	task := &tasks.Task{Title: title, Notes: notes}
	if due != "" {
		task.Due = due + "T00:00:00.000Z"
	}
	created, err := g.service.Tasks.Insert(taskListID, task).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}
	return &GTask{ID: created.Id, ListID: taskListID, Title: created.Title, Notes: created.Notes, Status: created.Status, Due: created.Due, UpdatedAt: created.Updated}, nil
}

// CompleteTask marks a task as completed.
func (g *GTasksClient) CompleteTask(ctx context.Context, taskListID, taskID string) error {
	if taskListID == "" {
		taskListID = "@default"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := g.service.Tasks.Patch(taskListID, taskID, &tasks.Task{Status: "completed", Completed: &now}).Context(ctx).Do()
	return err
}

// MoveTask moves a task to a different task list by deleting and recreating.
func (g *GTasksClient) MoveTask(ctx context.Context, fromListID, taskID, toListID string) (*GTask, error) {
	if fromListID == "" {
		fromListID = "@default"
	}
	if toListID == "" {
		toListID = "@default"
	}
	// Get the task first.
	existing, err := g.service.Tasks.Get(fromListID, taskID).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get task for move: %w", err)
	}
	// Create in new list.
	created, err := g.service.Tasks.Insert(toListID, &tasks.Task{Title: existing.Title, Notes: existing.Notes, Due: existing.Due}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("create task in target list: %w", err)
	}
	// Delete from old list.
	_ = g.service.Tasks.Delete(fromListID, taskID).Context(ctx).Do()
	return &GTask{ID: created.Id, ListID: toListID, Title: created.Title, Notes: created.Notes, Status: created.Status, Due: created.Due, UpdatedAt: created.Updated}, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
