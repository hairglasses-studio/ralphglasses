package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const todoistAPIBaseURL = "https://api.todoist.com/rest/v2"

// TodoistClient is a REST client for the Todoist API v2.
type TodoistClient struct {
	httpClient *http.Client
	token      string
}

// TodoistTask represents a Todoist task.
type TodoistTask struct {
	ID          string `json:"id"`
	Content     string `json:"content"`
	Description string `json:"description"`
	ProjectID   string `json:"project_id"`
	SectionID   string `json:"section_id"`
	Priority    int    `json:"priority"` // 1=normal, 4=urgent
	Due         *struct {
		Date     string `json:"date"`
		Datetime string `json:"datetime,omitempty"`
		String   string `json:"string"`
	} `json:"due"`
	Labels    []string `json:"labels"`
	CreatedAt string   `json:"created_at"`
	IsCompleted bool   `json:"is_completed"`
	URL       string   `json:"url"`
}

// TodoistProject represents a Todoist project.
type TodoistProject struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	ParentID string `json:"parent_id,omitempty"`
	Order    int    `json:"order"`
	URL      string `json:"url"`
}

// TodoistSection represents a Todoist section within a project.
type TodoistSection struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Order     int    `json:"order"`
}

// TodoistLabel represents a Todoist label.
type TodoistLabel struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	Order int    `json:"order"`
}

// NewTodoistClient creates a new Todoist REST API client.
func NewTodoistClient(token string) *TodoistClient {
	return &TodoistClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		token:      token,
	}
}

// ListTasks retrieves all active tasks, optionally filtered by project or label.
func (t *TodoistClient) ListTasks(ctx context.Context, projectID, filter string) ([]TodoistTask, error) {
	path := "/tasks"
	if projectID != "" {
		path += "?project_id=" + projectID
	} else if filter != "" {
		path += "?filter=" + filter
	}
	var tasks []TodoistTask
	if err := t.doGet(ctx, path, &tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

// GetTask retrieves a single task by ID.
func (t *TodoistClient) GetTask(ctx context.Context, taskID string) (*TodoistTask, error) {
	var task TodoistTask
	if err := t.doGet(ctx, "/tasks/"+taskID, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// CreateTask creates a new task.
func (t *TodoistClient) CreateTask(ctx context.Context, content, description, projectID, dueDate string, priority int, labels []string) (*TodoistTask, error) {
	body := map[string]interface{}{
		"content": content,
	}
	if description != "" {
		body["description"] = description
	}
	if projectID != "" {
		body["project_id"] = projectID
	}
	if dueDate != "" {
		body["due_date"] = dueDate
	}
	if priority > 0 {
		body["priority"] = priority
	}
	if len(labels) > 0 {
		body["labels"] = labels
	}

	var task TodoistTask
	if err := t.doPost(ctx, "/tasks", body, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// UpdateTask updates an existing task.
func (t *TodoistClient) UpdateTask(ctx context.Context, taskID string, updates map[string]interface{}) (*TodoistTask, error) {
	var task TodoistTask
	if err := t.doPost(ctx, "/tasks/"+taskID, updates, &task); err != nil {
		return nil, err
	}
	return &task, nil
}

// CompleteTask marks a task as completed.
func (t *TodoistClient) CompleteTask(ctx context.Context, taskID string) error {
	return t.doPostNoBody(ctx, "/tasks/"+taskID+"/close")
}

// ReopenTask reopens a completed task.
func (t *TodoistClient) ReopenTask(ctx context.Context, taskID string) error {
	return t.doPostNoBody(ctx, "/tasks/"+taskID+"/reopen")
}

// DeleteTask permanently deletes a task.
func (t *TodoistClient) DeleteTask(ctx context.Context, taskID string) error {
	return t.doDelete(ctx, "/tasks/"+taskID)
}

// ListProjects returns all projects.
func (t *TodoistClient) ListProjects(ctx context.Context) ([]TodoistProject, error) {
	var projects []TodoistProject
	if err := t.doGet(ctx, "/projects", &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// ListSections returns all sections, optionally filtered by project.
func (t *TodoistClient) ListSections(ctx context.Context, projectID string) ([]TodoistSection, error) {
	path := "/sections"
	if projectID != "" {
		path += "?project_id=" + projectID
	}
	var sections []TodoistSection
	if err := t.doGet(ctx, path, &sections); err != nil {
		return nil, err
	}
	return sections, nil
}

// ListLabels returns all labels.
func (t *TodoistClient) ListLabels(ctx context.Context) ([]TodoistLabel, error) {
	var labels []TodoistLabel
	if err := t.doGet(ctx, "/labels", &labels); err != nil {
		return nil, err
	}
	return labels, nil
}

func (t *TodoistClient) doGet(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, todoistAPIBaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	t.setHeaders(req)
	return t.doRequest(req, out)
}

func (t *TodoistClient) doPost(ctx context.Context, path string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, todoistAPIBaseURL+path, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	t.setHeaders(req)
	return t.doRequest(req, out)
}

func (t *TodoistClient) doPostNoBody(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, todoistAPIBaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	t.setHeaders(req)
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("todoist API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body), API: "todoist"}
	}
	return nil
}

func (t *TodoistClient) doDelete(ctx context.Context, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, todoistAPIBaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	t.setHeaders(req)
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("todoist API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body), API: "todoist"}
	}
	return nil
}

func (t *TodoistClient) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+t.token)
}

func (t *TodoistClient) doRequest(req *http.Request, out interface{}) error {
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("todoist API request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body), API: "todoist"}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
